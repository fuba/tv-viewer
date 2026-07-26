package webrtc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

// TestH264Streaming tests H.264 streaming between two peers
func TestH264Streaming(t *testing.T) {
	if os.Getenv("RUN_WEBRTC_INTEGRATION") != "1" {
		t.Skip("set RUN_WEBRTC_INTEGRATION=1 to run the local ICE integration test")
	}
	// Create sender and receiver peer connections
	sender, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create sender: %v", err)
	}
	defer sender.Close()

	receiver, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create receiver: %v", err)
	}
	defer receiver.Close()

	// Create video track on sender
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		"video",
		"test-video",
	)
	if err != nil {
		t.Fatalf("Failed to create video track: %v", err)
	}

	rtpSender, err := sender.AddTrack(videoTrack)
	if err != nil {
		t.Fatalf("Failed to add track: %v", err)
	}

	// Read RTCP packets (required for WebRTC)
	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, err := rtpSender.Read(buf); err != nil {
				return
			}
		}
	}()

	// Track received on receiver
	var wg sync.WaitGroup
	wg.Add(1)
	receivedFrames := 0
	var receivedMu sync.Mutex

	receiver.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		t.Logf("Received track: %s, codec: %s", track.Kind(), track.Codec().MimeType)

		defer wg.Done()

		buf := make([]byte, 1500)
		for i := 0; i < 100; i++ { // Read 100 packets
			n, _, err := track.Read(buf)
			if err != nil {
				t.Logf("Track read error: %v", err)
				return
			}
			receivedMu.Lock()
			receivedFrames++
			if receivedFrames <= 5 {
				t.Logf("Received RTP packet %d: %d bytes", receivedFrames, n)
			}
			receivedMu.Unlock()
		}
	})

	// Signal exchange
	senderConnected := make(chan struct{})
	sender.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		t.Logf("Sender ICE state: %s", state)
		if state == webrtc.ICEConnectionStateConnected {
			select {
			case <-senderConnected:
			default:
				close(senderConnected)
			}
		}
	})

	offer, err := sender.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}
	senderGatheringComplete := webrtc.GatheringCompletePromise(sender)
	if err := sender.SetLocalDescription(offer); err != nil {
		t.Fatalf("Failed to set sender local description: %v", err)
	}
	<-senderGatheringComplete

	if err := receiver.SetRemoteDescription(*sender.LocalDescription()); err != nil {
		t.Fatalf("Failed to set receiver remote description: %v", err)
	}

	answer, err := receiver.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("Failed to create answer: %v", err)
	}
	receiverGatheringComplete := webrtc.GatheringCompletePromise(receiver)
	if err := receiver.SetLocalDescription(answer); err != nil {
		t.Fatalf("Failed to set receiver local description: %v", err)
	}
	<-receiverGatheringComplete

	if err := sender.SetRemoteDescription(*receiver.LocalDescription()); err != nil {
		t.Fatalf("Failed to set sender remote description: %v", err)
	}

	// Wait for ICE connection
	select {
	case <-senderConnected:
		t.Log("Sender connected")
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for sender connection")
	}

	// Generate test H.264 data (SPS, PPS, IDR frame)
	// These are minimal valid NAL units for testing
	sps := []byte{0x67, 0x42, 0xc0, 0x1f, 0xda, 0x01, 0x40, 0x16, 0xec, 0x04, 0x40, 0x00, 0x00, 0x03, 0x00, 0x40, 0x00, 0x00, 0x0f, 0x03, 0xc5, 0x8b, 0x92, 0x80}
	pps := []byte{0x68, 0xce, 0x3c, 0x80}

	// Create a simple IDR frame (this is a minimal valid IDR NAL)
	idr := make([]byte, 1000)
	idr[0] = 0x65 // IDR NAL type
	for i := 1; i < len(idr); i++ {
		idr[i] = byte(i % 256)
	}

	// Use our RTP packetizer
	packetizer := NewRTPPacketizer(12345, 67890)

	// Send SPS
	spsNAL := NALUnit{Type: 7, Data: sps}
	packets, err := packetizer.PacketizeH264([]NALUnit{spsNAL})
	if err != nil {
		t.Fatalf("Failed to packetize SPS: %v", err)
	}
	for _, pkt := range packets {
		if err := videoTrack.WriteRTP(pkt); err != nil {
			t.Fatalf("Failed to write SPS RTP: %v", err)
		}
	}
	t.Logf("Sent SPS: %d packets", len(packets))

	// Send PPS
	ppsNAL := NALUnit{Type: 8, Data: pps}
	packets, err = packetizer.PacketizeH264([]NALUnit{ppsNAL})
	if err != nil {
		t.Fatalf("Failed to packetize PPS: %v", err)
	}
	for _, pkt := range packets {
		if err := videoTrack.WriteRTP(pkt); err != nil {
			t.Fatalf("Failed to write PPS RTP: %v", err)
		}
	}
	t.Logf("Sent PPS: %d packets", len(packets))

	// Send multiple IDR frames
	for frame := 0; frame < 30; frame++ {
		idrNAL := NALUnit{Type: 5, Data: idr}
		packets, err = packetizer.PacketizeH264([]NALUnit{idrNAL})
		if err != nil {
			t.Fatalf("Failed to packetize IDR: %v", err)
		}
		for _, pkt := range packets {
			if err := videoTrack.WriteRTP(pkt); err != nil {
				t.Fatalf("Failed to write IDR RTP: %v", err)
			}
		}
		if frame < 3 {
			t.Logf("Sent IDR frame %d: %d packets", frame, len(packets))
		}
		time.Sleep(33 * time.Millisecond) // ~30fps
	}

	// Wait for receiver to get packets
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("Received %d frames total", receivedFrames)
	case <-time.After(5 * time.Second):
		receivedMu.Lock()
		frames := receivedFrames
		receivedMu.Unlock()
		if frames > 0 {
			t.Logf("Received %d frames (timeout but got some data)", frames)
		} else {
			t.Fatal("Timeout waiting for frames")
		}
	}

	if receivedFrames == 0 {
		t.Fatal("No frames received")
	}
}

