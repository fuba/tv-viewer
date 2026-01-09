package wsmonitor

// ClientMessage represents a message from client to server
type ClientMessage struct {
	Type      string `json:"type"`      // "ping", "status"
	SessionID string `json:"sessionId"` // Associated session ID
	Timestamp int64  `json:"timestamp"` // Client-side timestamp
}

// ServerMessage represents a message from server to client
type ServerMessage struct {
	Type      string `json:"type"`                // "pong", "session_status", "error"
	SessionID string `json:"sessionId,omitempty"`
	Status    string `json:"status,omitempty"`  // "active", "stopping", "stopped"
	Message   string `json:"message,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// Message types constants
const (
	MsgTypePing          = "ping"
	MsgTypePong          = "pong"
	MsgTypeStatus        = "status"
	MsgTypeSessionStatus = "session_status"
	MsgTypeError         = "error"
)
