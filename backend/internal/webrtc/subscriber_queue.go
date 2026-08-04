package webrtc

import (
	"context"
	"sync"
	"time"
)

// subscriberQueue is a bounded per-subscriber FIFO. Its overflow reserve keeps
// publisher goroutines non-blocking while a paced writer crosses a brief
// startup stall.
type subscriberQueue[T any] struct {
	mu         sync.Mutex
	items      []T
	head       int
	size       int
	softLimit  int
	fullSince  time.Time
	ready      chan struct{}
	now        func() time.Time
	stallGrace time.Duration
}

func newSubscriberQueue[T any](softLimit, overflowReserve int) *subscriberQueue[T] {
	if softLimit < 1 {
		softLimit = 1
	}
	if overflowReserve < 1 {
		overflowReserve = 1
	}
	return &subscriberQueue[T]{
		items:      make([]T, softLimit+overflowReserve),
		softLimit:  softLimit,
		ready:      make(chan struct{}, 1),
		now:        time.Now,
		stallGrace: sharedQueueStallGrace,
	}
}

// Push never waits for a consumer. It accepts a bounded transient overflow,
// but rejects a subscriber whose soft limit remains exceeded past the grace
// period or whose overflow reserve is exhausted.
func (q *subscriberQueue[T]) Push(value T) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.size >= q.softLimit {
		now := q.now()
		if q.fullSince.IsZero() {
			q.fullSince = now
		}
		if now.Sub(q.fullSince) >= q.stallGrace || q.size == len(q.items) {
			return false
		}
	}

	wasEmpty := q.size == 0
	index := (q.head + q.size) % len(q.items)
	q.items[index] = value
	q.size++
	if wasEmpty {
		select {
		case q.ready <- struct{}{}:
		default:
		}
	}
	return true
}

func (q *subscriberQueue[T]) Pop(ctx context.Context, subscriberDone <-chan struct{}) (T, bool) {
	var zero T
	for {
		select {
		case <-ctx.Done():
			return zero, false
		case <-subscriberDone:
			return zero, false
		default:
		}

		q.mu.Lock()
		if q.size > 0 {
			value := q.items[q.head]
			q.items[q.head] = zero
			q.head = (q.head + 1) % len(q.items)
			q.size--
			if q.size < q.softLimit {
				q.fullSince = time.Time{}
			}
			q.mu.Unlock()
			return value, true
		}
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return zero, false
		case <-subscriberDone:
			return zero, false
		case <-q.ready:
		}
	}
}
