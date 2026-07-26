package encoder

import "testing"

func TestPTSDeltaMillisecondsHandlesWrap(t *testing.T) {
	if got := ptsDeltaMilliseconds(93_003, 90_000); got < 33.36 || got > 33.38 {
		t.Fatalf("delta = %.3fms, want about 33.367ms", got)
	}
	if got := ptsDeltaMilliseconds(1_000, broadcastPTSMask-1_000); got < 22.23 || got > 22.24 {
		t.Fatalf("wrapped delta = %.3fms, want about 22.233ms", got)
	}
}
