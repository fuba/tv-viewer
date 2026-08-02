package encoder

import (
	"testing"
	"time"

	"github.com/fuba/tv-viewer/internal/mpegts"
)

func TestVideoFormatAnnouncerSendsTheFirstFormat(t *testing.T) {
	var announcer videoFormatAnnouncer
	format := mpegts.VideoFormat{Width: 1440, Height: 1080, AspectNum: 16, AspectDen: 9}
	now := time.Unix(0, 0)

	if !announcer.shouldAnnounce(format, now) {
		t.Fatal("the first format must be announced")
	}
	if announcer.shouldAnnounce(format, now.Add(time.Second)) {
		t.Fatal("an unchanged format must not be repeated immediately")
	}
}

func TestVideoFormatAnnouncerSendsChanges(t *testing.T) {
	var announcer videoFormatAnnouncer
	now := time.Unix(0, 0)
	announcer.shouldAnnounce(mpegts.VideoFormat{Width: 1440, Height: 1080, AspectNum: 16, AspectDen: 9}, now)

	widescreenSD := mpegts.VideoFormat{Width: 720, Height: 480, AspectNum: 16, AspectDen: 9}
	if !announcer.changed(widescreenSD) {
		t.Fatal("a new coded size must count as a change")
	}
	if !announcer.shouldAnnounce(widescreenSD, now.Add(time.Second)) {
		t.Fatal("a changed format must be announced without waiting")
	}
	if announcer.changed(widescreenSD) {
		t.Fatal("the announced format must become the reference")
	}
}

func TestVideoFormatAnnouncerRepeatsForLateViewers(t *testing.T) {
	var announcer videoFormatAnnouncer
	format := mpegts.VideoFormat{Width: 720, Height: 480, AspectNum: 20, AspectDen: 11}
	now := time.Unix(0, 0)
	announcer.shouldAnnounce(format, now)

	if announcer.shouldAnnounce(format, now.Add(videoFormatRepeatInterval-time.Millisecond)) {
		t.Fatal("the repeat must wait for the full interval")
	}
	if !announcer.shouldAnnounce(format, now.Add(videoFormatRepeatInterval)) {
		t.Fatal("the format must be repeated once the interval elapses")
	}
	if announcer.shouldAnnounce(format, now.Add(videoFormatRepeatInterval+time.Second)) {
		t.Fatal("the repeat interval must restart after each announcement")
	}
}

func TestVideoFormatMessageCarriesTheAspect(t *testing.T) {
	message := videoFormatMessage(mpegts.VideoFormat{Width: 720, Height: 480, AspectNum: 20, AspectDen: 11})
	if message["type"] != "video-format" {
		t.Fatalf("message type = %v, want video-format", message["type"])
	}
	if message["aspectNum"] != 20 || message["aspectDen"] != 11 {
		t.Fatalf("aspect = %v:%v, want 20:11", message["aspectNum"], message["aspectDen"])
	}
	if message["width"] != 720 || message["height"] != 480 {
		t.Fatalf("coded size = %vx%v, want 720x480", message["width"], message["height"])
	}
}
