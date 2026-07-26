package nativevideo

import "testing"

func TestValidateYUV420FrameRejectsMismatchedLumaDimensions(t *testing.T) {
	frame := YUVFrame{
		Y: make([]byte, 1), U: make([]byte, 720*540), V: make([]byte, 720*540),
		Width: 1, Height: 1, ChromaWidth: 720, ChromaHeight: 540,
	}
	if err := validateYUV420Frame(frame, 1440, 1080); err == nil {
		t.Fatal("mismatched luma dimensions were accepted")
	}
}

func TestValidateYUV420FrameAcceptsExactFrame(t *testing.T) {
	frame := YUVFrame{
		Y: make([]byte, 16), U: make([]byte, 4), V: make([]byte, 4),
		Width: 4, Height: 4, ChromaWidth: 2, ChromaHeight: 2,
	}
	if err := validateYUV420Frame(frame, 4, 4); err != nil {
		t.Fatalf("exact frame rejected: %v", err)
	}
}
