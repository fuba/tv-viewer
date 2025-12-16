package encoder

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// UseHostFFmpeg checks if we should use host FFmpeg via SSH or direct execution
func UseHostFFmpeg() bool {
	return os.Getenv("USE_HOST_FFMPEG") == "true"
}

// GetHostFFmpegCommand returns a command that executes FFmpeg on the host
func GetHostFFmpegCommand(args ...string) *exec.Cmd {
	if UseHostFFmpeg() {
		// Option 1: Direct execution if running with host network mode
		// The stream directory needs to be accessible from both container and host
		hostStreamPath := os.Getenv("HOST_STREAM_PATH")
		if hostStreamPath != "" {
			// Replace /app/stream with host path in arguments
			for i, arg := range args {
				args[i] = strings.ReplaceAll(arg, "/app/stream", hostStreamPath)
			}
		}
		
		// Use host ffmpeg directly
		ffmpegPath := os.Getenv("HOST_FFMPEG_PATH")
		if ffmpegPath == "" {
			ffmpegPath = "/usr/bin/ffmpeg"
		}
		return exec.Command(ffmpegPath, args...)
	}
	
	// Use container FFmpeg
	return exec.Command("ffmpeg", args...)
}

// CheckHostNVENC checks NVENC support on the host system
func CheckHostNVENC() (*NVENCSupport, error) {
	if !UseHostFFmpeg() {
		return CheckNVENCSupport()
	}
	
	support := &NVENCSupport{
		Available: false,
		Encoders:  []string{},
	}
	
	// Check host FFmpeg for NVENC support
	ffmpegPath := os.Getenv("HOST_FFMPEG_PATH")
	if ffmpegPath == "" {
		ffmpegPath = "/usr/bin/ffmpeg"
	}
	
	cmd := exec.Command(ffmpegPath, "-encoders")
	output, err := cmd.Output()
	if err != nil {
		return support, fmt.Errorf("failed to check host ffmpeg encoders: %w", err)
	}
	
	encodersOutput := string(output)
	nvencEncoders := []string{"h264_nvenc", "hevc_nvenc"}
	
	for _, encoder := range nvencEncoders {
		if strings.Contains(encodersOutput, encoder) {
			support.Encoders = append(support.Encoders, encoder)
		}
	}
	
	if len(support.Encoders) > 0 {
		support.Available = true
	}
	
	return support, nil
}