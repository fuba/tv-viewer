package encoder

import (
	"log"
	"os/exec"
)

// NVENCSupport describes the direct NVIDIA encoder used by nativevideo.
type NVENCSupport struct {
	Available bool
	Encoders  []string
}

func CheckNVENCSupport() (*NVENCSupport, error) {
	support := &NVENCSupport{Encoders: []string{}}
	command, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return support, nil
	}
	if output, err := exec.Command(command, "--query-gpu=name", "--format=csv,noheader").Output(); err != nil || len(output) == 0 {
		return support, nil
	}
	support.Available = true
	support.Encoders = []string{"native-h264-nvenc"}
	log.Printf("Native NVENC path enabled")
	return support, nil
}
