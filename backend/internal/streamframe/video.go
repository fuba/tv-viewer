package streamframe

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"time"
)

const (
	VideoHeaderSize = 16
	MaxVideoSize    = 16 * 1024 * 1024
	NoPTS           = uint64(math.MaxUint64)
)

type Video struct {
	Data     []byte
	Duration time.Duration
	PTS      uint64
	HasPTS   bool
}

func WriteVideo(writer io.Writer, frame Video) error {
	if len(frame.Data) == 0 || len(frame.Data) > MaxVideoSize {
		return fmt.Errorf("invalid video access unit size %d", len(frame.Data))
	}
	micros := frame.Duration / time.Microsecond
	if micros < 10_000 || micros > 200_000 {
		return fmt.Errorf("invalid video duration %s", frame.Duration)
	}
	var header [VideoHeaderSize]byte
	binary.BigEndian.PutUint32(header[:4], uint32(len(frame.Data))) // #nosec G115 -- MaxVideoSize is below uint32 capacity
	binary.BigEndian.PutUint32(header[4:8], uint32(micros))
	pts := NoPTS
	if frame.HasPTS {
		pts = frame.PTS
	}
	binary.BigEndian.PutUint64(header[8:16], pts)
	if _, err := writer.Write(header[:]); err != nil {
		return err
	}
	_, err := writer.Write(frame.Data)
	return err
}

func ReadVideo(reader io.Reader) (Video, error) {
	var header [VideoHeaderSize]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return Video{}, err
	}
	length := binary.BigEndian.Uint32(header[:4])
	if length == 0 || length > MaxVideoSize {
		return Video{}, fmt.Errorf("invalid video access unit size %d", length)
	}
	duration := time.Duration(binary.BigEndian.Uint32(header[4:8])) * time.Microsecond
	if duration < 10*time.Millisecond || duration > 200*time.Millisecond {
		return Video{}, fmt.Errorf("invalid video duration %s", duration)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return Video{}, err
	}
	pts := binary.BigEndian.Uint64(header[8:16])
	return Video{Data: data, Duration: duration, PTS: pts, HasPTS: pts != NoPTS}, nil
}
