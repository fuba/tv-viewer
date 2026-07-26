package webrtc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/fuba/tv-viewer/internal/streamframe"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

var ErrPeerLimit = errors.New("peer limit reached")

// Peer represents a WebRTC peer connection with associated tracks
type Peer struct {
	ID          string
	ChannelID   string
	PC          *webrtc.PeerConnection
	VideoTrack  *webrtc.TrackLocalStaticSample
	AudioTrack  *webrtc.TrackLocalStaticSample
	VideoSender *webrtc.RTPSender
	AudioSender *webrtc.RTPSender
	DataChan    *webrtc.DataChannel
	CreatedAt   time.Time

	ctx        context.Context
	cancel     context.CancelFunc
	h264Parser *H264Parser

	// Streaming context - can be cancelled and replaced without closing the peer connection
	streamCtx    context.Context
	streamCancel context.CancelFunc
	streamMu     sync.Mutex

	onICECandidate func(*webrtc.ICECandidate)
	onStateChange  func(webrtc.PeerConnectionState)
}

// PeerManager manages multiple WebRTC peer connections
type PeerManager struct {
	peers map[string]*Peer
	mu    sync.RWMutex

	// Configuration
	iceServers []webrtc.ICEServer
	api        *webrtc.API
}

// NewPeerManager creates a new peer manager
func NewPeerManager() *PeerManager {
	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetInterfaceFilter(func(name string) bool {
		// Docker bridge and virtual Ethernet interfaces are not useful to remote viewers.
		return name != "lo" && !strings.HasPrefix(name, "docker") &&
			!strings.HasPrefix(name, "br-") && !strings.HasPrefix(name, "veth")
	})
	return &PeerManager{
		peers:      make(map[string]*Peer),
		api:        webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine)),
		iceServers: []webrtc.ICEServer{
			// No STUN/TURN needed for local network
		},
	}
}

// SetICEServers sets the ICE servers for new connections
func (pm *PeerManager) SetICEServers(servers []webrtc.ICEServer) {
	pm.iceServers = servers
}

// CreatePeerLimited atomically enforces a process-wide peer limit.
func (pm *PeerManager) CreatePeerLimited(channelID string, limit int) (*Peer, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if limit > 0 && len(pm.peers) >= limit {
		return nil, ErrPeerLimit
	}
	return pm.createPeerLocked(channelID)
}

// CreatePeer creates a new WebRTC peer for the given channel.
func (pm *PeerManager) CreatePeer(channelID string) (*Peer, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.createPeerLocked(channelID)
}

func (pm *PeerManager) createPeerLocked(channelID string) (*Peer, error) {
	// Generate unique peer ID
	peerID := fmt.Sprintf("%s-%d", channelID, time.Now().UnixNano())
	streamID := fmt.Sprintf("tv-stream-%s", channelID)

	// Create peer connection config
	config := webrtc.Configuration{
		ICEServers: pm.iceServers,
	}

	// Create peer connection
	pc, err := pm.api.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	// Create video track (H.264) using Sample-based track
	// This lets Pion handle RTP packetization internally
	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   H264ClockRate,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=64002a",
		},
		"video",
		streamID,
	)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to create video track: %w", err)
	}

	// Add video track to peer connection
	videoSender, err := pc.AddTrack(videoTrack)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to add video track: %w", err)
	}

	// Create audio track (Opus) using Sample-based track
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: OpusClockRate,
			Channels:  2,
		},
		"audio",
		streamID,
	)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to create audio track: %w", err)
	}

	// Add audio track to peer connection
	audioSender, err := pc.AddTrack(audioTrack)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to add audio track: %w", err)
	}

	// Create data channel for subtitles
	dataChan, err := pc.CreateDataChannel("subtitles", &webrtc.DataChannelInit{
		Ordered: boolPtr(true),
	})
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to create data channel: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	streamCtx, streamCancel := context.WithCancel(ctx)

	peer := &Peer{
		ID:           peerID,
		ChannelID:    channelID,
		PC:           pc,
		VideoTrack:   videoTrack,
		AudioTrack:   audioTrack,
		VideoSender:  videoSender,
		AudioSender:  audioSender,
		DataChan:     dataChan,
		CreatedAt:    time.Now(),
		ctx:          ctx,
		cancel:       cancel,
		streamCtx:    streamCtx,
		streamCancel: streamCancel,
		h264Parser:   NewH264Parser(),
	}

	// Set up ICE candidate handler
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil && peer.onICECandidate != nil {
			peer.onICECandidate(candidate)
		}
	})

	// Set up connection state handler
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[WebRTC] Peer %s connection state: %s", peerID, state)
		if peer.onStateChange != nil {
			peer.onStateChange(state)
		}

		// Auto-cleanup on failed or closed
		if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateClosed {
			pm.RemovePeer(peerID)
		}
	})

	pm.peers[peerID] = peer
	go peer.readRTCP(videoSender)
	go peer.readRTCP(audioSender)
	log.Printf("[WebRTC] Created peer %s for channel %s", peerID, channelID)

	return peer, nil
}

