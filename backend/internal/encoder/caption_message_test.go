package encoder

import (
	"strings"
	"testing"
	"time"

	"github.com/fuba/tv-viewer/internal/nativecaption"
	"github.com/fuba/tv-viewer/internal/voicetranslate"
)

func placedCaption() nativecaption.Caption {
	return nativecaption.Caption{
		Text:  "（筒井）でも　当たりだろ。\n言わなくても分かるよ。",
		Plane: nativecaption.Plane{Width: 960, Height: 540},
		Rows: []nativecaption.Row{
			{Text: "（筒井）でも　当たりだろ。", Bottom: 449, Height: 60, Spans: []nativecaption.Span{
				{Text: "（", Left: 358, Width: 20, Chars: 1, FontWidth: 18, FontHeight: 36, HorizontalSpace: 2, Foreground: 0xFFFFFF},
				{Text: "筒井", Left: 378, Width: 80, Chars: 2, FontWidth: 36, FontHeight: 36, HorizontalSpace: 4, Foreground: 0xFFFFFF},
			}},
			{Text: "言わなくても分かるよ。", Bottom: 509, Height: 60, Spans: []nativecaption.Span{
				{Text: "言わなくても分かるよ。", Left: 378, Width: 440, Chars: 11, FontWidth: 36, FontHeight: 36, HorizontalSpace: 4, Foreground: 0xFFFFFF},
			}},
		},
	}
}

func TestTranslationMessageDoesNotForwardSynthesizedAudio(t *testing.T) {
	translation := "こんにちは、世界。"
	message := translationMessage(voicetranslate.Event{
		Type: voicetranslate.EventFinal, CaptionID: "caption-1", Text: "Hello world.",
		Translation: &translation, TargetLanguage: "ja", AudioBase64: "must-not-leave-the-server",
	}, "channel-1", "stream-1")
	if message["type"] != "translation-caption" || message["phase"] != "final" || message["text"] != translation || message["originalText"] != "Hello world." {
		t.Fatalf("translation message = %v", message)
	}
	if _, exists := message["audio_base64"]; exists {
		t.Fatalf("synthesized audio leaked into DataChannel: %v", message)
	}
	if message["channelId"] != "channel-1" || message["streamId"] != "stream-1" {
		t.Fatalf("translation source identity = %v", message)
	}
}

func TestTranslationStatusMessageCarriesSafeState(t *testing.T) {
	secret := strings.Repeat("s", 32)
	message := translationMessage(voicetranslate.Event{Type: voicetranslate.EventError, Stage: "capacity", Message: "token=" + secret}, "channel-1", "stream-1")
	if message["type"] != "translation-status" || message["status"] != "error" || message["stage"] != "capacity" || message["message"] != "Translation service is busy" {
		t.Fatalf("translation status = %v", message)
	}
	if strings.Contains(message["message"].(string), secret) {
		t.Fatalf("upstream error leaked into DataChannel: %v", message)
	}
}

func TestTranslationMessageBoundsAllForwardedGatewayMetadata(t *testing.T) {
	translation := strings.Repeat("訳", 500)
	original := strings.Repeat("o", 500)
	message := translationMessage(voicetranslate.Event{
		Type: voicetranslate.EventFinal, CaptionID: strings.Repeat("c", 300), Text: original, Translation: &translation,
		SourceLanguage: strings.Repeat("s", 50), TargetLanguage: strings.Repeat("t", 50),
	}, strings.Repeat("h", 300), strings.Repeat("i", 300))
	if len([]rune(message["text"].(string))) != 400 || len(message["originalText"].(string)) != 400 || len(message["captionId"].(string)) != 256 {
		t.Fatalf("translation fields were not bounded: %v", message)
	}
	if len(message["sourceLanguage"].(string)) != 32 || len(message["targetLanguage"].(string)) != 32 {
		t.Fatalf("language fields were not bounded: %v", message)
	}
	if len(message["channelId"].(string)) != 256 || len(message["streamId"].(string)) != 256 {
		t.Fatalf("translation source identity was not bounded: %v", message)
	}

	status := translationMessage(voicetranslate.Event{
		Type: voicetranslate.EventError, CaptionID: strings.Repeat("c", 300),
		Stage: strings.Repeat("s", 100), Message: strings.Repeat("m", 600),
	}, "channel-1", "stream-1")
	if len(status["captionId"].(string)) != 256 || status["stage"] != "service" || status["message"] != "Translation service error" {
		t.Fatalf("status fields were not bounded: %v", status)
	}
}

func TestCaptionMessageCarriesThePlaneAndRows(t *testing.T) {
	message := captionMessage(placedCaption(), 3700*time.Millisecond)

	plane, ok := message["plane"].(map[string]any)
	if !ok || plane["width"] != 960 || plane["height"] != 540 {
		t.Fatalf("plane = %v", message["plane"])
	}
	rows, ok := message["rows"].([]map[string]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("rows = %v", message["rows"])
	}
	if rows[0]["bottom"] != 449 || rows[1]["bottom"] != 509 {
		t.Fatalf("rows lost their placement: %v", rows)
	}
	if rows[0]["height"] != 60 {
		t.Fatalf("a row must carry the full character block height: %v", rows[0])
	}
	spans, ok := rows[0]["spans"].([]map[string]any)
	if !ok || len(spans) != 2 {
		t.Fatalf("spans = %v", rows[0]["spans"])
	}
	if spans[0]["fontWidth"] != 18 || spans[0]["fontHeight"] != 36 || spans[0]["width"] != 20 {
		t.Fatalf("half width run lost its metrics: %v", spans[0])
	}
	if spans[0]["color"] != "#ffffff" {
		t.Fatalf("colour = %v, want #ffffff", spans[0]["color"])
	}
	if message["text"] != "（筒井）でも　当たりだろ。\n言わなくても分かるよ。" {
		t.Fatalf("flat text lost the line break: %v", message["text"])
	}
	if message["endTime"] != 3.7 {
		t.Fatalf("endTime = %v, want 3.7", message["endTime"])
	}
}

func TestCaptionMessageOmitsAnUnknownLayout(t *testing.T) {
	message := captionMessage(nativecaption.Caption{Text: "字幕"}, time.Second)

	if _, ok := message["plane"]; ok {
		t.Fatalf("a caption without geometry must not claim a plane: %v", message)
	}
	if _, ok := message["rows"]; ok {
		t.Fatalf("a caption without geometry must not claim rows: %v", message)
	}
	if message["text"] != "字幕" {
		t.Fatalf("text = %v", message["text"])
	}
}

func TestCaptionColoursAndOpacity(t *testing.T) {
	if colorHex(0xFFFFFF) != "#ffffff" || colorHex(0) != "#000000" || colorHex(0x00A0FF) != "#00a0ff" {
		t.Fatalf("colour conversion is wrong: %s %s %s", colorHex(0xFFFFFF), colorHex(0), colorHex(0x00A0FF))
	}
	// libaribb24 reports transparency, so zero means fully drawn.
	if alphaOpacity(0) != 1 {
		t.Fatalf("opaque alpha = %v, want 1", alphaOpacity(0))
	}
	if alphaOpacity(255) != 0 {
		t.Fatalf("transparent alpha = %v, want 0", alphaOpacity(255))
	}
	if opacity := alphaOpacity(51); opacity <= 0.79 || opacity >= 0.81 {
		t.Fatalf("partial alpha = %v, want 0.8", opacity)
	}
}
