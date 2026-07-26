package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fuba/tv-viewer/internal/encoder"
	"github.com/fuba/tv-viewer/internal/mirakurun"
	"github.com/fuba/tv-viewer/internal/webrtc"
	"github.com/gin-gonic/gin"
	pionwebrtc "github.com/pion/webrtc/v4"
)

var (
	peerManager    *webrtc.PeerManager
	webrtcSessions = make(map[string]*encoder.WebRTCSession)
	webrtcMu       sync.RWMutex
)

func init() {
	peerManager = webrtc.NewPeerManager()
}

// streamingContext holds the state for one WebSocket subscriber.
type streamingContext struct {
	channelID       string
	burnInSubtitles bool
	audioMode       encoder.AudioMode // Dual mono mode: "main", "sub", "both"
	mirakurunURL    string
	peer            *webrtc.Peer
	session         *encoder.WebRTCSession
	mu              sync.Mutex
	stopCh          chan struct{}
	retryCount      int
	maxRetries      int
	shared          *sharedSession
	stopOnce        sync.Once
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

type streamTarget struct {
	channelType    string
	channel        string
	programNumbers []uint16
}

// resolveStreamTarget maps a UI selection to a physical tuner channel and one
// or more MPEG-TS programs. Service selections must keep exactly one program.
func resolveStreamTarget(channels []mirakurun.Channel, selectionID string) (streamTarget, error) {
	if strings.HasPrefix(selectionID, "service:") {
		parts := strings.Split(strings.TrimPrefix(selectionID, "service:"), ":")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return streamTarget{}, fmt.Errorf("invalid service selection %q", selectionID)
		}
		requestedType, requestedChannel, rawServiceID := parts[0], parts[1], parts[2]
		serviceID, err := strconv.ParseInt(rawServiceID, 10, 64)
		if err != nil || serviceID <= 0 {
			return streamTarget{}, fmt.Errorf("invalid service selection %q", selectionID)
		}
		for _, channel := range channels {
			if channel.Type != requestedType || channel.Channel != requestedChannel {
				continue
			}
			for _, service := range channel.Services {
				if service.ID != serviceID && int64(service.ServiceID) != serviceID {
					continue
				}
				if service.ServiceID <= 0 || service.ServiceID > 0xffff {
					return streamTarget{}, fmt.Errorf("invalid MPEG-TS service ID %d for %s", service.ServiceID, selectionID)
				}
				return streamTarget{
					channelType:    channel.Type,
					channel:        channel.Channel,
					programNumbers: []uint16{uint16(service.ServiceID)},
				}, nil
			}
		}
		return streamTarget{}, fmt.Errorf("service not found: %d", serviceID)
	}

	for _, channel := range channels {
		if channel.Channel != selectionID {
			continue
		}
		programNumbers := make([]uint16, 0, len(channel.Services))
		for _, service := range channel.Services {
			if service.ServiceID <= 0 || service.ServiceID > 0xffff {
				return streamTarget{}, fmt.Errorf("invalid MPEG-TS service ID %d for channel %s", service.ServiceID, selectionID)
			}
			programNumbers = append(programNumbers, uint16(service.ServiceID))
		}
		return streamTarget{channelType: channel.Type, channel: channel.Channel, programNumbers: programNumbers}, nil
	}

	return streamTarget{}, fmt.Errorf("channel not found: %s", selectionID)
}

func (r *cancelReadCloser) Close() error {
	r.cancel()
	return r.ReadCloser.Close()
}

func (ctx *streamingContext) stop() {
	if ctx == nil {
		return
	}
	ctx.stopOnce.Do(func() {
		close(ctx.stopCh)
		ctx.mu.Lock()
		shared := ctx.shared
		session := ctx.session
		ctx.mu.Unlock()
		if shared != nil {
			shared.removePeer(ctx.peer.ID)
			return
		}
		if session != nil {
			session.Stop()
		}
	})
}

func (ctx *streamingContext) isStopped() bool {
	select {
	case <-ctx.stopCh:
		return true
	default:
		return false
	}
}

