package encoder

import (
	"bytes"
	"fmt"
	"log"
	"os"
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

	log.Printf("Checking for NVENC support...")

	// Check for NVIDIA GPU
	cmd := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Printf("nvidia-smi command failed: %v", err)
		return support, nil
	}

	gpuName := strings.TrimSpace(out.String())
	if gpuName == "" {
		log.Printf("No GPU name returned from nvidia-smi")
		return support, nil
	}

	log.Printf("NVIDIA GPU detected: %s", gpuName)

	// Check ffmpeg for NVENC support
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		log.Printf("ffmpeg not found in PATH")
		return support, fmt.Errorf("ffmpeg not found")
	}
	log.Printf("Found ffmpeg at: %s", ffmpegPath)

	// Get list of available encoders
	cmd = exec.Command(ffmpegPath, "-encoders")
	out.Reset()
	cmd.Stdout = &out
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		log.Printf("ffmpeg -encoders failed: %v, stderr: %s", err, stderr.String())
		return support, fmt.Errorf("failed to get ffmpeg encoders: %w", err)
	}

	encodersOutput := out.String()
	
	// Check for NVENC encoders
	nvencEncoders := []string{
		"h264_nvenc",
		"hevc_nvenc",
		"av1_nvenc",
	}

	for _, encoder := range nvencEncoders {
		if strings.Contains(encodersOutput, encoder) {
			support.Encoders = append(support.Encoders, encoder)
			log.Printf("Found NVENC encoder: %s", encoder)
		}
	}

	if len(support.Encoders) > 0 {
		support.Available = true
		log.Printf("NVENC is available with encoders: %v", support.Encoders)
	} else {
		log.Printf("No NVENC encoders found in ffmpeg output")
	}

	return support, nil
}

// GetVideoCodecArgs returns the appropriate video codec arguments based on NVENC availability
func GetVideoCodecArgs(useNVENC bool, quality string) []string {
	// Check if HEVC is requested
	useHEVC := os.Getenv("USE_HEVC") == "true"
	
	if useNVENC {
		// Choose codec based on HEVC preference
		codec := "h264_nvenc"
		if useHEVC {
			codec = "hevc_nvenc"
		}
		
		// NVENC hardware encoding arguments
		switch quality {
		case "high":
			args := []string{
				"-c:v", codec,
				"-preset", "p5", // p5 = slower encoding, better quality
				"-tune", "hq",
				"-rc", "vbr",
				"-cq", "19", // Lower CQ = higher quality (0-51, lower is better)
				"-b:v", "0",
				"-maxrate", "10M", // Increased for better quality
				"-bufsize", "20M",
			}
			
			// Add codec-specific profile and level
			if useHEVC {
				args = append(args, "-profile:v", "main", "-level", "5.1")
			} else {
				args = append(args, "-profile:v", "high", "-level", "4.2")
			}
			
			// Add quality enhancement options
			args = append(args,
				"-b_ref_mode", "2", // Better B-frame handling
				"-temporal-aq", "1", // Temporal AQ for better quality
				"-spatial-aq", "1", // Spatial AQ for better quality
			)
			
			return args
			
		case "medium":
			args := []string{
				"-c:v", codec,
				"-preset", "p4", // p4 = balanced quality/speed
				"-tune", "hq",
				"-rc", "vbr",
				"-cq", "22", // Improved quality (was 26)
				"-b:v", "0",
				"-maxrate", "6M", // Increased bitrate
				"-bufsize", "12M",
			}
			
			// Add codec-specific profile and level
			if useHEVC {
				args = append(args, "-profile:v", "main", "-level", "5.0")
			} else {
				args = append(args, "-profile:v", "high", "-level", "4.1")
			}
			
			args = append(args, "-spatial-aq", "1") // Spatial AQ for better quality
			return args
			
		default: // low/fast
			args := []string{
				"-c:v", codec,
				"-preset", "p2", // p2 = fast (was p1)
				"-rc", "vbr",
				"-cq", "26", // Better quality than 30
				"-b:v", "0",
				"-maxrate", "4M", // Increased
				"-bufsize", "8M",
			}
			
			// Add codec-specific profile and level
			if useHEVC {
				args = append(args, "-profile:v", "main", "-level", "4.1")
			} else {
				args = append(args, "-profile:v", "main", "-level", "4.1")
			}
			
			return args
		}
	} else {
		// CPU encoding with libx264 or libx265
		codec := "libx264"
		if useHEVC {
			codec = "libx265"
		}
		
		switch quality {
		case "high":
			return []string{
				"-c:v", codec,
				"-preset", "fast",
				"-crf", "20",
			}
		case "medium":
			return []string{
				"-c:v", codec,
				"-preset", "veryfast",
				"-crf", "23",
			}
		default: // low/fast
			return []string{
				"-c:v", codec,
				"-preset", "ultrafast",
				"-crf", "26",
			}
		}
	}
}