// readRTCP keeps Pion's feedback interceptors active. In particular, NACK
// reports must be consumed so transient packet loss can be retransmitted.
func (p *Peer) readRTCP(sender *webrtc.RTPSender) {
	buffer := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buffer); err != nil {
			return
		}
	}
}

// GetPeer returns a peer by ID
func (pm *PeerManager) GetPeer(peerID string) (*Peer, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	peer, ok := pm.peers[peerID]
	return peer, ok
}

// RemovePeer removes and closes a peer connection
func (pm *PeerManager) RemovePeer(peerID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if peer, ok := pm.peers[peerID]; ok {
		peer.Close()
		delete(pm.peers, peerID)
		log.Printf("[WebRTC] Removed peer %s", peerID)
	}
}

// GetPeerCount returns the number of active peers
func (pm *PeerManager) GetPeerCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.peers)
}

// CloseAll closes all peer connections
func (pm *PeerManager) CloseAll() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for id, peer := range pm.peers {
		peer.Close()
		delete(pm.peers, id)
	}
	log.Printf("[WebRTC] Closed all peers")
}

// SetOnICECandidate sets the ICE candidate callback
func (p *Peer) SetOnICECandidate(fn func(*webrtc.ICECandidate)) {
	p.onICECandidate = fn
}

// SetOnStateChange sets the connection state change callback
func (p *Peer) SetOnStateChange(fn func(webrtc.PeerConnectionState)) {
	p.onStateChange = fn
}

// HandleOffer handles an SDP offer and returns an answer
func (p *Peer) HandleOffer(offer webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	if err := p.PC.SetRemoteDescription(offer); err != nil {
		return nil, fmt.Errorf("failed to set remote description: %w", err)
	}

	answer, err := p.PC.CreateAnswer(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create answer: %w", err)
	}

	if err := p.PC.SetLocalDescription(answer); err != nil {
		return nil, fmt.Errorf("failed to set local description: %w", err)
	}

	return &answer, nil
}

// AddICECandidate adds an ICE candidate to the peer connection
func (p *Peer) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	return p.PC.AddICECandidate(candidate)
}

// WriteVideoSample writes a video sample to the video track
func (p *Peer) WriteVideoSample(data []byte, duration time.Duration) error {
	return p.VideoTrack.WriteSample(media.Sample{
		Data:     data,
		Duration: duration,
	})
}

// WriteAudioSample writes an audio sample to the audio track
func (p *Peer) WriteAudioSample(data []byte, duration time.Duration) error {
	return p.AudioTrack.WriteSample(media.Sample{
		Data:     data,
		Duration: duration,
	})
}

// SendSubtitle sends a subtitle message via data channel
func (p *Peer) SendSubtitle(data []byte) error {
	if p.DataChan == nil || p.DataChan.ReadyState() != webrtc.DataChannelStateOpen {
		return nil // Silently ignore if data channel not ready
	}
	return p.DataChan.Send(data)
}

