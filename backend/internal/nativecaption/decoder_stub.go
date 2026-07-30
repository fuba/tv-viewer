//go:build !native || !cgo

package nativecaption

import "errors"

type Decoder struct{}

func NewDecoder() (*Decoder, error) {
	return nil, errors.New("ARIB caption decoder is unavailable")
}

func (d *Decoder) DecodePES(_ []byte) (Caption, bool, error) {
	return Caption{}, false, errors.New("ARIB caption decoder is unavailable")
}

func (d *Decoder) Close() error { return nil }
