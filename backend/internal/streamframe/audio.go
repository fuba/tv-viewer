package streamframe

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

const (
	AudioHeaderSize = 16
	MaxAudioSize    = 64 * 1024
)

type Audio struct {
	Data     []byte
	Duration time.Duration
	PTS      uint64
	HasPTS   bool
}

func WriteAudio(writer io.Writer, frame Audio) error {
	if len(frame.Data) == 0 || len(frame.Data) > MaxAudioSize {
		return fmt.Errorf("invalid audio packet size %d", len(frame.Data))
	}
	micros := frame.Duration / time.Microsecond
	if micros < 2_000 || micros > 200_000 {
		return fmt.Errorf("invalid audio duration %s", frame.Duration)
	}
	var header [AudioHeaderSize]byte
	binary.BigEndian.PutUint32(header[:4], uint32(len(frame.Data)))
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

func ReadAudio(reader io.Reader) (Audio, error) {
	var header [AudioHeaderSize]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return Audio{}, err
	}
	length := binary.BigEndian.Uint32(header[:4])
	if length == 0 || length > MaxAudioSize {
		return Audio{}, fmt.Errorf("invalid audio packet size %d", length)
	}
	duration := time.Duration(binary.BigEndian.Uint32(header[4:8])) * time.Microsecond
	if duration < 2*time.Millisecond || duration > 200*time.Millisecond {
		return Audio{}, fmt.Errorf("invalid audio duration %s", duration)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return Audio{}, err
	}
	pts := binary.BigEndian.Uint64(header[8:16])
	return Audio{Data: data, Duration: duration, PTS: pts, HasPTS: pts != NoPTS}, nil
}
