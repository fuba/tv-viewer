package mpegts

import (
	"bytes"
	"errors"
)

const maxElementaryBuffer = 16 * 1024 * 1024

var ErrElementaryBufferTooLarge = errors.New("MPEG elementary stream buffer exceeds safety limit")

// VideoFrame is one MPEG-2 picture assembled from one or more PES payloads.
type VideoFrame struct {
	Data        []byte
	PTS         uint64
	DTS         uint64
	HasPTS      bool
	HasDTS      bool
	PictureType byte
}

// MPEG2VideoAssembler finds picture start codes without decoding or changing
// the MPEG-2 elementary stream. It keeps one incomplete picture between calls.
type MPEG2VideoAssembler struct {
	buffer     []byte
	hasPicture bool
	timings    []elementaryTiming
}

type elementaryTiming struct {
	offset int
	PTS    uint64
	DTS    uint64
	hasPTS bool
	hasDTS bool
}

func NewMPEG2VideoAssembler() *MPEG2VideoAssembler {
	return &MPEG2VideoAssembler{buffer: make([]byte, 0, 512*1024)}
}

func (a *MPEG2VideoAssembler) Push(packet PESPacket) ([]VideoFrame, error) {
	if packet.Discontinuity {
		a.Reset()
	}
	if packet.HasPTS || packet.HasDTS {
		a.timings = append(a.timings, elementaryTiming{
			offset: len(a.buffer), PTS: packet.PTS, DTS: packet.DTS,
			hasPTS: packet.HasPTS, hasDTS: packet.HasDTS,
		})
	}
	a.buffer = append(a.buffer, packet.Payload...)
	if len(a.buffer) > maxElementaryBuffer {
		return nil, ErrElementaryBufferTooLarge
	}
	return a.extract(), nil
}

func (a *MPEG2VideoAssembler) Reset() {
	a.buffer = nil
	a.timings = nil
	a.hasPicture = false
}

func (a *MPEG2VideoAssembler) Flush() []VideoFrame {
	if len(a.buffer) == 0 {
		return nil
	}
	frame := makeVideoFrame(a.buffer)
	if timing, ok := a.takeTiming(0); ok {
		applyVideoTiming(&frame, timing)
	}
	a.buffer = nil
	a.timings = nil
	return []VideoFrame{frame}
}

func (a *MPEG2VideoAssembler) extract() []VideoFrame {
	var frames []VideoFrame
	searchFrom := 0
	for {
		picture := indexStartCode(a.buffer, 0x00, searchFrom)
		if picture < 0 {
			// Preserve sequence/GOP headers until the first picture arrives. Once
			// synchronized, retain only a possible split start code after damage.
			if a.hasPicture && len(a.buffer) > 3 {
				a.consume(len(a.buffer) - 3)
			}
			return frames
		}
		a.hasPicture = true
		next := indexStartCode(a.buffer, 0x00, picture+4)
		if next < 0 {
			return frames
		}
		frame := makeVideoFrame(a.buffer[:next])
		if timing, ok := a.takeTiming(picture); ok {
			applyVideoTiming(&frame, timing)
		}
		frames = append(frames, frame)
		a.consume(next)
		searchFrom = 0
	}
}

func (a *MPEG2VideoAssembler) takeTiming(pictureOffset int) (elementaryTiming, bool) {
	selected := -1
	for i, timing := range a.timings {
		if timing.offset <= pictureOffset {
			selected = i
		}
	}
	if selected < 0 {
		return elementaryTiming{}, false
	}
	timing := a.timings[selected]
	a.timings = append(a.timings[:selected], a.timings[selected+1:]...)
	return timing, true
}

func (a *MPEG2VideoAssembler) consume(count int) {
	a.buffer = append([]byte(nil), a.buffer[count:]...)
	kept := a.timings[:0]
	for _, timing := range a.timings {
		timing.offset -= count
		if timing.offset >= 0 {
			kept = append(kept, timing)
		}
	}
	a.timings = kept
}

func applyVideoTiming(frame *VideoFrame, timing elementaryTiming) {
	frame.PTS, frame.DTS = timing.PTS, timing.DTS
	frame.HasPTS, frame.HasDTS = timing.hasPTS, timing.hasDTS
}

func makeVideoFrame(data []byte) VideoFrame {
	return VideoFrame{Data: append([]byte(nil), data...), PictureType: pictureCodingType(data)}
}

func pictureCodingType(data []byte) byte {
	picture := indexStartCode(data, 0x00, 0)
	if picture < 0 || picture+6 > len(data) {
		return 0
	}
	return (data[picture+5] >> 3) & 0x07
}

// AudioFrame is one complete AAC ADTS frame.
type AudioFrame struct {
	Data       []byte
	SampleRate int
	Channels   int
	Samples    int
	PTS        uint64
	HasPTS     bool
}

// AACADTSAssembler extracts AAC frames carried in stream type 0x0f PES
// payloads. It supports both CRC-present and CRC-absent ADTS headers.
type AACADTSAssembler struct {
	buffer  []byte
	timings []elementaryTiming
	nextPTS uint64
	hasPTS  bool
}

