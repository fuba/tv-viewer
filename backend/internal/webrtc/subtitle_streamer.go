package webrtc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-zeromq/zmq4"
)

// ASSEvent represents a parsed ASS subtitle event
type ASSEvent struct {
	Layer   int
	Start   time.Duration
	End     time.Duration
	Style   string
	Name    string
	MarginL int
	MarginR int
	MarginV int
	Effect  string
	Text    string
	ID      string // Unique ID for this event
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

			// Calculate duration - cap at reasonable max for live streaming
			// ARIB captions often have very large end times for live streams
			duration := event.End - event.Start
			maxDuration := 10 * time.Second
			if duration > maxDuration || duration <= 0 {
				duration = 5 * time.Second // Default display time for live captions
			}

			// Send show message
			showMsg := SubtitleMessage{
				Type:      "show",
				ID:        event.ID,
				Text:      event.Text,
				StartTime: currentTime.Seconds(),
				EndTime:   currentTime.Seconds() + duration.Seconds(),
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
	if err := p.SendSubtitle(data); err != nil {
		log.Printf("[WebRTC] Failed to clear subtitles for peer %s: %v", p.ID, err)
	}

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

// StreamSubtitlesToFile reads ASS subtitle data and writes current text to a file
// This is used for burn-in mode with FFmpeg's drawtext filter (reload=1)
func StreamSubtitlesToFile(ctx context.Context, reader io.Reader, textFilePath string) error {
	parser := NewASSParser()
	scanner := bufio.NewScanner(reader)

	log.Printf("[WebRTC] StreamSubtitlesToFile started: %s", textFilePath)

	eventCount := 0
	inEventsSection := false

	// Track active subtitles with their end times
	type activeSubtitle struct {
		text    string
		endTime time.Time
	}
	activeSubtitles := make(map[string]*activeSubtitle)
	var currentText string

	// Helper to update the text file
	updateTextFile := func(text string) error {
		if text == currentText {
			return nil // No change needed
		}
		currentText = text
		return os.WriteFile(textFilePath, []byte(text), 0600)
	}

	// Cleanup goroutine to remove expired subtitles and update file
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				changed := false
				for id, sub := range activeSubtitles {
					if now.After(sub.endTime) {
						delete(activeSubtitles, id)
						changed = true
					}
				}
				if changed {
					// Rebuild text from remaining subtitles
					var texts []string
					for _, sub := range activeSubtitles {
						if sub.text != "" {
							texts = append(texts, sub.text)
						}
					}
					newText := strings.Join(texts, "\n")
					if err := updateTextFile(newText); err != nil {
						log.Printf("[WebRTC] Failed to update subtitle file: %v", err)
					}
				}
			}
		}
	}()

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			log.Printf("[WebRTC] StreamSubtitlesToFile context done (events: %d)", eventCount)
			<-cleanupDone
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		line = strings.TrimSpace(line)

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

			// Calculate duration - cap at reasonable max
			duration := event.End - event.Start
			maxDuration := 10 * time.Second
			if duration > maxDuration || duration <= 0 {
				duration = 5 * time.Second
			}

			// Add to active subtitles
			activeSubtitles[event.ID] = &activeSubtitle{
				text:    event.Text,
				endTime: time.Now().Add(duration),
			}

			// Rebuild and update text file
			var texts []string
			for _, sub := range activeSubtitles {
				if sub.text != "" {
					texts = append(texts, sub.text)
				}
			}
			newText := strings.Join(texts, "\n")
			if err := updateTextFile(newText); err != nil {
				log.Printf("[WebRTC] Failed to update subtitle file: %v", err)
			}

			if eventCount <= 5 || eventCount%100 == 0 {
				log.Printf("[WebRTC] Subtitle file updated #%d: %q (duration: %v)", eventCount, event.Text, duration)
			}
		}
	}

	// Clear file when stream ends
	if err := updateTextFile(""); err != nil {
		log.Printf("[WebRTC] Failed to clear subtitle file: %v", err)
	}
	<-cleanupDone

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("subtitle scanner error: %w", err)
	}

	log.Printf("[WebRTC] StreamSubtitlesToFile ended (events: %d)", eventCount)
	return nil
}

