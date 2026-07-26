package encoder

import "testing"

func TestPresentationTimelineRestoresPTSOrder(t *testing.T) {
	var timeline presentationTimeline
	timeline.Add(93_003, true)
	timeline.Add(90_000, true)
	if pts, ok := timeline.Pop(); !ok || pts != 90_000 {
		t.Fatalf("first PTS = %d, %v, want 90000", pts, ok)
	}
	if pts, ok := timeline.Pop(); !ok || pts != 93_003 {
		t.Fatalf("second PTS = %d, %v, want 93003", pts, ok)
	}
}

func TestPresentationTimelineInfersMissingPTS(t *testing.T) {
	var timeline presentationTimeline
	timeline.Add(90_000, true)
	timeline.Add(0, false)
	_, _ = timeline.Pop()
	if pts, ok := timeline.Pop(); !ok || pts != 93_003 {
		t.Fatalf("inferred PTS = %d, %v, want 93003", pts, ok)
	}
}

func TestPresentationTimelineOrdersAcrossPTSWrap(t *testing.T) {
	var timeline presentationTimeline
	timeline.Add(broadcastPTSMask-1000, true)
	timeline.Add(2000, true)
	if pts, ok := timeline.Pop(); !ok || pts != broadcastPTSMask-1000 {
		t.Fatalf("first PTS = %d, %v", pts, ok)
	}
	if pts, ok := timeline.Pop(); !ok || pts != 2000 {
		t.Fatalf("wrapped PTS = %d, %v", pts, ok)
	}
}
