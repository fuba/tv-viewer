package mpegts

import (
	"bytes"
	"context"
	"testing"
)

func TestDemuxerExtractsPESPayloadAndPTS(t *testing.T) {
	d := NewDemuxer()
	// Install the PMT result directly; PAT/PMT parsing is tested separately.
	d.Analyzer.Map.Streams[0x101] = Stream{PID: 0x101, StreamType: 0x02}

	pes := []byte{0, 0, 1, 0xe0, 0, 10, 0x80, 0x80, 5, 0x21, 0, 1, 0, 1, 0xaa, 0xbb}
	first := makePayloadPacket(0x101, true, pes[:10])
	packet, _ := ParsePacket(first)
	if got, err := d.Push(packet); err != nil || got != nil {
		t.Fatalf("first PES push = %v, %v", got, err)
	}
	second := makePayloadPacket(0x101, false, pes[10:])
	second[3] = second[3]&0xf0 | 1
	packet, _ = ParsePacket(second)
	packets, err := d.Push(packet)
	if err != nil || len(packets) != 1 {
		t.Fatalf("second PES push failed: %v", err)
	}
	if packets[0].PTS != 0 || len(packets[0].Payload) != 2 || packets[0].Payload[0] != 0xaa {
		t.Fatalf("unexpected streaming PES: pts=%d payload=%x", packets[0].PTS, packets[0].Payload)
	}
	third := makePayloadPacket(0x101, true, pes[:10])
	third[3] = third[3]&0xf0 | 2
	packet, _ = ParsePacket(third)
	if packets, err = d.Push(packet); err != nil || len(packets) != 0 {
		t.Fatalf("next partial PES = %d, %v", len(packets), err)
	}
}

func TestDemuxerStreamsVideoContinuationWithoutWaitingForNextPES(t *testing.T) {
	d := NewDemuxer()
	d.Analyzer.Map.Streams[0x101] = Stream{PID: 0x101, StreamType: 0x02}
	pes := []byte{0, 0, 1, 0xe0, 0, 0, 0x80, 0, 0, 0xaa}
	start, _ := ParsePacket(makePayloadPacket(0x101, true, pes))
	packets, err := d.Push(start)
	if err != nil || len(packets) != 1 || !bytes.Equal(packets[0].Payload, []byte{0xaa}) {
		t.Fatalf("start packets = %#v, %v", packets, err)
	}
	continuation, _ := ParsePacket(makePayloadPacket(0x101, false, []byte{0xbb, 0xcc}))
	continuation.ContinuityCounter = 1
	packets, err = d.Push(continuation)
	if err != nil || len(packets) != 1 || !bytes.Equal(packets[0].Payload, []byte{0xbb, 0xcc}) {
		t.Fatalf("continuation packets = %#v, %v", packets, err)
	}
}

func TestDemuxerDropsVideoUntilPUSIAfterContinuityGap(t *testing.T) {
	demuxer := NewDemuxer()
	const pid = uint16(0x101)
	demuxer.Analyzer.Map.Streams[pid] = Stream{PID: pid, StreamType: 0x02}
	pes := []byte{0, 0, 1, 0xe0, 0, 0, 0x80, 0, 0, 0xaa}

	first, err := demuxer.Push(Packet{PID: pid, PayloadUnitStart: true, Payload: pes, ContinuityCounter: 1})
	if err != nil || len(first) != 1 {
		t.Fatalf("first push = %d, %v", len(first), err)
	}
	if packets, err := demuxer.Push(Packet{PID: pid, Payload: []byte{0xbb}, ContinuityCounter: 3}); err != nil || len(packets) != 0 {
		t.Fatalf("gap push = %d, %v", len(packets), err)
	}
	if packets, err := demuxer.Push(Packet{PID: pid, Payload: []byte{0xcc}, ContinuityCounter: 4}); err != nil || len(packets) != 0 {
		t.Fatalf("damaged continuation = %d, %v", len(packets), err)
	}
	recovered, err := demuxer.Push(Packet{PID: pid, PayloadUnitStart: true, Payload: pes, ContinuityCounter: 5})
	if err != nil || len(recovered) != 1 || !recovered[0].Discontinuity {
		t.Fatalf("recovery = %+v, %v", recovered, err)
	}
}

func TestDemuxerFiltersStreamsByProgramNumber(t *testing.T) {
	d := NewDemuxerForProgram(2)
	d.Analyzer.Map.Programs[2] = 0x100
	d.Analyzer.Map.Streams[0x101] = Stream{PID: 0x101, StreamType: 0x02, ProgramNumbers: map[uint16]struct{}{1: {}}}
	d.Analyzer.Map.Streams[0x201] = Stream{PID: 0x201, StreamType: 0x02, ProgramNumbers: map[uint16]struct{}{2: {}}}
	pes := []byte{0, 0, 1, 0xe0, 0, 4, 0x80, 0, 0, 0xaa}
	var packets []PESPacket
	for _, pid := range []uint16{0x101, 0x201} {
		packet, _ := ParsePacket(makePayloadPacket(pid, true, pes))
		emitted, err := d.Push(packet)
		if err != nil {
			t.Fatalf("push PID %#x: %v", pid, err)
		}
		packets = append(packets, emitted...)
	}
	if len(packets) != 1 || packets[0].PID != 0x201 {
		t.Fatalf("filtered packets = %+v, want only PID 0x201", packets)
	}
}

