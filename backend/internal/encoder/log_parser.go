package encoder

import (
	"regexp"
	"strconv"
	"strings"
)

// ParsedStreamInfo represents stream information extracted from FFmpeg logs
type ParsedStreamInfo struct {
	VideoStreams []ParsedStream `json:"video_streams"`
	AudioStreams []ParsedStream `json:"audio_streams"`
}

// ParsedStream represents a single stream from FFmpeg logs
type ParsedStream struct {
	Index      int    `json:"index"`
	Type       string `json:"type"`
	Codec      string `json:"codec"`
	Resolution string `json:"resolution,omitempty"`
	Bitrate    string `json:"bitrate,omitempty"`
	Language   string `json:"language,omitempty"`
	Channels   string `json:"channels,omitempty"`
	SampleRate string `json:"sample_rate,omitempty"`
}

// ParseStreamInfoFromLogs extracts stream information from FFmpeg logs
func ParseStreamInfoFromLogs(logs []string) *ParsedStreamInfo {
	info := &ParsedStreamInfo{
		VideoStreams: []ParsedStream{},
		AudioStreams: []ParsedStream{},
	}

	// Regular expressions for parsing FFmpeg stream information
	// Enhanced to catch more stream patterns
	streamRegex := regexp.MustCompile(`Stream #0[:\.](\d+)(?:\[0x[a-f0-9]+\])?(?:\(([^)]*)\))?: (Video|Audio): (.+)`)
	
	// Process all logs to find stream information
	fullLog := strings.Join(logs, "\n")
	
	// Look for stream declarations
	lines := strings.Split(fullLog, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Skip empty lines and non-stream lines
		if line == "" || !strings.Contains(line, "Stream #") {
			continue
		}
		
		matches := streamRegex.FindStringSubmatch(line)
		if len(matches) >= 5 {
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			
			language := strings.TrimSpace(matches[2])
			streamType := strings.TrimSpace(matches[3])
			details := strings.TrimSpace(matches[4])

			stream := ParsedStream{
				Index:    index,
				Type:     strings.ToLower(streamType),
				Language: language,
			}

			if streamType == "Video" {
				parseVideoDetails(&stream, details)
				info.VideoStreams = append(info.VideoStreams, stream)
			} else if streamType == "Audio" {
				parseAudioDetails(&stream, details)
				info.AudioStreams = append(info.AudioStreams, stream)
			}
		}
	}

	return info
}

func parseVideoDetails(stream *ParsedStream, details string) {
	// Parse codec
	parts := strings.Split(details, ",")
	if len(parts) > 0 {
		stream.Codec = strings.TrimSpace(parts[0])
	}

	// Look for resolution and bitrate
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "x") && (strings.Contains(part, "1920") || strings.Contains(part, "1440") || strings.Contains(part, "1280") || strings.Contains(part, "720") || strings.Contains(part, "480")) {
			stream.Resolution = part
		}
		if strings.Contains(part, "kb/s") {
			stream.Bitrate = part
		}
	}
}

func parseAudioDetails(stream *ParsedStream, details string) {
	// Parse codec
	parts := strings.Split(details, ",")
	if len(parts) > 0 {
		stream.Codec = strings.TrimSpace(parts[0])
	}

	// Look for sample rate, channels, and bitrate
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "Hz") {
			stream.SampleRate = part
		}
		if strings.Contains(part, "kb/s") {
			stream.Bitrate = part
		}
		if strings.Contains(part, "stereo") || strings.Contains(part, "mono") || strings.Contains(part, "5.1") {
			stream.Channels = part
		}
	}
}