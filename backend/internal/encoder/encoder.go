package encoder

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"
)

// Encoder manages encoding sessions
type Encoder struct {
	mu            sync.Mutex
	sessions      map[string]*Session
	maxConcurrent int
	logs          map[string][]string
	nvencSupport  *NVENCSupport
	useNVENC      bool
}

// Session represents an HLS encoding session (legacy, kept for backward compatibility)
type Session struct {
	ID               string
	ChannelID        string
	cmd              *exec.Cmd
	cancel           context.CancelFunc
	outputDir        string
	stream           io.ReadCloser
	StreamURL        string     // URL of the source stream
	VideoStreamIndex int        // Selected video stream index (-1 for auto)
	AudioStreamIndex int        // Selected audio stream index (-1 for auto)
	StreamInfo       *StreamInfo // Cached stream information
}

// New creates a new Encoder instance
func New() *Encoder {
	e := &Encoder{
		sessions:      make(map[string]*Session),
		maxConcurrent: 1,
		logs:          make(map[string][]string),
	}

	// Check NVENC support
	nvencSupport, err := CheckNVENCSupport()
	if err != nil {
		log.Printf("Error checking NVENC support: %v", err)
	} else {
		e.nvencSupport = nvencSupport
		e.useNVENC = nvencSupport.Available
		if e.useNVENC {
			log.Printf("NVENC hardware encoding enabled with encoders: %v", nvencSupport.Encoders)
		} else {
			log.Printf("NVENC not available, using CPU encoding (libx264)")
		}
	}

	return e
}

// StopEncoding stops encoding for a specific channel (legacy HLS sessions)
func (e *Encoder) StopEncoding(channelID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if session, ok := e.sessions[channelID]; ok {
		e.stopSession(session)
		delete(e.sessions, channelID)
	}
}

// stopSession stops a legacy HLS session
func (e *Encoder) stopSession(session *Session) {
	if session.cancel != nil {
		session.cancel()
	}
	// Close the input stream
	if session.stream != nil {
		session.stream.Close()
	}
	// Give it a moment to stop gracefully
	time.Sleep(100 * time.Millisecond)
	if session.cmd != nil && session.cmd.Process != nil {
		session.cmd.Process.Kill()
	}
}

// StopAllSessions stops all encoding sessions
func (e *Encoder) StopAllSessions() {
	e.mu.Lock()
	defer e.mu.Unlock()

	log.Printf("Stopping all %d encoding sessions", len(e.sessions))
	for channelID, session := range e.sessions {
		log.Printf("Stopping session for channel %s", channelID)
		e.stopSession(session)
		delete(e.sessions, channelID)
	}
}

// GetActiveSessions returns list of active channel IDs
func (e *Encoder) GetActiveSessions() []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	channels := make([]string, 0, len(e.sessions))
	for channelID := range e.sessions {
		channels = append(channels, channelID)
	}
	return channels
}

// GetChannelLogs returns FFmpeg logs for a specific channel
func (e *Encoder) GetChannelLogs(channelID string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if logs, exists := e.logs[channelID]; exists {
		// Return a copy to avoid concurrent access issues
		result := make([]string, len(logs))
		copy(result, logs)
		return result
	}
	return []string{}
}

// GetNVENCStatus returns the current NVENC support status
func (e *Encoder) GetNVENCStatus() map[string]interface{} {
	e.mu.Lock()
	defer e.mu.Unlock()

	status := map[string]interface{}{
		"nvenc_available": e.useNVENC,
		"encoders":        []string{},
	}

	if e.nvencSupport != nil {
		status["encoders"] = e.nvencSupport.Encoders
	}

	return status
}

// SetUseNVENC enables or disables NVENC usage
func (e *Encoder) SetUseNVENC(use bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if use && (e.nvencSupport == nil || !e.nvencSupport.Available) {
		return fmt.Errorf("NVENC is not available on this system")
	}

	e.useNVENC = use
	log.Printf("NVENC usage set to: %v", use)
	return nil
}

// GetSessionInfo returns information about a specific session (WebRTC sessions not tracked here)
func (e *Encoder) GetSessionInfo(channelID string) (map[string]interface{}, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check legacy HLS sessions
	if session, exists := e.sessions[channelID]; exists {
		info := map[string]interface{}{
			"channel_id":         session.ChannelID,
			"session_id":         session.ID,
			"stream_url":         session.StreamURL,
			"video_stream_index": session.VideoStreamIndex,
			"audio_stream_index": session.AudioStreamIndex,
		}
		if session.StreamInfo != nil {
			info["stream_info"] = session.StreamInfo
		}
		return info, nil
	}

	return nil, fmt.Errorf("no active session for channel %s", channelID)
}
