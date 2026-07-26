package mpegts

import (
	"encoding/binary"
	"testing"
)

func sectionPacket(pid uint16, section []byte) Packet {
	raw := make([]byte, PacketSize)
	raw[0] = SyncByte
	raw[1] = byte(pid>>8) | 0x40
	raw[2] = byte(pid)
	raw[3] = 0x10
	raw[4] = 0
	copy(raw[5:], section)
	p, _ := ParsePacket(raw)
	return p
}

func TestAnalyzerParsesPATAndPMT(t *testing.T) {
	pat := make([]byte, 16)
	pat[0] = 0x00
	binary.BigEndian.PutUint16(pat[1:3], 13) // section length
	binary.BigEndian.PutUint16(pat[8:10], 1)
	binary.BigEndian.PutUint16(pat[10:12], 0xe100|0x0100)
	a := NewAnalyzer()
	a.Push(sectionPacket(PATPID, pat))
	if got := a.Map.Programs[1]; got != 0x0100 {
		t.Fatalf("PMT PID = %#x, want %#x", got, 0x0100)
	}

	pmt := make([]byte, 21)
	pmt[0] = 0x02
	binary.BigEndian.PutUint16(pmt[1:3], 18)
	binary.BigEndian.PutUint16(pmt[10:12], 0)
	pmt[12] = 0x02
	binary.BigEndian.PutUint16(pmt[13:15], 0xe101)
	binary.BigEndian.PutUint16(pmt[15:17], 0)
	a.Push(sectionPacket(0x0100, pmt))
	if got := a.Map.Streams[0x0101].StreamType; got != 0x02 {
		t.Fatalf("stream type = %#x, want MPEG-2 video", got)
	}
	if !a.Map.Streams[0x0101].HasProgram(1) {
		t.Fatal("stream does not belong to program 1")
	}
}

func TestAnalyzerRetainsSharedStreamPrograms(t *testing.T) {
	a := NewAnalyzer()

	pat1 := make([]byte, 16)
	pat1[0] = 0x00
	binary.BigEndian.PutUint16(pat1[1:3], 13)
	binary.BigEndian.PutUint16(pat1[8:10], 1)
	binary.BigEndian.PutUint16(pat1[10:12], 0xe100)
	a.Push(sectionPacket(PATPID, pat1))

	pat2 := append([]byte(nil), pat1...)
	binary.BigEndian.PutUint16(pat2[8:10], 2)
	binary.BigEndian.PutUint16(pat2[10:12], 0xe200)
	a.Push(sectionPacket(PATPID, pat2))

	pmt := make([]byte, 21)
	pmt[0] = 0x02
	binary.BigEndian.PutUint16(pmt[1:3], 18)
	pmt[12] = 0x02
	binary.BigEndian.PutUint16(pmt[13:15], 0xe300)
	a.Push(sectionPacket(0x0100, pmt))
	a.Push(sectionPacket(0x0200, pmt))

	stream := a.Map.Streams[0x0300]
	if !stream.HasProgram(1) || !stream.HasProgram(2) {
		t.Fatalf("shared stream programs = %v, want 1 and 2", stream.ProgramNumbers)
	}
}

func TestAnalyzerReplacesProgramStreamsOnPMTUpdate(t *testing.T) {
	a := NewAnalyzer()
	pat := make([]byte, 16)
	pat[0] = 0x00
	binary.BigEndian.PutUint16(pat[1:3], 13)
	binary.BigEndian.PutUint16(pat[8:10], 1)
	binary.BigEndian.PutUint16(pat[10:12], 0xe100)
	a.Push(sectionPacket(PATPID, pat))

	pmt := make([]byte, 21)
	pmt[0] = 0x02
	binary.BigEndian.PutUint16(pmt[1:3], 18)
	pmt[12] = 0x02
	binary.BigEndian.PutUint16(pmt[13:15], 0xe200)
	a.Push(sectionPacket(0x0100, pmt))

	binary.BigEndian.PutUint16(pmt[13:15], 0xe201)
	a.Push(sectionPacket(0x0100, pmt))
	if _, exists := a.Map.Streams[0x0200]; exists {
		t.Fatal("obsolete stream PID remains after PMT update")
	}
	if !a.Map.Streams[0x0201].HasProgram(1) {
		t.Fatal("updated stream PID does not belong to program 1")
	}
}