// TestH264ParserWithRealData tests H264 parser with Annex B stream
func TestH264ParserWithRealData(t *testing.T) {
	// Simulated Annex B stream: start code + SPS + start code + PPS + start code + IDR
	annexB := bytes.Buffer{}

	// SPS
	annexB.Write([]byte{0x00, 0x00, 0x00, 0x01}) // 4-byte start code
	sps := []byte{0x67, 0x42, 0xc0, 0x1f, 0xda, 0x01, 0x40, 0x16}
	annexB.Write(sps)

	// PPS
	annexB.Write([]byte{0x00, 0x00, 0x00, 0x01})
	pps := []byte{0x68, 0xce, 0x3c, 0x80}
	annexB.Write(pps)

	// IDR
	annexB.Write([]byte{0x00, 0x00, 0x00, 0x01})
	idr := []byte{0x65, 0x88, 0x84, 0x00, 0x0a, 0xff, 0xff}
	annexB.Write(idr)

	// Add another start code to mark end
	annexB.Write([]byte{0x00, 0x00, 0x00, 0x01})

	parser := NewH264Parser()
	nalUnits, err := parser.Parse(annexB.Bytes())
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	t.Logf("Parsed %d NAL units", len(nalUnits))
	for i, nal := range nalUnits {
		t.Logf("NAL %d: type=%d, size=%d", i, nal.Type, len(nal.Data))
	}

	if len(nalUnits) != 3 {
		t.Fatalf("Expected 3 NAL units, got %d", len(nalUnits))
	}

	if nalUnits[0].Type != 7 {
		t.Errorf("First NAL should be SPS (7), got %d", nalUnits[0].Type)
	}
	if nalUnits[1].Type != 8 {
		t.Errorf("Second NAL should be PPS (8), got %d", nalUnits[1].Type)
	}
	if nalUnits[2].Type != 5 {
		t.Errorf("Third NAL should be IDR (5), got %d", nalUnits[2].Type)
	}
}

func TestNativeVideoAccessUnitPreservesFrameBoundary(t *testing.T) {
	stream := NewSharedStream()
	data := []byte{0, 0, 0, 1, 0x67, 0x42, 0x00, 0x1f, 0, 0, 0, 1, 0x68, 0xce, 0x00, 0x1f, 0, 0, 0, 1, 0x65, 0x88, 0x84}
	stream.publishVideoAccessUnit(data, 40*time.Millisecond, 0, false)
	stream.bootstrapMu.Lock()
	defer stream.bootstrapMu.Unlock()
	if len(stream.bootstrap) != 3 {
		t.Fatalf("bootstrap entries = %d, want SPS/PPS/IDR", len(stream.bootstrap))
	}
	if stream.bootstrap[2].duration != 40*time.Millisecond {
		t.Fatalf("IDR duration = %s, want 40ms", stream.bootstrap[2].duration)
	}
}

