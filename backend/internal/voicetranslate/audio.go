package voicetranslate

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"
)

const decimatorTaps = 63
const maxSpeechQueueSamples = 48000 * 2 * 8

// PCMConverter applies an anti-aliasing FIR filter while downmixing 48 kHz
// stereo PCM to the 16 kHz mono PCM required by VoiceTranslate.
type PCMConverter struct {
	coefficients [decimatorTaps]float64
	history      [decimatorTaps]float64
	historyAt    int
	phase        int
}

func NewPCMConverter() *PCMConverter {
	converter := &PCMConverter{}
	const cutoff = 0.15 // 7.2 kHz at 48 kHz, below the 8 kHz output Nyquist limit.
	middle := float64(decimatorTaps-1) / 2
	var sum float64
	for index := 0; index < decimatorTaps; index++ {
		distance := float64(index) - middle
		coefficient := 2 * cutoff
		if distance != 0 {
			coefficient = math.Sin(2*math.Pi*cutoff*distance) / (math.Pi * distance)
		}
		coefficient *= 0.54 - 0.46*math.Cos(2*math.Pi*float64(index)/float64(decimatorTaps-1))
		converter.coefficients[index] = coefficient
		sum += coefficient
	}
	for index := range converter.coefficients {
		converter.coefficients[index] /= sum
	}
	return converter
}

func (c *PCMConverter) Push48kStereo(input []int16) []byte {
	if c == nil || len(input) < 2 {
		return nil
	}
	output := make([]byte, 0, len(input)/3)
	for index := 0; index+1 < len(input); index += 2 {
		mono := float64(int32(input[index])+int32(input[index+1])) / 2
		c.history[c.historyAt] = mono
		c.historyAt = (c.historyAt + 1) % decimatorTaps
		c.phase++
		if c.phase != 3 {
			continue
		}
		c.phase = 0
		var filtered float64
		position := c.historyAt
		for coefficient := 0; coefficient < decimatorTaps; coefficient++ {
			position--
			if position < 0 {
				position = decimatorTaps - 1
			}
			filtered += c.coefficients[coefficient] * c.history[position]
		}
		if filtered > math.MaxInt16 {
			filtered = math.MaxInt16
		} else if filtered < math.MinInt16 {
			filtered = math.MinInt16
		}
		value := int16(math.Round(filtered))
		output = binary.LittleEndian.AppendUint16(output, uint16(value))
	}
	return output
}

// WAV is decoded PCM16 audio from a VoiceTranslate speech event.
type WAV struct {
	SampleRate int
	Channels   int
	Samples    []int16
}

func DecodeWAV(data []byte) (WAV, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return WAV{}, errors.New("invalid RIFF/WAVE header")
	}
	if len(data) > 4<<20 {
		return WAV{}, errors.New("VoiceTranslate WAV exceeds 4 MiB")
	}
	var sampleRate, channels, bits, audioFormat int
	var pcm []byte
	for offset := 12; offset+8 <= len(data); {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		start := offset + 8
		end := start + chunkSize
		if chunkSize < 0 || end < start || end > len(data) {
			return WAV{}, errors.New("invalid WAV chunk size")
		}
		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return WAV{}, errors.New("invalid WAV format chunk")
			}
			audioFormat = int(binary.LittleEndian.Uint16(data[start : start+2]))
			channels = int(binary.LittleEndian.Uint16(data[start+2 : start+4]))
			sampleRate = int(binary.LittleEndian.Uint32(data[start+4 : start+8]))
			bits = int(binary.LittleEndian.Uint16(data[start+14 : start+16]))
		case "data":
			pcm = data[start:end]
		}
		offset = end + chunkSize%2
	}
	if audioFormat != 1 || bits != 16 || (channels != 1 && channels != 2) || sampleRate < 8000 || sampleRate > 96000 {
		return WAV{}, fmt.Errorf("unsupported WAV format: pcm=%d rate=%d channels=%d bits=%d", audioFormat, sampleRate, channels, bits)
	}
	if len(pcm) == 0 || len(pcm)%2 != 0 || (len(pcm)/2)%channels != 0 {
		return WAV{}, errors.New("invalid WAV PCM payload")
	}
	samples := make([]int16, len(pcm)/2)
	for index := range samples {
		samples[index] = int16(binary.LittleEndian.Uint16(pcm[index*2 : index*2+2]))
	}
	return WAV{SampleRate: sampleRate, Channels: channels, Samples: samples}, nil
}

