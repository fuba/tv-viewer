package encoder

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// StreamInfo represents information about available streams in a media file
type StreamInfo struct {
	Streams []Stream `json:"streams"`
	Format  Format   `json:"format"`
}

// Stream represents a single stream (video, audio, subtitle, etc.)
type Stream struct {
	Index          int               `json:"index"`
	CodecName      string            `json:"codec_name"`
	CodecLongName  string            `json:"codec_long_name"`
	CodecType      string            `json:"codec_type"`
	CodecTimeBase  string            `json:"codec_time_base,omitempty"`
	CodecTagString string            `json:"codec_tag_string,omitempty"`
	CodecTag       string            `json:"codec_tag,omitempty"`
	Width          int               `json:"width,omitempty"`
	Height         int               `json:"height,omitempty"`
	CodedWidth     int               `json:"coded_width,omitempty"`
	CodedHeight    int               `json:"coded_height,omitempty"`
	HasBFrames     int               `json:"has_b_frames,omitempty"`
	PixFmt         string            `json:"pix_fmt,omitempty"`
	Level          int               `json:"level,omitempty"`
	ChromaLocation string            `json:"chroma_location,omitempty"`
	Refs           int               `json:"refs,omitempty"`
	IsAVC          string            `json:"is_avc,omitempty"`
	NalLengthSize  string            `json:"nal_length_size,omitempty"`
	RFrameRate     string            `json:"r_frame_rate,omitempty"`
	AvgFrameRate   string            `json:"avg_frame_rate,omitempty"`
	TimeBase       string            `json:"time_base,omitempty"`
	StartPts       int               `json:"start_pts,omitempty"`
	StartTime      string            `json:"start_time,omitempty"`
	DurationTs     int64             `json:"duration_ts,omitempty"`
	Duration       string            `json:"duration,omitempty"`
	BitRate        string            `json:"bit_rate,omitempty"`
	MaxBitRate     string            `json:"max_bit_rate,omitempty"`
	BitsPerSample  int               `json:"bits_per_sample,omitempty"`
	NbFrames       string            `json:"nb_frames,omitempty"`
	// Audio specific
	SampleFmt     string `json:"sample_fmt,omitempty"`
	SampleRate    string `json:"sample_rate,omitempty"`
	Channels      int    `json:"channels,omitempty"`
	ChannelLayout string `json:"channel_layout,omitempty"`
	BitsPerRawSample int `json:"bits_per_raw_sample,omitempty"`
	// Disposition flags
	Disposition    Disposition       `json:"disposition"`
	// Tags
	Tags           map[string]string `json:"tags,omitempty"`
}

// Disposition represents stream disposition flags
type Disposition struct {
	Default         int `json:"default"`
	Dub             int `json:"dub"`
	Original        int `json:"original"`
	Comment         int `json:"comment"`
	Lyrics          int `json:"lyrics"`
	Karaoke         int `json:"karaoke"`
	Forced          int `json:"forced"`
	HearingImpaired int `json:"hearing_impaired"`
	VisualImpaired  int `json:"visual_impaired"`
	CleanEffects    int `json:"clean_effects"`
	AttachedPic     int `json:"attached_pic"`
	TimedThumbnails int `json:"timed_thumbnails"`
}

// Format represents container format information
type Format struct {
	Filename       string            `json:"filename"`
	NbStreams      int               `json:"nb_streams"`
	NbPrograms     int               `json:"nb_programs"`
	FormatName     string            `json:"format_name"`
	FormatLongName string            `json:"format_long_name"`
	StartTime      string            `json:"start_time,omitempty"`
	Duration       string            `json:"duration,omitempty"`
	Size           string            `json:"size,omitempty"`
	BitRate        string            `json:"bit_rate,omitempty"`
	ProbeScore     int               `json:"probe_score"`
	Tags           map[string]string `json:"tags,omitempty"`
}

// GetStreamInfo analyzes a media stream and returns detailed information about all streams
func GetStreamInfo(streamURL string) (*StreamInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use ffprobe to get detailed stream information
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		"-analyzeduration", "10000000", // 10 seconds analysis
		"-probesize", "5000000", // 5MB probe size
		streamURL,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %v, output: %s", err, string(output))
	}

	var info StreamInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %v", err)
	}

	return &info, nil
}

// GetStreamDescription returns a human-readable description of a stream
func GetStreamDescription(stream Stream) string {
	switch stream.CodecType {
	case "video":
		desc := fmt.Sprintf("Video #%d: %s", stream.Index, stream.CodecLongName)
		if stream.Width > 0 && stream.Height > 0 {
			desc += fmt.Sprintf(" (%dx%d)", stream.Width, stream.Height)
		}
		if stream.BitRate != "" {
			desc += fmt.Sprintf(", %s", stream.BitRate)
		}
		if stream.Tags != nil && stream.Tags["language"] != "" {
			desc += fmt.Sprintf(", %s", stream.Tags["language"])
		}
		return desc
	
	case "audio":
		desc := fmt.Sprintf("Audio #%d: %s", stream.Index, stream.CodecLongName)
		if stream.ChannelLayout != "" {
			desc += fmt.Sprintf(" (%s)", stream.ChannelLayout)
		} else if stream.Channels > 0 {
			desc += fmt.Sprintf(" (%d channels)", stream.Channels)
		}
		if stream.SampleRate != "" {
			desc += fmt.Sprintf(", %s Hz", stream.SampleRate)
		}
		if stream.BitRate != "" {
			desc += fmt.Sprintf(", %s", stream.BitRate)
		}
		if stream.Tags != nil && stream.Tags["language"] != "" {
			desc += fmt.Sprintf(", %s", stream.Tags["language"])
		}
		return desc
	
	case "subtitle":
		desc := fmt.Sprintf("Subtitle #%d: %s", stream.Index, stream.CodecLongName)
		if stream.Tags != nil && stream.Tags["language"] != "" {
			desc += fmt.Sprintf(" (%s)", stream.Tags["language"])
		}
		return desc
	
	default:
		return fmt.Sprintf("%s #%d: %s", strings.Title(stream.CodecType), stream.Index, stream.CodecLongName)
	}
}

// FilterStreamsByType returns all streams of a specific type
func FilterStreamsByType(streams []Stream, codecType string) []Stream {
	var filtered []Stream
	for _, stream := range streams {
		if stream.CodecType == codecType {
			filtered = append(filtered, stream)
		}
	}
	return filtered
}