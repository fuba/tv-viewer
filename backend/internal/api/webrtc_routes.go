package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/fuba/tv-viewer/internal/encoder"
	"github.com/fuba/tv-viewer/internal/mirakurun"
	"github.com/fuba/tv-viewer/internal/webrtc"
	"github.com/gin-gonic/gin"
	pionwebrtc "github.com/pion/webrtc/v4"
)

var (
	peerManager     *webrtc.PeerManager
	webrtcSessions  = make(map[string]*encoder.WebRTCSession)
	webrtcMu        sync.RWMutex
)

func init() {
	peerManager = webrtc.NewPeerManager()
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

	mirakurunURL := os.Getenv("MIRAKURUN_URL")
	if mirakurunURL == "" {
		mirakurunURL = "http://tuner:40772"
	}

	client := mirakurun.NewClient(mirakurunURL)

	// Get channel info
	channels, err := client.GetChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get channels"})
		return
	}

	var channelType string
	var firstServiceID int64
	for _, ch := range channels {
		if ch.Channel == channelID {
			channelType = ch.Type
			if len(ch.Services) > 0 {
				firstServiceID = ch.Services[0].ID
			}
			break
		}
	}

	if channelType == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Channel not found"})
		return
	}

	// Get stream
	var stream io.ReadCloser
	var streamURL string
	if firstServiceID != 0 {
		stream, err = client.GetServiceStream(firstServiceID)
		streamURL = fmt.Sprintf("%s/api/services/%d/stream", mirakurunURL, firstServiceID)
	} else {
		stream, err = client.GetChannelStreamWithType(channelType, channelID)
		streamURL = fmt.Sprintf("%s/api/channels/%s/%s/stream", mirakurunURL, channelType, channelID)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get stream",
			"details": err.Error(),
		})
		return
	}

	// Start WebRTC encoding
	session, err := encoderInstance.StartWebRTCEncoding(channelID, stream, streamURL, -1, -1)
	if err != nil {
		stream.Close()
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to start WebRTC encoding",
			"details": err.Error(),
		})
		return
	}

	// Store session
	webrtcMu.Lock()
	webrtcSessions[channelID] = session
	webrtcMu.Unlock()

	// Create WebRTC peer
	peer, err := peerManager.CreatePeer(channelID)
	if err != nil {
		session.Stop()
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to create WebRTC peer",
			"details": err.Error(),
		})
		return
	}

	// Start streaming video to peer
	go func() {
		if err := peer.StreamH264(session.VideoPipe); err != nil {
			log.Printf("[WebRTC] Video stream ended for channel %s: %v", channelID, err)
		}
	}()

	// Start streaming audio to peer (wait for audio pipe to be ready)
	go func() {
		// Wait up to 5 seconds for audio pipe to be ready
		for i := 0; i < 50; i++ {
			if session.AudioPipe != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if session.AudioPipe == nil {
			log.Printf("[WebRTC] Audio pipe not ready for channel %s, skipping audio", channelID)
			return
		}
		if err := peer.StreamOpus(session.AudioPipe); err != nil {
			log.Printf("[WebRTC] Audio stream ended for channel %s: %v", channelID, err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":    "streaming",
		"peerId":    peer.ID,
		"channelId": channelID,
		"sessionId": session.ID,
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
		PeerID    string                       `json:"peerId"`
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

	// Stop associated encoding session
	webrtcMu.Lock()
	if session, exists := webrtcSessions[peer.ChannelID]; exists {
		session.Stop()
		delete(webrtcSessions, peer.ChannelID)
	}
	webrtcMu.Unlock()

	// Remove peer
	peerManager.RemovePeer(peerID)

	c.JSON(http.StatusOK, gin.H{"status": "closed"})
}

// getWebRTCStatus returns WebRTC status
func getWebRTCStatus(c *gin.Context) {
	webrtcMu.RLock()
	sessionCount := len(webrtcSessions)
	webrtcMu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"peerCount":    peerManager.GetPeerCount(),
		"sessionCount": sessionCount,
	})
}

// stopAllWebRTCSessions stops all existing WebRTC sessions to release tuners quickly
func stopAllWebRTCSessions() {
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
	wsPingInterval = 10 * time.Second
)

// handleWebRTCSignaling handles WebSocket-based signaling
func handleWebRTCSignaling(c *gin.Context) {
	channelID, err := url.QueryUnescape(c.Param("channelId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid channel ID"})
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WebRTC] WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("[WebRTC] WebSocket connected for channel %s", channelID)

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
				if err := conn.WriteControl(9, []byte{}, time.Now().Add(5*time.Second)); err != nil {
					// Ping failed, connection is likely dead
					log.Printf("[WebRTC] Ping failed for channel %s: %v", channelID, err)
					return
				}
			}
		}
	}()

	var peer *webrtc.Peer
	var session *encoder.WebRTCSession

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
			log.Printf("[WebRTC] Starting stream for channel %s", channelID)

			// Stop only THIS connection's existing session (not all sessions!)
			// This allows multiple tabs to have independent sessions
			if session != nil {
				log.Printf("[WebRTC] Stopping existing session for this connection: %s", session.ID)
				session.Stop()
				session = nil
			}
			if peer != nil {
				log.Printf("[WebRTC] Removing existing peer for this connection: %s", peer.ID)
				peerManager.RemovePeer(peer.ID)
				peer = nil
			}

			// Get Mirakurun stream
			mirakurunURL := os.Getenv("MIRAKURUN_URL")
			if mirakurunURL == "" {
				mirakurunURL = "http://tuner:40772"
			}

			client := mirakurun.NewClient(mirakurunURL)
			channels, err := client.GetChannels()
			if err != nil {
				sendError(conn, "", "Failed to get channels")
				continue
			}

			var channelType string
			var firstServiceID int64
			for _, ch := range channels {
				if ch.Channel == channelID {
					channelType = ch.Type
					if len(ch.Services) > 0 {
						firstServiceID = ch.Services[0].ID
					}
					break
				}
			}

			if channelType == "" {
				sendError(conn, "", "Channel not found")
				continue
			}

			var stream io.ReadCloser
			var streamURL string
			if firstServiceID != 0 {
				stream, err = client.GetServiceStream(firstServiceID)
				streamURL = fmt.Sprintf("%s/api/services/%d/stream", mirakurunURL, firstServiceID)
			} else {
				stream, err = client.GetChannelStreamWithType(channelType, channelID)
				streamURL = fmt.Sprintf("%s/api/channels/%s/%s/stream", mirakurunURL, channelType, channelID)
			}

			if err != nil {
				sendError(conn, "", "Failed to get stream: "+err.Error())
				continue
			}

			// Start WebRTC encoding
			session, err = encoderInstance.StartWebRTCEncoding(channelID, stream, streamURL, -1, -1)
			if err != nil {
				stream.Close()
				sendError(conn, "", "Failed to start encoding: "+err.Error())
				continue
			}

			// Create peer BEFORE adding to map
			peer, err = peerManager.CreatePeer(channelID)
			if err != nil {
				session.Stop()
				session = nil // Clear local variable on failure
				sendError(conn, "", "Failed to create peer: "+err.Error())
				continue
			}

			// Only add to map after both session and peer are successfully created
			webrtcMu.Lock()
			webrtcSessions[channelID] = session
			webrtcMu.Unlock()

			// Set up ICE candidate callback
			peer.SetOnICECandidate(func(candidate *pionwebrtc.ICECandidate) {
				if candidate == nil {
					return
				}
				candidateInit := candidate.ToJSON()
				response := webrtc.NewICECandidateMessage(peer.ID, candidateInit)
				data, _ := response.ToJSON()
				conn.WriteMessage(1, data)
			})

			// Create offer (server-side offer for push mode)
			offer, err := peer.PC.CreateOffer(nil)
			if err != nil {
				sendError(conn, peer.ID, "Failed to create offer: "+err.Error())
				continue
			}

			if err := peer.PC.SetLocalDescription(offer); err != nil {
				sendError(conn, peer.ID, "Failed to set local description: "+err.Error())
				continue
			}

			// Send offer to client
			response := webrtc.NewOfferMessage(peer.ID, offer)
			data, _ := response.ToJSON()
			conn.WriteMessage(1, data)

			// Start streaming video
			go func() {
				if err := peer.StreamH264(session.VideoPipe); err != nil {
					log.Printf("[WebRTC] Video stream ended: %v", err)
				}
			}()

			// Start streaming audio
			go func() {
				// Wait up to 5 seconds for audio pipe to be ready
				for i := 0; i < 50; i++ {
					if session.AudioPipe != nil {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
				if session.AudioPipe == nil {
					log.Printf("[WebRTC] Audio pipe not ready, skipping audio")
					return
				}
				if err := peer.StreamOpus(session.AudioPipe); err != nil {
					log.Printf("[WebRTC] Audio stream ended: %v", err)
				}
			}()

		case webrtc.MsgTypeAnswer:
			if peer == nil || msg.SDP == nil {
				continue
			}
			if err := peer.PC.SetRemoteDescription(*msg.SDP); err != nil {
				log.Printf("[WebRTC] Failed to set remote description: %v", err)
			}

		case webrtc.MsgTypeICECandidate:
			if peer == nil || msg.Candidate == nil {
				continue
			}
			if err := peer.AddICECandidate(*msg.Candidate); err != nil {
				log.Printf("[WebRTC] Failed to add ICE candidate: %v", err)
			}

		case webrtc.MsgTypeStreamStop:
			log.Printf("[WebRTC] Received stream-stop for channel %s", channelID)
			if peer != nil {
				peerManager.RemovePeer(peer.ID)
				peer = nil
			}
			if session != nil {
				session.Stop()
				webrtcMu.Lock()
				delete(webrtcSessions, channelID)
				webrtcMu.Unlock()
				session = nil
			}
			// Send stream-stopped acknowledgment
			response := &webrtc.SignalingMessage{Type: webrtc.MsgTypeStreamStopped}
			data, _ := response.ToJSON()
			conn.WriteMessage(1, data)
			log.Printf("[WebRTC] Sent stream-stopped acknowledgment for channel %s", channelID)

		case webrtc.MsgTypePing:
			response := &webrtc.SignalingMessage{Type: webrtc.MsgTypePong}
			data, _ := response.ToJSON()
			conn.WriteMessage(1, data)
		}
	}

	// Cleanup on disconnect
	if peer != nil {
		peerManager.RemovePeer(peer.ID)
	}
	if session != nil {
		session.Stop()
		webrtcMu.Lock()
		delete(webrtcSessions, channelID)
		webrtcMu.Unlock()
	}

	log.Printf("[WebRTC] WebSocket disconnected for channel %s", channelID)
}

// sendError sends an error message via WebSocket
func sendError(conn interface{ WriteMessage(int, []byte) error }, peerID, errorMsg string) {
	msg := webrtc.NewErrorMessage(peerID, errorMsg)
	data, _ := msg.ToJSON()
	conn.WriteMessage(1, data)
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

	session, err := encoderInstance.StartWebRTCEncoding(sessionID, stream, streamURL, -1, -1)
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
		if err := peer.StreamH264(session.VideoPipe); err != nil {
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
		if err := peer.StreamOpus(session.AudioPipe); err != nil {
			log.Printf("[WebRTC] Service audio stream ended: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":    "streaming",
		"peerId":    peer.ID,
		"serviceId": serviceID,
		"sessionId": session.ID,
	})
}