func ResampleTo48kStereo(input WAV) ([]int16, error) {
	if input.SampleRate < 8000 || input.SampleRate > 96000 || (input.Channels != 1 && input.Channels != 2) || len(input.Samples)%input.Channels != 0 {
		return nil, errors.New("invalid decoded WAV")
	}
	inputFrames := len(input.Samples) / input.Channels
	if inputFrames == 0 {
		return nil, nil
	}
	maxOutputFrames := maxSpeechQueueSamples / 2
	if inputFrames > maxOutputFrames*input.SampleRate/48000 {
		return nil, errors.New("VoiceTranslate speech exceeds 8 seconds")
	}
	outputFrames := (inputFrames*48000 + input.SampleRate - 1) / input.SampleRate
	if outputFrames > maxOutputFrames {
		return nil, errors.New("VoiceTranslate speech exceeds 8 seconds")
	}
	output := make([]int16, outputFrames*2)
	monoAt := func(frame int) float64 {
		if frame >= inputFrames {
			frame = inputFrames - 1
		}
		if input.Channels == 1 {
			return float64(input.Samples[frame])
		}
		return float64(int32(input.Samples[frame*2])+int32(input.Samples[frame*2+1])) / 2
	}
	for frame := 0; frame < outputFrames; frame++ {
		position := float64(frame) * float64(input.SampleRate) / 48000
		left := int(position)
		fraction := position - float64(left)
		value := monoAt(left)*(1-fraction) + monoAt(left+1)*fraction
		sample := int16(math.Round(value))
		output[frame*2] = sample
		output[frame*2+1] = sample
	}
	return output, nil
}

// SpeechMixer serializes synthesized utterances into the live audio timeline.
type SpeechMixer struct {
	mu        sync.Mutex
	queue     []speechSegment
	offset    int
	queued    int
	cancelled map[string]struct{}
	cancelIDs []string
}

type speechSegment struct {
	captionID string
	samples   []int16
}

func NewSpeechMixer() *SpeechMixer {
	return &SpeechMixer{cancelled: make(map[string]struct{})}
}

func (m *SpeechMixer) Enqueue(captionID string, samples []int16) bool {
	if m == nil || captionID == "" || len(samples) == 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, cancelled := m.cancelled[captionID]; cancelled {
		return false
	}
	if len(samples) > maxSpeechQueueSamples {
		samples = samples[:maxSpeechQueueSamples]
	}
	if m.queued+len(samples) > maxSpeechQueueSamples {
		m.queue = nil
		m.offset = 0
		m.queued = 0
	}
	m.queue = append(m.queue, speechSegment{captionID: captionID, samples: append([]int16(nil), samples...)})
	m.queued += len(samples)
	return true
}

func (m *SpeechMixer) Cancel(captionID string) {
	if m == nil || captionID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.cancelled[captionID]; !exists {
		m.cancelled[captionID] = struct{}{}
		m.cancelIDs = append(m.cancelIDs, captionID)
		if len(m.cancelIDs) > 256 {
			delete(m.cancelled, m.cancelIDs[0])
			m.cancelIDs = m.cancelIDs[1:]
		}
	}
	oldOffset := m.offset
	firstSurvived := len(m.queue) > 0 && m.queue[0].captionID != captionID
	kept := m.queue[:0]
	for index, segment := range m.queue {
		remaining := len(segment.samples)
		if index == 0 {
			remaining -= oldOffset
		}
		if segment.captionID == captionID {
			m.queued -= remaining
			continue
		}
		kept = append(kept, segment)
	}
	m.queue = kept
	if firstSurvived {
		m.offset = oldOffset
	} else {
		m.offset = 0
	}
}

func (m *SpeechMixer) TakeStereo(frames int) []int16 {
	if frames <= 0 {
		return nil
	}
	output := make([]int16, frames*2)
	if m == nil {
		return output
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	written := 0
	for written < len(output) && len(m.queue) > 0 {
		current := m.queue[0].samples
		count := min(len(output)-written, len(current)-m.offset)
		copy(output[written:written+count], current[m.offset:m.offset+count])
		written += count
		m.offset += count
		m.queued -= count
		if m.offset == len(current) {
			m.queue = m.queue[1:]
			m.offset = 0
		}
	}
	return output
}
