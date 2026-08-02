package nativecaption

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// ARIB draws captions on a fixed plane (960x540 for HD) and places every run of
// characters at an absolute position on it. Rows, ruby and placement are carried
// by that geometry alone: the decoded text is a flat string in which two rows are
// concatenated with nothing between them, and ruby is an ordinary run that
// happens to use the small character size directly above its base text.

// Region is one run of characters as the decoder placed it on the caption plane.
type Region struct {
	Text            string
	Start           int // byte offsets into the decoded text
	End             int
	Left            int
	Bottom          int
	FontWidth       int
	FontHeight      int
	HorizontalSpace int
	VerticalSpace   int
	PlaneWidth      int
	PlaneHeight     int
	Foreground      int
	Background      int
	ForegroundAlpha int
	BackgroundAlpha int
}

// Span is one run of characters within a row, in plane coordinates. Left and
// Width bound the character blocks the run occupies, which is what the drawn
// background covers - the glyphs themselves are inset by the character spacing.
type Span struct {
	Text            string
	Left            int
	Width           int
	Chars           int
	FontWidth       int
	FontHeight      int
	HorizontalSpace int
	Foreground      int
	Background      int
	ForegroundAlpha int
	BackgroundAlpha int
}

// Row is one caption line. Bottom is the lower edge of its character blocks and
// Height their full height, so consecutive rows tile without a seam, the way a
// caption is drawn on television.
type Row struct {
	Text   string
	Bottom int
	Height int
	Spans  []Span
}

// Plane is the coordinate system the rows are placed on.
type Plane struct {
	Width  int
	Height int
}

type Caption struct {
	Text     string
	Duration time.Duration
	Plane    Plane
	Rows     []Row
}

// BuildRows turns placed regions into rows ordered top to bottom, with the
// spans of each row ordered left to right. Ruby keeps its own row because it
// sits above the base text, which is exactly where the broadcaster put it.
func BuildRows(decoded string, regions []Region) []Row {
	grouped := make(map[int][]Region)
	order := make([]int, 0, len(regions))
	for _, region := range regions {
		if region.Text == "" {
			continue
		}
		if _, seen := grouped[region.Bottom]; !seen {
			order = append(order, region.Bottom)
		}
		grouped[region.Bottom] = append(grouped[region.Bottom], region)
	}
	sort.Ints(order)

	rows := make([]Row, 0, len(order))
	for _, bottom := range order {
		members := grouped[bottom]
		sort.SliceStable(members, func(i, j int) bool { return members[i].Left < members[j].Left })
		// charbottom is the last line of the block, so the block edge is one below.
		row := Row{Bottom: bottom + 1, Spans: make([]Span, 0, len(members))}
		rightEdge := 0
		for i, region := range members {
			chars := utf8.RuneCountInString(region.Text)
			cellWidth := region.FontWidth + region.HorizontalSpace
			cellHeight := region.FontHeight + region.VerticalSpace
			if cellHeight > row.Height {
				row.Height = cellHeight
			}
			// The decoder advances the cursor past the first character before it
			// records the region, so a run starts one character block earlier -
			// except after the punctuation it treats specially (、。→), where the
			// correction overshoots. Cells are never drawn on top of each other,
			// so a run that lands inside its predecessor starts where that ended.
			left := region.Left - cellWidth
			if i > 0 && left < rightEdge {
				left = rightEdge
			}
			rightEdge = left + chars*cellWidth
			row.Spans = append(row.Spans, Span{
				Text:            region.Text,
				Left:            left,
				Width:           chars * cellWidth,
				Chars:           chars,
				FontWidth:       region.FontWidth,
				FontHeight:      region.FontHeight,
				HorizontalSpace: region.HorizontalSpace,
				Foreground:      region.Foreground,
				Background:      region.Background,
				ForegroundAlpha: region.ForegroundAlpha,
				BackgroundAlpha: region.BackgroundAlpha,
			})
		}
		row.Text = rowText(decoded, members)
		rows = append(rows, row)
	}
	return rows
}

// rowText rebuilds the readable line, keeping the blanks the decoder wrote
// between runs and dropping whatever belongs to another row.
func rowText(decoded string, members []Region) string {
	ordered := append([]Region(nil), members...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })
	var builder strings.Builder
	previousEnd := -1
	for _, region := range ordered {
		if previousEnd >= 0 && region.Start > previousEnd &&
			region.Start <= len(decoded) && previousEnd <= len(decoded) {
			between := decoded[previousEnd:region.Start]
			if strings.TrimSpace(between) == "" {
				builder.WriteString(between)
			}
		}
		builder.WriteString(region.Text)
		previousEnd = region.End
	}
	return strings.TrimSpace(builder.String())
}

// PlainText joins the rows the way they are drawn, so a client without the
// layout still gets the broadcaster's line breaks instead of one run-on line.
func PlainText(rows []Row) string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Text == "" {
			continue
		}
		lines = append(lines, row.Text)
	}
	return strings.Join(lines, "\n")
}

// PlaneOf reports the caption plane the regions were placed on.
func PlaneOf(regions []Region) Plane {
	for _, region := range regions {
		if region.PlaneWidth > 0 && region.PlaneHeight > 0 {
			return Plane{Width: region.PlaneWidth, Height: region.PlaneHeight}
		}
	}
	return Plane{}
}