func TestDemuxerSelectsFirstAvailableCandidateProgram(t *testing.T) {
	d := NewDemuxerForPrograms([]uint16{1, 2, 3})
	d.Analyzer.Map.Programs[2] = 0x102
	d.Analyzer.Map.Programs[3] = 0x103
	d.Analyzer.Map.Streams[0x201] = Stream{PID: 0x201, StreamType: 0x02, ProgramNumbers: map[uint16]struct{}{2: {}}}
	d.Analyzer.Map.Streams[0x301] = Stream{PID: 0x301, StreamType: 0x02, ProgramNumbers: map[uint16]struct{}{3: {}}}

	// Program 3 arrives first, but program 2 has higher candidate priority.
	if _, err := d.Push(Packet{PID: 0x301, PayloadUnitStart: true, Payload: []byte{0, 0, 1, 0xe0, 0, 3, 0x80, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Push(Packet{PID: 0x201, PayloadUnitStart: true, Payload: []byte{0, 0, 1, 0xe0, 0, 3, 0x80, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	if d.SelectedProgram() != 2 {
		t.Fatalf("selected program = %d, want 2", d.SelectedProgram())
	}
}

func TestDemuxerReadPESEmitsElementaryPackets(t *testing.T) {
	pat := makePATPacket(1, 0x100)
	pmt := makePMTPacket(0x100, 0x101, 0x102)
	firstPES := []byte{0, 0, 1, 0xe0, 0, 4, 0x80, 0, 0, 0xaa}
	secondPES := []byte{0, 0, 1, 0xe0, 0, 4, 0x80, 0, 0, 0xbb}
	firstPacket := makePayloadPacket(0x101, true, firstPES)
	secondPacket := makePayloadPacket(0x101, true, secondPES)
	secondPacket[3] = secondPacket[3]&0xf0 | 1
	input := append(append(pat, pmt...), firstPacket...)
	input = append(input, secondPacket...)

	d := NewDemuxer()
	var payloads [][]byte
	stats, err := d.ReadPES(context.Background(), bytes.NewReader(input), func(packet PESPacket) error {
		payloads = append(payloads, packet.Payload)
		return nil
	})
	if err != nil {
		t.Fatalf("ReadPES failed: %v", err)
	}
	if len(payloads) != 2 || string(payloads[0]) != "\xaa" || string(payloads[1]) != "\xbb" {
		t.Fatalf("unexpected PES payloads: %x stats=%+v programs=%v streams=%v", payloads, stats, d.Analyzer.Map.Programs, d.Analyzer.Map.Streams)
	}
	if stats.Packets != 4 {
		t.Fatalf("packets = %d, want 4", stats.Packets)
	}
}

func makePATPacket(program uint16, pmtPID uint16) []byte {
	section := make([]byte, 16)
	section[0] = 0x00
	section[1] = 0xb0
	section[2] = 0x0d
	section[8] = byte(program >> 8)
	section[9] = byte(program)
	section[10] = 0xe0 | byte(pmtPID>>8)
	section[11] = byte(pmtPID)
	packet := sectionPacket(PATPID, section)
	return packet.Raw[:]
}

func makePMTPacket(pmtPID, videoPID, audioPID uint16) []byte {
	section := make([]byte, 26)
	section[0] = 0x02
	section[1] = 0xb0
	section[2] = 0x17
	section[8] = 0xe0 | byte(videoPID>>8)
	section[9] = byte(videoPID)
	section[10] = 0xf0
	section[11] = 0x00
	section[12] = 0x02
	section[13] = 0xe0 | byte(videoPID>>8)
	section[14] = byte(videoPID)
	section[15] = 0xf0
	section[16] = 0x00
	section[17] = 0x0f
	section[18] = 0xe0 | byte(audioPID>>8)
	section[19] = byte(audioPID)
	section[20] = 0xf0
	section[21] = 0x00
	packet := sectionPacket(pmtPID, section)
	return packet.Raw[:]
}

func makePayloadPacket(pid uint16, start bool, payload []byte) []byte {
	p := make([]byte, PacketSize)
	p[0] = SyncByte
	p[1] = byte(pid >> 8)
	if start {
		p[1] |= 0x40
	}
	p[2] = byte(pid)
	p[3] = 0x30 // adaptation field and payload
	adaptationLength := PacketSize - 5 - len(payload)
	p[4] = byte(adaptationLength)
	copy(p[5+adaptationLength:], payload)
	return p
}
