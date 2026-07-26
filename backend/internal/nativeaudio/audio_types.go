package nativeaudio

import "errors"

var ErrUnavailable = errors.New("native audio codecs are unavailable")

// PCMFrame contains interleaved signed 16-bit PCM samples.
type PCMFrame struct {
	Data       []int16
	SampleRate int
	Channels   int
}

// OpusFrame is a WebRTC-compatible Opus packet.
type OpusFrame struct {
	Data     []byte
	Duration int
}
