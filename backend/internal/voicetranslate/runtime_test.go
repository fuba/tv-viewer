package voicetranslate

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"testing"
	"time"
)

type runnerFunc func(context.Context, <-chan []byte, chan<- Event) error

func (fn runnerFunc) Run(ctx context.Context, audio <-chan []byte, events chan<- Event) error {
	return fn(ctx, audio, events)
}

func TestRuntimePacesAudioAndReplacesBroadcastWithSpeech(t *testing.T) {
	t.Parallel()

	receivedAudio := make(chan []byte, 1)
	runner := runnerFunc(func(ctx context.Context, audio <-chan []byte, events chan<- Event) error {
		events <- Event{Type: EventReady, SampleRate: 16000, Channels: 1, Format: "pcm_s16le"}
		select {
		case frame := <-audio:
			receivedAudio <- frame
		case <-ctx.Done():
			return ctx.Err()
		}
		wav := pcm16WAVForTest(24000, 1, []int16{1000, 2000, 3000, 4000})
		events <- Event{
			Type: EventSpeech, CaptionID: "caption-1", MIMEType: "audio/wav",
			AudioBase64: base64.StdEncoding.EncodeToString(wav),
		}
		<-ctx.Done()
		return ctx.Err()
	})

	var mu sync.Mutex
	seen := make([]Event, 0, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := newRuntime(ctx, runner, func(event Event) {
		mu.Lock()
		seen = append(seen, event)
		mu.Unlock()
	}, time.Millisecond)
	defer runtime.Close()

	runtime.Push48kStereo(make([]int16, 4800*2))
	select {
	case frame := <-receivedAudio:
		if len(frame) != 3200 {
			t.Fatalf("paced frame length = %d, want 3200", len(frame))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for paced PCM")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		output := runtime.ReplaceStereo(make([]int16, 16))
		if output[0] != 0 {
			if output[0] != 1000 || output[1] != 1000 {
				t.Fatalf("replacement begins with %v", output[:4])
			}
			mu.Lock()
			defer mu.Unlock()
			if len(seen) < 2 || seen[0].Type != EventReady || seen[1].Type != EventSpeech || seen[1].AudioBase64 != "" {
				t.Fatalf("events = %+v", seen)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for synthesized speech")
}

func TestRuntimeFallsBackToBroadcastAudioWhenConnectionFails(t *testing.T) {
	t.Parallel()
	runner := runnerFunc(func(context.Context, <-chan []byte, chan<- Event) error {
		return context.DeadlineExceeded
	})
	events := make(chan Event, 1)
	runtime := newRuntime(context.Background(), runner, func(event Event) { events <- event }, time.Millisecond)
	defer runtime.Close()

	original := []int16{10, 20, 30, 40}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		select {
		case event := <-events:
			if event.Type != EventError || event.Stage != "connection" || !strings.Contains(event.Message, "deadline") {
				t.Fatalf("unexpected error event: %+v", event)
			}
			if got := runtime.ReplaceStereo(original); string(int16Bytes(got)) != string(int16Bytes(original)) {
				t.Fatalf("fallback audio = %v, want %v", got, original)
			}
			return
		default:
			time.Sleep(time.Millisecond)
		}
	}
	t.Fatal("timed out waiting for connection failure")
}

func TestRuntimeDoesNotDiscardReceivedSpeechOnLateCancellation(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{mixer: NewSpeechMixer()}
	wav := pcm16WAVForTest(48000, 1, []int16{1000, 2000, 3000})
	runtime.handleEvent(Event{
		Type: EventSpeech, CaptionID: "caption-1", MIMEType: "audio/wav",
		AudioBase64: base64.StdEncoding.EncodeToString(wav),
	})
	runtime.handleEvent(Event{Type: EventSpeechCancelled, CaptionID: "caption-1"})
	if got := runtime.mixer.TakeStereo(2); got[0] != 1000 || got[1] != 1000 {
		t.Fatalf("late cancellation discarded received speech: %v", got)
	}
}

func TestRuntimeFailsTranslationInsteadOfDroppingInputFrames(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{frames: make(chan []byte, 1), mixer: NewSpeechMixer()}
	runtime.enqueueFrame(make([]byte, translationFrameBytes))
	runtime.enqueueFrame(make([]byte, translationFrameBytes))
	if !runtime.failed.Load() {
		t.Fatal("translation input overflow did not fail closed")
	}
}

func int16Bytes(values []int16) []byte {
	result := make([]byte, len(values)*2)
	for index, value := range values {
		result[index*2] = byte(value)
		result[index*2+1] = byte(uint16(value) >> 8)
	}
	return result
}
