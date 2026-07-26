package webrtc

import (
	"context"
	"sync"
	"time"
)

const (
	mpegClockRate = uint64(90_000)
	mpegPTSMask   = uint64(1<<33 - 1)
	maxPacingLag  = 100 * time.Millisecond
)

type mediaPacer struct {
	now  func() time.Time
	wait func(context.Context, time.Duration) error

	started  bool
	wallBase time.Time
	ptsBase  uint64
	next     time.Time
	maxLag   time.Duration
}

type sharedMediaClock struct {
	mu       sync.Mutex
	started  bool
	wallBase time.Time
	ptsBase  uint64
	revision uint64
	ready    chan struct{}
}

func newSharedMediaClock() *sharedMediaClock {
	return &sharedMediaClock{ready: make(chan struct{})}
}

func (c *sharedMediaClock) StartVideo(pts uint64) (time.Time, uint64, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		c.started = true
		c.wallBase = time.Now().Add(videoPrebuffer)
		c.ptsBase = pts & mpegPTSMask
		c.revision = 1
		close(c.ready)
	}
	return c.wallBase, c.ptsBase, c.revision
}

func (c *sharedMediaClock) WaitEpoch(ctx context.Context) (time.Time, uint64, uint64, error) {
	select {
	case <-ctx.Done():
		return time.Time{}, 0, 0, ctx.Err()
	case <-c.ready:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.wallBase, c.ptsBase, c.revision, nil
}

// Rebase atomically moves both media tracks to one new live epoch. A stale
// caller cannot overwrite a newer rebase performed by the other track.
func (c *sharedMediaClock) Rebase(expectedRevision uint64, wallBase time.Time, ptsBase uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started || c.revision != expectedRevision {
		return false
	}
	c.wallBase = wallBase
	c.ptsBase = ptsBase & mpegPTSMask
	c.revision++
	return true
}

func newMediaPacer() *mediaPacer {
	return &mediaPacer{now: time.Now, wait: waitContext, maxLag: maxPacingLag}
}

func (p *mediaPacer) SetEpoch(wallBase time.Time, ptsBase uint64) {
	if p.started {
		return
	}
	p.started = true
	p.wallBase = wallBase
	p.next = p.wallBase
	p.ptsBase = ptsBase & mpegPTSMask
}

func (p *mediaPacer) ResetEpoch(wallBase time.Time, ptsBase uint64) {
	p.started = true
	p.wallBase = wallBase
	p.next = wallBase
	p.ptsBase = ptsBase & mpegPTSMask
}

func signedPTSDelta(current, base uint64) int64 {
	delta := (current - base) & mpegPTSMask
	if delta > mpegPTSMask/2 {
		return int64(delta) - int64(mpegPTSMask+1)
	}
	return int64(delta)
}

func waitContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func waitUntil(ctx context.Context, target time.Time) error {
	return waitContext(ctx, time.Until(target))
}

// Pace maps MPEG's 90 kHz presentation clock to wall time. It resets after a
// discontinuity or excessive lag so a live stream can recover without building
// an ever-growing queue.
func (p *mediaPacer) Pace(ctx context.Context, pts uint64, hasPTS bool, duration time.Duration) (bool, error) {
	now := p.now()
	if !p.started {
		p.started = true
		p.wallBase = now
		p.next = now
		if hasPTS {
			p.ptsBase = pts & mpegPTSMask
		}
	}

	target := p.next
	if hasPTS {
		current := pts & mpegPTSMask
		delta := signedPTSDelta(current, p.ptsBase)
		if delta < -int64(10*time.Second/time.Microsecond)*int64(mpegClockRate)/1_000_000 ||
			delta > int64(10*time.Second/time.Microsecond)*int64(mpegClockRate)/1_000_000 {
			p.wallBase, p.ptsBase, target = now, current, now
		} else {
			target = p.wallBase.Add(time.Duration(delta) * time.Second / time.Duration(mpegClockRate))
		}
	}

	resynced := false
	if now.Sub(target) > p.maxLag {
		resynced = true
		p.wallBase = now
		target = now
		if hasPTS {
			p.ptsBase = pts & mpegPTSMask
		}
	}
	if err := p.wait(ctx, target.Sub(now)); err != nil {
		return resynced, err
	}
	p.next = target.Add(duration)
	return resynced, nil
}
