package api

import (
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/fuba/tv-viewer/internal/encoder"
	"github.com/fuba/tv-viewer/internal/mirakurun"
	"github.com/fuba/tv-viewer/internal/webrtc"
)

func TestUnexpectedSharedSessionEndDisconnectsAffectedPeers(t *testing.T) {
	var gotPeerIDs []string
	var gotErr error
	session := &sharedSession{
		channelID: "service:CS:CS16:700353",
		session:   &encoder.WebRTCSession{ID: "test-session"},
		stream:    webrtc.NewSharedStream(),
		peers:     map[string]struct{}{"viewer-1": {}, "viewer-2": {}},
		onUnexpectedStop: func(peerIDs []string, err error) {
			gotPeerIDs = peerIDs
			gotErr = err
		},
	}

	session.stopUnexpected(io.EOF)

	if !session.stopped {
		t.Fatal("unexpected pipeline end did not stop the shared session")
	}
	if !errors.Is(gotErr, io.EOF) {
		t.Fatalf("unexpected stop error = %v, want EOF", gotErr)
	}
	if !reflect.DeepEqual(gotPeerIDs, []string{"viewer-1", "viewer-2"}) &&
		!reflect.DeepEqual(gotPeerIDs, []string{"viewer-2", "viewer-1"}) {
		t.Fatalf("disconnected peers = %v, want viewer-1 and viewer-2", gotPeerIDs)
	}
}

func TestNormalSharedSessionStopDoesNotDisconnectPeerConnection(t *testing.T) {
	called := false
	session := &sharedSession{
		channelID:        "service:CS:CS16:700353",
		session:          &encoder.WebRTCSession{ID: "test-session"},
		stream:           webrtc.NewSharedStream(),
		peers:            map[string]struct{}{"viewer-1": {}},
		onUnexpectedStop: func([]string, error) { called = true },
	}

	session.stop()
	session.stopUnexpected(io.EOF)

	if called {
		t.Fatal("an EOF racing after normal stop disconnected the reusable peer connection")
	}
}

func TestIdleStopCannotClaimSessionWithNewPeer(t *testing.T) {
	session := &sharedSession{
		channelID: "service:CS:CS16:700353",
		session:   &encoder.WebRTCSession{ID: "test-session"},
		stream:    webrtc.NewSharedStream(),
		peers:     map[string]struct{}{"viewer-1": {}},
	}

	session.stopIfIdle()
	if session.stopped {
		t.Fatal("idle stop claimed a session after a peer was added")
	}

	session.mu.Lock()
	delete(session.peers, "viewer-1")
	session.mu.Unlock()
	session.stopIfIdle()
	if !session.stopped {
		t.Fatal("idle session was not stopped")
	}
}

func TestFailedPeerAttachKeepsIdleShutdownArmed(t *testing.T) {
	stream := webrtc.NewSharedStream()
	stream.Stop()
	session := &sharedSession{
		channelID: "service:CS:CS16:700353",
		session:   &encoder.WebRTCSession{ID: "test-session"},
		stream:    stream,
		peers:     make(map[string]struct{}),
	}
	timer := time.AfterFunc(time.Hour, func() {})
	defer timer.Stop()
	session.idleStop = timer

	if err := session.addPeer(nil); err == nil {
		t.Fatal("attachment to a stopped shared stream unexpectedly succeeded")
	}
	if session.idleStop != timer {
		t.Fatal("failed peer attachment disarmed idle session cleanup")
	}
	if !timer.Stop() {
		t.Fatal("failed peer attachment stopped idle session cleanup timer")
	}
}

func TestSharedSessionSettingsMatch(t *testing.T) {
	session := &sharedSession{burnInSubtitles: true, audioMode: encoder.AudioModeBoth, translationEnabled: true}
	if !session.matchesSettings(true, encoder.AudioModeBoth, true) {
		t.Fatal("identical settings should share a session")
	}
	if session.matchesSettings(false, encoder.AudioModeBoth, true) {
		t.Fatal("subtitle change must replace the session")
	}
	if session.matchesSettings(true, encoder.AudioModeMain, true) {
		t.Fatal("audio change must replace the session")
	}
	if session.matchesSettings(true, encoder.AudioModeBoth, false) {
		t.Fatal("translation change must replace the session")
	}
}

