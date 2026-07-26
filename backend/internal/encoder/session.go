package encoder

import (
	"context"
	"io"
	"log"
	"sync"
)

// AudioMode selects the main, sub, or stereo broadcast audio channels.
type AudioMode string

const (
	AudioModeMain AudioMode = "main"
	AudioModeSub  AudioMode = "sub"
	AudioModeBoth AudioMode = "both"
)

// WebRTCSession owns one direct Go media pipeline.
type WebRTCSession struct {
	ID           string
	ChannelID    string
	ctx          context.Context
	cancel       context.CancelFunc
	stream       io.ReadCloser
	VideoPipe    io.ReadCloser
	VideoRaw     bool
	AudioPipe    io.ReadCloser
	AudioRaw     bool
	SubtitlePipe io.ReadCloser
	SubtitleRaw  bool
	AudioMode    AudioMode
	StreamURL    string

	mu      sync.Mutex
	stopped bool
}

func (s *WebRTCSession) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	stream := s.stream
	video := s.VideoPipe
	audio := s.AudioPipe
	subtitle := s.SubtitlePipe
	s.stream = nil
	s.VideoPipe = nil
	s.AudioPipe = nil
	s.SubtitlePipe = nil
	s.mu.Unlock()

	log.Printf("[Native] Stopping session %s", s.ID)
	if s.cancel != nil {
		s.cancel()
	}
	if stream != nil {
		_ = stream.Close()
	}
	if video != nil {
		_ = video.Close()
	}
	if audio != nil {
		_ = audio.Close()
	}
	if subtitle != nil {
		_ = subtitle.Close()
	}
}

func (s *WebRTCSession) Context() context.Context { return s.ctx }

func (s *WebRTCSession) AudioReader() io.ReadCloser {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.AudioPipe
}

func (s *WebRTCSession) VideoReader() io.ReadCloser {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.VideoPipe
}

func (s *WebRTCSession) SubtitleReader() io.ReadCloser {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.SubtitlePipe
}

func (s *WebRTCSession) IsRunning() bool {
	select {
	case <-s.ctx.Done():
		return false
	default:
		return true
	}
}
