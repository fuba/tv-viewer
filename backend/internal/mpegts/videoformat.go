package mpegts

// VideoFormat is what an MPEG-2 sequence header says about picture geometry.
// AspectNum:AspectDen is the ratio the *coded* picture must be displayed at,
// which broadcasts almost never state directly: 1440x1080 (SAR 4:3) and
// 720x480 (SAR 32:27) are both 16:9, so the coded size alone means nothing.
type VideoFormat struct {
	Width     int
	Height    int
	AspectNum int
	AspectDen int
}

const (
	maxCodedWidth  = 4096
	maxCodedHeight = 2160
)

// displayAspects maps aspect_ratio_information (ISO/IEC 13818-2 table 6-3) to a
// display aspect ratio. Value 1 means square pixels and is derived per picture.
var displayAspects = map[byte][2]int{
	2: {4, 3},
	3: {16, 9},
	4: {221, 100},
}

// ParseVideoFormat reads sequence_header() and, when present,
// sequence_display_extension() from an MPEG-2 access unit. It reports false for
// access units that carry no sequence header, which is most of them.
func ParseVideoFormat(data []byte) (VideoFormat, bool) {
	header := indexStartCode(data, 0xB3, 0)
	if header < 0 || header+8 > len(data) {
		return VideoFormat{}, false
	}
	body := data[header+4:]
	width := int(body[0])<<4 | int(body[1])>>4
	height := int(body[1]&0x0F)<<8 | int(body[2])
	aspect := body[3] >> 4
	if width <= 0 || width > maxCodedWidth || height <= 0 || height > maxCodedHeight || aspect == 0 || aspect > 4 {
		return VideoFormat{}, false
	}

	// The declared ratio applies to the display rectangle, which may be narrower
	// than the coded picture (704 of 720 active pixels, for example).
	displayWidth, displayHeight := width, height
	if w, h, ok := parseDisplayExtension(data, header+4); ok && w <= width && h <= height {
		displayWidth, displayHeight = w, h
	}

	// Square pixels mean the display rectangle already carries the ratio.
	ratio, ok := displayAspects[aspect]
	if !ok {
		ratio = [2]int{displayWidth, displayHeight}
	}

	num := ratio[0] * displayHeight * width
	den := ratio[1] * displayWidth * height
	divisor := greatestCommonDivisor(num, den)
	return VideoFormat{Width: width, Height: height, AspectNum: num / divisor, AspectDen: den / divisor}, true
}

// parseDisplayExtension reads sequence_display_extension() from the extension
// data that follows a sequence header, stopping at the first picture.
func parseDisplayExtension(data []byte, from int) (int, int, bool) {
	limit := indexStartCode(data, 0x00, from)
	for offset := from; ; {
		extension := indexStartCode(data, 0xB5, offset)
		if extension < 0 || (limit >= 0 && extension > limit) || extension+9 > len(data) {
			return 0, 0, false
		}
		body := data[extension+4:]
		if body[0]>>4 != 2 { // extension_start_code_identifier
			offset = extension + 4
			continue
		}
		// video_format (3 bits) and colour_description (1 bit) follow the id,
		// then an optional 3 byte colour description, then the display size.
		size := 1
		if body[0]&0x01 == 1 {
			size += 3
		}
		if extension+4+size+4 > len(data) {
			return 0, 0, false
		}
		width := int(body[size])<<6 | int(body[size+1])>>2
		height := int(body[size+1]&0x01)<<13 | int(body[size+2])<<5 | int(body[size+3])>>3
		if width <= 0 || width > maxCodedWidth || height <= 0 || height > maxCodedHeight {
			return 0, 0, false
		}
		return width, height, true
	}
}

func greatestCommonDivisor(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}
