package encoder

import (
	"time"

	"github.com/fuba/tv-viewer/internal/mpegts"
)

const maxAudioPTSDrift = int64(450) // 5 ms at 90 kHz.
const audioDecoderResetCooldown = 5 * time.Second

func shouldResetAudioDecoder(consecutiveErrors int, lastReset, now time.Time) bool {
	if consecutiveErrors != 1 && consecutiveErrors%100 != 0 {
		return false
	}
	return lastReset.IsZero() || now.Sub(lastReset) >= audioDecoderResetCooldown
}

func primaryAudioPID(streams map[uint16]mpegts.Stream, programNumber uint16) uint16 {
	var selected uint16
	for pid, stream := range streams {
		if stream.StreamType != 0x0f || (programNumber != 0 && !stream.HasProgram(programNumber)) {
			continue
		}
		if selected == 0 || pid < selected {
			selected = pid
		}
	}
	return selected
}

// alignAudioPTS keeps the timestamp at the head of the stereo PCM queue tied
// to broadcast PTS. It requests a queue reset after loss instead of allowing
// audio to drift permanently behind video.
func alignAudioPTS(current uint64, valid bool, bufferedStereoSamples int, source uint64, sourceValid bool) (uint64, bool, bool) {
	if !sourceValid {
		return current, valid, false
	}
	if !valid {
		return source & broadcastPTSMask, true, true
	}
	bufferedFrames := bufferedStereoSamples / 2
	expectedTail := (current + uint64(bufferedFrames)*90_000/48_000) & broadcastPTSMask
	delta := int64((source - expectedTail) & broadcastPTSMask)
	if delta > int64(broadcastPTSMask/2) {
		delta -= int64(broadcastPTSMask + 1)
	}
	if delta < -maxAudioPTSDrift || delta > maxAudioPTSDrift {
		return source & broadcastPTSMask, true, true
	}
	return current, true, false
}
