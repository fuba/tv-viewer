package encoder

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// WebRTCSession represents an active WebRTC encoding session
type WebRTCSession struct {
	ID               string
	ChannelID        string
	cmd              *exec.Cmd
	cancel           context.CancelFunc
	ctx              context.Context
	stream           io.ReadCloser
	VideoPipe        io.ReadCloser // FFmpeg H.264 video output (stdout)
	AudioPipe        io.ReadCloser // FFmpeg Opus audio output
	SubtitlePipe     io.ReadCloser // FFmpeg subtitle output
	audioFifoPath    string        // Path to audio FIFO
	subtitleFifoPath string        // Path to subtitle FIFO
	VideoStreamIndex int
	AudioStreamIndex int
	StreamURL        string
	mu               sync.Mutex
	started          bool
	stopped          bool // Flag to ensure idempotent Stop()
}

// StartWebRTCEncoding starts FFmpeg encoding for WebRTC output (H.264 + Opus to stdout)
func (e *Encoder) StartWebRTCEncoding(channelID string, input io.ReadCloser, streamURL string, videoIndex, audioIndex int) (*WebRTCSession, error) {
	log.Printf("[WebRTC] Starting encoding for channel %s", channelID)

	e.mu.Lock()
	defer e.mu.Unlock()

	// Stop existing HLS session if any (for backward compatibility)
	if existing, ok := e.sessions[channelID]; ok {
		log.Printf("[WebRTC] Stopping existing HLS session for channel %s", channelID)
		e.stopSession(existing)
		delete(e.sessions, channelID)
	}

	// Auto-select video stream if not specified
	if videoIndex == -1 && streamURL != "" {
		streamInfo, err := GetStreamInfo(streamURL)
		if err != nil {
			log.Printf("[WebRTC] Failed to get stream info: %v, using default", err)
		} else {
			selectedIndex := SelectLargestVideoStream(streamInfo)
			if selectedIndex >= 0 {
				videoIndex = selectedIndex
				log.Printf("[WebRTC] Auto-selected video stream %d", videoIndex)
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	sessionID := fmt.Sprintf("webrtc-%s-%d", channelID, time.Now().Unix())

	// Build FFmpeg command for WebRTC output
	ffmpegPath := e.getFFmpegPath()
	if ffmpegPath == "" {
		cancel()
		return nil, fmt.Errorf("ffmpeg not found")
	}

	// Create audio FIFO for Opus output
	audioFifoPath := fmt.Sprintf("/tmp/webrtc-audio-%s.fifo", sessionID)
	// Remove existing FIFO if any
	os.Remove(audioFifoPath)
	if err := syscall.Mkfifo(audioFifoPath, 0644); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create audio FIFO: %w", err)
	}
	log.Printf("[WebRTC] Created audio FIFO: %s", audioFifoPath)

	// Create subtitle FIFO for ARIB caption output
	subtitleFifoPath := fmt.Sprintf("/tmp/webrtc-subtitle-%s.fifo", sessionID)
	os.Remove(subtitleFifoPath)
	if err := syscall.Mkfifo(subtitleFifoPath, 0644); err != nil {
		cancel()
		os.Remove(audioFifoPath)
		return nil, fmt.Errorf("failed to create subtitle FIFO: %w", err)
	}
	log.Printf("[WebRTC] Created subtitle FIFO: %s", subtitleFifoPath)

	// Build FFmpeg args for WebRTC (H.264 to stdout, Opus to FIFO, subtitles to FIFO)
	args := e.buildWebRTCFFmpegArgs(channelID, videoIndex, audioIndex, audioFifoPath, subtitleFifoPath)

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)

	// Set up stdin pipe for input stream
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	// Set up stdout pipe for H.264 video output
	videoPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Set up stderr for logging
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	session := &WebRTCSession{
		ID:               sessionID,
		ChannelID:        channelID,
		cmd:              cmd,
		cancel:           cancel,
		ctx:              ctx,
		stream:           input,
		VideoPipe:        videoPipe,
		audioFifoPath:    audioFifoPath,
		subtitleFifoPath: subtitleFifoPath,
		VideoStreamIndex: videoIndex,
		AudioStreamIndex: audioIndex,
		StreamURL:        streamURL,
	}

	// Start FFmpeg
	log.Printf("[WebRTC] Starting FFmpeg: %s %v", ffmpegPath, args)
	if err := cmd.Start(); err != nil {
		cancel()
		os.Remove(audioFifoPath)    // Clean up FIFOs on failure
		os.Remove(subtitleFifoPath)
		return nil, fmt.Errorf("failed to start FFmpeg: %w", err)
	}
	session.started = true
	log.Printf("[WebRTC] FFmpeg started with PID %d", cmd.Process.Pid)

	// Open audio FIFO for reading (this blocks until FFmpeg opens it for writing)
	go func() {
		audioPipe, err := os.Open(audioFifoPath)
		if err != nil {
			log.Printf("[WebRTC] Failed to open audio FIFO: %v", err)
			return
		}
		session.mu.Lock()
		session.AudioPipe = audioPipe
		session.mu.Unlock()
		log.Printf("[WebRTC] Audio FIFO opened for reading")
	}()

	// Open subtitle FIFO for reading (this blocks until FFmpeg opens it for writing)
	go func() {
		subtitlePipe, err := os.Open(subtitleFifoPath)
		if err != nil {
			log.Printf("[WebRTC] Failed to open subtitle FIFO: %v", err)
			return
		}
		session.mu.Lock()
		session.SubtitlePipe = subtitlePipe
		session.mu.Unlock()
		log.Printf("[WebRTC] Subtitle FIFO opened for reading")
	}()

	// Handle stderr logging
	go func() {
		defer stderrPipe.Close()
		buf := make([]byte, 4096)
		for {
			n, err := stderrPipe.Read(buf)
			if err != nil {
				if err.Error() != "EOF" {
					log.Printf("[WebRTC] stderr read error: %v", err)
				}
				break
			}
			if n > 0 {
				logMsg := string(buf[:n])
				// Log important messages only
				if strings.Contains(logMsg, "Error") ||
					strings.Contains(logMsg, "error") ||
					strings.Contains(logMsg, "Stream #") ||
					strings.Contains(logMsg, "Output #") {
					log.Printf("[WebRTC FFmpeg] %s", logMsg)
				}
				// Store logs
				e.mu.Lock()
				if e.logs[channelID] == nil {
					e.logs[channelID] = []string{}
				}
				e.logs[channelID] = append(e.logs[channelID], logMsg)
				if len(e.logs[channelID]) > 500 {
					e.logs[channelID] = e.logs[channelID][len(e.logs[channelID])-500:]
				}
				e.mu.Unlock()
			}
		}
	}()

	// Copy input stream to FFmpeg stdin with timeout support
	go func() {
		defer func() {
			stdinPipe.Close()
			log.Printf("[WebRTC] Input stream closed for channel %s", channelID)
		}()

		var reader io.Reader
		if os.Getenv("SKIP_TS_FILTER") == "true" {
			reader = input
		} else {
			filteredInput := NewFilteredReader(input)
			defer filteredInput.Close()
			reader = filteredInput
		}

		buf := make([]byte, 188*1024) // ~188KB buffer

		// Read result type for channel communication
		type readResult struct {
			n   int
			err error
		}

		for {
			// Check context first
			select {
			case <-ctx.Done():
				log.Printf("[WebRTC] Context cancelled, stopping input copy for channel %s", channelID)
				return
			default:
			}

			// Start async read
			readDone := make(chan readResult, 1)
			go func() {
				n, err := reader.Read(buf)
				readDone <- readResult{n, err}
			}()

			// Wait for read with timeout, checking context periodically
			select {
			case <-ctx.Done():
				log.Printf("[WebRTC] Context cancelled during read for channel %s", channelID)
				return
			case result := <-readDone:
				if result.err != nil {
					if result.err != io.EOF {
						log.Printf("[WebRTC] Input read error: %v", result.err)
					}
					return
				}
				if result.n > 0 {
					_, err := stdinPipe.Write(buf[:result.n])
					if err != nil {
						log.Printf("[WebRTC] FFmpeg write error: %v", err)
						return
					}
				}
			case <-time.After(5 * time.Second):
				// Timeout - check context and retry
				select {
				case <-ctx.Done():
					log.Printf("[WebRTC] Context cancelled after timeout for channel %s", channelID)
					return
				default:
					// Continue - the async read goroutine may still complete
					// We'll just start another iteration
					continue
				}
			}
		}
	}()

	// Monitor process
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("[WebRTC] FFmpeg ended with error: %v", err)
		} else {
			log.Printf("[WebRTC] FFmpeg ended successfully")
		}
	}()

	return session, nil
}

