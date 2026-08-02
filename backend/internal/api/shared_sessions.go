package api

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"time"

	"github.com/fuba/tv-viewer/internal/encoder"
	"github.com/fuba/tv-viewer/internal/webrtc"
)

const idleSessionGrace = 5 * time.Second

type sharedSession struct {
	channelID          string
	session            *encoder.WebRTCSession
	stream             *webrtc.SharedStream
	burnInSubtitles    bool
	audioMode          encoder.AudioMode
	translationEnabled bool

	mu       sync.Mutex
	peers    map[string]struct{}
	idleStop *time.Timer
	stopped  bool

	// onUnexpectedStop closes signaling peers so clients can reconnect instead
	// of remaining connected to a dead media pipeline.
	onUnexpectedStop func([]string, error)
}

func (s *sharedSession) matchesSettings(burnInSubtitles bool, audioMode encoder.AudioMode, translationEnabled bool) bool {
	return s != nil && s.burnInSubtitles == burnInSubtitles && s.audioMode == audioMode &&
		s.translationEnabled == translationEnabled
}

func newSharedSession(channelID string, session *encoder.WebRTCSession, burnInSubtitles bool, audioMode encoder.AudioMode, translationEnabled bool) *sharedSession {
	s := &sharedSession{
		channelID:          channelID,
		session:            session,
		stream:             webrtc.NewSharedStream(),
		burnInSubtitles:    burnInSubtitles,
		audioMode:          audioMode,
		translationEnabled: translationEnabled,
		peers:              make(map[string]struct{}),
		onUnexpectedStop: func(peerIDs []string, _ error) {
			for _, peerID := range peerIDs {
				peerManager.RemovePeer(peerID)
			}
		},
	}
	s.stream.SetOnPeerRemoved(s.peerRemoved)
	return s
}

func (s *sharedSession) runReaders() {
	go func() {
		reader, waitErr := waitForPipe(s.session.Context(), s.session.VideoReader)
		if waitErr != nil {
			return
		}
		var err error
		if s.session.VideoRaw {
			err = s.stream.RunVideoRaw(reader)
		} else {
			err = s.stream.RunVideo(reader)
		}
		if err == nil {
			err = io.EOF
		}
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.ErrClosedPipe) && s.session.Context().Err() == nil {
			s.stopUnexpected(err)
		}
	}()

	go func() {
		if reader, err := waitForPipe(s.session.Context(), s.session.AudioReader); err == nil {
			var err error
			if s.session.AudioRaw {
				err = s.stream.RunAudioRaw(reader)
			} else {
				err = s.stream.RunAudio(reader)
			}
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.ErrClosedPipe) {
				log.Printf("[WebRTC] Shared audio stream ended for channel %s: %v", s.channelID, err)
			}
		}
	}()

	go func() {
		if reader, err := waitForPipe(s.session.Context(), s.session.SubtitleReader); err == nil {
			var err error
			if s.session.SubtitleRaw {
				err = s.stream.RunSubtitlesRaw(reader)
			} else {
				err = s.stream.RunSubtitles(reader)
			}
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.ErrClosedPipe) {
				log.Printf("[WebRTC] Shared subtitle stream ended for channel %s: %v", s.channelID, err)
			}
		}
	}()
}

func waitForPipe(ctx context.Context, getter func() io.ReadCloser) (io.ReadCloser, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if reader := getter(); reader != nil {
			return reader, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *sharedSession) addPeer(peer *webrtc.Peer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return context.Canceled
	}

	if err := s.stream.AddPeer(peer); err != nil {
		return err
	}
	// Keep the idle timer armed until attachment succeeds. A failed bootstrap
	// must not turn an empty session into a permanent tuner reservation.
	if s.idleStop != nil {
		s.idleStop.Stop()
		s.idleStop = nil
	}
	s.peers[peer.ID] = struct{}{}
	return nil
}

func (s *sharedSession) removePeer(peerID string) {
	s.stream.RemovePeer(peerID)
}

func (s *sharedSession) peerRemoved(peerID string) {
	s.mu.Lock()
	delete(s.peers, peerID)
	if len(s.peers) == 0 && !s.stopped && s.idleStop == nil {
		s.idleStop = time.AfterFunc(idleSessionGrace, s.stopIfIdle)
	}
	s.mu.Unlock()
}

// stopIfIdle releases the encoder immediately when a subscriber switches away.
// Ordinary disconnects retain the idle grace period so quick reconnects can reuse
// the existing tuner session, but a channel switch must free the old session
// before the new channel is admitted.
func (s *sharedSession) stopIfIdle() {
	s.stopWithCause(nil, true)
}

func (s *sharedSession) stop() {
	s.stopWithCause(nil, false)
}

func (s *sharedSession) stopUnexpected(err error) {
	s.stopWithCause(err, false)
}

func (s *sharedSession) stopWithCause(cause error, requireIdle bool) {
	s.mu.Lock()
	if s.stopped || (requireIdle && len(s.peers) != 0) {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	peerIDs := make([]string, 0, len(s.peers))
	if cause != nil {
		for peerID := range s.peers {
			peerIDs = append(peerIDs, peerID)
		}
	}
	onUnexpectedStop := s.onUnexpectedStop
	if s.idleStop != nil {
		s.idleStop.Stop()
		s.idleStop = nil
	}
	s.mu.Unlock()

	s.stream.Stop()
	s.session.Stop()
	webrtcMu.Lock()
	if webrtcSessions[s.channelID] == s.session {
		delete(webrtcSessions, s.channelID)
	}
	webrtcMu.Unlock()
	sharedSessions.delete(s.channelID, s)
	if cause != nil {
		log.Printf("[WebRTC] Shared video stream ended unexpectedly for channel %s: %v", s.channelID, cause)
		if onUnexpectedStop != nil {
			onUnexpectedStop(peerIDs, cause)
		}
	}
	log.Printf("[WebRTC] Shared session stopped for channel %s", s.channelID)
}

type sharedSessionRegistry struct {
	mu       sync.Mutex
	startMu  sync.Mutex
	sessions map[string]*sharedSession
}

var sharedSessions = &sharedSessionRegistry{sessions: make(map[string]*sharedSession)}

func (r *sharedSessionRegistry) get(channelID string) *sharedSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[channelID]
}

func (r *sharedSessionRegistry) register(channelID string, session *encoder.WebRTCSession, burnInSubtitles bool, audioMode encoder.AudioMode, translationEnabled bool) *sharedSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing := r.sessions[channelID]; existing != nil {
		return existing
	}
	shared := newSharedSession(channelID, session, burnInSubtitles, audioMode, translationEnabled)
	r.sessions[channelID] = shared
	go shared.runReaders()
	return shared
}

func (r *sharedSessionRegistry) delete(channelID string, target *sharedSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions[channelID] == target {
		delete(r.sessions, channelID)
	}
}

func (r *sharedSessionRegistry) stopAll() {
	r.mu.Lock()
	sessions := make([]*sharedSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.mu.Unlock()
	for _, session := range sessions {
		session.stop()
	}
}

func (r *sharedSessionRegistry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sessions)
}

func (r *sharedSessionRegistry) activeChannel() (string, int) {
	r.mu.Lock()
	sessions := make([]*sharedSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.mu.Unlock()
	for _, session := range sessions {
		if viewers := session.peerCount(); viewers > 0 {
			return session.channelID, viewers
		}
	}
	return "", 0
}

func (s *sharedSession) peerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.peers)
}