func NewAACADTSAssembler() *AACADTSAssembler {
	return &AACADTSAssembler{buffer: make([]byte, 0, 64*1024)}
}

func (a *AACADTSAssembler) Push(packet PESPacket) ([]AudioFrame, error) {
	if packet.Discontinuity {
		a.Reset()
	}
	if packet.HasPTS {
		a.timings = append(a.timings, elementaryTiming{
			offset: len(a.buffer), PTS: packet.PTS, hasPTS: true,
		})
	}
	a.buffer = append(a.buffer, packet.Payload...)
	if len(a.buffer) > maxElementaryBuffer {
		return nil, ErrElementaryBufferTooLarge
	}
	var frames []AudioFrame
	for {
		start := findADTSHeader(a.buffer)
		if start < 0 {
			if len(a.buffer) > 1 {
				a.consume(len(a.buffer) - 1)
			}
			return frames, nil
		}
		if start > 0 {
			a.consume(start)
		}
		if len(a.buffer) < 7 {
			return frames, nil
		}
		headerLength := 7
		if a.buffer[1]&0x01 == 0 {
			headerLength = 9
		}
		frameLength := int(a.buffer[3]&0x03)<<11 | int(a.buffer[4])<<3 | int(a.buffer[5]>>5)
		if frameLength < headerLength {
			a.consume(2)
			continue
		}
		if len(a.buffer) < frameLength {
			return frames, nil
		}
		sampleRate, ok := adtsSampleRates[(a.buffer[2]>>2)&0x0f]
		if !ok {
			// False sync candidates can occur in a damaged or mid-PES payload.
			// Drop two bytes and continue searching instead of killing the live stream.
			a.consume(2)
			continue
		}
		channels := int((a.buffer[2]&0x01)<<2 | (a.buffer[3] >> 6))
		if channels == 0 {
			channels = 2 // Channel configuration 0 is signaled through PCE.
		}
		a.applyTiming()
		samples := 1024 * (int(a.buffer[6]&0x03) + 1)
		frames = append(frames, AudioFrame{
			Data:       append([]byte(nil), a.buffer[:frameLength]...),
			SampleRate: sampleRate,
			Channels:   channels,
			Samples:    samples,
			PTS:        a.nextPTS,
			HasPTS:     a.hasPTS,
		})
		if a.hasPTS {
			a.nextPTS = (a.nextPTS + uint64(samples)*90_000/uint64(sampleRate)) & (1<<33 - 1) // #nosec G115 -- parsed ADTS values are strictly positive
		}
		a.consume(frameLength)
	}
}

func (a *AACADTSAssembler) Reset() {
	a.buffer = nil
	a.timings = nil
	a.nextPTS = 0
	a.hasPTS = false
}

func (a *AACADTSAssembler) applyTiming() {
	selected := -1
	for i, timing := range a.timings {
		if timing.offset <= 0 {
			selected = i
		}
	}
	if selected < 0 {
		return
	}
	a.nextPTS = a.timings[selected].PTS & (1<<33 - 1)
	a.hasPTS = a.timings[selected].hasPTS
	a.timings = append([]elementaryTiming(nil), a.timings[selected+1:]...)
}

func (a *AACADTSAssembler) consume(count int) {
	a.buffer = append([]byte(nil), a.buffer[count:]...)
	for i := range a.timings {
		a.timings[i].offset -= count
	}
}

var adtsSampleRates = map[byte]int{
	0: 96000, 1: 88200, 2: 64000, 3: 48000, 4: 44100,
	5: 32000, 6: 24000, 7: 22050, 8: 16000, 9: 12000,
	10: 11025, 11: 8000, 12: 7350,
}

func findADTSHeader(data []byte) int {
	for i := 0; i+2 < len(data); i++ {
		sampleRateIndex := (data[i+2] >> 2) & 0x0f
		if data[i] == 0xff && data[i+1]&0xf6 == 0xf0 &&
			(data[i+2]>>6)&0x03 != 0x03 && sampleRateIndex < 13 {
			return i
		}
	}
	return -1
}

func indexStartCode(data []byte, code byte, from int) int {
	needle := []byte{0, 0, 1, code}
	index := bytes.Index(data[from:], needle)
	if index < 0 {
		return -1
	}
	return index + from
}

// BuildADTSFrame creates a small valid ADTS frame for unit tests and tools.
func BuildADTSFrame(payload []byte, sampleRateIndex byte, channels byte) []byte {
	if len(payload) > 0x1fff-7 {
		return nil
	}
	frameLength := 7 + len(payload)
	header := make([]byte, 7)
	header[0] = 0xff
	header[1] = 0xf1
	header[2] = 0x40 | (sampleRateIndex << 2) | (channels >> 2)
	header[3] = (channels&0x03)<<6 | byte(frameLength>>11) // #nosec G115 -- ADTS frame length is bounded above
	header[4] = byte(frameLength >> 3)                     // #nosec G115 -- only the encoded low byte is required
	header[5] = byte(frameLength&0x07)<<5 | 0x1f
	header[6] = 0xfc
	return append(header, payload...)
}
