package voicetranslate

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestPCMConverterProducesFragmentSafe16kMonoFrames(t *testing.T) {
	t.Parallel()
	converter := NewPCMConverter()
	input := make([]int16, 4800*2)
	for frame := 0; frame < 4800; frame++ {
		value := int16(math.Sin(2*math.Pi*1000*float64(frame)/48000) * 12000)
		input[frame*2] = value
		input[frame*2+1] = value
	}

	first := converter.Push48kStereo(input[:317*2])
	second := converter.Push48kStereo(input[317*2:])
	output := append(first, second...)
	if len(output) != 3200 {
		t.Fatalf("converted byte length = %d, want 3200", len(output))
	}
	if output[0] == 0 && output[1] == 0 && output[200] == 0 && output[201] == 0 {
		t.Fatal("non-silent input unexpectedly converted to silence")
	}
	for index := 0; index+1 < len(output); index += 2 {
		_ = int16(binary.LittleEndian.Uint16(output[index : index+2]))
	}
}

func TestPCMConverterAttenuatesFrequenciesAboveOutputNyquist(t *testing.T) {
	t.Parallel()
	convertTone := func(frequency float64) float64 {
		converter := NewPCMConverter()
		input := make([]int16, 48000*2)
		for frame := 0; frame < 48000; frame++ {
			value := int16(math.Sin(2*math.Pi*frequency*float64(frame)/48000) * 12000)
			input[frame*2] = value
			input[frame*2+1] = value
		}
		output := converter.Push48kStereo(input)
		var energy float64
		for index := 200; index+1 < len(output); index += 2 {
			value := float64(int16(binary.LittleEndian.Uint16(output[index : index+2])))
			energy += value * value
		}
		return math.Sqrt(energy / float64((len(output)-200)/2))
	}
	low := convertTone(1000)
	high := convertTone(12000)
	if high >= low/10 {
		t.Fatalf("12 kHz alias energy was not filtered: low RMS %.1f, high RMS %.1f", low, high)
	}
}

func TestDecodeWAVAndResampleTo48kStereo(t *testing.T) {
	t.Parallel()
	wav := pcm16WAVForTest(24000, 1, []int16{0, 12000, -12000, 6000})
	decoded, err := DecodeWAV(wav)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}
	if decoded.SampleRate != 24000 || decoded.Channels != 1 {
		t.Fatalf("unexpected WAV format: %+v", decoded)
	}
	stereo, err := ResampleTo48kStereo(decoded)
	if err != nil {
		t.Fatalf("ResampleTo48kStereo: %v", err)
	}
	if len(stereo) != len(decoded.Samples)*2*2 {
		t.Fatalf("resampled length = %d, want %d", len(stereo), len(decoded.Samples)*4)
	}
	for index := 0; index+1 < len(stereo); index += 2 {
		if stereo[index] != stereo[index+1] {
			t.Fatalf("stereo pair %d differs: %d != %d", index/2, stereo[index], stereo[index+1])
		}
	}
}

func TestResampleRejectsSpeechLongerThanMixerCapacity(t *testing.T) {
	t.Parallel()
	wav := WAV{
		SampleRate: 8000,
		Channels:   1,
		Samples:    make([]int16, 8000*8+1),
	}
	if _, err := ResampleTo48kStereo(wav); err == nil {
		t.Fatal("expected oversized speech to be rejected before resampling")
	}
}

func TestSpeechMixerUsesSilenceUntilSpeechArrives(t *testing.T) {
	t.Parallel()
	mixer := NewSpeechMixer()
	if got := mixer.TakeStereo(4); len(got) != 8 || got[0] != 0 {
		t.Fatalf("initial mix = %v", got)
	}
	mixer.Enqueue("caption-1", []int16{1, 1, 2, 2, 3, 3})
	if got := mixer.TakeStereo(2); len(got) != 4 || got[0] != 1 || got[2] != 2 {
		t.Fatalf("first speech mix = %v", got)
	}
	if got := mixer.TakeStereo(2); len(got) != 4 || got[0] != 3 || got[2] != 0 {
		t.Fatalf("tail speech mix = %v", got)
	}
}

func TestSpeechMixerDropsStaleQueuedSpeech(t *testing.T) {
	t.Parallel()
	mixer := NewSpeechMixer()
	mixer.Enqueue("stale", make([]int16, maxSpeechQueueSamples))
	mixer.Enqueue("latest", []int16{7, 7})
	if got := mixer.TakeStereo(1); len(got) != 2 || got[0] != 7 || got[1] != 7 {
		t.Fatalf("latest speech was not preferred: %v", got)
	}
}

func TestSpeechMixerCancelsQueuedAndFutureSpeechByCaptionID(t *testing.T) {
	t.Parallel()
	mixer := NewSpeechMixer()
	mixer.Enqueue("caption-1", []int16{1, 1, 2, 2})
	mixer.Enqueue("caption-2", []int16{3, 3})
	mixer.Cancel("caption-1")
	if got := mixer.TakeStereo(1); got[0] != 3 || got[1] != 3 {
		t.Fatalf("cancelled speech remained in queue: %v", got)
	}
	mixer.Cancel("caption-3")
	if mixer.Enqueue("caption-3", []int16{4, 4}) {
		t.Fatal("speech arriving after cancellation was accepted")
	}
}

func pcm16WAVForTest(sampleRate, channels int, samples []int16) []byte {
	dataSize := len(samples) * 2
	result := make([]byte, 44+dataSize)
	copy(result[0:4], "RIFF")
	binary.LittleEndian.PutUint32(result[4:8], uint32(36+dataSize))
	copy(result[8:12], "WAVE")
	copy(result[12:16], "fmt ")
	binary.LittleEndian.PutUint32(result[16:20], 16)
	binary.LittleEndian.PutUint16(result[20:22], 1)
	binary.LittleEndian.PutUint16(result[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(result[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(result[28:32], uint32(sampleRate*channels*2))
	binary.LittleEndian.PutUint16(result[32:34], uint16(channels*2))
	binary.LittleEndian.PutUint16(result[34:36], 16)
	copy(result[36:40], "data")
	binary.LittleEndian.PutUint32(result[40:44], uint32(dataSize))
	for index, sample := range samples {
		binary.LittleEndian.PutUint16(result[44+index*2:], uint16(sample))
	}
	return result
}
