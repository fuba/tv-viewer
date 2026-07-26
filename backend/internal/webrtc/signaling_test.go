package webrtc

import "testing"

func TestSignalingRequestIDRoundTrip(t *testing.T) {
	message := &SignalingMessage{
		Type:      MsgTypeEncodingRestarted,
		ChannelID: "service:GR:27:3273601024",
		RequestID: "request-1",
	}
	data, err := message.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseSignalingMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.RequestID != message.RequestID {
		t.Fatalf("request ID = %q, want %q", parsed.RequestID, message.RequestID)
	}
}