// startStreamingWithRetry attaches a subscriber to a shared channel encoder.
func startStreamingWithRetry(ctx *streamingContext, safeWrite func(int, []byte) error) error {
	sharedSessions.startMu.Lock()
	defer sharedSessions.startMu.Unlock()

	log.Printf("[WebRTC] Starting shared stream for channel %s", ctx.channelID)
	if ctx.isStopped() {
		return context.Canceled
	}
	if err := sharedSessions.canAccept(ctx.channelID); err != nil {
		return err
	}

	ctx.mu.Lock()
	if existing := sharedSessions.get(ctx.channelID); existing != nil {
		if !existing.matchesSettings(ctx.burnInSubtitles, ctx.audioMode) {
			ctx.mu.Unlock()
			return fmt.Errorf("channel %s is already streaming with different audio/subtitle settings", ctx.channelID)
		}
		ctx.shared = existing
		ctx.session = existing.session
		ctx.mu.Unlock()
		if ctx.isStopped() {
			return context.Canceled
		}
		if err := existing.addPeer(ctx.peer); err != nil {
			existing.stopIfIdle()
			return err
		}
		if ctx.isStopped() {
			existing.removePeer(ctx.peer.ID)
			return context.Canceled
		}
		return nil
	}
	ctx.mu.Unlock()

	// Get Mirakurun stream
	log.Printf("[WebRTC] startStreamingWithRetry: fetching channels from Mirakurun for %s", ctx.channelID)
	client := mirakurun.NewClient(ctx.mirakurunURL)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	requestFinished := make(chan struct{})
	go func() {
		select {
		case <-ctx.stopCh:
			cancelRequest()
		case <-requestFinished:
		}
	}()

	channels, err := client.GetChannelsContext(requestCtx)
	if err != nil {
		close(requestFinished)
		cancelRequest()
		log.Printf("[WebRTC] startStreamingWithRetry: failed to get channels: %v", err)
		return fmt.Errorf("failed to get channels: %w", err)
	}
	if ctx.isStopped() {
		close(requestFinished)
		cancelRequest()
		return context.Canceled
	}
	log.Printf("[WebRTC] startStreamingWithRetry: got %d channels", len(channels))

	target, err := resolveStreamTarget(channels, ctx.channelID)
	if err != nil {
		close(requestFinished)
		cancelRequest()
		return err
	}

	// Keep Mirakurun responsible for tuning and decoding, then select the
	// requested service from the physical channel TS in the native Go demuxer.
	stream, err := client.GetChannelStreamWithTypeContext(requestCtx, target.channelType, target.channel)
	streamURL := fmt.Sprintf("%s/api/channels/%s/%s/stream", ctx.mirakurunURL, target.channelType, target.channel)

	if stream == nil {
		close(requestFinished)
		cancelRequest()
		return fmt.Errorf("failed to get stream: %w", err)
	}
	close(requestFinished)
	stream = &cancelReadCloser{ReadCloser: stream, cancel: cancelRequest}
	if ctx.isStopped() {
		stream.Close()
		return context.Canceled
	}

	// Start WebRTC encoding with audio mode
	audioMode := ctx.audioMode
	if audioMode == "" {
		audioMode = encoder.AudioModeBoth // Default to stereo
	}
	ctx.audioMode = audioMode
	var session *encoder.WebRTCSession
	log.Printf("[WebRTC] Using native Go MPEG-TS pipeline for channel %s", ctx.channelID)
	session, err = encoderInstance.StartNativeWebRTCEncoding(ctx.channelID, stream, streamURL, target.programNumbers, ctx.burnInSubtitles, audioMode)
	if err != nil {
		stream.Close()
		return fmt.Errorf("failed to start encoding: %w", err)
	}

	ctx.mu.Lock()
	ctx.session = session
	ctx.mu.Unlock()
	if ctx.isStopped() {
		session.Stop()
		return context.Canceled
	}

	shared := sharedSessions.register(ctx.channelID, session, ctx.burnInSubtitles, audioMode)
	if shared.session != session {
		// Another request won the registration race. Release the duplicate encoder.
		session.Stop()
	} else {
		webrtcMu.Lock()
		webrtcSessions[ctx.channelID] = session
		webrtcMu.Unlock()
	}
	ctx.mu.Lock()
	ctx.session = shared.session
	ctx.shared = shared
	ctx.mu.Unlock()
	if ctx.isStopped() {
		shared.stopIfIdle()
		return context.Canceled
	}
	if err := shared.addPeer(ctx.peer); err != nil {
		if shared.session == session {
			shared.stop()
		}
		return err
	}
	if ctx.isStopped() {
		shared.removePeer(ctx.peer.ID)
		shared.stopIfIdle()
		return context.Canceled
	}
	return nil
}

