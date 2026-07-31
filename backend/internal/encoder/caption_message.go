package encoder

import (
	"time"

	"github.com/fuba/tv-viewer/internal/nativecaption"
)

// captionMessage carries the caption exactly as the broadcaster laid it out:
// the plane it was drawn on, every row in drawing order, and the runs within a
// row with their own character size. Rows recover the line breaks, a small
// character size directly above a row is ruby, and the coordinates are the
// placement. The flat text stays for clients that cannot draw the layout.
func captionMessage(caption nativecaption.Caption, duration time.Duration) map[string]any {
	message := map[string]any{
		"type":    "show",
		"id":      "arib-caption",
		"text":    caption.Text,
		"endTime": duration.Seconds(),
	}
	if caption.Plane.Width <= 0 || caption.Plane.Height <= 0 || len(caption.Rows) == 0 {
		return message
	}
	rows := make([]map[string]any, 0, len(caption.Rows))
	for _, row := range caption.Rows {
		spans := make([]map[string]any, 0, len(row.Spans))
		for _, span := range row.Spans {
			spans = append(spans, map[string]any{
				"text":              span.Text,
				"left":              span.Left,
				"width":             span.Width,
				"chars":             span.Chars,
				"fontWidth":         span.FontWidth,
				"fontHeight":        span.FontHeight,
				"charSpace":         span.HorizontalSpace,
				"color":             colorHex(span.Foreground),
				"background":        colorHex(span.Background),
				"opacity":           alphaOpacity(span.ForegroundAlpha),
				"backgroundOpacity": alphaOpacity(span.BackgroundAlpha),
			})
		}
		rows = append(rows, map[string]any{
			"text":   row.Text,
			"bottom": row.Bottom,
			"height": row.Height,
			"spans":  spans,
		})
	}
	message["plane"] = map[string]any{"width": caption.Plane.Width, "height": caption.Plane.Height}
	message["rows"] = rows
	return message
}

func colorHex(value int) string {
	if value < 0 {
		value = 0
	}
	return "#" + hexByte(value>>16) + hexByte(value>>8) + hexByte(value)
}

func hexByte(value int) string {
	const digits = "0123456789abcdef"
	value &= 0xFF
	return string([]byte{digits[value>>4], digits[value&0x0F]})
}

// libaribb24 reports transparency, so an alpha of zero is a fully drawn colour.
func alphaOpacity(alpha int) float64 {
	if alpha <= 0 {
		return 1
	}
	if alpha >= 255 {
		return 0
	}
	return float64(255-alpha) / 255
}
