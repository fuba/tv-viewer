package webrtc

import (
	"encoding/json"

	"github.com/pion/webrtc/v4"
)

// Message types for WebRTC signaling
const (
	MsgTypeOffer             = "offer"
	MsgTypeAnswer            = "answer"
	MsgTypeICECandidate      = "ice-candidate"
	MsgTypeStreamStart       = "stream-start"
	MsgTypeStreamStop        = "stream-stop"
	MsgTypeStreamStopped     = "stream-stopped"     // Response to stream-stop
	MsgTypeRestartEncoding   = "restart-encoding"   // Restart encoding with new settings (keeps WebRTC connection)
	MsgTypeEncodingRestarted = "encoding-restarted" // Response to restart-encoding
	MsgTypeReady             = "ready"
	MsgTypeError             = "error"
	MsgTypePing              = "ping"
	MsgTypePong              = "pong"
)

// SignalingMessage represents a WebRTC signaling message
type SignalingMessage struct {
	Type               string                     `json:"type"`
	PeerID             string                     `json:"peerId,omitempty"`
	ChannelID          string                     `json:"channelId,omitempty"`
	RequestID          string                     `json:"requestId,omitempty"`
	SDP                *webrtc.SessionDescription `json:"sdp,omitempty"`
	Candidate          *webrtc.ICECandidateInit   `json:"candidate,omitempty"`
	Error              string                     `json:"error,omitempty"`
	Timestamp          int64                      `json:"timestamp,omitempty"`
	BurnInSubtitles    *bool                      `json:"burnInSubtitles,omitempty"`    // If true, ARIB captions are sent over the data channel
	AudioMode          *string                    `json:"audioMode,omitempty"`          // Dual mono mode: "main", "sub", or "both"
	TranslationEnabled *bool                      `json:"translationEnabled,omitempty"` // If true, replace audio and captions with Japanese translation
}

// SubtitleMessage represents a subtitle event sent via DataChannel
type SubtitleMessage struct {
	Type      string  `json:"type"` // "show", "hide", "clear"
	ID        string  `json:"id,omitempty"`
	Text      string  `json:"text,omitempty"`
	StartTime float64 `json:"startTime,omitempty"`
	EndTime   float64 `json:"endTime,omitempty"`
	Style     string  `json:"style,omitempty"`
}

// ParseSignalingMessage parses a JSON message into SignalingMessage
func ParseSignalingMessage(data []byte) (*SignalingMessage, error) {
	var msg SignalingMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ToJSON serializes a SignalingMessage to JSON
func (m *SignalingMessage) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// NewOfferMessage creates a new offer message
func NewOfferMessage(peerID string, sdp webrtc.SessionDescription) *SignalingMessage {
	return &SignalingMessage{
		Type:   MsgTypeOffer,
		PeerID: peerID,
		SDP:    &sdp,
	}
}

// NewAnswerMessage creates a new answer message
func NewAnswerMessage(peerID string, sdp webrtc.SessionDescription) *SignalingMessage {
	return &SignalingMessage{
		Type:   MsgTypeAnswer,
		PeerID: peerID,
		SDP:    &sdp,
	}
}

// NewICECandidateMessage creates a new ICE candidate message
func NewICECandidateMessage(peerID string, candidate webrtc.ICECandidateInit) *SignalingMessage {
	return &SignalingMessage{
		Type:      MsgTypeICECandidate,
		PeerID:    peerID,
		Candidate: &candidate,
	}
}

// NewReadyMessage creates a new ready message indicating stream is ready
func NewReadyMessage(peerID, channelID string) *SignalingMessage {
	return &SignalingMessage{
		Type:      MsgTypeReady,
		PeerID:    peerID,
		ChannelID: channelID,
	}
}

// NewErrorMessage creates a new error message
func NewErrorMessage(peerID, errorMsg string) *SignalingMessage {
	return &SignalingMessage{
		Type:   MsgTypeError,
		PeerID: peerID,
		Error:  errorMsg,
	}
}
