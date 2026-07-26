package mpegts

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func makePacket(pid uint16, payloadStart bool, payload byte) []byte {
	p := make([]byte, PacketSize)
	p[0] = SyncByte
	p[1] = byte(pid >> 8)
	if payloadStart {
		p[1] |= 0x40
	}
	p[2] = byte(pid)
	p[3] = 0x10
	for i := 4; i < len(p); i++ {
		p[i] = payload
	}
	return p
}

func TestReaderResynchronizesAndPreservesPackets(t *testing.T) {
	first := makePacket(0x100, true, 0xaa)
	second := makePacket(0x101, false, 0xbb)
	input := append([]byte{0x01, 0x02, 0x03}, first...)
	input = append(input, second...)

	r := NewReader(bytes.NewReader(input))
	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !bytes.Equal(output, append(first, second...)) {
		t.Fatalf("reader changed valid packets")
	}
	if got := r.Stats().ResyncBytes; got != 3 {
		t.Fatalf("resync bytes = %d, want 3", got)
	}
}

func TestReaderDropsNullAndTransportErrorPackets(t *testing.T) {
	null := makePacket(NullPID, false, 0)
	errorPacket := makePacket(0x120, false, 0)
	errorPacket[1] |= 0x80
	valid := makePacket(0x121, false, 0xcc)

	r := NewReader(bytes.NewReader(append(append(null, errorPacket...), valid...)))
	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !bytes.Equal(output, valid) {
		t.Fatalf("unexpected output length/content: %d", len(output))
	}
	stats := r.Stats()
	if stats.NullPackets != 1 || stats.TransportErrors != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestReadAllPacketsStopsAtEOF(t *testing.T) {
	data := append(makePacket(0x100, false, 1), makePacket(0x101, false, 2)...)
	packets, _, err := ReadAllPackets(data)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("read all failed: %v", err)
	}
	if len(packets) != 2 {
		t.Fatalf("packets = %d, want 2", len(packets))
	}
}
