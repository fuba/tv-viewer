//go:build !native || !cgo

package nativevideo

// Decoder is the fallback implementation used by builds without libmpeg2.
type Decoder struct{}

func NewDecoder() (*Decoder, error) {
	return nil, ErrUnavailable
}

func (d *Decoder) Decode(_ []byte) ([]YUVFrame, error) {
	return nil, ErrUnavailable
}

func (d *Decoder) Close() error { return nil }

// AdaptiveDecoder is unavailable in builds without NVIDIA decode support.
type AdaptiveDecoder struct{}

func NewAdaptiveDecoder() (*AdaptiveDecoder, error) {
	return nil, ErrUnavailable
}

func (d *AdaptiveDecoder) Decode(_ []byte) ([]YUVFrame, error) {
	return nil, ErrUnavailable
}

func (d *AdaptiveDecoder) Close() error { return nil }
