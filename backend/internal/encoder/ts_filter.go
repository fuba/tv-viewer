package encoder

import (
	"io"
	"log"
)

// MPEG-TS packet constants
const (
	TSPacketSize = 188
	TSSyncByte   = 0x47
)

// TSPacket represents an MPEG-TS packet
type TSPacket struct {
	SyncByte                 byte
	TransportErrorIndicator  bool
	PayloadUnitStartIndicator bool
	TransportPriority        bool
	PID                      uint16
	TransportScramblingControl byte
	AdaptationFieldControl   byte
	ContinuityCounter        byte
	Data                     []byte
}

// FilteredReader wraps an io.Reader and filters out problematic MPEG-TS packets
type FilteredReader struct {
	source     io.Reader
	buffer     []byte
	bufferPos  int
	bufferLen  int
	packetsRead int64
	packetsFiltered int64
}

// NewFilteredReader creates a new filtered reader for MPEG-TS streams
func NewFilteredReader(source io.Reader) *FilteredReader {
	return &FilteredReader{
		source: source,
		buffer: make([]byte, TSPacketSize*1024), // Buffer for 1024 packets
	}
}

// Read implements io.Reader interface with MPEG-TS packet filtering
func (fr *FilteredReader) Read(p []byte) (n int, err error) {
	outputPos := 0
	
	for outputPos < len(p) {
		// Refill buffer if needed
		if fr.bufferPos >= fr.bufferLen {
			fr.bufferLen, err = fr.source.Read(fr.buffer)
			fr.bufferPos = 0
			if err != nil {
				if outputPos > 0 {
					return outputPos, nil // Return what we have
				}
				return 0, err
			}
		}
		
		// Find next sync byte
		syncPos := fr.findNextSyncByte()
		if syncPos == -1 {
			// No sync byte found, skip remaining buffer
			fr.bufferPos = fr.bufferLen
			continue
		}
		
		// Move to sync position
		fr.bufferPos = syncPos
		
		// Check if we have a complete packet
		if fr.bufferPos + TSPacketSize > fr.bufferLen {
			// Incomplete packet, move to beginning and refill
			copy(fr.buffer[0:], fr.buffer[fr.bufferPos:fr.bufferLen])
			fr.bufferLen = fr.bufferLen - fr.bufferPos
			fr.bufferPos = 0
			
			// Try to read more data
			readLen, readErr := fr.source.Read(fr.buffer[fr.bufferLen:])
			fr.bufferLen += readLen
			if readErr != nil && readLen == 0 {
				if outputPos > 0 {
					return outputPos, nil
				}
				return 0, readErr
			}
			continue
		}
		
		// Parse packet
		packet := fr.parsePacket(fr.buffer[fr.bufferPos:fr.bufferPos+TSPacketSize])
		fr.packetsRead++
		
		// Filter problematic packets
		if fr.shouldFilterPacket(packet) {
			fr.packetsFiltered++
			fr.bufferPos += TSPacketSize
			continue
		}
		
		// Copy valid packet to output
		copyLen := TSPacketSize
		if outputPos + copyLen > len(p) {
			copyLen = len(p) - outputPos
		}
		
		copy(p[outputPos:outputPos+copyLen], fr.buffer[fr.bufferPos:fr.bufferPos+copyLen])
		outputPos += copyLen
		fr.bufferPos += TSPacketSize
	}
	
	return outputPos, nil
}

