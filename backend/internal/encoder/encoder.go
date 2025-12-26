package encoder

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	ID            string
	ChannelID     string
	cmd           *exec.Cmd
	cancel        context.CancelFunc
	outputDir     string
	stream        io.ReadCloser
	StreamURL     string // URL of the source stream
	VideoStreamIndex int  // Selected video stream index (-1 for auto)
	AudioStreamIndex int  // Selected audio stream index (-1 for auto)
	StreamInfo    *StreamInfo // Cached stream information
}

func New() *Encoder {
	e := &Encoder{
		sessions:      make(map[string]*Session),
		maxConcurrent: 1, // Allow 1 concurrent encoding
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

	// Clean up old stream directories on startup
	streamDir := "stream"
	if err := os.RemoveAll(streamDir); err != nil {
		log.Printf("Warning: Failed to clean up old stream directory: %v", err)
	} else {
		log.Printf("Cleaned up old stream directories")
	}
	// Recreate the stream directory
	if err := os.MkdirAll(streamDir, 0755); err != nil {
		log.Printf("Warning: Failed to create stream directory: %v", err)
	}

	return e
}

func (e *Encoder) StartEncoding(channelID string, input io.ReadCloser) (*Session, error) {
	// Check if this is a CS channel (simple heuristic)
	isCSChannel := strings.HasPrefix(channelID, "CS")
	
	log.Printf("Starting encoding for channel %s (CS: %v)", channelID, isCSChannel)
	return e.startEncodingWithType(channelID, input, isCSChannel)
}

// StartEncodingWithStreamSelection starts encoding with specific stream selection
func (e *Encoder) StartEncodingWithStreamSelection(channelID string, input io.ReadCloser, streamURL string, videoIndex, audioIndex int) (*Session, error) {
	isCSChannel := strings.HasPrefix(channelID, "CS")
	log.Printf("Starting encoding for channel %s with stream selection (video: %d, audio: %d)", channelID, videoIndex, audioIndex)
	return e.startEncodingWithTypeAndStreams(channelID, input, streamURL, isCSChannel, videoIndex, audioIndex)
}

func (e *Encoder) startEncodingWithType(channelID string, input io.ReadCloser, isCSChannel bool) (*Session, error) {
	// Default to automatic stream selection
	return e.startEncodingWithTypeAndStreams(channelID, input, "", isCSChannel, -1, -1)
}

func (e *Encoder) startEncodingWithTypeAndStreams(channelID string, input io.ReadCloser, streamURL string, isCSChannel bool, videoStreamIndex, audioStreamIndex int) (*Session, error) {
	log.Printf("Starting encoding for channel %s (isCS: %v, video: %d, audio: %d)", channelID, isCSChannel, videoStreamIndex, audioStreamIndex)

	// Auto-select largest video stream if not specified and streamURL is available
	// This is important because:
	// 1. Different channels have video streams at different indices (e.g., #0 or #1)
	// 2. Some channels may have multiple video streams with different resolutions
	// 3. We want to automatically select the highest quality stream when available
	// Note: For Japanese terrestrial digital TV, all channels typically have a single
	// 1440x1080 MPEG-2 stream, but the stream index varies between channels.
	if videoStreamIndex == -1 && streamURL != "" {
		log.Printf("Auto-selecting largest video stream for channel %s", channelID)
		streamInfo, err := GetStreamInfo(streamURL)
		if err != nil {
			log.Printf("Failed to get stream info for auto-selection: %v, using default", err)
		} else {
			selectedIndex := SelectLargestVideoStream(streamInfo)
			if selectedIndex >= 0 {
				videoStreamIndex = selectedIndex
				log.Printf("Auto-selected video stream %d (largest resolution) for channel %s", videoStreamIndex, channelID)

				// Log all video streams for debugging
				videoStreams := FilterStreamsByType(streamInfo.Streams, "video")
				for _, vs := range videoStreams {
					log.Printf("  Video stream %d: %dx%d", vs.Index, vs.Width, vs.Height)
				}
			}
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	
	// Check if we're at max capacity
	if len(e.sessions) >= e.maxConcurrent {
		log.Printf("Maximum concurrent encodings (%d) reached. Finding oldest session to stop.", e.maxConcurrent)
		// Find the oldest session to stop (simple FIFO)
		var oldestChannelID string
		var oldestSession *Session
		for cid, session := range e.sessions {
			if oldestSession == nil || oldestChannelID > cid { // Simple string comparison for FIFO-like behavior
				oldestChannelID = cid
				oldestSession = session
			}
		}
		if oldestSession != nil {
			log.Printf("Stopping oldest session for channel %s to make room", oldestChannelID)
			e.stopSession(oldestSession)
			delete(e.sessions, oldestChannelID)
			// Wait for cleanup
			time.Sleep(500 * time.Millisecond)
		}
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
	
	// Use direct URL if provided (better performance)
	useDirectFFmpeg := os.Getenv("USE_DIRECT_FFMPEG")
	useDirectURL := streamURL != "" && useDirectFFmpeg == "true"
	inputSource := "pipe:0"
	
	log.Printf("Channel %s: USE_DIRECT_FFMPEG=%s, streamURL=%s, useDirectURL=%v", 
		channelID, useDirectFFmpeg, streamURL, useDirectURL)
	
	if useDirectURL {
		inputSource = streamURL
		log.Printf("Using direct URL for channel %s: %s", channelID, streamURL)
	} else {
		log.Printf("Using pipe mode for channel %s", channelID)
	}
	
	if isCSChannel {
		// CS channel configuration - experimental copy mode for problematic channels
		cmd = exec.CommandContext(ctx,
			ffmpegPath,
			// Input configuration for CS channels - minimal processing
			"-f", "mpegts",
			"-fflags", "+genpts+discardcorrupt+igndts+ignidx", 
			"-analyzeduration", "10000000", // Reasonable analysis time
			"-probesize", "5000000", // Reasonable probe size
			"-avoid_negative_ts", "make_zero",
			"-thread_queue_size", "1024",
			"-err_detect", "ignore_err", 
			"-max_streams", "50", 
			"-i", inputSource,
			"-y",
			// Try copy mode first to avoid decoding issues
			"-c", "copy", // Copy all streams without re-encoding
			// HLS output - minimal processing
			"-f", "hls",
			"-hls_time", "6", // Longer segments for copy mode
			"-hls_list_size", "6",
			"-hls_flags", "delete_segments+round_durations+independent_segments",
			"-hls_allow_cache", "0",
			"-hls_start_number_source", "generic",
			"-hls_init_time", "3.0",
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
				// Input configuration for BS channels with error resilience
				"-f", "mpegts",
				"-fflags", "+genpts+discardcorrupt+igndts+ignidx", // Enhanced error handling
				"-analyzeduration", "10000000", // Much longer analysis for BS complex streams
				"-probesize", "5000000", // Much larger probe for BS multi-program streams
				"-avoid_negative_ts", "make_zero",
				"-thread_queue_size", "1024", // Larger queue for BS multi-program streams
				"-err_detect", "ignore_err", // Ignore minor errors
			)
			// Add hardware decoder with deinterlacing if NVENC is available
			// Japanese digital TV is typically 1080i (interlaced), so we need to deinterlace
			if e.useNVENC {
				cmd.Args = append(cmd.Args,
					"-c:v", "mpeg2_cuvid",    // CUDA MPEG-2 hardware decoder
					"-deint", "2",             // Adaptive deinterlacing at hardware level
					"-drop_second_field", "1", // Drop second field to convert 30fps -> 29.97fps
				)
			}
			cmd.Args = append(cmd.Args,
				"-i", inputSource,
				"-y",
			)
			// Stream mapping based on selection
			// We use absolute stream indices (e.g., "0:1") instead of relative indices (e.g., "0:v:0")
			// because the stream index values come from ffprobe/API and represent absolute positions
			// in the stream list. This matches what the frontend expects and displays to users.
			if videoStreamIndex >= 0 {
				cmd.Args = append(cmd.Args, "-map", fmt.Sprintf("0:%d", videoStreamIndex))
			} else {
				cmd.Args = append(cmd.Args, "-map", "0:v:0") // Map first video stream by type
			}
			if audioStreamIndex >= 0 {
				cmd.Args = append(cmd.Args, "-map", fmt.Sprintf("0:%d", audioStreamIndex))
			} else {
				cmd.Args = append(cmd.Args, "-map", "0:a:0") // Map first audio stream by type
			}
			cmd.Args = append(cmd.Args,
			)
			// Add video codec args based on NVENC availability
			quality := GetEncodingQuality()
			log.Printf("BS channel encoding with quality=%s, useNVENC=%v", quality, e.useNVENC)
			videoCodecArgs := GetVideoCodecArgs(e.useNVENC, quality)
			log.Printf("Video codec args: %v", videoCodecArgs)
			cmd.Args = append(cmd.Args, videoCodecArgs...)
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
				"-hls_list_size", "10", // Keep 20 seconds of segments for browser compatibility
				"-hls_flags", "delete_segments+round_durations+independent_segments+omit_endlist",
				"-hls_allow_cache", "0",
				"-hls_start_number_source", "generic",
				"-hls_init_time", "0.5",
				"-force_key_frames", "expr:gte(t,n_forced*1)",
				"-hls_segment_filename", filepath.Join(outputDir, "segment%03d.ts"),
				filepath.Join(outputDir, "playlist.m3u8"),
			)
		} else {
			// Standard configuration for other channels (GR)
			cmd = exec.CommandContext(ctx,
				ffmpegPath,
				// Input configuration with enhanced error resilience
				"-f", "mpegts", // Specify input format as MPEG2-TS
				"-fflags", "+genpts+discardcorrupt+igndts+ignidx", // Enhanced error handling
				"-analyzeduration", "5000000", // Increased analysis time - 5 seconds
				"-probesize", "2000000", // Increased probe size for better stream detection
				"-avoid_negative_ts", "make_zero", // Handle negative timestamps
				"-thread_queue_size", "512", // Increase input thread queue size
				"-err_detect", "ignore_err", // Ignore minor errors
			)
			// Add hardware decoder with deinterlacing if NVENC is available
			// Japanese digital TV is typically 1080i (interlaced), so we need to deinterlace
			if e.useNVENC {
				cmd.Args = append(cmd.Args,
					"-c:v", "mpeg2_cuvid",    // CUDA MPEG-2 hardware decoder
					"-deint", "2",             // Adaptive deinterlacing at hardware level
					"-drop_second_field", "1", // Drop second field to convert 30fps -> 29.97fps
				)
			}
			cmd.Args = append(cmd.Args,
				"-i", inputSource, // Input from pipe
				"-y", // Overwrite output files
			)
			// Stream mapping based on selection
			// We use absolute stream indices (e.g., "0:1") instead of relative indices (e.g., "0:v:0")
			// because the stream index values come from ffprobe/API and represent absolute positions
			// in the stream list. This matches what the frontend expects and displays to users.
			if videoStreamIndex >= 0 {
				cmd.Args = append(cmd.Args, "-map", fmt.Sprintf("0:%d", videoStreamIndex))
			} else {
				cmd.Args = append(cmd.Args, "-map", "0:v:0") // Map first video stream by type
			}
			if audioStreamIndex >= 0 {
				cmd.Args = append(cmd.Args, "-map", fmt.Sprintf("0:%d", audioStreamIndex))
			} else {
				cmd.Args = append(cmd.Args, "-map", "0:a:0") // Map first audio stream by type
			}
			cmd.Args = append(cmd.Args,
			)
			// Add video codec args based on NVENC availability
			quality := GetEncodingQuality()
			log.Printf("BS channel encoding with quality=%s, useNVENC=%v", quality, e.useNVENC)
			videoCodecArgs := GetVideoCodecArgs(e.useNVENC, quality)
			log.Printf("Video codec args: %v", videoCodecArgs)
			cmd.Args = append(cmd.Args, videoCodecArgs...)
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
				"-hls_time", "2", // 2-second segments
				"-hls_list_size", "10", // Keep 20 seconds of segments for browser compatibility
				"-hls_flags", "delete_segments+round_durations+independent_segments+omit_endlist",
				"-hls_allow_cache", "0",
				"-hls_start_number_source", "generic",
				"-hls_init_time", "0.5", // Force first segment at 0.5 seconds
				"-force_key_frames", "expr:gte(t,n_forced*1)", // Force keyframes every 1 second
				"-hls_segment_filename", filepath.Join(outputDir, "segment%03d.ts"),
				filepath.Join(outputDir, "playlist.m3u8"),
			)
		}
	}
	
	// Create buffered stdin pipe only if not using direct URL
	var stdinPipe io.WriteCloser
	if !useDirectURL {
		var err error
		stdinPipe, err = cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
	}

	// Capture stderr for debugging with proper goroutine handling
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	
	// Handle stderr in a non-blocking goroutine with real-time stream info parsing
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
				
				// Store in memory logs and parse stream info in real-time
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
					
					// Parse stream info in real-time if session exists and no stream info yet
					if session, exists := e.sessions[channelID]; exists && session.StreamInfo == nil {
						// Look for stream detection patterns in the log message
						if strings.Contains(msg, "Stream #") {
							// Parse all accumulated logs to build stream info
							parsedInfo := ParseStreamInfoFromLogs(e.logs[channelID])
							if len(parsedInfo.VideoStreams) > 0 || len(parsedInfo.AudioStreams) > 0 {
								// Convert to StreamInfo format
								streamInfo := &StreamInfo{
									Streams: make([]Stream, 0),
									Format: Format{
										FormatName: "mpegts",
										FormatLongName: "MPEG-2 Transport Stream",
									},
								}
								
								// Add video streams
								for _, vs := range parsedInfo.VideoStreams {
									stream := Stream{
										Index:         vs.Index,
										CodecName:     vs.Codec,
										CodecLongName: vs.Codec,
										CodecType:     "video",
										BitRate:       vs.Bitrate,
									}
									if vs.Resolution != "" {
										parts := strings.Split(vs.Resolution, "x")
										if len(parts) == 2 {
											if width, err := strconv.Atoi(parts[0]); err == nil {
												stream.Width = width
											}
											if height, err := strconv.Atoi(parts[1]); err == nil {
												stream.Height = height
											}
										}
									}
									if vs.Language != "" {
										if stream.Tags == nil {
											stream.Tags = make(map[string]string)
										}
										stream.Tags["language"] = vs.Language
									}
									streamInfo.Streams = append(streamInfo.Streams, stream)
								}
								
								// Add audio streams
								for _, as := range parsedInfo.AudioStreams {
									stream := Stream{
										Index:         as.Index,
										CodecName:     as.Codec,
										CodecLongName: as.Codec,
										CodecType:     "audio",
										BitRate:       as.Bitrate,
										SampleRate:    strings.TrimSuffix(as.SampleRate, " Hz"),
										ChannelLayout: as.Channels,
									}
									if as.Language != "" {
										if stream.Tags == nil {
											stream.Tags = make(map[string]string)
										}
										stream.Tags["language"] = as.Language
									}
									streamInfo.Streams = append(streamInfo.Streams, stream)
								}
								
								// Update session with stream info
								session.StreamInfo = streamInfo
								log.Printf("Channel %s: Detected %d video streams, %d audio streams", 
									channelID, len(parsedInfo.VideoStreams), len(parsedInfo.AudioStreams))
							}
						}
					}
				}(logMsg)
			}
		}
	}()
	
	session := &Session{
		ID:               sessionID,
		ChannelID:        channelID,
		cmd:              cmd,
		cancel:           cancel,
		outputDir:        outputDir,
		stream:           input,
		StreamURL:        streamURL,
		VideoStreamIndex: videoStreamIndex,
		AudioStreamIndex: audioStreamIndex,
		StreamInfo:       nil, // Will be populated from FFmpeg logs
	}
	
	// Initialize logs for this channel before starting (mutex already held)
	e.logs[channelID] = []string{}
	
	log.Printf("About to start FFmpeg command for channel %s: %s %v", channelID, ffmpegPath, cmd.Args[1:])
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start FFmpeg process for channel %s: %v", channelID, err)
		return nil, err
	}
	
	log.Printf("FFmpeg process started for channel %s with PID %d", channelID, cmd.Process.Pid)
	
	// Copy input stream to FFmpeg stdin with MPEG-TS filtering (only if not using direct URL)
	if !useDirectURL {
		go func() {
			defer func() {
				stdinPipe.Close()
				log.Printf("Input stream reader for channel %s stopped", channelID)
			}()
			
			// Create filtered reader to remove problematic MPEG-TS packets
			// Skip filtering if SKIP_TS_FILTER is set (for better performance)
			var reader io.Reader
			if os.Getenv("SKIP_TS_FILTER") == "true" {
				log.Printf("Skipping TS filtering for channel %s", channelID)
				reader = input
			} else {
				filteredInput := NewFilteredReader(input)
				defer func() {
					filteredInput.LogStats(channelID)
					filteredInput.Close()
				}()
				reader = filteredInput
			}
			
			// Use larger buffer for better performance
			buf := make([]byte, 188*1024*4) // Increased buffer size (752KB)
			totalBytes := int64(0)
			
			for {
				select {
				case <-ctx.Done():
					log.Printf("Context cancelled for channel %s", channelID)
					return
				default:
					n, err := reader.Read(buf)
					if err != nil {
						if err != io.EOF {
							log.Printf("Error reading from filtered stream for channel %s after %d bytes: %v", channelID, totalBytes, err)
						} else {
							log.Printf("Filtered stream EOF for channel %s after %d bytes", channelID, totalBytes)
						}
						return
					}
					if n > 0 {
						totalBytes += int64(n)
						if totalBytes%(10*1024*1024) == 0 { // Log every 10MB instead of 1MB
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
	} else {
		// If using direct URL, just close the input stream since we don't need it
		if input != nil {
			go func() {
				defer input.Close()
				log.Printf("Closing unused input stream for channel %s (using direct URL)", channelID)
			}()
		}
	}
	
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

// GetSessionInfo returns information about a specific session including stream info
func (e *Encoder) GetSessionInfo(channelID string) (map[string]interface{}, error) {
	e.mu.Lock()
	session, exists := e.sessions[channelID]
	logs := make([]string, 0)
	if exists {
		if sessionLogs, logExists := e.logs[channelID]; logExists {
			logs = make([]string, len(sessionLogs))
			copy(logs, sessionLogs)
		}
	}
	e.mu.Unlock()
	
	if !exists {
		return nil, fmt.Errorf("no active session for channel %s", channelID)
	}
	
	info := map[string]interface{}{
		"channel_id": session.ChannelID,
		"session_id": session.ID,
		"stream_url": session.StreamURL,
		"video_stream_index": session.VideoStreamIndex,
		"audio_stream_index": session.AudioStreamIndex,
	}
	
	// Parse stream info from FFmpeg logs if not already cached
	if session.StreamInfo == nil && len(logs) > 0 {
		parsedInfo := ParseStreamInfoFromLogs(logs)
		if len(parsedInfo.VideoStreams) > 0 || len(parsedInfo.AudioStreams) > 0 {
			// Convert parsed info to StreamInfo format
			streamInfo := &StreamInfo{
				Streams: make([]Stream, 0),
			}
			
			// Add video streams
			for _, vs := range parsedInfo.VideoStreams {
				stream := Stream{
					Index:         vs.Index,
					CodecName:     vs.Codec,
					CodecLongName: vs.Codec,
					CodecType:     "video",
					BitRate:       vs.Bitrate,
				}
				if vs.Resolution != "" {
					// Parse resolution (e.g., "1920x1080")
					parts := strings.Split(vs.Resolution, "x")
					if len(parts) == 2 {
						if width, err := strconv.Atoi(parts[0]); err == nil {
							stream.Width = width
						}
						if height, err := strconv.Atoi(parts[1]); err == nil {
							stream.Height = height
						}
					}
				}
				if vs.Language != "" {
					if stream.Tags == nil {
						stream.Tags = make(map[string]string)
					}
					stream.Tags["language"] = vs.Language
				}
				streamInfo.Streams = append(streamInfo.Streams, stream)
			}
			
			// Add audio streams
			for _, as := range parsedInfo.AudioStreams {
				stream := Stream{
					Index:         as.Index,
					CodecName:     as.Codec,
					CodecLongName: as.Codec,
					CodecType:     "audio",
					BitRate:       as.Bitrate,
					SampleRate:    strings.TrimSuffix(as.SampleRate, " Hz"),
					ChannelLayout: as.Channels,
				}
				if as.Language != "" {
					if stream.Tags == nil {
						stream.Tags = make(map[string]string)
					}
					stream.Tags["language"] = as.Language
				}
				streamInfo.Streams = append(streamInfo.Streams, stream)
			}
			
			session.StreamInfo = streamInfo
		}
	}
	
	if session.StreamInfo != nil {
		info["stream_info"] = session.StreamInfo
	}
	
	return info, nil
}

// UpdateStreamSelection updates the stream selection for an active session
func (e *Encoder) UpdateStreamSelection(channelID string, videoIndex, audioIndex int) error {
	e.mu.Lock()
	session, exists := e.sessions[channelID]
	e.mu.Unlock()
	
	if !exists {
		return fmt.Errorf("no active session for channel %s", channelID)
	}
	
	// Store the current input stream
	currentStream := session.stream
	
	// Stop the current session
	e.StopEncoding(channelID)
	
	// Wait for cleanup
	time.Sleep(500 * time.Millisecond)
	
	// Restart with new stream selection
	_, err := e.StartEncodingWithStreamSelection(channelID, currentStream, session.StreamURL, videoIndex, audioIndex)
	return err
}