// buildWebRTCFFmpegArgs builds FFmpeg arguments for WebRTC output
func (e *Encoder) buildWebRTCFFmpegArgs(channelID string, videoStreamIndex, audioStreamIndex int, audioFifoPath, subtitleFifoPath string) []string {
	isBSChannel := strings.HasPrefix(channelID, "BS")
	isCSChannel := strings.HasPrefix(channelID, "CS")

	args := []string{
		// Input configuration
		"-f", "mpegts",
		"-fflags", "+genpts+discardcorrupt+igndts+ignidx",
		"-err_detect", "ignore_err",
		"-thread_queue_size", "1024",
	}

	// Analysis duration based on channel type
	if isBSChannel || isCSChannel {
		args = append(args, "-analyzeduration", "10000000", "-probesize", "5000000")
	} else {
		args = append(args, "-analyzeduration", "5000000", "-probesize", "2000000")
	}

	// Note: CUDA decoder (mpeg2_cuvid) disabled because scale_cuda/hwdownload
	// are not working properly with this FFmpeg build. Using CPU decode + NVENC encode instead.

	args = append(args,
		"-i", "pipe:0",
		"-y",
	)

	// === Output 1: Video (H.264 to stdout) ===
	args = append(args, "-map", "0:v:0")

	// Video encoding - H.264 for WebRTC
	if isCSChannel {
		// CS channels: copy mode (avoid decoding issues)
		args = append(args, "-c:v", "copy")
	} else if e.useNVENC {
		// NVENC H.264 with balanced latency/quality settings
		// Scale down to 1136x640 (16:9) for lower bandwidth
		// CPU decode + scale, NVENC encode
		args = append(args,
			"-vf", "yadif=1,scale=1136:640",  // Deinterlace + scale
			"-c:v", "h264_nvenc",
			"-preset", "p4",        // Balanced preset
			"-tune", "ll",          // Low latency tuning
			"-rc", "cbr",           // Constant bitrate for stable streaming
			"-b:v", "2M",           // 2 Mbps for smoother 640p
			"-maxrate", "2M",
			"-bufsize", "1M",       // Larger buffer for stable framerate
			"-g", "60",             // GOP size (2 seconds at 30fps)
			"-bf", "0",             // No B-frames for lower latency
			"-profile:v", "baseline", // Baseline profile for WebRTC compatibility
			"-level", "3.1",
		)
	} else {
		// CPU encoding with low latency
		args = append(args,
			"-vf", "scale=1136:640",  // Scale to 640p
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-tune", "zerolatency",
			"-b:v", "1500k",
			"-maxrate", "1500k",
			"-bufsize", "300k",
			"-g", "30",
			"-bf", "0",
			"-profile:v", "baseline",
			"-level", "3.1",
		)
	}

	// Frame rate and video output format
	args = append(args,
		"-r", "30",
		"-an",         // No audio in video output
		"-f", "h264",
		"pipe:1",
	)

	// === Output 2: Audio (Opus to FIFO) ===
	// Output raw Opus frames with minimal container overhead
	args = append(args,
		"-map", "0:a:0?",
		"-c:a", "libopus",
		"-b:a", "128k",
		"-ar", "48000",               // 48kHz sample rate (WebRTC standard)
		"-ac", "2",                   // Stereo
		"-application", "lowdelay",   // Low delay mode for real-time
		"-frame_duration", "20",      // 20ms frames (WebRTC standard)
		"-vn",                        // No video in audio output
		"-f", "ogg",                  // OGG container
		"-page_duration", "20000",    // 20ms page duration (in microseconds)
		"-flush_packets", "1",        // Flush immediately
		audioFifoPath,
	)

	// === Output 3: Subtitles (ASS to FIFO) ===
	// Extract ARIB captions as ASS format for DataChannel transmission
	args = append(args,
		"-map", "0:s:0?",             // First subtitle stream (optional)
		"-c:s", "ass",                // Convert to ASS format
		"-f", "ass",                  // ASS container
		subtitleFifoPath,
	)

	return args
}

