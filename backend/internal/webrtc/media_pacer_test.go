package webrtc

import (
	"context"
	"testing"
	"time"
)

func TestMediaPacerUsesPTSSpacing(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	pacer := &mediaPacer{
		now: func() time.Time { return now },
		wait: func(_ context.Context, duration time.Duration) error {
			waits = append(waits, duration)
			now = now.Add(duration)
			return nil
		},
		maxLag: maxPacingLag,
	}
	if _, err := pacer.Pace(context.Background(), 90_000, true, 16_683*time.Microsecond); err != nil {
		t.Fatal(err)
	}
	if _, err := pacer.Pace(context.Background(), 91_502, true, 16_683*time.Microsecond); err != nil {
		t.Fatal(err)
	}
	if len(waits) != 2 || waits[0] != 0 || waits[1] < 16*time.Millisecond || waits[1] > 17*time.Millisecond {
		t.Fatalf("waits = %v, want about [0s 16.69ms]", waits)
	}
}

func TestMediaPacerSchedulesAudioBeforeVideoEpoch(t *testing.T) {
	now := time.Unix(100, 0)
	var waited time.Duration
	pacer := &mediaPacer{
		now: func() time.Time { return now },
		wait: func(_ context.Context, duration time.Duration) error {
			waited = duration
			return nil
		},
		maxLag: maxPacingLag,
	}
	pacer.SetEpoch(now.Add(videoPrebuffer), 90_000)
	if _, err := pacer.Pace(context.Background(), 81_000, true, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if waited != videoPrebuffer-100*time.Millisecond {
		t.Fatalf("audio wait = %s, want %s", waited, videoPrebuffer-100*time.Millisecond)
	}
}

func TestMediaPacerBoundsLiveLag(t *testing.T) {
	now := time.Unix(100, 0)
	pacer := &mediaPacer{
		now:    func() time.Time { return now },
		wait:   func(_ context.Context, _ time.Duration) error { return nil },
		maxLag: maxPacingLag,
	}
	if _, err := pacer.Pace(context.Background(), 0, true, 16_683*time.Microsecond); err != nil {
		t.Fatal(err)
	}
	now = now.Add(250 * time.Millisecond)
	resynced, err := pacer.Pace(context.Background(), 1_502, true, 16_683*time.Microsecond)
	if err != nil {
		t.Fatal(err)
	}
	if !resynced {
		t.Fatal("pacer did not reset after excessive live lag")
	}
}

func TestSharedMediaClockKeepsAudioAndVideoOnOneEpoch(t *testing.T) {
	clock := newSharedMediaClock()
	wallBase, got, revision := clock.StartVideo(90_000)
	if got != 90_000 {
		t.Fatalf("first base = %d, want 90000", got)
	}
	_, got, nextRevision := clock.StartVideo(91_800)
	if got != 90_000 {
		t.Fatalf("second base = %d, want shared 90000", got)
	}
	if nextRevision != revision {
		t.Fatalf("second revision = %d, want %d", nextRevision, revision)
	}
	_, audioBase, audioRevision, err := clock.WaitEpoch(context.Background())
	if err != nil || audioBase != 90_000 || audioRevision != revision {
		t.Fatalf("audio epoch = %d, revision %d, %v", audioBase, audioRevision, err)
	}

	rebasedWall := wallBase.Add(time.Second)
	if !clock.Rebase(revision, rebasedWall, 180_000) {
		t.Fatal("current epoch rebase was rejected")
	}
	gotWall, gotPTS, rebasedRevision, err := clock.WaitEpoch(context.Background())
	if err != nil || gotWall != rebasedWall || gotPTS != 180_000 || rebasedRevision == revision {
		t.Fatalf("rebased epoch = %s, %d, revision %d, %v", gotWall, gotPTS, rebasedRevision, err)
	}
	if clock.Rebase(revision, rebasedWall.Add(time.Second), 270_000) {
		t.Fatal("stale epoch rebase was accepted")
	}
	if got := signedPTSDelta(500, mpegPTSMask-499); got != 1000 {
		t.Fatalf("wrapped delta = %d, want 1000", got)
	}
}

func TestBootstrapCacheStaysBounded(t *testing.T) {
	stream := NewSharedStream()
	for i := 0; i < 200; i++ {
		stream.rememberBootstrap(NALUnit{Type: NALTypeSPS}, sharedSample{data: []byte{0, 0, 0, 1, 0x67, byte(i)}})
		stream.rememberBootstrap(NALUnit{Type: NALTypePPS}, sharedSample{data: []byte{0, 0, 0, 1, 0x68, byte(i)}})
		stream.rememberBootstrap(NALUnit{Type: NALTypeIDR}, sharedSample{data: []byte{0, 0, 0, 1, 0x65, byte(i)}})
	}
	if len(stream.bootstrap) != 3 {
		t.Fatalf("bootstrap entries = %d, want 3", len(stream.bootstrap))
	}
}
