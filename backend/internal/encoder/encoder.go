package encoder

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Encoder struct {
	mu            sync.Mutex
	sessions      map[string]*Session
	maxConcurrent int
	logs          map[string][]string
	nvencSupport  *NVENCSupport
	useNVENC      bool
}

type Session struct {
	ID        string
	ChannelID string
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	outputDir string
	stream    io.ReadCloser
}

func New() *Encoder {
	e := &Encoder{
		sessions:      make(map[string]*Session),
		maxConcurrent: 1, // Limit to 1 concurrent encoding for now
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
			log.Printf("NVENC hardware encoding enabled")
		} else {
			log.Printf("Using CPU encoding (libx264)")
		}
	}
	
	return e
}

func (e *Encoder) StartEncoding(channelID string, input io.ReadCloser) (*Session, error) {
	// Check if this is a CS channel (simple heuristic)
	isCSChannel := strings.HasPrefix(channelID, "CS")
	
	// Temporarily disable CS channels due to segmentation faults
	if isCSChannel {
		log.Printf("CS channel %s is currently not supported due to encoding issues", channelID)
		input.Close()
		return nil, fmt.Errorf("CS channels are temporarily unavailable")
	}
	
	return e.startEncodingWithType(channelID, input, isCSChannel)
}

func (e *Encoder) startEncodingWithType(channelID string, input io.ReadCloser, isCSChannel bool) (*Session, error) {
	log.Printf("Starting encoding for channel %s (isCS: %v)", channelID, isCSChannel)
	
	e.mu.Lock()
	defer e.mu.Unlock()
	
	// Check if we're at max capacity
	if len(e.sessions) >= e.maxConcurrent {
		log.Printf("Maximum concurrent encodings (%d) reached. Stopping all existing sessions.", e.maxConcurrent)
		// Stop all existing sessions when at capacity
		for cid, session := range e.sessions {
			log.Printf("Stopping session for channel %s to make room", cid)
			e.stopSession(session)
			delete(e.sessions, cid)
		}
		// Wait for cleanup
		time.Sleep(1 * time.Second)
	}
	
	// Stop existing session for this channel
	if existing, ok := e.sessions[channelID]; ok {
		log.Printf("Stopping existing session for channel %s", channelID)
		e.stopSession(existing)
		delete(e.sessions, channelID) // Ensure it's removed
		// Wait a bit for cleanup
		time.Sleep(500 * time.Millisecond)
	}
	
	// Clean up old stream files for this channel
	streamDir := filepath.Join("stream", channelID)
	if _, err := os.Stat(streamDir); err == nil {
		log.Printf("Cleaning old stream files for channel %s", channelID)
		os.RemoveAll(streamDir)
		// Recreate the directory
		os.MkdirAll(streamDir, 0755)
	}
	
	sessionID := fmt.Sprintf("%s-%d", channelID, time.Now().Unix())
	outputDir := filepath.Join("stream", channelID)
	
	// Create output directory with full path
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}
	
	// Ensure directory is writable
	testFile := filepath.Join(outputDir, ".test")
	if f, err := os.Create(testFile); err != nil {
		return nil, fmt.Errorf("output directory %s is not writable: %w", outputDir, err)
	} else {
		f.Close()
		os.Remove(testFile)
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	// Clean old segments with context
	go e.cleanOldSegments(outputDir, ctx)
	
	// FFmpeg command for HLS encoding (subtitles will be extracted separately if needed)
	ffmpegPath := os.Getenv("FFMPEG_PATH")
	if ffmpegPath == "" {
		var err error
		ffmpegPath, err = exec.LookPath("ffmpeg")
		if err != nil {
			// Try common locations
			for _, path := range []string{"/usr/bin/ffmpeg", "/usr/local/bin/ffmpeg", "ffmpeg"} {
				if _, err := os.Stat(path); err == nil {
					ffmpegPath = path
					break
				}
			}
			if ffmpegPath == "" {
				return nil, fmt.Errorf("ffmpeg not found in PATH")
			}
		}
	}
	log.Printf("Using FFmpeg at: %s", ffmpegPath)
	
	var cmd *exec.Cmd
	
	if isCSChannel {
		// Special handling for CS channels - select first program
		cmd = exec.CommandContext(ctx,
			ffmpegPath,
			// Input configuration for CS channels
			"-f", "mpegts",
			"-analyzeduration", "10000000",
			"-probesize", "10000000",
			"-i", "pipe:0",
			"-y",
			// Select first program's streams
			"-map", "0:1", "-map", "0:2",
		)
		// Add video codec args based on NVENC availability
		quality := GetEncodingQuality()
		cmd.Args = append(cmd.Args, GetVideoCodecArgs(e.useNVENC, quality)...)
		cmd.Args = append(cmd.Args,
			"-c:a", "aac",
			"-b:a", "128k",
			// HLS output
			"-f", "hls",
			"-hls_time", "4",
			"-hls_list_size", "10",
			"-hls_flags", "delete_segments+round_durations",
			"-hls_segment_filename", filepath.Join(outputDir, "segment%03d.ts"),
			filepath.Join(outputDir, "playlist.m3u8"),
		)
	} else {
		// Check if this is a BS channel that needs special handling
		isBSChannel := strings.HasPrefix(channelID, "BS")
		
		if isBSChannel {
			// BS channel configuration - explicit stream mapping
			cmd = exec.CommandContext(ctx,
				ffmpegPath,
				// Input configuration for BS channels
				"-f", "mpegts",
				"-fflags", "+genpts+discardcorrupt", 
				"-analyzeduration", "10000000", // Much longer analysis for BS complex streams
				"-probesize", "5000000", // Much larger probe for BS multi-program streams
				"-avoid_negative_ts", "make_zero",
				"-thread_queue_size", "1024", // Larger queue for BS multi-program streams
				"-i", "pipe:0",
				"-y",
				// Use automatic stream selection for BS channels with fallback
				"-map", "0:v:0", // Map first video stream (remove ? to make it required)
				"-map", "0:a:0", // Map first audio stream (remove ? to make it required)
			)
			// Add video codec args based on NVENC availability
			quality := GetEncodingQuality()
			cmd.Args = append(cmd.Args, GetVideoCodecArgs(e.useNVENC, quality)...)
			cmd.Args = append(cmd.Args,
				"-r", "30",
				"-g", "30",
				"-keyint_min", "30",
				"-sc_threshold", "0",
				// Audio encoding
				"-c:a", "aac",
				"-b:a", "128k",
				"-ac", "2",
				"-ar", "48000",
				// HLS output
				"-f", "hls",
				"-hls_time", "2",
				"-hls_list_size", "3",
				"-hls_flags", "delete_segments+round_durations+independent_segments+omit_endlist",
				"-hls_allow_cache", "0",
				"-hls_start_number_source", "epoch",
				"-hls_init_time", "0.5",
				"-force_key_frames", "expr:gte(t,n_forced*1)",
				"-hls_segment_filename", filepath.Join(outputDir, "segment%03d.ts"),
				filepath.Join(outputDir, "playlist.m3u8"),
			)
		} else {
			// Standard configuration for other channels (GR)
			cmd = exec.CommandContext(ctx,
				ffmpegPath,
				// Input configuration
				"-f", "mpegts", // Specify input format as MPEG2-TS
				"-fflags", "+genpts+discardcorrupt", // Generate PTS, discard corrupt
				"-analyzeduration", "5000000", // Increased analysis time - 5 seconds
				"-probesize", "2000000", // Increased probe size for better stream detection
				"-avoid_negative_ts", "make_zero", // Handle negative timestamps
				"-thread_queue_size", "512", // Increase input thread queue size
				"-i", "pipe:0", // Input from pipe
				"-y", // Overwrite output files
				// Stream selection - more robust mapping
				"-map", "0:v:0", // Map first video stream (remove ? to make it required)
				"-map", "0:a:0", // Map first audio stream (remove ? to make it required)
			)
			// Add video codec args based on NVENC availability
			quality := GetEncodingQuality()
			cmd.Args = append(cmd.Args, GetVideoCodecArgs(e.useNVENC, quality)...)
			cmd.Args = append(cmd.Args,
				"-r", "30", // Force 30fps output
				"-g", "30", // GOP size (1 second at 30fps for 2-second segments)
				"-keyint_min", "30",
				"-sc_threshold", "0",
				// Audio encoding
				"-c:a", "aac",
				"-b:a", "128k",
				"-ac", "2",
				"-ar", "48000",
				// HLS output
				"-f", "hls",
				"-hls_time", "2", // Reduced segment time for faster startup
				"-hls_list_size", "3", // Even smaller playlist for fastest generation
				"-hls_flags", "delete_segments+round_durations+independent_segments+omit_endlist",
				"-hls_allow_cache", "0",
				"-hls_start_number_source", "epoch",
				"-hls_init_time", "0.5", // Force first segment at 0.5 seconds
				"-force_key_frames", "expr:gte(t,n_forced*1)", // Force keyframes every 1 second
				"-hls_segment_filename", filepath.Join(outputDir, "segment%03d.ts"),
				filepath.Join(outputDir, "playlist.m3u8"),
			)
		}
	}
	
	// Create buffered stdin pipe
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	// Capture stderr for debugging with proper goroutine handling
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	
	// Handle stderr in a non-blocking goroutine
	go func() {
		defer stderrPipe.Close()
		buf := make([]byte, 4096)
		for {
			n, err := stderrPipe.Read(buf)
			if err != nil {
				if err.Error() != "EOF" {
					log.Printf("Stderr read error for channel %s: %v", channelID, err)
				}
				break
			}
			if n > 0 {
				logMsg := string(buf[:n])
				log.Printf("FFmpeg [%s]: %s", channelID, logMsg)
				
				// Store in memory logs (use a separate goroutine to avoid mutex deadlock)
				go func(msg string) {
					e.mu.Lock()
					defer e.mu.Unlock()
					if e.logs[channelID] == nil {
						e.logs[channelID] = []string{}
					}
					// Keep last 1000 log entries per channel
					e.logs[channelID] = append(e.logs[channelID], msg)
					if len(e.logs[channelID]) > 1000 {
						e.logs[channelID] = e.logs[channelID][len(e.logs[channelID])-1000:]
					}
				}(logMsg)
			}
		}
	}()
	
	session := &Session{
		ID:        sessionID,
		ChannelID: channelID,
		cmd:       cmd,
		cancel:    cancel,
		outputDir: outputDir,
		stream:    input,
	}
	
	// Initialize logs for this channel before starting (mutex already held)
	e.logs[channelID] = []string{}
	
	log.Printf("About to start FFmpeg command for channel %s: %s %v", channelID, ffmpegPath, cmd.Args[1:])
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start FFmpeg process for channel %s: %v", channelID, err)
		return nil, err
	}
	
	log.Printf("FFmpeg process started for channel %s with PID %d", channelID, cmd.Process.Pid)
	
	// Copy input stream to FFmpeg stdin in a separate goroutine with improved buffering
	go func() {
		defer func() {
			stdinPipe.Close()
			log.Printf("Input stream reader for channel %s stopped", channelID)
		}()
		
		buf := make([]byte, 188*1024) // Use TS packet aligned buffer (188 bytes * 1024)
		totalBytes := int64(0)
		
		for {
			select {
			case <-ctx.Done():
				log.Printf("Context cancelled for channel %s", channelID)
				return
			default:
				n, err := input.Read(buf)
				if err != nil {
					if err != io.EOF {
						log.Printf("Error reading from input stream for channel %s after %d bytes: %v", channelID, totalBytes, err)
					} else {
						log.Printf("Input stream EOF for channel %s after %d bytes", channelID, totalBytes)
					}
					return
				}
				if n > 0 {
					totalBytes += int64(n)
					if totalBytes%1024*1024 == 0 { // Log every MB
						log.Printf("Channel %s: processed %d MB", channelID, totalBytes/(1024*1024))
					}
					
					written := 0
					for written < n {
						select {
						case <-ctx.Done():
							return
						default:
							w, err := stdinPipe.Write(buf[written:n])
							if err != nil {
								log.Printf("Error writing to FFmpeg stdin for channel %s: %v", channelID, err)
								return
							}
							written += w
						}
					}
				}
			}
		}
	}()
	
	e.sessions[channelID] = session
	
	// Monitor the process
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("Encoder for channel %s ended with error: %v", channelID, err)
		} else {
			log.Printf("Encoder for channel %s ended successfully", channelID)
		}
		
		// Clean up when process ends
		e.mu.Lock()
		if session, exists := e.sessions[channelID]; exists {
			if session.stream != nil {
				session.stream.Close()
			}
			delete(e.sessions, channelID)
		}
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
	// Close the input stream
	if session.stream != nil {
		session.stream.Close()
	}
	// Give it a moment to stop gracefully
	time.Sleep(100 * time.Millisecond)
	if session.cmd.Process != nil {
		session.cmd.Process.Kill()
	}
}

func (e *Encoder) cleanOldSegments(dir string, ctx context.Context) {
	// Clean segments older than 1 minute
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			log.Printf("Stopping segment cleaner for %s", dir)
			return
		case <-ticker.C:
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
}

func (e *Encoder) GetPlaylistPath(channelID string) string {
	return filepath.Join("stream", channelID, "playlist.m3u8")
}

func (e *Encoder) GetSubtitlePath(channelID string) string {
	return filepath.Join("stream", channelID, "subtitles.ass")
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