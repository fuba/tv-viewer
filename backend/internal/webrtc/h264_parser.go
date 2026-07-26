package webrtc

import (
	"bytes"
	"errors"
	"io"
)

const maxH264ParserBuffer = 16 * 1024 * 1024

var ErrH264BufferTooLarge = errors.New("H.264 parser buffer exceeds safety limit")

// H.264 NAL unit types
const (
	NALTypeSlice    = 1  // Non-IDR slice
	NALTypeDPA      = 2  // Data partition A
	NALTypeDPB      = 3  // Data partition B
	NALTypeDPC      = 4  // Data partition C
	NALTypeIDR      = 5  // IDR slice (keyframe)
	NALTypeSEI      = 6  // Supplemental enhancement information
	NALTypeSPS      = 7  // Sequence parameter set
	NALTypePPS      = 8  // Picture parameter set
	NALTypeAUD      = 9  // Access unit delimiter
	NALTypeEOSeq    = 10 // End of sequence
	NALTypeEOStream = 11 // End of stream
	NALTypeFiller   = 12 // Filler data
)

// NALUnit represents a single H.264 NAL unit
type NALUnit struct {
	Type uint8
	Data []byte
}

// H264Parser parses H.264 Annex B stream into NAL units
type H264Parser struct {
	buffer []byte
}

// NewH264Parser creates a new H.264 parser
func NewH264Parser() *H264Parser {
	return &H264Parser{
		buffer: make([]byte, 0, 1024*1024), // 1MB initial capacity
	}
}

// startCodes for NAL unit detection
var startCode3 = []byte{0x00, 0x00, 0x01}
var startCode4 = []byte{0x00, 0x00, 0x00, 0x01}

// Parse reads data and extracts complete NAL units
// Returns NAL units found and any remaining incomplete data
func (p *H264Parser) Parse(data []byte) ([]NALUnit, error) {
	if len(data) > maxH264ParserBuffer-len(p.buffer) {
		p.Reset()
		return nil, ErrH264BufferTooLarge
	}
	p.buffer = append(p.buffer, data...)

	var nalUnits []NALUnit
	offset := 0

	for {
		// Find next start code
		startIdx := findStartCode(p.buffer[offset:])
		if startIdx < 0 {
			break
		}
		startIdx += offset

		// Determine start code length (3 or 4 bytes)
		startCodeLen := 3
		if startIdx > 0 && p.buffer[startIdx-1] == 0x00 {
			startCodeLen = 4
			startIdx--
		}

		// Find the next start code to determine NAL unit boundary
		nextStart := findStartCode(p.buffer[startIdx+startCodeLen:])
		if nextStart < 0 {
			// No complete NAL unit yet, keep remaining data
			break
		}
		nextStart += startIdx + startCodeLen

		// Check for 4-byte start code
		if nextStart > 0 && p.buffer[nextStart-1] == 0x00 {
			nextStart--
		}

		// Extract NAL unit (without start code)
		nalData := p.buffer[startIdx+startCodeLen : nextStart]
		if len(nalData) > 0 {
			nalType := nalData[0] & 0x1F
			nalUnits = append(nalUnits, NALUnit{
				Type: nalType,
				Data: nalData,
			})
		}

		offset = nextStart
	}

	// Keep unprocessed data in buffer
	if offset > 0 {
		p.buffer = p.buffer[offset:]
	}

	return nalUnits, nil
}

// findStartCode finds the index of the next start code (0x000001)
func findStartCode(data []byte) int {
	return bytes.Index(data, startCode3)
}

// Reset clears the parser buffer
func (p *H264Parser) Reset() {
	p.buffer = p.buffer[:0]
}

// IsKeyframe returns true if the NAL unit is a keyframe (IDR or SPS/PPS)
func (n *NALUnit) IsKeyframe() bool {
	return n.Type == NALTypeIDR || n.Type == NALTypeSPS || n.Type == NALTypePPS
}

// IsVideoSlice returns true if the NAL unit contains video data
func (n *NALUnit) IsVideoSlice() bool {
	return n.Type == NALTypeSlice || n.Type == NALTypeIDR
}

// H264Reader wraps an io.Reader and parses H.264 NAL units
type H264Reader struct {
	reader io.Reader
	parser *H264Parser
	buf    []byte
}

// NewH264Reader creates a new H.264 reader
func NewH264Reader(r io.Reader) *H264Reader {
	return &H264Reader{
		reader: r,
		parser: NewH264Parser(),
		buf:    make([]byte, 32*1024), // 32KB read buffer
	}
}

// ReadNALUnits reads from the underlying reader and returns parsed NAL units
func (r *H264Reader) ReadNALUnits() ([]NALUnit, error) {
	n, err := r.reader.Read(r.buf)
	if err != nil {
		return nil, err
	}

	return r.parser.Parse(r.buf[:n])
}

// Reset clears the parser state
func (r *H264Reader) Reset() {
	r.parser.Reset()
}
