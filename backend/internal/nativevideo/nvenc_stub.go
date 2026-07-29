//go:build !native || !cgo

package nativevideo

// NVEncoder is unavailable in non-native builds.
type NVEncoder struct{}

// DisplayAspect is the ratio the coded picture must be displayed at.
type DisplayAspect struct {
	Num int
	Den int
}

func NewNVEncoder(_ int, _ int, _ int, _ int) (*NVEncoder, error) {
	return nil, ErrUnavailable
}

func NewNVEncoderForAdaptiveDecoder(_ int, _ int, _ int, _ int, _ DisplayAspect, _ *AdaptiveDecoder) (*NVEncoder, error) {
	return nil, ErrUnavailable
}

func (e *NVEncoder) Encode(_ YUVFrame) (H264Frame, error) {
	return H264Frame{}, ErrUnavailable
}

func (e *NVEncoder) Close() error { return nil }
