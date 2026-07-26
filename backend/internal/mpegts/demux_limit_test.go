package mpegts

import (
	"errors"
	"testing"
)

func TestDemuxerLimitsUnterminatedPES(t *testing.T) {
	d := NewDemuxer()
	d.Analyzer.Map.Streams[0x101] = Stream{PID: 0x101, StreamType: 0x0f}
	packetBytes := make([]byte, PacketSize)
	packetBytes[0] = SyncByte
	packetBytes[1] = 0x01
	packetBytes[2] = 0x01
	packetBytes[3] = 0x11
	for i := 4; i < len(packetBytes); i++ {
		packetBytes[i] = 0xaa
	}
	packet, _ := ParsePacket(packetBytes)
	packet.PayloadUnitStart = true
	if _, err := d.Push(packet); err != nil {
		t.Fatalf("initial PES push failed: %v", err)
	}
	packet.PayloadUnitStart = false
	for i := 0; i <= maxPESPayloadSize/len(packet.Payload); i++ {
		packet.ContinuityCounter = byte(i+1) & 0x0f
		if _, err := d.Push(packet); err != nil {
			if !errors.Is(err, ErrPESPayloadTooLarge) {
				t.Fatalf("unexpected error: %v", err)
			}
			return
		}
	}
	t.Fatal("unterminated PES was not limited")
}