func parseAudioMode(value *string) (encoder.AudioMode, error) {
	if value == nil || *value == "" {
		return encoder.AudioModeBoth, nil
	}
	mode := encoder.AudioMode(*value)
	switch mode {
	case encoder.AudioModeMain, encoder.AudioModeSub, encoder.AudioModeBoth:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid audioMode %q", *value)
	}
}

// SetupWebRTCRoutes adds WebRTC-specific routes
func SetupWebRTCRoutes(api *gin.RouterGroup) {
	// WebRTC signaling endpoints
	api.POST("/webrtc/channels/:id/stream", startWebRTCStream)
	api.POST("/webrtc/offer", handleWebRTCOffer)
	api.POST("/webrtc/ice-candidate", handleICECandidate)
	api.DELETE("/webrtc/peer/:peerId", closePeer)
	api.GET("/webrtc/status", getWebRTCStatus)

	// WebSocket-based signaling
	api.GET("/ws/webrtc/:channelId", handleWebRTCSignaling)

	log.Println("[WebRTC] Routes initialized")
}

// startWebRTCStream starts a WebRTC stream for a channel
func startWebRTCStream(c *gin.Context) {
	channelID, err := url.QueryUnescape(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid channel ID"})
		return
	}

	peer, err := peerManager.CreatePeer(channelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to create WebRTC peer",
			"details": err.Error(),
		})
		return
	}
	mirakurunURL := os.Getenv("MIRAKURUN_URL")
	if mirakurunURL == "" {
		mirakurunURL = "http://tuner:40772"
	}
	streamCtx := &streamingContext{
		channelID: channelID, burnInSubtitles: true, audioMode: encoder.AudioModeBoth,
		mirakurunURL: mirakurunURL, peer: peer, stopCh: make(chan struct{}), maxRetries: 5,
	}
	if err := startStreamingWithRetry(streamCtx, func(int, []byte) error { return nil }); err != nil {
		peerManager.RemovePeer(peer.ID)
		status := http.StatusInternalServerError
		if err == errViewerLimit || err == errChannelLimit {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":    "streaming",
		"peerId":    peer.ID,
		"channelId": channelID,
		"sessionId": streamCtx.session.ID,
	})
}