// StreamSubtitlesViaZMQ reads ASS subtitle data and sends text updates via ZMQ to FFmpeg's drawtext filter
// This provides lower latency than file-based approach
func StreamSubtitlesViaZMQ(ctx context.Context, reader io.Reader, zmqAddress string) error {
	parser := NewASSParser()
	scanner := bufio.NewScanner(reader)

	log.Printf("[WebRTC] StreamSubtitlesViaZMQ started: %s", zmqAddress)

	// Connect to FFmpeg's zmq filter
	zmqCtx := context.Background()
	socket := zmq4.NewReq(zmqCtx)
	defer socket.Close()

	// Retry connection a few times since FFmpeg might not be ready yet
	var connected bool
	for i := 0; i < 10; i++ {
		if err := socket.Dial(zmqAddress); err != nil {
			log.Printf("[WebRTC] ZMQ connect attempt %d failed: %v", i+1, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
				continue
			}
		}
		connected = true
		log.Printf("[WebRTC] ZMQ connected to %s", zmqAddress)
		break
	}

	if !connected {
		return fmt.Errorf("failed to connect to ZMQ socket after retries")
	}

	eventCount := 0
	inEventsSection := false

	// Track active subtitles with their end times
	type activeSubtitle struct {
		text    string
		endTime time.Time
	}
	activeSubtitles := make(map[string]*activeSubtitle)
	var currentText string

	// Helper to send text update via ZMQ
	sendTextUpdate := func(text string) error {
		if text == currentText {
			return nil // No change needed
		}
		currentText = text

		// Escape single quotes in the text for FFmpeg command
		escapedText := strings.ReplaceAll(text, "'", "'\\''")
		escapedText = strings.ReplaceAll(escapedText, "\n", "\\n")

		// Send command to drawtext filter
		// Format: "Parsed_drawtext_N reinit text='new text'"
		cmd := fmt.Sprintf("Parsed_drawtext_2 reinit text='%s'", escapedText)

		msg := zmq4.NewMsgString(cmd)
		if err := socket.Send(msg); err != nil {
			return fmt.Errorf("ZMQ send error: %w", err)
		}

		// Receive response
		reply, err := socket.Recv()
		if err != nil {
			return fmt.Errorf("ZMQ recv error: %w", err)
		}
		replyStr := string(reply.Bytes())
		if replyStr != "OK" && replyStr != "" {
			log.Printf("[WebRTC] ZMQ response: %s", replyStr)
		}

		return nil
	}

	// Cleanup goroutine to remove expired subtitles
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				changed := false
				for id, sub := range activeSubtitles {
					if now.After(sub.endTime) {
						delete(activeSubtitles, id)
						changed = true
					}
				}
				if changed {
					var texts []string
					for _, sub := range activeSubtitles {
						if sub.text != "" {
							texts = append(texts, sub.text)
						}
					}
					newText := strings.Join(texts, "\\n")
					if err := sendTextUpdate(newText); err != nil {
						log.Printf("[WebRTC] Failed to send ZMQ update: %v", err)
					}
				}
			}
		}
	}()

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			log.Printf("[WebRTC] StreamSubtitlesViaZMQ context done (events: %d)", eventCount)
			<-cleanupDone
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[Events]") {
			inEventsSection = true
			continue
		}

		if strings.HasPrefix(line, "[") {
			inEventsSection = false
			continue
		}

		if inEventsSection && strings.HasPrefix(line, "Format:") {
			parser.SetFormat(line)
			continue
		}

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

			duration := event.End - event.Start
			maxDuration := 10 * time.Second
			if duration > maxDuration || duration <= 0 {
				duration = 5 * time.Second
			}

			activeSubtitles[event.ID] = &activeSubtitle{
				text:    event.Text,
				endTime: time.Now().Add(duration),
			}

			var texts []string
			for _, sub := range activeSubtitles {
				if sub.text != "" {
					texts = append(texts, sub.text)
				}
			}
			newText := strings.Join(texts, "\\n")
			if err := sendTextUpdate(newText); err != nil {
				log.Printf("[WebRTC] Failed to send ZMQ update: %v", err)
			}

			if eventCount <= 5 || eventCount%100 == 0 {
				log.Printf("[WebRTC] ZMQ subtitle sent #%d: %q (duration: %v)", eventCount, event.Text, duration)
			}
		}
	}

	// Clear text when stream ends
	if err := sendTextUpdate(""); err != nil {
		log.Printf("[WebRTC] Failed to clear ZMQ subtitle text: %v", err)
	}
	<-cleanupDone

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("subtitle scanner error: %w", err)
	}

	log.Printf("[WebRTC] StreamSubtitlesViaZMQ ended (events: %d)", eventCount)
	return nil
}