// StreamH264 reads H.264 data from reader and streams to the peer
func (p *Peer) StreamH264(reader io.Reader) error {
	h264Reader := NewH264Reader(reader)
	streamCtx := p.StreamContext()
	log.Printf("[WebRTC] StreamH264 started for peer %s", p.ID)

	nalCount := 0
	sampleCount := 0
	lastLogTime := time.Now()

	// Frame duration at 30fps
	frameDuration := time.Second / 30

	for {
		select {
		case <-streamCtx.Done():
			log.Printf("[WebRTC] StreamH264 streaming context done for peer %s (NALs: %d, Samples: %d)", p.ID, nalCount, sampleCount)
			return streamCtx.Err()
		case <-p.ctx.Done():
			log.Printf("[WebRTC] StreamH264 peer context done for peer %s (NALs: %d, Samples: %d)", p.ID, nalCount, sampleCount)
			return p.ctx.Err()
		default:
		}

		nalUnits, err := h264Reader.ReadNALUnits()
		if err != nil {
			if err == io.EOF {
				log.Printf("[WebRTC] StreamH264 EOF for peer %s (NALs: %d, Samples: %d)", p.ID, nalCount, sampleCount)
				return nil
			}
			return fmt.Errorf("failed to read NAL units: %w", err)
		}

		if len(nalUnits) == 0 {
			continue
		}

		nalCount += len(nalUnits)

		// Log NAL unit types for debugging (first 10 NALs only)
		if nalCount <= 10 {
			for _, nal := range nalUnits {
				nalType := nal.Type
				typeName := "unknown"
				switch nalType {
				case 1:
					typeName = "Slice"
				case 5:
					typeName = "IDR"
				case 6:
					typeName = "SEI"
				case 7:
					typeName = "SPS"
				case 8:
					typeName = "PPS"
				case 9:
					typeName = "AUD"
				}
				log.Printf("[WebRTC] NAL unit type=%d (%s), size=%d bytes", nalType, typeName, len(nal.Data))
			}
		}

		// Send each NAL unit as a sample
		// For H.264, each NAL unit should be sent with Annex B start code for the sample writer
		for _, nal := range nalUnits {
			// Skip SEI NAL units
			if nal.Type == NALTypeSEI {
				continue
			}

			// Create sample data with Annex B start code
			// The sample writer expects Annex B format
			sampleData := append([]byte{0x00, 0x00, 0x00, 0x01}, nal.Data...)

			if err := p.WriteVideoSample(sampleData, frameDuration); err != nil {
				log.Printf("[WebRTC] Sample write error for peer %s: %v", p.ID, err)
				return fmt.Errorf("failed to write sample: %w", err)
			}
			sampleCount++
		}

		// Log progress every 5 seconds
		if time.Since(lastLogTime) > 5*time.Second {
			log.Printf("[WebRTC] Peer %s streaming: NALs=%d, Samples=%d", p.ID, nalCount, sampleCount)
			lastLogTime = time.Now()
		}
	}
}

// StreamRawH264 reads length-prefixed H.264 access units from the native pipeline.
func (p *Peer) StreamRawH264(reader io.Reader) error {
	streamCtx := p.StreamContext()
	for {
		select {
		case <-streamCtx.Done():
			return streamCtx.Err()
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
		}
		frame, err := streamframe.ReadVideo(reader)
		if err != nil {
			return err
		}
		select {
		case <-streamCtx.Done():
			return streamCtx.Err()
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
		}
		if err := p.WriteVideoSample(frame.Data, frame.Duration); err != nil {
			return err
		}
	}
}

