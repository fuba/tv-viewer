package voicetranslate

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"
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

func TestPlaybackRateForBacklogExtendsStandardClientCurve(t *testing.T) {
	t.Parallel()
	tests := []struct {
		backlogSeconds float64
		want           float64
	}{
		{backlogSeconds: 0, want: 1},
		{backlogSeconds: 1.49, want: 1},
		{backlogSeconds: 1.5, want: 1.1},
		{backlogSeconds: 4, want: 1.2},
		{backlogSeconds: 8, want: 1.3},
		{backlogSeconds: 12, want: 1.5},
		{backlogSeconds: 20, want: 1.75},
		{backlogSeconds: 30, want: 2},
		{backlogSeconds: 60, want: 2},
	}
	for _, test := range tests {
		if got := playbackRateForBacklog(test.backlogSeconds); got != test.want {
			t.Errorf("playbackRateForBacklog(%v) = %v, want %v", test.backlogSeconds, got, test.want)
		}
	}
}

func TestSpeechMixerBackpressuresWithoutDiscardingSpeech(t *testing.T) {
	t.Parallel()
	mixer := NewSpeechMixer()
	segment := make([]int16, maxSpeechSegmentSamples)
	for index := range segment {
		segment[index] = 5
	}
	for index := 0; index < maxSpeechBacklogSamples/maxSpeechSegmentSamples; index++ {
		if !mixer.Enqueue("queued", segment) {
			t.Fatal("could not fill speech backlog")
		}
	}
	enqueued := make(chan bool, 1)
	go func() {
		enqueued <- mixer.Enqueue("waiting", []int16{9, 9})
	}()
	select {
	case <-enqueued:
		t.Fatal("enqueue did not apply backpressure")
	case <-time.After(20 * time.Millisecond):
	}
	for mixer.QueuedFrames() == maxSpeechBacklogSamples/speechChannels {
		mixer.TakeStereo(960)
	}
	select {
	case accepted := <-enqueued:
		if !accepted {
			t.Fatal("waiting speech was rejected")
		}
	case <-time.After(time.Second):
		t.Fatal("waiting speech did not resume after queue space became available")
	}
}

