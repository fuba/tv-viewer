package encoder

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Encoder struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

type Session struct {
	ID        string
	ChannelID string
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	outputDir string
}

func New() *Encoder {
	return &Encoder{
		sessions: make(map[string]*Session),
	}
}

func (e *Encoder) StartEncoding(channelID string, input io.Reader) (*Session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	// Stop existing session for this channel
	if existing, ok := e.sessions[channelID]; ok {
		e.stopSession(existing)
	}
	
	sessionID := fmt.Sprintf("%s-%d", channelID, time.Now().Unix())
	outputDir := filepath.Join("stream", channelID)
	
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}
	
	// Clean old segments
	go e.cleanOldSegments(outputDir)
	
	ctx, cancel := context.WithCancel(context.Background())
	
	// FFmpeg command for HLS encoding (subtitles will be extracted separately if needed)
	cmd := exec.CommandContext(ctx,
		"ffmpeg",
		"-i", "pipe:0", // Input from pipe
		"-y", // Overwrite output files
		// Video encoding for HLS
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "23",
		"-g", "30",
		"-sc_threshold", "0",
		// Audio encoding
		"-c:a", "aac",
		"-b:a", "128k",
		"-ac", "2",
		// HLS output
		"-f", "hls",
		"-hls_time", "4",
		"-hls_list_size", "10",
		"-hls_flags", "delete_segments",
		"-hls_segment_filename", filepath.Join(outputDir, "segment%03d.ts"),
		filepath.Join(outputDir, "playlist.m3u8"),
	)
	
	cmd.Stdin = input
	cmd.Stderr = os.Stderr // For debugging
	
	session := &Session{
		ID:        sessionID,
		ChannelID: channelID,
		cmd:       cmd,
		cancel:    cancel,
		outputDir: outputDir,
	}
	
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	
	e.sessions[channelID] = session
	
	// Monitor the process
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("Encoder for channel %s ended with error: %v", channelID, err)
		}
		e.mu.Lock()
		delete(e.sessions, channelID)
		e.mu.Unlock()
	}()
	
	return session, nil
}

func (e *Encoder) StopEncoding(channelID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	if session, ok := e.sessions[channelID]; ok {
		e.stopSession(session)
		delete(e.sessions, channelID)
	}
}

func (e *Encoder) stopSession(session *Session) {
	if session.cancel != nil {
		session.cancel()
	}
	// Give it a moment to stop gracefully
	time.Sleep(100 * time.Millisecond)
	if session.cmd.Process != nil {
		session.cmd.Process.Kill()
	}
}

func (e *Encoder) cleanOldSegments(dir string) {
	// Clean segments older than 1 minute
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		files, err := filepath.Glob(filepath.Join(dir, "*.ts"))
		if err != nil {
			continue
		}
		
		cutoff := time.Now().Add(-1 * time.Minute)
		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				os.Remove(file)
			}
		}
	}
}

func (e *Encoder) GetPlaylistPath(channelID string) string {
	return filepath.Join("stream", channelID, "playlist.m3u8")
}

func (e *Encoder) GetSubtitlePath(channelID string) string {
	return filepath.Join("stream", channelID, "subtitles.ass")
}