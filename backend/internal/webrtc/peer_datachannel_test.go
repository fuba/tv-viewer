package webrtc

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitDataChannelOpen(t *testing.T) {
	t.Parallel()
	peerCtx, cancelPeer := context.WithCancel(context.Background())
	defer cancelPeer()
	peer := &Peer{ctx: peerCtx, dataOpen: make(chan struct{})}

	waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	result := make(chan error, 1)
	go func() { result <- peer.WaitDataChannelOpen(waitCtx) }()
	peer.markDataChannelOpen()
	if err := <-result; err != nil {
		t.Fatalf("WaitDataChannelOpen() = %v", err)
	}
}

func TestWaitDataChannelOpenHonorsCancellation(t *testing.T) {
	t.Parallel()
	peerCtx, cancelPeer := context.WithCancel(context.Background())
	peer := &Peer{ctx: peerCtx, dataOpen: make(chan struct{})}
	cancelPeer()
	if err := peer.WaitDataChannelOpen(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitDataChannelOpen() = %v, want context.Canceled", err)
	}
}
