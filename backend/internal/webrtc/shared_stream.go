package webrtc

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/fuba/tv-viewer/internal/streamframe"
)

const (
	sharedVideoBuffer    = 120
	sharedAudioBuffer    = 120
	sharedSubtitleBuffer = 32
	videoPrebuffer       = 900 * time.Millisecond
)

type sharedSample struct {
	data      []byte
	duration  time.Duration
	pts       uint64
	hasPTS    bool
	bootstrap bool
}

type sharedSubscriber struct {
	peer          *Peer
	video         chan sharedSample
	audio         chan sharedSample
	subtitles     chan []byte
	subtitleJSON  chan []byte
	subtitleRead  *io.PipeReader
	subtitleWrite *io.PipeWriter
	clock         *sharedMediaClock
	done          chan struct{}
	once          sync.Once
}

// SharedStream reads each encoded output once and fans samples out to all peers.
// A slow subscriber is removed instead of blocking the live stream.
type SharedStream struct {
	mu            sync.RWMutex
	subscribers   map[string]*sharedSubscriber
	ctx           context.Context
	cancel        context.CancelFunc
	stopped       bool
	onPeerRemoved func(string)

	bootstrapMu sync.Mutex
	bootstrap   []sharedSample
	publishMu   sync.Mutex
}

func (s *SharedStream) SetOnPeerRemoved(callback func(string)) {
	s.mu.Lock()
	s.onPeerRemoved = callback
	s.mu.Unlock()
}

func NewSharedStream() *SharedStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &SharedStream{
		subscribers: make(map[string]*sharedSubscriber),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// AddPeer subscribes a peer and sends the latest H.264 parameter sets/keyframe first.
func (s *SharedStream) AddPeer(peer *Peer) error {
	s.publishMu.Lock()
	defer s.publishMu.Unlock()

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return fmt.Errorf("shared stream is stopped")
	}
	if _, exists := s.subscribers[peer.ID]; exists {
		s.mu.Unlock()
		return fmt.Errorf("peer %s is already subscribed", peer.ID)
	}

	subtitleRead, subtitleWrite := io.Pipe()
	sub := &sharedSubscriber{
		peer:          peer,
		video:         make(chan sharedSample, sharedVideoBuffer),
		audio:         make(chan sharedSample, sharedAudioBuffer),
		subtitles:     make(chan []byte, sharedSubtitleBuffer),
		subtitleJSON:  make(chan []byte, sharedSubtitleBuffer),
		subtitleRead:  subtitleRead,
		subtitleWrite: subtitleWrite,
		clock:         newSharedMediaClock(),
		done:          make(chan struct{}),
	}
	s.bootstrapMu.Lock()
	bootstrap := append([]sharedSample(nil), s.bootstrap...)
	s.bootstrapMu.Unlock()
	for _, sample := range bootstrap {
		sample.bootstrap = true
		if !s.enqueueVideo(sub, sample) {
			s.mu.Unlock()
			return fmt.Errorf("peer %s cannot accept bootstrap video", peer.ID)
		}
	}
	s.subscribers[peer.ID] = sub
	s.mu.Unlock()

	go s.writeVideo(sub)
	go s.writeAudio(sub)
	go s.writeSubtitles(sub)
	go s.writeSubtitleJSON(sub)
	go func() {
		select {
		case <-peer.Context().Done():
			s.RemovePeer(peer.ID)
		case <-sub.done:
		case <-s.ctx.Done():
		}
	}()
	go func() {
		if err := peer.StreamSubtitles(subtitleRead); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.ErrClosedPipe) {
			log.Printf("[WebRTC] Subtitle fanout failed for peer %s: %v", peer.ID, err)
		}
	}()

	log.Printf("[WebRTC] Peer %s subscribed to shared stream", peer.ID)
	return nil
}

func (s *SharedStream) enqueueVideo(sub *sharedSubscriber, sample sharedSample) bool {
	select {
	case <-sub.done:
		return false
	case sub.video <- sample:
		return true
	default:
		return false
	}
}

func (s *SharedStream) enqueueAudio(sub *sharedSubscriber, sample sharedSample) bool {
	select {
	case <-sub.done:
		return false
	case sub.audio <- sample:
		return true
	default:
		return false
	}
}

