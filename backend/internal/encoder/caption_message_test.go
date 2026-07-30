package encoder

import (
	"testing"
	"time"

	"github.com/fuba/tv-viewer/internal/nativecaption"
)

func placedCaption() nativecaption.Caption {
	return nativecaption.Caption{
		Text:  "（筒井）でも　当たりだろ。\n言わなくても分かるよ。",
		Plane: nativecaption.Plane{Width: 960, Height: 540},
		Rows: []nativecaption.Row{
			{Text: "（筒井）でも　当たりだろ。", Bottom: 448, Spans: []nativecaption.Span{
				{Text: "（", Left: 378, Advance: 40, FontWidth: 18, FontHeight: 36, HorizontalSpace: 2, Foreground: 0xFFFFFF},
				{Text: "筒井", Left: 418, FontWidth: 36, FontHeight: 36, HorizontalSpace: 4, Foreground: 0xFFFFFF},
			}},
			{Text: "言わなくても分かるよ。", Bottom: 508, Spans: []nativecaption.Span{
				{Text: "言わなくても分かるよ。", Left: 418, FontWidth: 36, FontHeight: 36, HorizontalSpace: 4, Foreground: 0xFFFFFF},
			}},
		},
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
	if rows[0]["bottom"] != 448 || rows[1]["bottom"] != 508 {
		t.Fatalf("rows lost their placement: %v", rows)
	}
	spans, ok := rows[0]["spans"].([]map[string]any)
	if !ok || len(spans) != 2 {
		t.Fatalf("spans = %v", rows[0]["spans"])
	}
	if spans[0]["fontWidth"] != 18 || spans[0]["fontHeight"] != 36 || spans[0]["advance"] != 40 {
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
