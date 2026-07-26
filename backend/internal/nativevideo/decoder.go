package nativevideo

import "errors"

var ErrUnavailable = errors.New("native MPEG-2 decoder is unavailable")

// YUVFrame is a decoded planar 4:2:0 frame.
type YUVFrame struct {
	Y                 []byte
	U                 []byte
	V                 []byte
	Width             int
	Height            int
	ChromaWidth       int
	ChromaHeight      int
	PictureType       byte
	Interlaced        bool
	TopFieldFirst     bool
	TemporalReference uint16
	// Deinterlaced reports that this progressive output represents one source field.
	Deinterlaced bool
}

// GPUFrame identifies one NVDEC output surface that remains on the GPU until
// it is synchronously consumed by NVENC.
type GPUFrame struct {
	Slot         int
	Width        int
	Height       int
	Deinterlaced bool
	FieldIndex   int
	FieldCount   int
}

// BobDeinterlace creates two progressive frames from one interlaced frame.
func BobDeinterlace(frame YUVFrame) []YUVFrame {
	return new(BobDeinterlacer).Frames(frame)
}

// BobDeinterlacer reuses output buffers across frames to avoid live-stream GC pressure.
type BobDeinterlacer struct {
	frames [2]YUVFrame
}

func (d *BobDeinterlacer) Frames(frame YUVFrame) []YUVFrame {
	if !frame.Interlaced {
		return []YUVFrame{frame}
	}
	for outputIndex := 0; outputIndex < 2; outputIndex++ {
		field := outputIndex
		if !frame.TopFieldFirst {
			field = 1 - outputIndex
		}
		out := &d.frames[outputIndex]
		if len(out.Y) != len(frame.Y) {
			out.Y = make([]byte, len(frame.Y))
		}
		if len(out.U) != len(frame.U) {
			out.U = make([]byte, len(frame.U))
		}
		if len(out.V) != len(frame.V) {
			out.V = make([]byte, len(frame.V))
		}
		out.Width, out.Height = frame.Width, frame.Height
		out.ChromaWidth, out.ChromaHeight = frame.ChromaWidth, frame.ChromaHeight
		out.PictureType, out.Interlaced = frame.PictureType, false
		out.Deinterlaced = true
		out.TopFieldFirst, out.TemporalReference = frame.TopFieldFirst, frame.TemporalReference
		for y := 0; y < frame.Height; y++ {
			sourceY := (y/2)*2 + field
			if sourceY >= frame.Height {
				sourceY = frame.Height - 1
			}
			copy(out.Y[y*frame.Width:(y+1)*frame.Width], frame.Y[sourceY*frame.Width:(sourceY+1)*frame.Width])
		}
		for y := 0; y < frame.ChromaHeight; y++ {
			sourceY := (y/2)*2 + field
			if sourceY >= frame.ChromaHeight {
				sourceY = frame.ChromaHeight - 1
			}
			copy(out.U[y*frame.ChromaWidth:(y+1)*frame.ChromaWidth], frame.U[sourceY*frame.ChromaWidth:(sourceY+1)*frame.ChromaWidth])
			copy(out.V[y*frame.ChromaWidth:(y+1)*frame.ChromaWidth], frame.V[sourceY*frame.ChromaWidth:(sourceY+1)*frame.ChromaWidth])
		}
	}
	return d.frames[:]
}