// StreamOpus reads Opus audio from an OGG stream and sends to the peer
func (p *Peer) StreamOpus(reader io.Reader) error {
	oggReader := NewOGGReader(reader)
	streamCtx := p.StreamContext()
	log.Printf("[WebRTC] StreamOpus started for peer %s", p.ID)

	packetCount := 0
	lastLogTime := time.Now()

	// Opus frame duration (20ms at 48kHz = 960 samples)
	frameDuration := 20 * time.Millisecond

	for {
		select {
		case <-streamCtx.Done():
			log.Printf("[WebRTC] StreamOpus streaming context done for peer %s (packets: %d)", p.ID, packetCount)
			return streamCtx.Err()
		case <-p.ctx.Done():
			log.Printf("[WebRTC] StreamOpus peer context done for peer %s (packets: %d)", p.ID, packetCount)
			return p.ctx.Err()
		default:
		}

		packet, err := oggReader.ReadOpusPacket()
		if err != nil {
			if err == io.EOF {
				log.Printf("[WebRTC] StreamOpus EOF for peer %s (packets: %d)", p.ID, packetCount)
				return nil
			}
			return fmt.Errorf("failed to read Opus packet: %w", err)
		}

		if len(packet) == 0 {
			continue
		}

		// Write audio sample
		if err := p.WriteAudioSample(packet, frameDuration); err != nil {
			log.Printf("[WebRTC] Audio sample write error for peer %s: %v", p.ID, err)
			return fmt.Errorf("failed to write audio sample: %w", err)
		}
		packetCount++

		// Log progress every 5 seconds
		if time.Since(lastLogTime) > 5*time.Second {
			log.Printf("[WebRTC] Peer %s audio streaming: packets=%d", p.ID, packetCount)
			lastLogTime = time.Now()
		}
	}
}

// StreamRawOpus reads the native pipeline's length-prefixed Opus packets.
func (p *Peer) StreamRawOpus(reader io.Reader) error {
	streamCtx := p.StreamContext()
	var size [4]byte
	for {
		select {
		case <-streamCtx.Done():
			return streamCtx.Err()
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
		}
		if _, err := io.ReadFull(reader, size[:]); err != nil {
			return err
		}
		length := binary.BigEndian.Uint32(size[:])
		if length == 0 || length > 64*1024 {
			return fmt.Errorf("invalid native Opus packet size %d", length)
		}
		packet := make([]byte, length)
		if _, err := io.ReadFull(reader, packet); err != nil {
			return err
		}
		select {
		case <-streamCtx.Done():
			return streamCtx.Err()
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
		}
		if err := p.WriteAudioSample(packet, 20*time.Millisecond); err != nil {
			return err
		}
	}
}

// Close closes the peer connection and releases resources
func (p *Peer) Close() {
	p.cancel()

	if p.DataChan != nil {
		p.DataChan.Close()
	}

	if p.PC != nil {
		p.PC.Close()
	}
}

// Context returns the peer's context
func (p *Peer) Context() context.Context {
	return p.ctx
}

// StopStreaming cancels the current streaming context, stopping StreamH264/StreamOpus goroutines
func (p *Peer) StopStreaming() {
	p.streamMu.Lock()
	defer p.streamMu.Unlock()

	if p.streamCancel != nil {
		p.streamCancel()
		log.Printf("[WebRTC] Stopped streaming for peer %s", p.ID)
	}
}

// RestartStreaming creates a new streaming context and returns it
// This allows starting new StreamH264/StreamOpus goroutines without recreating the peer
func (p *Peer) RestartStreaming() context.Context {
	p.streamMu.Lock()
	defer p.streamMu.Unlock()

	// Cancel existing streaming context
	if p.streamCancel != nil {
		p.streamCancel()
	}

	// Create new streaming context
	p.streamCtx, p.streamCancel = context.WithCancel(p.ctx)
	log.Printf("[WebRTC] Restarted streaming context for peer %s", p.ID)
	return p.streamCtx
}

// StreamContext returns the current streaming context
func (p *Peer) StreamContext() context.Context {
	p.streamMu.Lock()
	defer p.streamMu.Unlock()
	return p.streamCtx
}

// boolPtr returns a pointer to a bool
func boolPtr(b bool) *bool {
	return &b
}
