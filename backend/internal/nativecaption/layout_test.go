package nativecaption

import (
	"strings"
	"testing"
)

// Captured from a live NHK Eテレ broadcast. The decoder returns both drawn rows
// as one flat string with nothing between them, so only the geometry says where
// the line break belongs.
func twoRowCaption() (string, []Region) {
	decoded := "（筒井）でも　当たりだろ。言わなくても分かるよ。"
	texts := []struct {
		text                  string
		left, bottom          int
		fontWidth, fontHeight int
		space                 int
	}{
		{"（", 378, 448, 18, 36, 2},
		{"筒井", 418, 448, 36, 36, 4},
		{"）", 478, 448, 18, 36, 2},
		{"でも", 518, 448, 36, 36, 4},
		{"当たりだろ。", 618, 448, 36, 36, 4},
		{"言わなくても分かるよ。", 418, 508, 36, 36, 4},
	}
	regions := make([]Region, 0, len(texts))
	cursor := 0
	for _, entry := range texts {
		start := strings.Index(decoded[cursor:], entry.text)
		if start < 0 {
			panic("fixture text not found: " + entry.text)
		}
		start += cursor
		end := start + len(entry.text)
		cursor = end
		regions = append(regions, Region{
			Text: entry.text, Start: start, End: end,
			Left: entry.left, Bottom: entry.bottom,
			FontWidth: entry.fontWidth, FontHeight: entry.fontHeight,
			HorizontalSpace: entry.space, VerticalSpace: 24,
			PlaneWidth: 960, PlaneHeight: 540,
			Foreground: 0xFFFFFF,
		})
	}
	return decoded, regions
}

func TestBuildRowsSplitsTheDrawnLines(t *testing.T) {
	decoded, regions := twoRowCaption()
	rows := BuildRows(decoded, regions)

	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Bottom != 448 || rows[1].Bottom != 508 {
		t.Fatalf("rows ordered %d then %d, want the higher line first", rows[0].Bottom, rows[1].Bottom)
	}
	if rows[0].Text != "（筒井）でも　当たりだろ。" {
		t.Fatalf("first row = %q", rows[0].Text)
	}
	if rows[1].Text != "言わなくても分かるよ。" {
		t.Fatalf("second row = %q", rows[1].Text)
	}
}

func TestBuildRowsOrdersSpansAndMeasuresAdvance(t *testing.T) {
	decoded, regions := twoRowCaption()
	rows := BuildRows(decoded, regions)

	spans := rows[0].Spans
	if len(spans) != 5 {
		t.Fatalf("spans = %d, want 5", len(spans))
	}
	for i := 1; i < len(spans); i++ {
		if spans[i-1].Left >= spans[i].Left {
			t.Fatalf("spans are not ordered left to right: %+v", spans)
		}
	}
	// The advance lets a client paint one unbroken row background.
	if spans[0].Advance != 40 || spans[1].Advance != 60 {
		t.Fatalf("advances = %d, %d, want 40, 60", spans[0].Advance, spans[1].Advance)
	}
	if spans[len(spans)-1].Advance != 0 {
		t.Fatalf("the last span must not claim an advance, got %d", spans[len(spans)-1].Advance)
	}
	// The advance divided by the characters is the cell the broadcaster drew in,
	// which is the only reliable way to fit a font into the broadcast layout.
	if spans[1].Chars != 2 || spans[1].Advance != 60 {
		t.Fatalf("run metrics = %d chars over %d, want 2 over 60", spans[1].Chars, spans[1].Advance)
	}
	if spans[0].FontWidth != 18 || spans[1].FontWidth != 36 {
		t.Fatalf("half width and normal runs must keep their own font: %+v", spans[:2])
	}
}

func TestPlainTextRestoresTheBroadcastLineBreak(t *testing.T) {
	decoded, regions := twoRowCaption()
	text := PlainText(BuildRows(decoded, regions))

	if text != "（筒井）でも　当たりだろ。\n言わなくても分かるよ。" {
		t.Fatalf("plain text = %q", text)
	}
	if strings.Contains(decoded, "\n") {
		t.Fatal("the fixture must reproduce the decoder joining both rows without a break")
	}
}

func TestBuildRowsKeepsRubyAboveItsBaseText(t *testing.T) {
	// Ruby is an ordinary run in the small character size placed on its own line
	// directly above the base text.
	decoded := "漢字かんじ"
	regions := []Region{
		{Text: "漢字", Start: 0, End: len("漢字"), Left: 200, Bottom: 508,
			FontWidth: 36, FontHeight: 36, HorizontalSpace: 4, PlaneWidth: 960, PlaneHeight: 540},
		{Text: "かんじ", Start: len("漢字"), End: len(decoded), Left: 200, Bottom: 472,
			FontWidth: 18, FontHeight: 18, HorizontalSpace: 2, PlaneWidth: 960, PlaneHeight: 540},
	}

	rows := BuildRows(decoded, regions)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want the ruby to keep its own line", len(rows))
	}
	if rows[0].Bottom != 472 || rows[0].Spans[0].FontHeight != 18 {
		t.Fatalf("the small run must come first, got %+v", rows[0])
	}
	if rows[1].Spans[0].Text != "漢字" {
		t.Fatalf("base text row = %+v", rows[1])
	}
}

func TestBuildRowsIgnoresBlankRegions(t *testing.T) {
	regions := []Region{
		{Text: "  ", Left: 100, Bottom: 508, FontWidth: 36, FontHeight: 36, PlaneWidth: 960, PlaneHeight: 540},
		{Text: "本文", Start: 2, End: 2 + len("本文"), Left: 140, Bottom: 508,
			FontWidth: 36, FontHeight: 36, PlaneWidth: 960, PlaneHeight: 540},
	}
	rows := BuildRows("  本文", regions)
	if len(rows) != 1 || len(rows[0].Spans) != 1 || rows[0].Text != "本文" {
		t.Fatalf("blank regions must not create spans: %+v", rows)
	}
}

func TestPlaneOfReportsTheCaptionPlane(t *testing.T) {
	_, regions := twoRowCaption()
	if plane := PlaneOf(regions); plane.Width != 960 || plane.Height != 540 {
		t.Fatalf("plane = %+v, want 960x540", plane)
	}
	if plane := PlaneOf(nil); plane.Width != 0 || plane.Height != 0 {
		t.Fatalf("an empty caption must not claim a plane: %+v", plane)
	}
}
