package webrtc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ASSEvent represents a parsed ASS subtitle event
type ASSEvent struct {
	Layer     int
	Start     time.Duration
	End       time.Duration
	Style     string
	Name      string
	MarginL   int
	MarginR   int
	MarginV   int
	Effect    string
	Text      string
	ID        string // Unique ID for this event
}

// ASSParser parses ASS subtitle format
type ASSParser struct {
	eventFormat []string // Format fields order
	eventRegex  *regexp.Regexp
	tagRegex    *regexp.Regexp
	eventID     int
}

// NewASSParser creates a new ASS parser
func NewASSParser() *ASSParser {
	return &ASSParser{
		eventFormat: []string{"Layer", "Start", "End", "Style", "Name", "MarginL", "MarginR", "MarginV", "Effect", "Text"},
		tagRegex:    regexp.MustCompile(`\{[^}]*\}`),
	}
}

// parseTime parses ASS timestamp format (H:MM:SS.cc)
func (p *ASSParser) parseTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)

	// Format: H:MM:SS.cc (centiseconds)
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid time format: %s", s)
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}

	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}

	// Split seconds and centiseconds
	secParts := strings.Split(parts[2], ".")
	if len(secParts) != 2 {
		return 0, fmt.Errorf("invalid seconds format: %s", parts[2])
	}

	seconds, err := strconv.Atoi(secParts[0])
	if err != nil {
		return 0, err
	}

	centiseconds, err := strconv.Atoi(secParts[1])
	if err != nil {
		return 0, err
	}

	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(centiseconds)*10*time.Millisecond, nil
}

// ParseEvent parses a Dialogue line from ASS format
func (p *ASSParser) ParseEvent(line string) (*ASSEvent, error) {
	// Check for Dialogue prefix
	if !strings.HasPrefix(line, "Dialogue:") {
		return nil, nil // Not a dialogue line
	}

	// Remove "Dialogue: " prefix
	content := strings.TrimPrefix(line, "Dialogue:")
	content = strings.TrimSpace(content)

	// Split by commas, but Text field may contain commas
	// Format has 10 fields, Text is the last one
	parts := strings.SplitN(content, ",", 10)
	if len(parts) < 10 {
		return nil, fmt.Errorf("not enough fields in dialogue line")
	}

	layer, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	start, err := p.parseTime(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse start time: %w", err)
	}
	end, err := p.parseTime(parts[2])
	if err != nil {
		return nil, fmt.Errorf("failed to parse end time: %w", err)
	}

	marginL, _ := strconv.Atoi(strings.TrimSpace(parts[5]))
	marginR, _ := strconv.Atoi(strings.TrimSpace(parts[6]))
	marginV, _ := strconv.Atoi(strings.TrimSpace(parts[7]))

	// Strip ASS tags from text
	text := parts[9]
	text = p.tagRegex.ReplaceAllString(text, "")
	// Handle line breaks
	text = strings.ReplaceAll(text, "\\N", "\n")
	text = strings.ReplaceAll(text, "\\n", "\n")
	text = strings.TrimSpace(text)

	p.eventID++

	return &ASSEvent{
		Layer:   layer,
		Start:   start,
		End:     end,
		Style:   strings.TrimSpace(parts[3]),
		Name:    strings.TrimSpace(parts[4]),
		MarginL: marginL,
		MarginR: marginR,
		MarginV: marginV,
		Effect:  strings.TrimSpace(parts[8]),
		Text:    text,
		ID:      fmt.Sprintf("sub-%d", p.eventID),
	}, nil
}

// SetFormat sets the format from Format: line
func (p *ASSParser) SetFormat(line string) {
	if !strings.HasPrefix(line, "Format:") {
		return
	}
	content := strings.TrimPrefix(line, "Format:")
	parts := strings.Split(content, ",")
	p.eventFormat = make([]string, len(parts))
	for i, part := range parts {
		p.eventFormat[i] = strings.TrimSpace(part)
	}
}

