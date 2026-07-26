package nativevideo

import "fmt"

// H264Frame is one Annex-B H.264 access unit.
type H264Frame struct {
	Data     []byte
	Keyframe bool
}

func validateYUV420Frame(frame YUVFrame, width, height int) error {
	chromaWidth, chromaHeight := width/2, height/2
	if width <= 0 || height <= 0 || width&1 != 0 || height&1 != 0 ||
		frame.Width != width || frame.Height != height ||
		frame.ChromaWidth != chromaWidth || frame.ChromaHeight != chromaHeight ||
		len(frame.Y) < width*height || len(frame.U) < chromaWidth*chromaHeight || len(frame.V) < chromaWidth*chromaHeight {
		return fmt.Errorf("invalid YUV420 frame: %dx%d chroma=%dx%d for encoder %dx%d", frame.Width, frame.Height, frame.ChromaWidth, frame.ChromaHeight, width, height)
	}
	return nil
}
