package mpegts

import (
	"bytes"
	"testing"
)

func TestMPEG2VideoAssemblerFindsPictureBoundaries(t *testing.T) {
	assembler := NewMPEG2VideoAssembler()
	first := PESPacket{Payload: []byte{0, 0, 1, 0xb3, 0x11, 0x22, 0, 0, 1, 0, 0x00, 0x18, 0xaa}}
	second := PESPacket{Payload: []byte{0xbb, 0, 0, 1, 0, 0x00, 0x38, 0xcc}}

	frames, err := assembler.Push(first)
	if err != nil || len(frames) != 0 {
		t.Fatalf("first push = %d, %v", len(frames), err)
	}
	frames, err = assembler.Push(second)
	if err != nil || len(frames) != 1 {
		t.Fatalf("second push = %d, %v", len(frames), err)
	}
	wantFirst := append(append([]byte(nil), first.Payload...), second.Payload[0])
	if !bytes.Equal(frames[0].Data, wantFirst) || frames[0].PictureType != 3 {
		t.Fatalf("unexpected first frame: type=%d data=%x", frames[0].PictureType, frames[0].Data)
	}
	flushed := assembler.Flush()
	if len(flushed) != 1 || !bytes.HasSuffix(flushed[0].Data, []byte{0xcc}) {
		t.Fatalf("unexpected flushed frame: %+v", flushed)
	}
}

func TestMPEG2VideoAssemblerEmitsEveryPictureInOnePayload(t *testing.T) {
	assembler := NewMPEG2VideoAssembler()
	picture := func(marker byte) []byte { return []byte{0, 0, 1, 0, 0, 0x08, marker} }
	payload := append(append(append([]byte{}, picture(1)...), picture(2)...), picture(3)...)

	frames, err := assembler.Push(PESPacket{Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(frames))
	}
	if frames[0].Data[len(frames[0].Data)-1] != 1 || frames[1].Data[len(frames[1].Data)-1] != 2 {
		t.Fatalf("picture boundaries were merged: %x / %x", frames[0].Data, frames[1].Data)
	}
}

func TestMPEG2VideoAssemblerPreservesHeadersBeforeFirstPicture(t *testing.T) {
	assembler := NewMPEG2VideoAssembler()
	header := []byte{0, 0, 1, 0xb3, 0x11, 0x22, 0x33, 0x44}
	frames, err := assembler.Push(PESPacket{Payload: header})
	if err != nil || len(frames) != 0 {
		t.Fatalf("header push = %d, %v", len(frames), err)
	}
	firstPicture := []byte{0, 0, 1, 0, 0, 0x08, 0xaa}
	secondPicture := []byte{0, 0, 1, 0, 0, 0x10, 0xbb}
	frames, err = assembler.Push(PESPacket{Payload: append(firstPicture, secondPicture...)})
	if err != nil || len(frames) != 1 {
		t.Fatalf("picture push = %d, %v", len(frames), err)
	}
	if !bytes.HasPrefix(frames[0].Data, header) {
		t.Fatalf("sequence header was discarded: %x", frames[0].Data)
	}
}

func TestMPEG2VideoAssemblerKeepsPTSWithPictureStart(t *testing.T) {
	assembler := NewMPEG2VideoAssembler()
	picture := func(marker byte) []byte { return []byte{0, 0, 1, 0, 0, 0x08, marker} }
	first := PESPacket{Payload: picture(1), PTS: 90_000, HasPTS: true}
	second := PESPacket{Payload: picture(2), PTS: 93_003, HasPTS: true}
	third := PESPacket{Payload: picture(3), PTS: 96_006, HasPTS: true}

	if frames, err := assembler.Push(first); err != nil || len(frames) != 0 {
		t.Fatalf("first push = %d, %v", len(frames), err)
	}
	frames, err := assembler.Push(second)
	if err != nil || len(frames) != 1 {
		t.Fatalf("second push = %d, %v", len(frames), err)
	}
	if !frames[0].HasPTS || frames[0].PTS != first.PTS {
		t.Fatalf("first picture PTS = %d, %v, want %d", frames[0].PTS, frames[0].HasPTS, first.PTS)
	}
	frames, err = assembler.Push(third)
	if err != nil || len(frames) != 1 {
		t.Fatalf("third push = %d, %v", len(frames), err)
	}
	if !frames[0].HasPTS || frames[0].PTS != second.PTS {
		t.Fatalf("second picture PTS = %d, %v, want %d", frames[0].PTS, frames[0].HasPTS, second.PTS)
	}
}

func TestAACADTSAssemblerHandlesSplitFrames(t *testing.T) {
	frame1 := BuildADTSFrame([]byte{1, 2, 3}, 3, 2)
	frame2 := BuildADTSFrame([]byte{4, 5}, 3, 2)
	assembler := NewAACADTSAssembler()

	frames, err := assembler.Push(PESPacket{Payload: frame1[:5]})
	if err != nil || len(frames) != 0 {
		t.Fatalf("split header push = %d, %v", len(frames), err)
	}
	frames, err = assembler.Push(PESPacket{Payload: append(frame1[5:], frame2...)})
	if err != nil || len(frames) != 2 {
		t.Fatalf("complete push = %d, %v", len(frames), err)
	}
	if frames[0].SampleRate != 48000 || frames[0].Channels != 2 || frames[0].Samples != 1024 {
		t.Fatalf("unexpected AAC metadata: %+v", frames[0])
	}
}

func TestAACADTSAssemblerPreservesPTSForSplitFrame(t *testing.T) {
	frame := BuildADTSFrame([]byte{1, 2, 3}, 3, 2)
	assembler := NewAACADTSAssembler()

	frames, err := assembler.Push(PESPacket{Payload: frame[:5], PTS: 90_000, HasPTS: true})
	if err != nil || len(frames) != 0 {
		t.Fatalf("split push = %d, %v", len(frames), err)
	}
	frames, err = assembler.Push(PESPacket{Payload: frame[5:]})
	if err != nil || len(frames) != 1 {
		t.Fatalf("completion push = %d, %v", len(frames), err)
	}
	if !frames[0].HasPTS || frames[0].PTS != 90_000 {
		t.Fatalf("split frame PTS = %d, %v, want 90000", frames[0].PTS, frames[0].HasPTS)
	}
}

func TestAACADTSAssemblerAdvancesPTSForFramesInOnePES(t *testing.T) {
	frame1 := BuildADTSFrame([]byte{1, 2, 3}, 3, 2)
	frame2 := BuildADTSFrame([]byte{4, 5}, 3, 2)
	assembler := NewAACADTSAssembler()

	frames, err := assembler.Push(PESPacket{
		Payload: append(frame1, frame2...), PTS: 90_000, HasPTS: true,
	})
	if err != nil || len(frames) != 2 {
		t.Fatalf("push = %d, %v", len(frames), err)
	}
	if frames[0].PTS != 90_000 || frames[1].PTS != 91_920 ||
		!frames[0].HasPTS || !frames[1].HasPTS {
		t.Fatalf("frame PTS values = (%d, %v), (%d, %v)",
			frames[0].PTS, frames[0].HasPTS, frames[1].PTS, frames[1].HasPTS)
	}
}
