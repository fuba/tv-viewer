package encoder

import "os"

// GetEncodingQuality returns the quality setting based on environment or default
func GetEncodingQuality() string {
	quality := os.Getenv("ENCODING_QUALITY")
	if quality == "" {
		// Default to high quality when using NVENC
		return "high"
	}
	return quality
}