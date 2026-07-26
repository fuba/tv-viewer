package nativevideo

import "testing"

func TestBobDeinterlacePreservesEachField(t *testing.T) {
	frame := YUVFrame{
		Y: []byte{10, 11, 20, 21, 30, 31, 40, 41},
		U: []byte{50, 60}, V: []byte{70, 80},
		Width: 2, Height: 4, ChromaWidth: 1, ChromaHeight: 2, Interlaced: true, TopFieldFirst: true,
	}
	frames := BobDeinterlace(frame)
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(frames))
	}
	wantTop := []byte{10, 11, 10, 11, 30, 31, 30, 31}
	wantBottom := []byte{20, 21, 20, 21, 40, 41, 40, 41}
	for i := range wantTop {
		if frames[0].Y[i] != wantTop[i] || frames[1].Y[i] != wantBottom[i] {
			t.Fatalf("unexpected luma at %d: top=%d bottom=%d", i, frames[0].Y[i], frames[1].Y[i])
		}
	}
}

func TestBobDeinterlaceHonorsBottomFieldFirst(t *testing.T) {
	frame := YUVFrame{
		Y: []byte{10, 11, 20, 21, 30, 31, 40, 41},
		U: []byte{50, 60}, V: []byte{70, 80},
		Width: 2, Height: 4, ChromaWidth: 1, ChromaHeight: 2, Interlaced: true,
	}
	frames := BobDeinterlace(frame)
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(frames))
	}
	if frames[0].Y[0] != 20 || frames[1].Y[0] != 10 {
		t.Fatalf("field order = [%d, %d], want bottom then top", frames[0].Y[0], frames[1].Y[0])
	}
}

func TestBobDeinterlaceLeavesProgressiveFrameUntouched(t *testing.T) {
	frame := YUVFrame{Y: []byte{1}, Width: 1, Height: 1}
	frames := BobDeinterlace(frame)
	if len(frames) != 1 || &frames[0].Y[0] != &frame.Y[0] {
		t.Fatal("progressive frame should be returned without copying")
	}
}