func TestSpeechMixerBackpressureStopsWithContext(t *testing.T) {
	t.Parallel()
	mixer := NewSpeechMixer()
	segment := make([]int16, maxSpeechSegmentSamples)
	for index := 0; index < maxSpeechBacklogSamples/maxSpeechSegmentSamples; index++ {
		mixer.Enqueue("queued", segment)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if mixer.EnqueueContext(ctx, "cancelled", []int16{1, 1}) {
		t.Fatal("cancelled enqueue unexpectedly succeeded")
	}
}

func TestSpeechMixerBackpressuresAtTheSegmentLimit(t *testing.T) {
	t.Parallel()
	mixer := NewSpeechMixer()
	for index := 0; index < maxSpeechBacklogSegments; index++ {
		if !mixer.Enqueue("tiny", []int16{1, 1}) {
			t.Fatal("could not fill speech segment backlog")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if mixer.EnqueueContext(ctx, "overflow", []int16{2, 2}) {
		t.Fatal("segment count limit did not apply backpressure")
	}
}

func TestSpeechMixerPreservesQueuedSpeechBeyondEightSeconds(t *testing.T) {
	t.Parallel()
	mixer := NewSpeechMixer()
	first := make([]int16, maxSpeechSegmentSamples)
	for index := range first {
		first[index] = 3
	}
	if !mixer.Enqueue("first", first) || !mixer.Enqueue("second", []int16{7, 7}) {
		t.Fatal("speech was not enqueued")
	}
	if got := mixer.TakeStereo(1); len(got) != 2 || got[0] != 3 || got[1] != 3 {
		t.Fatalf("oldest queued speech was discarded: %v", got)
	}
}

func TestSpeechMixerSpeedsUpBacklogWithoutChangingPitch(t *testing.T) {
	t.Parallel()
	mixer := NewSpeechMixer()
	const frequency = 200.0
	const durationFrames = 48000 * 2
	segment := make([]int16, durationFrames*2)
	for frame := 0; frame < durationFrames; frame++ {
		value := int16(math.Sin(2*math.Pi*frequency*float64(frame)/48000) * 12000)
		segment[frame*2], segment[frame*2+1] = value, value
	}
	mixer.Enqueue("caption-1", segment)
	mixer.Enqueue("caption-2", segment)

	var output []int16
	for mixer.QueuedFrames() > 0 {
		output = append(output, mixer.TakeStereo(960)...)
	}
	for len(output) >= 2 && output[len(output)-1] == 0 && output[len(output)-2] == 0 {
		output = output[:len(output)-2]
	}
	outputFrames := len(output) / 2
	if outputFrames >= durationFrames*2 {
		t.Fatalf("backlogged speech was not accelerated: output=%d input=%d", outputFrames, durationFrames*2)
	}
	if outputFrames < int(math.Floor(float64(durationFrames*2)/1.31)) {
		t.Fatalf("speech accelerated beyond the 1.3x cap: output=%d", outputFrames)
	}
	zeroCrossings := 0
	for frame := 1; frame < outputFrames; frame++ {
		if output[(frame-1)*2] <= 0 && output[frame*2] > 0 {
			zeroCrossings++
		}
	}
	outputSeconds := float64(outputFrames) / 48000
	measuredPitch := float64(zeroCrossings) / outputSeconds
	if math.Abs(measuredPitch-frequency) > 12 {
		t.Fatalf("pitch changed while accelerating: got %.1f Hz, want %.1f Hz", measuredPitch, frequency)
	}
}

func TestSpeechMixerLeavesAnUnbackloggedUtteranceAtNormalSpeed(t *testing.T) {
	t.Parallel()
	mixer := NewSpeechMixer()
	segment := make([]int16, speechSampleRate*speechChannels*2)
	for index := range segment {
		segment[index] = 11
	}
	mixer.Enqueue("only", segment)
	var outputFrames int
	for mixer.QueuedFrames() > 0 {
		outputFrames += len(mixer.TakeStereo(960)) / speechChannels
	}
	if outputFrames != len(segment)/speechChannels {
		t.Fatalf("an unbacklogged utterance was accelerated: output=%d input=%d", outputFrames, len(segment)/speechChannels)
	}
}

func TestTimeStretchPreservesTheEndOfAnUtterance(t *testing.T) {
	t.Parallel()
	const inputFrames = speechSampleRate * 4
	input := make([]int16, inputFrames*speechChannels)
	for frame := 0; frame < inputFrames; frame++ {
		frequency := 180.0
		if frame >= inputFrames/2 {
			frequency = 620
		}
		value := int16(math.Sin(2*math.Pi*frequency*float64(frame)/speechSampleRate) * 12000)
		input[frame*speechChannels], input[frame*speechChannels+1] = value, value
	}
	output := timeStretchStereo(input, 1.3)
	outputFrames := len(output) / speechChannels
	windowFrames := speechSampleRate / 2
	zeroCrossings := 0
	for frame := outputFrames - windowFrames + 1; frame < outputFrames; frame++ {
		if output[(frame-1)*speechChannels] <= 0 && output[frame*speechChannels] > 0 {
			zeroCrossings++
		}
	}
	measuredFrequency := float64(zeroCrossings) / (float64(windowFrames) / speechSampleRate)
	if math.Abs(measuredFrequency-620) > 35 {
		t.Fatalf("utterance tail was not preserved: got %.1f Hz, want 620 Hz", measuredFrequency)
	}
}

func TestTimeStretchCrossfadesTheFinalTenMilliseconds(t *testing.T) {
	t.Parallel()
	const inputFrames = speechSampleRate * 4
	input := make([]int16, inputFrames*speechChannels)
	for frame := inputFrames - speechSampleRate/100; frame < inputFrames; frame++ {
		input[frame*speechChannels], input[frame*speechChannels+1] = 12000, 12000
	}
	output := timeStretchStereo(input, 1.3)
	outputFrames := len(output) / speechChannels
	var total int64
	const measuredFrames = 120
	for frame := outputFrames - measuredFrames; frame < outputFrames; frame++ {
		total += int64(output[frame*speechChannels])
	}
	average := total / measuredFrames
	if average < 10000 {
		t.Fatalf("final consonant window was lost: average tail sample=%d", average)
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
