//go:build !native || !cgo

package encoder

import (
	"fmt"
	"io"
)

func (e *Encoder) StartNativeWebRTCEncoding(_ string, _ io.ReadCloser, _ string, _ []uint16, _ bool, _ AudioMode) (*WebRTCSession, error) {
	return nil, fmt.Errorf("native WebRTC pipeline is unavailable in this build")
}
