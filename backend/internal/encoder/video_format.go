package encoder

import (
	"time"

	"github.com/fuba/tv-viewer/internal/mpegts"
)

// Viewers share one encoder session, so a client that joins an established
// stream never sees the first announcement. Repeating it slowly lets late
// joiners learn the picture geometry without waiting for a broadcast change.
const videoFormatRepeatInterval = 5 * time.Second

type videoFormatAnnouncer struct {
	last      mpegts.VideoFormat
	announced bool
	sentAt    time.Time
}

// shouldAnnounce reports whether this format has to reach the clients now.
func (a *videoFormatAnnouncer) shouldAnnounce(format mpegts.VideoFormat, now time.Time) bool {
	if !a.announced || format != a.last {
		a.last, a.announced, a.sentAt = format, true, now
		return true
	}
	if now.Sub(a.sentAt) < videoFormatRepeatInterval {
		return false
	}
	a.sentAt = now
	return true
}

// changed reports whether the format differs from the last announced one, which
// is what the encoder needs to know to restate the aspect ratio it emits.
func (a *videoFormatAnnouncer) changed(format mpegts.VideoFormat) bool {
	return !a.announced || format != a.last
}

func videoFormatMessage(format mpegts.VideoFormat) map[string]any {
	return map[string]any{
		"type":      "video-format",
		"width":     format.Width,
		"height":    format.Height,
		"aspectNum": format.AspectNum,
		"aspectDen": format.AspectDen,
	}
}