// handleWebRTCOffer handles SDP offer from client
func handleWebRTCOffer(c *gin.Context) {
	var req struct {
		PeerID string                        `json:"peerId"`
		SDP    pionwebrtc.SessionDescription `json:"sdp"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	peer, ok := peerManager.GetPeer(req.PeerID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Peer not found"})
		return
	}

	answer, err := peer.HandleOffer(req.SDP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to handle offer",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"type":   "answer",
		"peerId": req.PeerID,
		"sdp":    answer,
	})
}

// handleICECandidate handles ICE candidate from client
func handleICECandidate(c *gin.Context) {
	var req struct {
		PeerID    string                      `json:"peerId"`
		Candidate pionwebrtc.ICECandidateInit `json:"candidate"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	peer, ok := peerManager.GetPeer(req.PeerID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Peer not found"})
		return
	}

	if err := peer.AddICECandidate(req.Candidate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to add ICE candidate",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// closePeer closes a WebRTC peer connection
func closePeer(c *gin.Context) {
	peerID := c.Param("peerId")

	peer, ok := peerManager.GetPeer(peerID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Peer not found"})
		return
	}

	if shared := sharedSessions.get(peer.ChannelID); shared != nil {
		shared.removePeer(peer.ID)
	} else {
		webrtcMu.Lock()
		if session, exists := webrtcSessions[peer.ChannelID]; exists {
			session.Stop()
			delete(webrtcSessions, peer.ChannelID)
		}
		webrtcMu.Unlock()
	}

	// Remove peer
	peerManager.RemovePeer(peerID)

	c.JSON(http.StatusOK, gin.H{"status": "closed"})
}

// getWebRTCStatus returns WebRTC status
func getWebRTCStatus(c *gin.Context) {
	webrtcMu.RLock()
	legacySessionCount := 0
	for channelID := range webrtcSessions {
		if sharedSessions.get(channelID) == nil {
			legacySessionCount++
		}
	}
	webrtcMu.RUnlock()
	sessionCount := sharedSessions.count() + legacySessionCount
	activeChannelID, activeViewerCount := sharedSessions.activeChannel()

	c.JSON(http.StatusOK, gin.H{
		"peerCount":         peerManager.GetPeerCount(),
		"sessionCount":      sessionCount,
		"activeChannelId":   activeChannelID,
		"activeViewerCount": activeViewerCount,
	})
}

// stopAllWebRTCSessions stops all existing WebRTC sessions to release tuners quickly
func stopAllWebRTCSessions() {
	sharedSessions.stopAll()
	webrtcMu.Lock()
	defer webrtcMu.Unlock()

	for channelID, session := range webrtcSessions {
		log.Printf("[WebRTC] Stopping session for channel %s", channelID)
		session.Stop()
		delete(webrtcSessions, channelID)
	}

	// Close all peers
	peerManager.CloseAll()
}

// WebSocket timeout constants
const (
	// How long to wait for any message before considering connection dead
	wsReadTimeout = 30 * time.Second
	// How often to send pings from server
	wsPingInterval          = 10 * time.Second
	maxSignalingMessageSize = 64 * 1024
	maxPendingICECandidates = 128
)

func validChannelID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune(":._-", char) {
			continue
		}
		return false
	}
	return true
}

// handleWebRTCSignaling handles WebSocket-based signaling
func handleWebRTCSignaling(c *gin.Context) {
	channelID, err := url.QueryUnescape(c.Param("channelId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid channel ID"})
		return
	}
	if !validChannelID(channelID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid channel ID"})
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WebRTC] WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(maxSignalingMessageSize)

	// Mutex for synchronized WebSocket writes (gorilla/websocket doesn't support concurrent writes)
	var wsMu sync.Mutex
	safeWrite := func(messageType int, data []byte) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		return conn.WriteMessage(messageType, data)
	}
	safeWriteControl := func(messageType int, data []byte, deadline time.Time) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		return conn.WriteControl(messageType, data, deadline)
	}

	log.Printf("[WebRTC] WebSocket connected for channel %s", channelID)

	// Get Mirakurun URL (used by both stream-start and restart-encoding)
	mirakurunURL := os.Getenv("MIRAKURUN_URL")
	if mirakurunURL == "" {
		mirakurunURL = "http://tuner:40772"
	}

	// Helper to send error messages safely
	sendErrorSafe := func(peerID, errorMsg string) {
		msg := webrtc.NewErrorMessage(peerID, errorMsg)
		data, _ := msg.ToJSON()
		safeWrite(1, data)
	}
	sendRequestErrorSafe := func(peerID, requestID, errorMsg string) {
		msg := webrtc.NewErrorMessage(peerID, errorMsg)
		msg.RequestID = requestID
		data, _ := msg.ToJSON()
		safeWrite(1, data)
	}

	// Set initial read deadline
	conn.SetReadDeadline(time.Now().Add(wsReadTimeout))

	// Set pong handler to extend read deadline when pong is received
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		return nil
	})

	// Start server-side ping goroutine to detect dead connections
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(wsPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := safeWriteControl(9, []byte{}, time.Now().Add(5*time.Second)); err != nil {
					// Ping failed, connection is likely dead
					log.Printf("[WebRTC] Ping failed for channel %s: %v", channelID, err)
					return
				}
			}
		}
	}()

	var peer *webrtc.Peer
	var streamCtx *streamingContext
	var streamGeneration atomic.Uint64
	var pendingICECandidates []pionwebrtc.ICECandidateInit

	// Message handling loop
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[WebRTC] WebSocket read error for channel %s: %v", channelID, err)
			break
		}

		// Extend read deadline on any message received
		conn.SetReadDeadline(time.Now().Add(wsReadTimeout))

		var msg webrtc.SignalingMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("[WebRTC] Invalid message: %v", err)
			continue
		}

		log.Printf("[WebRTC] Received message type: %s for channel %s", msg.Type, channelID)

		switch msg.Type {
		case webrtc.MsgTypeStreamStart:
			// Start streaming
			// Default: send ARIB captions over the data channel.
			burnInSubtitles := true
			if msg.BurnInSubtitles != nil {
				burnInSubtitles = *msg.BurnInSubtitles
			}
			// Default: stereo audio (both channels)
			audioMode, err := parseAudioMode(msg.AudioMode)
			if err != nil {
				sendErrorSafe("", err.Error())
				continue
			}
			log.Printf("[WebRTC] Starting stream for channel %s (burnInSubtitles=%v, audioMode=%s)", channelID, burnInSubtitles, audioMode)

			// Stop existing streaming context and session
			if streamCtx != nil {
				log.Printf("[WebRTC] Stopping existing stream for this connection")
				streamCtx.stop()
				streamCtx = nil
			}
			if peer != nil {
				log.Printf("[WebRTC] Removing existing peer for this connection: %s", peer.ID)
				peerManager.RemovePeer(peer.ID)
				peer = nil
			}

			if err := sharedSessions.canAccept(channelID); err != nil {
				sendErrorSafe("", err.Error())
				continue
			}

			peer, err = peerManager.CreatePeerLimited(channelID, maxViewers())
			if err != nil {
				sendErrorSafe("", "Failed to create peer: "+err.Error())
				continue
			}
			pendingICECandidates = nil

			// Create offer (server-side offer for push mode)
			offer, err := peer.PC.CreateOffer(nil)
			if err != nil {
				sendErrorSafe(peer.ID, "Failed to create offer: "+err.Error())
				continue
			}

			gatheringComplete := pionwebrtc.GatheringCompletePromise(peer.PC)
			if err := peer.PC.SetLocalDescription(offer); err != nil {
				sendErrorSafe(peer.ID, "Failed to set local description: "+err.Error())
				continue
			}
			<-gatheringComplete

			// Send the fully gathered offer so clients do not need to process a
			// candidate before the remote description is installed.
			response := webrtc.NewOfferMessage(peer.ID, *peer.PC.LocalDescription())
			data, _ := response.ToJSON()
			safeWrite(1, data)

			// Create streaming context with retry support
			streamCtx = &streamingContext{
				channelID:       channelID,
				burnInSubtitles: burnInSubtitles,
				audioMode:       audioMode,
				mirakurunURL:    mirakurunURL,
				peer:            peer,
				stopCh:          make(chan struct{}),
				maxRetries:      5, // Max 5 retries on crash
			}

			// Start streaming with automatic retry on native pipeline failure
			// Run in goroutine to prevent blocking WebSocket message loop
			// This allows answer and ICE candidate messages to be processed during pipeline init
			currentStreamCtx := streamCtx
			currentPeerID := peer.ID
			currentGeneration := streamGeneration.Add(1)
			go func() {
				if err := startStreamingWithRetry(currentStreamCtx, safeWrite); err != nil {
					if errors.Is(err, context.Canceled) || currentStreamCtx.isStopped() {
						return
					}
					if streamGeneration.Load() != currentGeneration {
						return
					}
					log.Printf("[WebRTC] stream-start: startStreamingWithRetry failed: %v", err)
					currentStreamCtx.stop()
					peerManager.RemovePeer(currentPeerID)
					sendErrorSafe(currentPeerID, "Failed to start streaming: "+err.Error())
				}
			}()

		case webrtc.MsgTypeAnswer:
			if peer == nil || msg.SDP == nil {
				continue
			}
			if err := peer.PC.SetRemoteDescription(*msg.SDP); err != nil {
				log.Printf("[WebRTC] Failed to set remote description: %v", err)
				continue
			}
			for _, candidate := range pendingICECandidates {
				if err := peer.AddICECandidate(candidate); err != nil {
					log.Printf("[WebRTC] Failed to add queued ICE candidate: %v", err)
				}
			}
			pendingICECandidates = nil

		case webrtc.MsgTypeICECandidate:
			if peer == nil || msg.Candidate == nil {
				continue
			}
			if peer.PC.RemoteDescription() == nil {
				if len(pendingICECandidates) >= maxPendingICECandidates {
					log.Printf("[WebRTC] Dropping excess ICE candidate for peer %s", peer.ID)
					continue
				}
				pendingICECandidates = append(pendingICECandidates, *msg.Candidate)
				continue
			}
			if err := peer.AddICECandidate(*msg.Candidate); err != nil {
				log.Printf("[WebRTC] Failed to add ICE candidate: %v", err)
			}

		case webrtc.MsgTypeRestartEncoding:
			// Restart encoding with new settings (keeps WebRTC connection)
			// This is used for:
			// 1. Toggling ARIB caption delivery
			// 2. Changing channels without reconnecting
			// 3. Switching audio mode (dual mono)
			newChannelID := channelID
			if msg.ChannelID != "" {
				newChannelID = msg.ChannelID
			}
			if !validChannelID(newChannelID) {
				sendRequestErrorSafe("", msg.RequestID, "Invalid channel ID")
				continue
			}
			burnInSubtitles := true
			if msg.BurnInSubtitles != nil {
				burnInSubtitles = *msg.BurnInSubtitles
			}
			if peer == nil {
				sendRequestErrorSafe("", msg.RequestID, "No peer connection")
				continue
			}
			// Default: stereo audio (both channels)
			audioMode, err := parseAudioMode(msg.AudioMode)
			if err != nil {
				sendRequestErrorSafe(peer.ID, msg.RequestID, err.Error())
				continue
			}
			log.Printf("[WebRTC] Restarting encoding for channel %s -> %s (burnInSubtitles=%v, audioMode=%s)", channelID, newChannelID, burnInSubtitles, audioMode)

			if streamCtx != nil {
				streamCtx.mu.Lock()
				currentShared := streamCtx.shared
				streamCtx.mu.Unlock()
				if currentShared != nil && newChannelID != channelID && currentShared.peerCount() > 1 {
					sendRequestErrorSafe(peer.ID, msg.RequestID, "channel cannot be changed while other viewers are watching")
					continue
				}
				if currentShared != nil && !currentShared.matchesSettings(burnInSubtitles, audioMode) && currentShared.peerCount() > 1 {
					sendRequestErrorSafe(peer.ID, msg.RequestID, "audio settings cannot be changed while this channel has multiple viewers")
					continue
				}
			}
			currentGeneration := streamGeneration.Add(1)

			// Stop current streaming context and session (prevents auto-retry)
			log.Printf("[WebRTC] restart-encoding: stopping old streaming context")
			oldShared := (*sharedSession)(nil)
			oldChannelID := channelID
			if streamCtx != nil {
				streamCtx.mu.Lock()
				oldShared = streamCtx.shared
				streamCtx.mu.Unlock()
				streamCtx.stop()
				streamCtx = nil
			}
			if oldShared != nil && (oldChannelID != newChannelID || !oldShared.matchesSettings(burnInSubtitles, audioMode)) {
				oldShared.stopIfIdle()
			}

			// Update channel ID if changed
			channelID = newChannelID
			log.Printf("[WebRTC] restart-encoding: updated channelID to %s", channelID)

			// Create new streaming context with retry support
			log.Printf("[WebRTC] restart-encoding: creating new streaming context")
			streamCtx = &streamingContext{
				channelID:       channelID,
				burnInSubtitles: burnInSubtitles,
				audioMode:       audioMode,
				mirakurunURL:    mirakurunURL,
				peer:            peer,
				stopCh:          make(chan struct{}),
				maxRetries:      5,
			}

			// Start streaming with retry in a goroutine to prevent blocking WebSocket message loop
			// This allows ping/pong to continue while pipeline initialization happens
			log.Printf("[WebRTC] restart-encoding: starting streamingWithRetry in goroutine")
			currentStreamCtx := streamCtx
			currentChannelID := channelID
			currentPeerID := peer.ID
			currentRequestID := msg.RequestID
			go func() {
				if err := startStreamingWithRetry(currentStreamCtx, safeWrite); err != nil {
					if errors.Is(err, context.Canceled) || currentStreamCtx.isStopped() {
						return
					}
					if streamGeneration.Load() != currentGeneration {
						return
					}
					log.Printf("[WebRTC] restart-encoding: startStreamingWithRetry failed: %v", err)
					currentStreamCtx.stop()
					peerManager.RemovePeer(currentPeerID)
					sendRequestErrorSafe(currentPeerID, currentRequestID, "Failed to restart streaming: "+err.Error())
					return
				}
				if streamGeneration.Load() != currentGeneration {
					currentStreamCtx.stop()
					return
				}
				log.Printf("[WebRTC] restart-encoding: startStreamingWithRetry succeeded")

				// Send encoding-restarted acknowledgment
				response := &webrtc.SignalingMessage{Type: webrtc.MsgTypeEncodingRestarted, ChannelID: currentChannelID, RequestID: currentRequestID}
				data, _ := response.ToJSON()
				safeWrite(1, data)
				log.Printf("[WebRTC] Encoding restarted for channel %s", currentChannelID)
			}()

		case webrtc.MsgTypeStreamStop:
			streamGeneration.Add(1)
			log.Printf("[WebRTC] Received stream-stop for channel %s", channelID)
			// Stop streaming context first (prevents auto-retry)
			if streamCtx != nil {
				streamCtx.stop()
				streamCtx = nil
			}
			if peer != nil {
				peerManager.RemovePeer(peer.ID)
				peer = nil
			}
			pendingICECandidates = nil
			// Send stream-stopped acknowledgment
			response := &webrtc.SignalingMessage{Type: webrtc.MsgTypeStreamStopped}
			data, _ := response.ToJSON()
			safeWrite(1, data)
			log.Printf("[WebRTC] Sent stream-stopped acknowledgment for channel %s", channelID)

		case webrtc.MsgTypePing:
			response := &webrtc.SignalingMessage{Type: webrtc.MsgTypePong}
			data, _ := response.ToJSON()
			safeWrite(1, data)
		}
	}

	// Cleanup on disconnect
	streamGeneration.Add(1)
	if streamCtx != nil {
		streamCtx.stop()
	}
	if peer != nil {
		peerManager.RemovePeer(peer.ID)
	}

	log.Printf("[WebRTC] WebSocket disconnected for channel %s", channelID)
}

