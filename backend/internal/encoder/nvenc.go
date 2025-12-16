package encoder

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// NVENCSupport represents NVENC encoder availability
type NVENCSupport struct {
	Available bool
	Encoders  []string
}

// CheckNVENCSupport checks if NVENC hardware encoding is available
func CheckNVENCSupport() (*NVENCSupport, error) {
	support := &NVENCSupport{
		Available: false,
		Encoders:  []string{},
	}

	// Check for NVIDIA GPU
	cmd := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Printf("No NVIDIA GPU detected: %v", err)
		return support, nil
	}

	gpuName := strings.TrimSpace(out.String())
	if gpuName == "" {
		return support, nil
	}

	log.Printf("NVIDIA GPU detected: %s", gpuName)

	// Check ffmpeg for NVENC support
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return support, fmt.Errorf("ffmpeg not found")
	}

	// Get list of available encoders
	cmd = exec.Command(ffmpegPath, "-encoders")
	out.Reset()
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		return support, fmt.Errorf("failed to get ffmpeg encoders: %w", err)
	}

	encodersOutput := out.String()
	
	// Check for NVENC encoders
	nvencEncoders := []string{
		"h264_nvenc",
		"hevc_nvenc",
	}

	for _, encoder := range nvencEncoders {
		if strings.Contains(encodersOutput, encoder) {
			support.Encoders = append(support.Encoders, encoder)
		}
	}

	if len(support.Encoders) > 0 {
		support.Available = true
		log.Printf("NVENC encoders available: %v", support.Encoders)
	} else {
		log.Printf("No NVENC encoders found in ffmpeg")
	}

	return support, nil
}

// GetVideoCodecArgs returns the appropriate video codec arguments based on NVENC availability
func GetVideoCodecArgs(useNVENC bool, quality string) []string {
	if useNVENC {
		// NVENC hardware encoding arguments
		switch quality {
		case "high":
			return []string{
				"-c:v", "h264_nvenc",
				"-preset", "p4", // p4 = medium quality/speed
				"-tune", "hq",
				"-rc", "vbr",
				"-cq", "23",
				"-b:v", "0",
				"-maxrate", "5M",
				"-bufsize", "10M",
				"-profile:v", "high",
				"-level", "4.1",
			}
		case "medium":
			return []string{
				"-c:v", "h264_nvenc",
				"-preset", "p2", // p2 = fast
				"-rc", "vbr",
				"-cq", "26",
				"-b:v", "0",
				"-maxrate", "3M",
				"-bufsize", "6M",
				"-profile:v", "main",
				"-level", "4.1",
			}
		default: // low/fast
			return []string{
				"-c:v", "h264_nvenc",
				"-preset", "p1", // p1 = fastest
				"-rc", "vbr",
				"-cq", "30",
				"-b:v", "0",
				"-maxrate", "2M",
				"-bufsize", "4M",
				"-profile:v", "main",
				"-level", "4.0",
			}
		}
	} else {
		// CPU encoding with libx264
		switch quality {
		case "high":
			return []string{
				"-c:v", "libx264",
				"-preset", "fast",
				"-crf", "20",
			}
		case "medium":
			return []string{
				"-c:v", "libx264",
				"-preset", "veryfast",
				"-crf", "23",
			}
		default: // low/fast
			return []string{
				"-c:v", "libx264",
				"-preset", "ultrafast",
				"-crf", "26",
			}
		}
	}
}