func (s *SharedStream) enqueueSubtitle(sub *sharedSubscriber, data []byte) bool {
	select {
	case <-sub.done:
		return false
	case sub.subtitles <- data:
		return true
	default:
		return false
	}
}

func (s *SharedStream) enqueueSubtitleJSON(sub *sharedSubscriber, data []byte) bool {
	select {
	case <-sub.done:
		return false
	case sub.subtitleJSON <- data:
		return true
	default:
		return false
	}
}

func (s *SharedStream) writeVideo(sub *sharedSubscriber) {
	pacer := newMediaPacer()
	prebuffered := false
	var clockRevision uint64
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	go func() {
		select {
		case <-sub.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	for {
		select {
		case <-sub.done:
			return
		case sample := <-sub.video:
			if !sample.bootstrap {
				wallBase, ptsBase, revision := sub.clock.StartVideo(sample.pts)
				if !prebuffered {
					if err := waitUntil(ctx, wallBase); err != nil {
						return
					}
					prebuffered = true
				}
				if sample.hasPTS && revision != clockRevision {
					pacer.ResetEpoch(wallBase, ptsBase)
					clockRevision = revision
				}
				resynced, err := pacer.Pace(ctx, sample.pts, sample.hasPTS, sample.duration)
				if err != nil {
					return
				}
				if resynced {
					if sub.clock.Rebase(clockRevision, pacer.wallBase, pacer.ptsBase) {
						clockRevision++
					}
					log.Printf("[WebRTC] Video pacer resynchronized peer=%s lag-limit=%s", sub.peer.ID, maxPacingLag)
				}
			}
			if err := sub.peer.WriteVideoSample(sample.data, sample.duration); err != nil {
				log.Printf("[WebRTC] Video fanout failed for peer %s: %v", sub.peer.ID, err)
				s.RemovePeer(sub.peer.ID)
				return
			}
		}
	}
}

func (s *SharedStream) writeAudio(sub *sharedSubscriber) {
	pacer := newMediaPacer()
	prebuffered := false
	var clockRevision uint64
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	go func() {
		select {
		case <-sub.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	for {
		select {
		case <-sub.done:
			return
		case sample := <-sub.audio:
			wallBase, ptsBase, revision, err := sub.clock.WaitEpoch(ctx)
			if err != nil {
				return
			}
			if sample.hasPTS && (!prebuffered || revision != clockRevision) {
				pacer.ResetEpoch(wallBase, ptsBase)
				clockRevision = revision
				prebuffered = true
			}
			resynced, err := pacer.Pace(ctx, sample.pts, sample.hasPTS, sample.duration)
			if err != nil {
				return
			}
			if resynced {
				if sub.clock.Rebase(clockRevision, pacer.wallBase, pacer.ptsBase) {
					clockRevision++
				}
				log.Printf("[WebRTC] Audio pacer resynchronized peer=%s lag-limit=%s", sub.peer.ID, maxPacingLag)
			}
			if err := sub.peer.WriteAudioSample(sample.data, sample.duration); err != nil {
				log.Printf("[WebRTC] Audio fanout failed for peer %s: %v", sub.peer.ID, err)
				s.RemovePeer(sub.peer.ID)
				return
			}
		}
	}
}

func (s *SharedStream) writeSubtitles(sub *sharedSubscriber) {
	for {
		select {
		case <-sub.done:
			return
		case data := <-sub.subtitles:
			if _, err := sub.subtitleWrite.Write(data); err != nil {
				s.RemovePeer(sub.peer.ID)
				return
			}
		}
	}
}

func (s *SharedStream) writeSubtitleJSON(sub *sharedSubscriber) {
	for {
		select {
		case <-sub.done:
			return
		case data := <-sub.subtitleJSON:
			if err := sub.peer.SendSubtitle(data); err != nil {
				s.RemovePeer(sub.peer.ID)
				return
			}
		}
	}
}

// RemovePeer unsubscribes a peer and closes its per-peer delivery queues.
func (s *SharedStream) RemovePeer(peerID string) {
	s.mu.Lock()
	sub, exists := s.subscribers[peerID]
	if exists {
		delete(s.subscribers, peerID)
	}
	s.mu.Unlock()
	if !exists {
		return
	}

	sub.once.Do(func() {
		close(sub.done)
		if err := sub.subtitleRead.Close(); err != nil {
			log.Printf("Failed to close subtitle reader for peer %s: %v", peerID, err)
		}
		if err := sub.subtitleWrite.Close(); err != nil {
			log.Printf("Failed to close subtitle writer for peer %s: %v", peerID, err)
		}
	})
	s.mu.RLock()
	callback := s.onPeerRemoved
	s.mu.RUnlock()
	if callback != nil {
		callback(peerID)
	}
	log.Printf("[WebRTC] Peer %s unsubscribed from shared stream", peerID)
}

func (s *SharedStream) subscriberSnapshot() []*sharedSubscriber {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*sharedSubscriber, 0, len(s.subscribers))
	for _, sub := range s.subscribers {
		result = append(result, sub)
	}
	return result
}

func (s *SharedStream) rememberBootstrap(nal NALUnit, sample sharedSample) {
	if nal.Type != NALTypeSPS && nal.Type != NALTypePPS && nal.Type != NALTypeIDR {
		return
	}
	s.bootstrapMu.Lock()
	defer s.bootstrapMu.Unlock()
	if nal.Type == NALTypeSPS || nal.Type == NALTypePPS {
		for i, existing := range s.bootstrap {
			if len(existing.data) > 4 && existing.data[4]&0x1f == nal.Type {
				s.bootstrap[i] = sample
				return
			}
		}
		s.bootstrap = append(s.bootstrap, sample)
		return
	}
	// Keep only the parameter sets and the most recent IDR frame.
	parameterSets := make([]sharedSample, 0, 2)
	for _, existing := range s.bootstrap {
		if len(existing.data) > 4 {
			typ := existing.data[4] & 0x1f
			if typ == NALTypeSPS || typ == NALTypePPS {
				parameterSets = append(parameterSets, existing)
			}
		}
	}
	s.bootstrap = append(parameterSets, sample)
}

// RunVideo reads Annex-B H.264 once and fans NAL samples out to subscribers.
func (s *SharedStream) RunVideo(reader io.Reader) error {
	h264Reader := NewH264Reader(reader)
	frameDuration := time.Second / 30
	accessUnit := make([]NALUnit, 0, 8)
	hasAccessUnitDelimiter := false
	flushAccessUnit := func() {
		if len(accessUnit) == 0 {
			return
		}
		data := make([]byte, 0, len(accessUnit)*1024)
		for _, nal := range accessUnit {
			if nal.Type == NALTypeSEI {
				continue
			}
			data = append(data, 0, 0, 0, 1)
			data = append(data, nal.Data...)
		}
		if len(data) == 0 {
			accessUnit = accessUnit[:0]
			return
		}
		s.publishMu.Lock()
		defer s.publishMu.Unlock()
		sample := sharedSample{data: data, duration: frameDuration}
		for _, nal := range accessUnit {
			switch nal.Type {
			case NALTypeSPS, NALTypePPS:
				bootstrapData := append([]byte{0, 0, 0, 1}, nal.Data...)
				s.rememberBootstrap(nal, sharedSample{data: bootstrapData, duration: frameDuration})
			case NALTypeIDR:
				s.rememberBootstrap(nal, sample)
			}
		}
		for _, sub := range s.subscriberSnapshot() {
			if !s.enqueueVideo(sub, sample) {
				s.RemovePeer(sub.peer.ID)
			}
		}
		accessUnit = accessUnit[:0]
	}
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}
		nals, err := h264Reader.ReadNALUnits()
		if err != nil {
			return err
		}
		for _, nal := range nals {
			if nal.Type == NALTypeAUD {
				hasAccessUnitDelimiter = true
				flushAccessUnit()
			}
			accessUnit = append(accessUnit, nal)
		}
		// Some H.264 sources emit AUDs. If a source omits them, flush at read
		// boundaries so malformed input cannot hold the entire stream in memory.
		if !hasAccessUnitDelimiter && len(accessUnit) > 0 {
			flushAccessUnit()
		}
	}
}

// RunVideoRaw reads length-prefixed H.264 access units from the native pipeline.
func (s *SharedStream) RunVideoRaw(reader io.Reader) error {
	for {
		frame, err := streamframe.ReadVideo(reader)
		if err != nil {
			return err
		}
		s.publishVideoAccessUnit(frame.Data, frame.Duration, frame.PTS, frame.HasPTS)
	}
}

func (s *SharedStream) publishVideoAccessUnit(data []byte, duration time.Duration, pts uint64, hasPTS bool) {
	parser := NewH264Parser()
	// A trailing start code makes the final NAL boundary explicit.
	nals, err := parser.Parse(append(append([]byte(nil), data...), 0, 0, 1))
	if err != nil {
		return
	}
	s.publishMu.Lock()
	defer s.publishMu.Unlock()
	sample := sharedSample{data: append([]byte(nil), data...), duration: duration, pts: pts, hasPTS: hasPTS}
	for _, nal := range nals {
		if nal.Type == NALTypeSPS || nal.Type == NALTypePPS {
			s.rememberBootstrap(nal, sharedSample{data: append([]byte{0, 0, 0, 1}, nal.Data...), duration: sample.duration, pts: pts, hasPTS: hasPTS})
		}
		if nal.Type == NALTypeIDR {
			s.rememberBootstrap(nal, sample)
		}
	}
	for _, sub := range s.subscriberSnapshot() {
		if !s.enqueueVideo(sub, sample) {
			s.RemovePeer(sub.peer.ID)
		}
	}
}

// RunAudio reads OGG/Opus once and fans packets out to subscribers.
func (s *SharedStream) RunAudio(reader io.Reader) error {
	oggReader := NewOGGReader(reader)
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}
		packet, err := oggReader.ReadOpusPacket()
		if err != nil {
			return err
		}
		if len(packet) == 0 {
			continue
		}
		sample := sharedSample{data: packet, duration: 20 * time.Millisecond}
		for _, sub := range s.subscriberSnapshot() {
			if !s.enqueueAudio(sub, sample) {
				s.RemovePeer(sub.peer.ID)
			}
		}
	}
}

