package encoder

import (
	"log"
	"os/exec"

	"github.com/fuba/tv-viewer/internal/nativevideo"
)

// NVENCSupport describes the direct NVIDIA encoder used by nativevideo.
type NVENCSupport struct {
	Available bool
	Encoders  []string
}

func CheckNVENCSupport() (*NVENCSupport, error) {
	support := &NVENCSupport{Encoders: []string{}}
	_, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return support, nil
	}
	if output, err := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader").Output(); err != nil || len(output) == 0 { // #nosec G204 -- executable name and arguments are constants
		return support, nil
	}
	probe, err := nativevideo.NewNVEncoder(640, 360, 30, 1_000_000)
	if err != nil {
		log.Printf("NVENC initialization probe failed: %v", err)
		return support, nil
	}
	if err := probe.Close(); err != nil {
		log.Printf("NVENC initialization probe cleanup failed: %v", err)
		return support, nil
	}
	support.Available = true
	support.Encoders = []string{"native-h264-nvenc"}
	log.Printf("Native NVENC path enabled")
	return support, nil
}
