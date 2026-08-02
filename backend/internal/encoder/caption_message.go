package encoder

import (
	"time"

	"github.com/fuba/tv-viewer/internal/nativecaption"
	"github.com/fuba/tv-viewer/internal/voicetranslate"
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

func translationMessage(event voicetranslate.Event) map[string]any {
	switch event.Type {
	case voicetranslate.EventReady:
		return map[string]any{"type": "translation-status", "status": "ready"}
	case voicetranslate.EventPartial, voicetranslate.EventFinal:
		if event.Translation == nil || *event.Translation == "" {
			return map[string]any{
				"type": "translation-status", "status": "unavailable",
				"stage": "translation", "captionId": boundedTranslationText(event.CaptionID, 256),
			}
		}
		return map[string]any{
			"type": "translation-caption", "phase": event.Type,
			"id": "translation-live", "captionId": boundedTranslationText(event.CaptionID, 256),
			"text": boundedTranslationText(*event.Translation, 400), "sourceLanguage": boundedTranslationText(event.SourceLanguage, 32),
			"targetLanguage": boundedTranslationText(event.TargetLanguage, 32),
		}
	case voicetranslate.EventSpeech:
		return map[string]any{
			"type": "translation-status", "status": "speaking",
			"captionId": boundedTranslationText(event.CaptionID, 256), "speaker": boundedTranslationText(event.Speaker, 128),
		}
	case voicetranslate.EventSpeechCancelled:
		return map[string]any{
			"type": "translation-status", "status": "speech-cancelled",
			"captionId": boundedTranslationText(event.CaptionID, 256),
		}
	case voicetranslate.EventError:
		return map[string]any{
			"type": "translation-status", "status": "error",
			"stage": boundedTranslationText(event.Stage, 64), "message": boundedTranslationText(event.Message, 512),
			"captionId": boundedTranslationText(event.CaptionID, 256),
		}
	default:
		return map[string]any{"type": "translation-status", "status": "unknown"}
	}
}

func boundedTranslationText(value string, maximum int) string {
	if maximum <= 0 {
		return value
	}
	count := 0
	for index := range value {
		if count == maximum {
			return value[:index]
		}
		count++
	}
	return value
}