func TestParseAudioMode(t *testing.T) {
	for _, value := range []string{"main", "sub", "both"} {
		value := value
		mode, err := parseAudioMode(&value)
		if err != nil || string(mode) != value {
			t.Fatalf("parseAudioMode(%q) = %q, %v", value, mode, err)
		}
	}
	invalid := "unexpected"
	if _, err := parseAudioMode(&invalid); err == nil {
		t.Fatal("invalid audio mode was accepted")
	}
}

func TestResolveStreamTargetByService(t *testing.T) {
	channels := []mirakurun.Channel{
		{
			Type: "CS", Channel: "CS6",
			Services: []mirakurun.Service{
				{ID: 700294, ServiceID: 294, Name: "Home Drama Channel"},
				{ID: 700354, ServiceID: 354, Name: "CNNj"},
			},
		},
	}

	target, err := resolveStreamTarget(channels, "service:CS:CS6:700354")
	if err != nil {
		t.Fatalf("resolveStreamTarget returned an error: %v", err)
	}
	if target.channelType != "CS" || target.channel != "CS6" {
		t.Fatalf("unexpected tuning target: %#v", target)
	}
	if !reflect.DeepEqual(target.programNumbers, []uint16{354}) {
		t.Fatalf("program numbers = %v, want [354]", target.programNumbers)
	}
}

func TestResolveStreamTargetByMPEGServiceID(t *testing.T) {
	channels := []mirakurun.Channel{{
		Type: "CS", Channel: "CS6",
		Services: []mirakurun.Service{
			{ID: 700294, ServiceID: 294},
			{ID: 700354, ServiceID: 354},
		},
	}}

	target, err := resolveStreamTarget(channels, "service:CS:CS6:354")
	if err != nil {
		t.Fatalf("resolveStreamTarget returned an error: %v", err)
	}
	if !reflect.DeepEqual(target.programNumbers, []uint16{354}) {
		t.Fatalf("program numbers = %v, want [354]", target.programNumbers)
	}
}

func TestResolveStreamTargetRejectsUnknownService(t *testing.T) {
	channels := []mirakurun.Channel{{
		Type: "CS", Channel: "CS6",
		Services: []mirakurun.Service{{ID: 700294, ServiceID: 294}},
	}}

	if _, err := resolveStreamTarget(channels, "service:CS:CS6:700354"); err == nil {
		t.Fatal("unknown service was accepted")
	}
}

func TestResolveStreamTargetRejectsUnscopedService(t *testing.T) {
	channels := []mirakurun.Channel{
		{Type: "GR", Channel: "27", Services: []mirakurun.Service{{ID: 1800354, ServiceID: 354}}},
		{Type: "CS", Channel: "CS6", Services: []mirakurun.Service{{ID: 700354, ServiceID: 354}}},
	}

	if _, err := resolveStreamTarget(channels, "service:354"); err == nil {
		t.Fatal("ambiguous unscoped service selection was accepted")
	}
}

func TestResolveStreamTargetRejectsServiceOnWrongChannel(t *testing.T) {
	channels := []mirakurun.Channel{{
		Type: "CS", Channel: "CS6",
		Services: []mirakurun.Service{{ID: 700354, ServiceID: 354}},
	}}

	if _, err := resolveStreamTarget(channels, "service:CS:CS8:700354"); err == nil {
		t.Fatal("service outside the requested physical channel was accepted")
	}
}

func TestRegistryRejectsSecondEncodedChannel(t *testing.T) {
	registry := &sharedSessionRegistry{sessions: map[string]*sharedSession{
		"service:CS:CS6:700354": {
			channelID: "service:CS:CS6:700354",
			stream:    webrtc.NewSharedStream(),
			peers:     map[string]struct{}{"viewer-1": {}},
		},
	}}

	if err := registry.canAccept("service:CS:CS6:700354"); err != nil {
		t.Fatalf("same channel subscriber was rejected: %v", err)
	}
	if err := registry.canAccept("service:CS:CS16:700353"); !errors.Is(err, errChannelLimit) {
		t.Fatalf("second channel error = %v, want %v", err, errChannelLimit)
	}
}

func TestRegistryReportsOnlyViewedActiveChannel(t *testing.T) {
	registry := &sharedSessionRegistry{sessions: map[string]*sharedSession{
		"viewed": {channelID: "viewed", peers: map[string]struct{}{"viewer-1": {}}},
		"idle":   {channelID: "idle", peers: map[string]struct{}{}},
	}}

	channelID, viewers := registry.activeChannel()
	if channelID != "viewed" || viewers != 1 {
		t.Fatalf("activeChannel() = %q, %d, want viewed, 1", channelID, viewers)
	}
}
