package voicetranslate

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	translationFrameBytes  = 3200
	translationQueueFrames = 100
	defaultPaceInterval    = 100 * time.Millisecond
)

type clientRunner interface {
	Run(context.Context, <-chan []byte, chan<- Event) error
}

// Runtime bridges live broadcast PCM to VoiceTranslate and supplies replacement speech.
type Runtime struct {
	ctx      context.Context
	cancel   context.CancelFunc
	onEvent  func(Event)
	ready    atomic.Bool
	failed   atomic.Bool
	frames   chan []byte
	pcm      chan []byte
	mixer    *SpeechMixer
	convert  *PCMConverter
	convertM sync.Mutex
	pending  []byte
}

func StartRuntime(ctx context.Context, config Config, onEvent func(Event)) *Runtime {
	return newRuntime(ctx, NewClient(config), onEvent, defaultPaceInterval)
}

func newRuntime(ctx context.Context, runner clientRunner, onEvent func(Event), paceInterval time.Duration) *Runtime {
	runtimeCtx, cancel := context.WithCancel(ctx)
	runtime := &Runtime{
		ctx: runtimeCtx, cancel: cancel, onEvent: onEvent,
		frames: make(chan []byte, translationQueueFrames), pcm: make(chan []byte, 1),
		mixer: NewSpeechMixer(), convert: NewPCMConverter(),
	}
	// Keep large speech events close to the socket so a fast gateway receives backpressure.
	events := make(chan Event, 2)
	result := make(chan error, 1)
	go func() { result <- runner.Run(runtimeCtx, runtime.pcm, events) }()
	go runtime.pace(paceInterval)
	go runtime.consumeEvents(events, result)
	return runtime
}

func (r *Runtime) pace(interval time.Duration) {
	if interval <= 0 {
		interval = defaultPaceInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			select {
			case frame := <-r.frames:
				select {
				case r.pcm <- frame:
				case <-r.ctx.Done():
					return
				}
			default:
			}
		}
	}
}

func (r *Runtime) consumeEvents(events <-chan Event, result <-chan error) {
	for {
		select {
		case <-r.ctx.Done():
			return
		case err := <-result:
			if r.ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return
			}
			r.failed.Store(true)
			message := "VoiceTranslate connection stopped"
			if err != nil {
				message = err.Error()
			}
			r.emit(Event{Type: EventError, Stage: "connection", Message: message})
			r.cancel()
			return
		case event := <-events:
			r.handleEvent(event)
		}
	}
}

func (r *Runtime) handleEvent(event Event) {
	switch event.Type {
	case EventReady:
		r.ready.Store(true)
		r.emit(event)
	case EventSpeech:
		if err := r.enqueueSpeech(event); err != nil {
			r.emit(Event{Type: EventError, Stage: "speech", Message: err.Error(), CaptionID: event.CaptionID})
			return
		}
		// The WAV can be several MiB; it is consumed on the server and never forwarded.
		event.AudioBase64 = ""
		r.emit(event)
	case EventSpeechCancelled:
		r.mixer.Cancel(event.CaptionID)
		r.emit(event)
	default:
		r.emit(event)
	}
}

func (r *Runtime) enqueueSpeech(event Event) error {
	if event.CaptionID == "" {
		return errors.New("VoiceTranslate speech event has no caption ID")
	}
	if event.MIMEType != "audio/wav" {
		return fmt.Errorf("unexpected VoiceTranslate speech MIME type %q", event.MIMEType)
	}
	if len(event.AudioBase64) > maxSpeechBase64Bytes {
		return errors.New("VoiceTranslate speech payload is too large")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(event.AudioBase64)
	if err != nil {
		return fmt.Errorf("decode VoiceTranslate speech: %w", err)
	}
	wav, err := DecodeWAV(data)
	if err != nil {
		return fmt.Errorf("decode VoiceTranslate WAV: %w", err)
	}
	pcm, err := ResampleTo48kStereo(wav)
	if err != nil {
		return fmt.Errorf("resample VoiceTranslate speech: %w", err)
	}
	r.mixer.Enqueue(event.CaptionID, pcm)
	return nil
}

func (r *Runtime) emit(event Event) {
	if r.onEvent != nil {
		r.onEvent(event)
	}
}

// Push48kStereo accepts live selected broadcast audio without blocking the TV pipeline.
func (r *Runtime) Push48kStereo(samples []int16) {
	if r == nil || len(samples) == 0 || r.failed.Load() {
		return
	}
	r.convertM.Lock()
	r.pending = append(r.pending, r.convert.Push48kStereo(samples)...)
	for len(r.pending) >= translationFrameBytes {
		frame := append([]byte(nil), r.pending[:translationFrameBytes]...)
		r.pending = r.pending[translationFrameBytes:]
		r.enqueueFrame(frame)
	}
	r.convertM.Unlock()
}

func (r *Runtime) enqueueFrame(frame []byte) {
	select {
	case r.frames <- frame:
		return
	default:
	}
	// Keep live audio current if the gateway stalls instead of accumulating lag.
	select {
	case <-r.frames:
	default:
	}
	select {
	case r.frames <- frame:
	default:
	}
}

// ReplaceStereo returns original audio until the translation session is ready,
// then returns queued speech or silence on the same 48 kHz stereo timeline.
func (r *Runtime) ReplaceStereo(original []int16) []int16 {
	if r == nil || !r.ready.Load() || r.failed.Load() {
		return original
	}
	return r.mixer.TakeStereo(len(original) / 2)
}

func (r *Runtime) Close() {
	if r != nil {
		r.cancel()
	}
}
