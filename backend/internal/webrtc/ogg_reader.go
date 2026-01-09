package webrtc

import (
	"encoding/binary"
	"errors"
	"io"
)

// OGG page header structure:
// - "OggS" (4 bytes) - sync pattern
// - version (1 byte) - always 0
// - flags (1 byte) - continuation, BOS, EOS
// - granule_position (8 bytes)
// - serial_number (4 bytes)
// - page_sequence (4 bytes)
// - checksum (4 bytes)
// - page_segments (1 byte)
// - segment_table (page_segments bytes)
// - segment_data (sum of segment_table values)

const (
	oggSyncPattern    = "OggS"
	oggHeaderSize     = 27
	oggFlagContinued  = 0x01
	oggFlagBOS        = 0x02 // Beginning of stream
	oggFlagEOS        = 0x04 // End of stream
)

var (
	ErrInvalidOGG = errors.New("invalid OGG stream")
	ErrOGGSync    = errors.New("OGG sync pattern not found")
)

// OGGReader reads Opus packets from an OGG stream
type OGGReader struct {
	reader         io.Reader
	headerBuf      []byte
	segmentTable   []byte
	currentPacket  []byte
	packetComplete bool
	initialized    bool
}

// NewOGGReader creates a new OGG reader
func NewOGGReader(reader io.Reader) *OGGReader {
	return &OGGReader{
		reader:    reader,
		headerBuf: make([]byte, oggHeaderSize),
	}
}

// ReadOpusPacket reads the next Opus packet from the OGG stream
// Returns the raw Opus packet data (without OGG framing)
func (r *OGGReader) ReadOpusPacket() ([]byte, error) {
	for {
		packet, err := r.readNextPacket()
		if err != nil {
			return nil, err
		}

		// Skip Opus header packets (OpusHead and OpusTags)
		// OpusHead starts with "OpusHead"
		// OpusTags starts with "OpusTags"
		if len(packet) >= 8 {
			header := string(packet[:8])
			if header == "OpusHead" || header == "OpusTags" {
				continue
			}
		}

		// Return audio data packet
		return packet, nil
	}
}

// readNextPacket reads the next complete packet from the OGG stream
func (r *OGGReader) readNextPacket() ([]byte, error) {
	for {
		// Read page header
		if _, err := io.ReadFull(r.reader, r.headerBuf); err != nil {
			return nil, err
		}

		// Verify sync pattern
		if string(r.headerBuf[:4]) != oggSyncPattern {
			// Try to resync
			if err := r.resync(); err != nil {
				return nil, err
			}
			continue
		}

		// Parse header
		pageSegments := int(r.headerBuf[26])

		// Read segment table
		if len(r.segmentTable) < pageSegments {
			r.segmentTable = make([]byte, pageSegments)
		}
		if _, err := io.ReadFull(r.reader, r.segmentTable[:pageSegments]); err != nil {
			return nil, err
		}

		// Calculate total segment data size
		totalSize := 0
		for i := 0; i < pageSegments; i++ {
			totalSize += int(r.segmentTable[i])
		}

		// Read segment data
		segmentData := make([]byte, totalSize)
		if _, err := io.ReadFull(r.reader, segmentData); err != nil {
			return nil, err
		}

		// Check continuation flag
		flags := r.headerBuf[5]
		isContinued := (flags & oggFlagContinued) != 0

		// Parse segments into packets
		offset := 0
		for i := 0; i < pageSegments; i++ {
			segmentSize := int(r.segmentTable[i])

			if isContinued && i == 0 && len(r.currentPacket) > 0 {
				// Continue previous packet
				r.currentPacket = append(r.currentPacket, segmentData[offset:offset+segmentSize]...)
			} else if len(r.currentPacket) > 0 {
				// Append to current packet
				r.currentPacket = append(r.currentPacket, segmentData[offset:offset+segmentSize]...)
			} else {
				// Start new packet
				r.currentPacket = append([]byte{}, segmentData[offset:offset+segmentSize]...)
			}

			offset += segmentSize

			// A segment size < 255 indicates packet end
			if segmentSize < 255 {
				if len(r.currentPacket) > 0 {
					packet := r.currentPacket
					r.currentPacket = nil
					return packet, nil
				}
			}
		}

		// If we reach here with data in currentPacket, it continues in next page
		isContinued = true
	}
}

// resync attempts to find the next OGG sync pattern
func (r *OGGReader) resync() error {
	buf := make([]byte, 1)
	syncBuf := make([]byte, 4)
	copy(syncBuf, r.headerBuf[:4])

	for i := 0; i < 65536; i++ { // Search up to 64KB
		if _, err := io.ReadFull(r.reader, buf); err != nil {
			return err
		}

		// Shift buffer and add new byte
		copy(syncBuf, syncBuf[1:])
		syncBuf[3] = buf[0]

		if string(syncBuf) == oggSyncPattern {
			// Found sync, read rest of header
			copy(r.headerBuf[:4], syncBuf)
			if _, err := io.ReadFull(r.reader, r.headerBuf[4:]); err != nil {
				return err
			}
			return nil
		}
	}

	return ErrOGGSync
}

// GetGranulePosition returns the granule position from the last read page
func (r *OGGReader) GetGranulePosition() uint64 {
	return binary.LittleEndian.Uint64(r.headerBuf[6:14])
}
