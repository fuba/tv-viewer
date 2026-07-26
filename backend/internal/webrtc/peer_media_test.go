package webrtc

import (
	"strings"
	"testing"
)

func TestPeerAdvertisesAudioAndVideoInSameMediaStream(t *testing.T) {
	manager := NewPeerManager()
	peer, err := manager.CreatePeer("sync-test")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.RemovePeer(peer.ID)

	offer, err := peer.PC.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(offer.SDP, "a=msid:tv-stream-sync-test "); got != 2 {
		t.Fatalf("shared MediaStream msid count = %d, want 2\n%s", got, offer.SDP)
	}
	if peer.VideoSender == nil || peer.AudioSender == nil {
		t.Fatal("peer did not retain RTP senders for RTCP feedback")
	}
}