// getFFmpegPath returns the path to FFmpeg
func (e *Encoder) getFFmpegPath() string {
	ffmpegPath := os.Getenv("FFMPEG_PATH")
	if ffmpegPath != "" {
		return ffmpegPath
	}

	path, err := exec.LookPath("ffmpeg")
	if err == nil {
		return path
	}

	// Try common locations
	for _, p := range []string{"/usr/bin/ffmpeg", "/usr/local/bin/ffmpeg"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// StopWebRTCEncoding stops a WebRTC encoding session
func (s *WebRTCSession) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure idempotent Stop() - only stop once
	if s.stopped {
		log.Printf("[WebRTC] Session %s already stopped, skipping", s.ID)
		return
	}
	s.stopped = true

	log.Printf("[WebRTC] Stopping session %s", s.ID)

	// Cancel context first to signal all goroutines to stop
	if s.cancel != nil {
		s.cancel()
	}

	// Close the Mirakurun stream first to release the tuner immediately
	if s.stream != nil {
		s.stream.Close()
		s.stream = nil
		log.Printf("[WebRTC] Mirakurun stream closed for session %s", s.ID)
	}

	// Kill FFmpeg process immediately (don't wait for graceful shutdown)
	if s.cmd != nil && s.cmd.Process != nil && s.started {
		s.cmd.Process.Kill()
		s.started = false
	}

	// Close pipes after killing the process
	if s.VideoPipe != nil {
		s.VideoPipe.Close()
		s.VideoPipe = nil
	}

	if s.AudioPipe != nil {
		s.AudioPipe.Close()
		s.AudioPipe = nil
	}

	if s.SubtitlePipe != nil {
		s.SubtitlePipe.Close()
		s.SubtitlePipe = nil
	}

	// Clean up audio FIFO
	if s.audioFifoPath != "" {
		os.Remove(s.audioFifoPath)
		s.audioFifoPath = ""
	}

	// Clean up subtitle FIFO
	if s.subtitleFifoPath != "" {
		os.Remove(s.subtitleFifoPath)
		s.subtitleFifoPath = ""
	}

	log.Printf("[WebRTC] Session %s stopped", s.ID)
}

// Context returns the session's context
func (s *WebRTCSession) Context() context.Context {
	return s.ctx
}

// IsRunning returns true if the session is still running
func (s *WebRTCSession) IsRunning() bool {
	select {
	case <-s.ctx.Done():
		return false
	default:
		return true
	}
}