// StartWebRTCServiceStream starts a WebRTC stream for a specific service
func StartWebRTCServiceStream(c *gin.Context) {
	serviceIDStr := c.Param("serviceId")
	serviceID, err := strconv.ParseInt(serviceIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid service ID"})
		return
	}

	mirakurunURL := os.Getenv("MIRAKURUN_URL")
	if mirakurunURL == "" {
		mirakurunURL = "http://tuner:40772"
	}

	client := mirakurun.NewClient(mirakurunURL)

	stream, err := client.GetServiceStream(serviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get service stream",
			"details": err.Error(),
		})
		return
	}

	streamURL := fmt.Sprintf("%s/api/services/%d/stream", mirakurunURL, serviceID)
	sessionID := fmt.Sprintf("SVC_%d", serviceID)

	// Start the direct Go MPEG-TS pipeline (stereo audio).
	programNumber := serviceID % 100000
	if programNumber <= 0 || programNumber > 0xffff {
		stream.Close()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid MPEG-TS program number"})
		return
	}
	session, err := encoderInstance.StartNativeWebRTCEncoding(sessionID, stream, streamURL, []uint16{uint16(programNumber)}, false, encoder.AudioModeBoth)
	if err != nil {
		stream.Close()
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to start WebRTC encoding",
			"details": err.Error(),
		})
		return
	}

	webrtcMu.Lock()
	webrtcSessions[sessionID] = session
	webrtcMu.Unlock()

	peer, err := peerManager.CreatePeer(sessionID)
	if err != nil {
		session.Stop()
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to create WebRTC peer",
			"details": err.Error(),
		})
		return
	}

	go func() {
		var err error
		if session.VideoRaw {
			err = peer.StreamRawH264(session.VideoPipe)
		} else {
			err = peer.StreamH264(session.VideoPipe)
		}
		if err != nil {
			log.Printf("[WebRTC] Service video stream ended: %v", err)
		}
	}()

	go func() {
		// Wait up to 5 seconds for audio pipe to be ready
		for i := 0; i < 50; i++ {
			if session.AudioPipe != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if session.AudioPipe == nil {
			log.Printf("[WebRTC] Audio pipe not ready for service, skipping audio")
			return
		}
		var err error
		if session.AudioRaw {
			err = peer.StreamRawOpus(session.AudioPipe)
		} else {
			err = peer.StreamOpus(session.AudioPipe)
		}
		if err != nil {
			log.Printf("[WebRTC] Service audio stream ended: %v", err)
		}
	}()

	go func() {
		// Wait up to 5 seconds for subtitle pipe to be ready
		for i := 0; i < 50; i++ {
			if session.SubtitlePipe != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if session.SubtitlePipe == nil {
			log.Printf("[WebRTC] Subtitle pipe not ready for service, skipping subtitles")
			return
		}
		if err := peer.StreamSubtitles(session.SubtitlePipe); err != nil {
			log.Printf("[WebRTC] Service subtitle stream ended: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":    "streaming",
		"peerId":    peer.ID,
		"serviceId": serviceID,
		"sessionId": session.ID,
	})
}
