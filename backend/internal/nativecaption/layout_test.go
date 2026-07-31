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
	// charbottom is the last line of the block, so a row reports the edge below it
	// and consecutive rows tile: 389..449 and 449..509 for a 60 high block.
	if rows[0].Bottom != 449 || rows[1].Bottom != 509 {
		t.Fatalf("rows ordered %d then %d, want the higher line first", rows[0].Bottom, rows[1].Bottom)
	}
	if rows[0].Height != 60 || rows[1].Height != 60 {
		t.Fatalf("row heights = %d, %d, want the full character block of 60", rows[0].Height, rows[1].Height)
	}
	if rows[0].Bottom-rows[0].Height != 389 || rows[1].Bottom-rows[1].Height != rows[0].Bottom {
		t.Fatal("consecutive rows must tile without a seam")
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
	// The decoder records a region after the cursor has passed its first
	// character, so a run starts one character block before the reported left.
	// Corrected, the runs of a row are exactly adjacent.
	if spans[0].Left != 358 || spans[0].Width != 20 {
		t.Fatalf("half width run = %d wide at %d, want 20 at 358", spans[0].Width, spans[0].Left)
	}
	if spans[1].Left != 378 || spans[1].Width != 80 {
		t.Fatalf("normal run = %d wide at %d, want 80 at 378", spans[1].Width, spans[1].Left)
	}
	for i := 1; i < len(spans); i++ {
		if gap := spans[i].Left - (spans[i-1].Left + spans[i-1].Width); gap < 0 {
			t.Fatalf("runs overlap by %d: %+v", -gap, spans[i-1:i+1])
		}
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
	if rows[0].Bottom != 473 || rows[0].Spans[0].FontHeight != 18 {
		t.Fatalf("the small run must come first, got %+v", rows[0])
	}
	if rows[1].Spans[0].Text != "漢字" {
		t.Fatalf("base text row = %+v", rows[1])
	}
}

func TestBuildRowsNeverOverlapsRuns(t *testing.T) {
	// Captured live: an arrow after a full stop, where the decoder reports a
	// position that would put the arrow inside the preceding run.
	regions := []Region{
		{Text: "それだけじゃない。", Start: 0, End: len("それだけじゃない。"), Left: 458, Bottom: 508,
			FontWidth: 36, FontHeight: 36, HorizontalSpace: 4, VerticalSpace: 24,
			PlaneWidth: 960, PlaneHeight: 540},
		{Text: "→", Start: len("それだけじゃない。"), End: len("それだけじゃない。→"), Left: 778, Bottom: 508,
			FontWidth: 18, FontHeight: 36, HorizontalSpace: 2, VerticalSpace: 24,
			PlaneWidth: 960, PlaneHeight: 540},
	}
	spans := BuildRows("それだけじゃない。→", regions)[0].Spans
	if spans[0].Left != 418 || spans[0].Width != 360 {
		t.Fatalf("run = %d wide at %d, want 360 at 418", spans[0].Width, spans[0].Left)
	}
	if spans[1].Left != 778 {
		t.Fatalf("the arrow starts at %d, want it snapped to 778 where the run ends", spans[1].Left)
	}
}

func TestBuildRowsKeepsDrawnSpaces(t *testing.T) {
	// A space a broadcaster draws carries the caption background, so dropping it
	// would tear the black band the caption is written on.
	regions := []Region{
		{Text: "本文", Start: 0, End: len("本文"), Left: 140, Bottom: 508,
			FontWidth: 36, FontHeight: 36, HorizontalSpace: 4, PlaneWidth: 960, PlaneHeight: 540},
		{Text: "　", Start: len("本文"), End: len("本文　"), Left: 220, Bottom: 508,
			FontWidth: 36, FontHeight: 36, HorizontalSpace: 4, PlaneWidth: 960, PlaneHeight: 540},
	}
	rows := BuildRows("本文　", regions)
	if len(rows) != 1 || len(rows[0].Spans) != 2 {
		t.Fatalf("a drawn space must keep its cell: %+v", rows)
	}
	first, second := rows[0].Spans[0], rows[0].Spans[1]
	if first.Left+first.Width != second.Left {
		t.Fatalf("the space must continue the band: %+v", rows[0].Spans)
	}
}

func TestBuildRowsIgnoresEmptyRegions(t *testing.T) {
	regions := []Region{
		{Text: "", Left: 100, Bottom: 508, FontWidth: 36, FontHeight: 36, PlaneWidth: 960, PlaneHeight: 540},
		{Text: "本文", Left: 140, Bottom: 508, FontWidth: 36, FontHeight: 36, PlaneWidth: 960, PlaneHeight: 540},
	}
	if rows := BuildRows("本文", regions); len(rows) != 1 || len(rows[0].Spans) != 1 {
		t.Fatalf("an empty region must not create a span: %+v", rows)
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
