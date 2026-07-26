//go:build !native || !cgo

package nativeaudio

type Decoder struct{}

func NewDecoder() (*Decoder, error)                        { return nil, ErrUnavailable }
func (d *Decoder) DecodeADTS(_ []byte) ([]PCMFrame, error) { return nil, ErrUnavailable }
func (d *Decoder) Close() error                            { return nil }

type Encoder struct{}

func NewEncoder(_ int) (*Encoder, error)               { return nil, ErrUnavailable }
func (e *Encoder) Encode(_ []int16) (OpusFrame, error) { return OpusFrame{}, ErrUnavailable }
func (e *Encoder) Close() error                        { return nil }