// Close implements io.Closer interface
func (fr *FilteredReader) Close() error {
	if closer, ok := fr.source.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// GetStats returns filtering statistics
func (fr *FilteredReader) GetStats() (read, filtered int64) {
	return fr.packetsRead, fr.packetsFiltered
}

// findNextSyncByte finds the next MPEG-TS sync byte in the buffer
func (fr *FilteredReader) findNextSyncByte() int {
	for i := fr.bufferPos; i < fr.bufferLen; i++ {
		if fr.buffer[i] == TSSyncByte {
			// Verify this is likely a valid sync by checking if next packet also has sync
			if i+TSPacketSize < fr.bufferLen && fr.buffer[i+TSPacketSize] == TSSyncByte {
				return i
			} else if i+TSPacketSize >= fr.bufferLen {
				// Can't verify next packet, assume this is valid
				return i
			}
		}
	}
	return -1
}

// parsePacket parses an MPEG-TS packet
func (fr *FilteredReader) parsePacket(data []byte) *TSPacket {
	if len(data) < 4 {
		return nil
	}
	
	packet := &TSPacket{
		SyncByte: data[0],
		Data:     data,
	}
	
	// Parse header
	if len(data) >= 4 {
		packet.TransportErrorIndicator = (data[1] & 0x80) != 0
		packet.PayloadUnitStartIndicator = (data[1] & 0x40) != 0
		packet.TransportPriority = (data[1] & 0x20) != 0
		packet.PID = uint16((data[1]&0x1F))<<8 | uint16(data[2])
		packet.TransportScramblingControl = (data[3] & 0xC0) >> 6
		packet.AdaptationFieldControl = (data[3] & 0x30) >> 4
		packet.ContinuityCounter = data[3] & 0x0F
	}
	
	return packet
}

// shouldFilterPacket determines if a packet should be filtered out
func (fr *FilteredReader) shouldFilterPacket(packet *TSPacket) bool {
	if packet == nil {
		return true
	}
	
	// Filter packets with transport errors
	if packet.TransportErrorIndicator {
		return true
	}
	
	// Filter null packets (PID 0x1FFF)
	if packet.PID == 0x1FFF {
		return true
	}
	
	// Check for corrupted packet structure
	if packet.SyncByte != TSSyncByte {
		return true
	}
	
	// Check for invalid adaptation field control
	if packet.AdaptationFieldControl == 0x00 {
		return true // Reserved value
	}
	
	// Additional checks for video PIDs with problematic data
	if fr.isVideoPID(packet.PID) && fr.hasInvalidVideoData(packet) {
		return true
	}
	
	return false
}

// isVideoPID checks if the PID is likely a video stream
func (fr *FilteredReader) isVideoPID(pid uint16) bool {
	// Common video PIDs in Japanese digital broadcasting
	// This is a heuristic - in a full implementation, you'd parse the PAT/PMT
	return pid >= 0x100 && pid <= 0x200
}

// hasInvalidVideoData checks for problematic video data patterns
func (fr *FilteredReader) hasInvalidVideoData(packet *TSPacket) bool {
	if len(packet.Data) < 10 {
		return false
	}
	
	// Look for MPEG-2 video start codes in payload
	if packet.PayloadUnitStartIndicator {
		// Skip TS header and adaptation field if present
		payloadStart := 4
		if packet.AdaptationFieldControl&0x02 != 0 {
			if len(packet.Data) > 4 {
				adaptationLength := int(packet.Data[4])
				payloadStart += 1 + adaptationLength
			}
		}
		
		if payloadStart < len(packet.Data) {
			payload := packet.Data[payloadStart:]
			
			// Check for PES header and video start codes
			if len(payload) >= 9 && payload[0] == 0x00 && payload[1] == 0x00 && payload[2] == 0x01 {
				// This is a PES packet, check for video start codes
				if len(payload) >= 15 {
					// Look for sequence header start code (0x000001B3)
					for i := 9; i < len(payload)-4; i++ {
						if payload[i] == 0x00 && payload[i+1] == 0x00 && 
						   payload[i+2] == 0x01 && payload[i+3] == 0xB3 {
							// Found sequence header, check for valid dimensions
							if i+7 < len(payload) {
								width := (uint16(payload[i+4])<<4) | (uint16(payload[i+5])>>4)
								height := (uint16(payload[i+5]&0x0F)<<8) | uint16(payload[i+6])
								
								// Filter out invalid dimensions (0x0, too large, etc.)
								if width == 0 || height == 0 || width > 4096 || height > 2160 {
									return true
								}
							}
						}
					}
					
					// Also look for picture header start code (0x00000100) with invalid data
					for i := 9; i < len(payload)-4; i++ {
						if payload[i] == 0x00 && payload[i+1] == 0x00 && 
						   payload[i+2] == 0x01 && payload[i+3] == 0x00 {
							// Found picture header, this packet might contain corrupted video data
							// In CS channels, these often cause the 0x0 frame dimension error
							// Filter out picture headers that appear too frequently (likely corrupted)
							return true
						}
					}
				}
			}
		}
	}
	
	// Check for repeated error patterns in video data
	if fr.isVideoPID(packet.PID) {
		// Look for patterns that typically cause FFmpeg to report 0x0 dimensions
		payload := packet.Data[4:] // Skip TS header
		if len(payload) >= 8 {
			// Check for sequences of null bytes that might indicate corrupted data
			nullCount := 0
			for i := 0; i < len(payload) && i < 32; i++ {
				if payload[i] == 0x00 {
					nullCount++
				}
			}
			// If more than 75% of the first 32 bytes are null, likely corrupted
			if nullCount > 24 {
				return true
			}
		}
	}
	
	return false
}

// LogStats logs filtering statistics
func (fr *FilteredReader) LogStats(channelID string) {
	read, filtered := fr.GetStats()
	if read > 0 {
		filterRate := float64(filtered) / float64(read) * 100
		log.Printf("Channel %s: Filtered %d/%d packets (%.2f%%)", channelID, filtered, read, filterRate)
	}
}