func TestH264ParserRejectsOversizedUnframedInput(t *testing.T) {
	parser := NewH264Parser()
	if _, err := parser.Parse(make([]byte, maxH264ParserBuffer+1)); !errors.Is(err, ErrH264BufferTooLarge) {
		t.Fatalf("Parse error = %v, want %v", err, ErrH264BufferTooLarge)
	}
	if len(parser.buffer) != 0 {
		t.Fatalf("parser retained %d bytes after rejecting input", len(parser.buffer))
	}
}

func TestRunSubtitlesRawPreservesJSONMessage(t *testing.T) {
	stream := NewSharedStream()
	sub := &sharedSubscriber{
		peer:         &Peer{ID: "test"},
		subtitleJSON: make(chan []byte, 1),
		done:         make(chan struct{}),
	}
	stream.subscribers["test"] = sub
	message := []byte(`{"type":"show","id":"1","text":"字幕"}`)
	var framed bytes.Buffer
	if err := binary.Write(&framed, binary.BigEndian, uint32(len(message))); err != nil {
		t.Fatal(err)
	}
	framed.Write(message)
	if err := stream.RunSubtitlesRaw(&framed); !errors.Is(err, io.EOF) {
		t.Fatalf("RunSubtitlesRaw error = %v, want EOF", err)
	}
	select {
	case got := <-sub.subtitleJSON:
		if !bytes.Equal(got, message) {
			t.Fatalf("subtitle = %s, want %s", got, message)
		}
	default:
		t.Fatal("subtitle was not delivered")
	}
}

func TestSharedStreamNotifiesWhenPeerIsRemoved(t *testing.T) {
	stream := NewSharedStream()
	removed := make(chan string, 1)
	stream.SetOnPeerRemoved(func(peerID string) { removed <- peerID })

	// A minimal subscriber is sufficient because this test exercises removal only.
	reader, writer := io.Pipe()
	sub := &sharedSubscriber{
		peer:          &Peer{ID: "peer-1"},
		subtitleRead:  reader,
		subtitleWrite: writer,
		done:          make(chan struct{}),
	}
	stream.subscribers[sub.peer.ID] = sub
	stream.RemovePeer(sub.peer.ID)

	select {
	case peerID := <-removed:
		if peerID != sub.peer.ID {
			t.Fatalf("removed peer = %q, want %q", peerID, sub.peer.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("peer removal callback was not called")
	}
}

// TestRTPPacketizer tests RTP packetization
func TestRTPPacketizer(t *testing.T) {
	packetizer := NewRTPPacketizer(12345, 67890)

	// Small NAL (fits in single packet)
	smallNAL := NALUnit{Type: 7, Data: []byte{0x67, 0x42, 0xc0, 0x1f}}
	packets, err := packetizer.PacketizeH264([]NALUnit{smallNAL})
	if err != nil {
		t.Fatalf("Packetize error: %v", err)
	}
	t.Logf("Small NAL: %d packets", len(packets))
	if len(packets) != 1 {
		t.Errorf("Expected 1 packet for small NAL, got %d", len(packets))
	}

	// Large NAL (needs fragmentation)
	largeData := make([]byte, 5000)
	largeData[0] = 0x65 // IDR
	for i := 1; i < len(largeData); i++ {
		largeData[i] = byte(i % 256)
	}
	largeNAL := NALUnit{Type: 5, Data: largeData}
	packets, err = packetizer.PacketizeH264([]NALUnit{largeNAL})
	if err != nil {
		t.Fatalf("Packetize error: %v", err)
	}
	t.Logf("Large NAL (%d bytes): %d packets", len(largeData), len(packets))
	if len(packets) < 2 {
		t.Errorf("Expected multiple packets for large NAL, got %d", len(packets))
	}

	// Check marker bit on last packet
	if !packets[len(packets)-1].Marker {
		t.Error("Last packet should have marker bit set")
	}

	// Check sequence numbers are sequential
	for i := 1; i < len(packets); i++ {
		if packets[i].SequenceNumber != packets[i-1].SequenceNumber+1 {
			t.Errorf("Sequence numbers not sequential: %d -> %d",
				packets[i-1].SequenceNumber, packets[i].SequenceNumber)
		}
	}
}