// RunAudioRaw reads length-prefixed Opus packets produced by the native pipeline.
func (s *SharedStream) RunAudioRaw(reader io.Reader) error {
	for {
		frame, err := streamframe.ReadAudio(reader)
		if err != nil {
			return err
		}
		sample := sharedSample{data: frame.Data, duration: frame.Duration, pts: frame.PTS, hasPTS: frame.HasPTS}
		for _, sub := range s.subscriberSnapshot() {
			if !s.enqueueAudio(sub, sample) {
				s.RemovePeer(sub.peer.ID)
			}
		}
	}
}

// RunSubtitles forwards ASS lines through per-peer pipes so existing subtitle parsing remains shared-input safe.
func (s *SharedStream) RunSubtitles(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		data := append([]byte(scanner.Bytes()), '\n')
		for _, sub := range s.subscriberSnapshot() {
			if !s.enqueueSubtitle(sub, data) {
				s.RemovePeer(sub.peer.ID)
			}
		}
	}
	return scanner.Err()
}

// RunSubtitlesRaw forwards length-prefixed subtitle JSON from the native ARIB decoder.
func (s *SharedStream) RunSubtitlesRaw(reader io.Reader) error {
	var size [4]byte
	for {
		if _, err := io.ReadFull(reader, size[:]); err != nil {
			return err
		}
		messageSize := binary.BigEndian.Uint32(size[:])
		if messageSize == 0 || messageSize > 64*1024 {
			return fmt.Errorf("invalid native subtitle message size %d", messageSize)
		}
		message := make([]byte, messageSize)
		if _, err := io.ReadFull(reader, message); err != nil {
			return err
		}
		for _, sub := range s.subscriberSnapshot() {
			if !s.enqueueSubtitleJSON(sub, message) {
				s.RemovePeer(sub.peer.ID)
			}
		}
	}
}

// Stop disconnects all subscribers and cancels shared readers.
func (s *SharedStream) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	ids := make([]string, 0, len(s.subscribers))
	for id := range s.subscribers {
		ids = append(ids, id)
	}
	s.mu.Unlock()

	s.cancel()
	for _, id := range ids {
		s.RemovePeer(id)
	}
}

func (s *SharedStream) SubscriberCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscribers)
}
