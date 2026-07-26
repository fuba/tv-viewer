package encoder

import (
	"testing"
	"time"

	"github.com/fuba/tv-viewer/internal/mpegts"
)

func TestPrimaryAudioPIDUsesLowestAACPIDInSelectedProgram(t *testing.T) {
	streams := map[uint16]mpegts.Stream{
		0x120: {PID: 0x120, StreamType: 0x0f, ProgramNumbers: map[uint16]struct{}{10: {}}},
		0x110: {PID: 0x110, StreamType: 0x0f, ProgramNumbers: map[uint16]struct{}{10: {}}},
		0x100: {PID: 0x100, StreamType: 0x02, ProgramNumbers: map[uint16]struct{}{10: {}}},
		0x101: {PID: 0x101, StreamType: 0x0f, ProgramNumbers: map[uint16]struct{}{11: {}}},
	}
	if got := primaryAudioPID(streams, 10); got != 0x110 {
		t.Fatalf("primary audio PID = %#x, want 0x110", got)
	}
}

func TestAACDecoderResetPolicyRecoversConfigurationChangesWithoutThrashing(t *testing.T) {
	now := time.Unix(100, 0)
	for _, test := range []struct {
		errors int
		want   bool
	}{
		{1, true},
		{2, false},
		{99, false},
		{100, true},
	} {
		if got := shouldResetAudioDecoder(test.errors, time.Time{}, now); got != test.want {
			t.Errorf("shouldResetAudioDecoder(%d) = %v, want %v", test.errors, got, test.want)
		}
	}
	if shouldResetAudioDecoder(1, now, now.Add(audioDecoderResetCooldown-time.Millisecond)) {
		t.Fatal("intermittent error reset the decoder during cooldown")
	}
	if !shouldResetAudioDecoder(1, now, now.Add(audioDecoderResetCooldown)) {
		t.Fatal("decoder did not reset after cooldown")
	}
}

func TestAlignAudioPTSAccountsForBufferedStereoSamples(t *testing.T) {
	pts, valid, reset := alignAudioPTS(0, false, 0, 90_000, true)
	if pts != 90_000 || !valid || !reset {
		t.Fatalf("initial alignment = %d, %v, %v", pts, valid, reset)
	}

	// 64 stereo sample frames remain after encoding 960 samples from one
	// 1024-sample AAC frame. Their duration bridges 91800 to 91920 exactly.
	pts, valid, reset = alignAudioPTS(91_800, true, 64*2, 91_920, true)
	if pts != 91_800 || !valid || reset {
		t.Fatalf("contiguous alignment = %d, %v, %v", pts, valid, reset)
	}

	_, _, reset = alignAudioPTS(91_800, true, 64*2, 93_840, true)
	if !reset {
		t.Fatal("timestamp gap did not request a PCM queue reset")
	}
}