// StreamSubtitles reads ASS subtitle data from reader and sends to peer via DataChannel
func (p *Peer) StreamSubtitles(reader io.Reader) error {
	parser := NewASSParser()
	scanner := bufio.NewScanner(reader)

	log.Printf("[WebRTC] StreamSubtitles started for peer %s", p.ID)

	eventCount := 0
	inEventsSection := false

	// Active subtitles map for tracking end times
	type activeSubtitle struct {
		event   *ASSEvent
		endTime time.Time
	}
	activeSubtitles := make(map[string]*activeSubtitle)

	// Get stream start time
	streamStartTime := time.Now()

	for scanner.Scan() {
		select {
		case <-p.ctx.Done():
			log.Printf("[WebRTC] StreamSubtitles context done for peer %s (events: %d)", p.ID, eventCount)
			return p.ctx.Err()
		default:
		}

		line := scanner.Text()
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Check for Events section
		if strings.HasPrefix(line, "[Events]") {
			inEventsSection = true
			continue
		}

		// Check for other sections
		if strings.HasPrefix(line, "[") {
			inEventsSection = false
			continue
		}

		// Process Format line
		if inEventsSection && strings.HasPrefix(line, "Format:") {
			parser.SetFormat(line)
			continue
		}

		// Process Dialogue lines
		if inEventsSection && strings.HasPrefix(line, "Dialogue:") {
			event, err := parser.ParseEvent(line)
			if err != nil {
				log.Printf("[WebRTC] Failed to parse subtitle event: %v", err)
				continue
			}
			if event == nil || event.Text == "" {
				continue
			}

			eventCount++

			// Calculate when to show/hide based on stream time
			// For live streaming, we send immediately since FFmpeg outputs in real-time
			currentTime := time.Since(streamStartTime)

			// Check and hide expired subtitles
			for id, sub := range activeSubtitles {
				if time.Now().After(sub.endTime) {
					hideMsg := SubtitleMessage{
						Type: "hide",
						ID:   id,
					}
					data, _ := json.Marshal(hideMsg)
					if err := p.SendSubtitle(data); err != nil {
						log.Printf("[WebRTC] Failed to send hide subtitle: %v", err)
					}
					delete(activeSubtitles, id)
				}
			}

			// Send show message
			showMsg := SubtitleMessage{
				Type:      "show",
				ID:        event.ID,
				Text:      event.Text,
				StartTime: currentTime.Seconds(),
				EndTime:   currentTime.Seconds() + event.End.Seconds() - event.Start.Seconds(),
				Style:     event.Style,
			}

			data, err := json.Marshal(showMsg)
			if err != nil {
				log.Printf("[WebRTC] Failed to marshal subtitle message: %v", err)
				continue
			}

			if err := p.SendSubtitle(data); err != nil {
				log.Printf("[WebRTC] Failed to send subtitle: %v", err)
				continue
			}

			// Track active subtitle for hiding later
			duration := event.End - event.Start
			activeSubtitles[event.ID] = &activeSubtitle{
				event:   event,
				endTime: time.Now().Add(duration),
			}

			// Log occasionally
			if eventCount <= 5 || eventCount%100 == 0 {
				log.Printf("[WebRTC] Sent subtitle #%d: %q (duration: %v)", eventCount, event.Text, duration)
			}
		}
	}

	// Send clear message when stream ends
	clearMsg := SubtitleMessage{
		Type: "clear",
	}
	data, _ := json.Marshal(clearMsg)
	p.SendSubtitle(data)

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("subtitle scanner error: %w", err)
	}

	log.Printf("[WebRTC] StreamSubtitles ended for peer %s (events: %d)", p.ID, eventCount)
	return nil
}

// StartSubtitleCleanup starts a goroutine to clean up expired subtitles
func (p *Peer) StartSubtitleCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Cleanup logic is handled in StreamSubtitles
			}
		}
	}()
}
