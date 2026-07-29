package mpegts

import (
	"encoding/hex"
	"testing"
)

// Captured from live broadcasts: a 720x480 community channel whose display
// rectangle is the 704 active pixels of BT.601, and a 1440x1080 anamorphic HD
// service. Both declare aspect_ratio_information = 3 (16:9).
const (
	communitySequenceHex = "000001b32d01e034084d2382102020202020202020202020202020202020202020202020" +
		"20202020202020202020202020202020202020202020202020202020202020202020202020" +
		"20000001b5148200010000000001b52b0606060b020f0000"
	terrestrialSequenceHex = "000001b35a04383430d432aa1020202620262c2c2c2c2c2c343034363636343434343636" +
		"363a3a3a4444443a3a3a36363a3a404044444a4c4a464644464c4c50505060605c5c707074" +
		"8a8aa71011111212121313131314141414141515151515151616161616161619191917171717" +
		"1718181819191919191b1b1b1a1a1a191b1b1c1c1c212120202626282f2f37000001b5144200" +
		"010000000001b52b010101168221c000"
)

func decodeHex(t *testing.T, value string) []byte {
	t.Helper()
	data, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("invalid fixture: %v", err)
	}
	return data
}

func sequenceHeader(width, height int, aspect byte) []byte {
	return []byte{
		0x00, 0x00, 0x01, 0xB3,
		byte(width >> 4),
		byte((width&0x0F)<<4 | (height>>8)&0x0F),
		byte(height),
		aspect<<4 | 0x04, // frame_rate_code 4 = 30000/1001
		0x00, 0x00, 0x00, 0x00,
	}
}

func displayExtension(width, height int, colour bool) []byte {
	header := byte(2<<4 | 5<<1) // extension id 2, video_format 5 (unspecified)
	body := []byte{0x00, 0x00, 0x01, 0xB5, header}
	if colour {
		body[4] |= 0x01
		body = append(body, 0x01, 0x01, 0x01)
	}
	return append(body,
		byte(width>>6),
		byte(width&0x3F)<<2|0x02|byte(height>>13), // marker bit set
		byte(height>>5),
		byte(height&0x1F)<<3,
	)
}

func TestVideoFormatReadsAnamorphicHD(t *testing.T) {
	format, ok := ParseVideoFormat(decodeHex(t, terrestrialSequenceHex))
	if !ok {
		t.Fatal("expected a sequence header in the captured access unit")
	}
	if format.Width != 1440 || format.Height != 1080 {
		t.Fatalf("coded size = %dx%d, want 1440x1080", format.Width, format.Height)
	}
	if format.AspectNum != 16 || format.AspectDen != 9 {
		t.Fatalf("aspect = %d:%d, want 16:9", format.AspectNum, format.AspectDen)
	}
}

func TestVideoFormatNarrowsAspectToTheDisplayRectangle(t *testing.T) {
	format, ok := ParseVideoFormat(decodeHex(t, communitySequenceHex))
	if !ok {
		t.Fatal("expected a sequence header in the captured access unit")
	}
	if format.Width != 720 || format.Height != 480 {
		t.Fatalf("coded size = %dx%d, want 720x480", format.Width, format.Height)
	}
	// 16:9 covers the 704 active pixels, so the full 720 are slightly wider.
	if format.AspectNum != 20 || format.AspectDen != 11 {
		t.Fatalf("aspect = %d:%d, want 20:11", format.AspectNum, format.AspectDen)
	}
}

func TestVideoFormatReadsStandardDefinition(t *testing.T) {
	format, ok := ParseVideoFormat(sequenceHeader(720, 480, 3))
	if !ok || format.AspectNum != 16 || format.AspectDen != 9 {
		t.Fatalf("720x480 16:9 = %+v ok=%v, want 16:9", format, ok)
	}

	format, ok = ParseVideoFormat(sequenceHeader(720, 480, 2))
	if !ok || format.AspectNum != 4 || format.AspectDen != 3 {
		t.Fatalf("720x480 4:3 = %+v ok=%v, want 4:3", format, ok)
	}
}

func TestVideoFormatDerivesSquarePixelAspectFromCodedSize(t *testing.T) {
	format, ok := ParseVideoFormat(sequenceHeader(640, 480, 1))
	if !ok || format.AspectNum != 4 || format.AspectDen != 3 {
		t.Fatalf("square pixel 640x480 = %+v ok=%v, want 4:3", format, ok)
	}
}

func TestVideoFormatAcceptsColourDescription(t *testing.T) {
	access := append(sequenceHeader(720, 480, 3), displayExtension(704, 480, true)...)
	format, ok := ParseVideoFormat(access)
	if !ok || format.AspectNum != 20 || format.AspectDen != 11 {
		t.Fatalf("display extension with colour description = %+v ok=%v, want 20:11", format, ok)
	}
}

func TestVideoFormatIgnoresExtensionsOfTheNextPicture(t *testing.T) {
	access := append(sequenceHeader(720, 480, 3), 0x00, 0x00, 0x01, 0x00, 0x12, 0x34)
	access = append(access, displayExtension(352, 240, false)...)
	format, ok := ParseVideoFormat(access)
	if !ok || format.AspectNum != 16 || format.AspectDen != 9 {
		t.Fatalf("picture extension leaked into the aspect: %+v ok=%v", format, ok)
	}
}

func TestVideoFormatRejectsAccessUnitsWithoutASequenceHeader(t *testing.T) {
	if _, ok := ParseVideoFormat([]byte{0x00, 0x00, 0x01, 0x00, 0x12, 0x34, 0x56}); ok {
		t.Fatal("a picture without a sequence header must not report a format")
	}
	if _, ok := ParseVideoFormat(nil); ok {
		t.Fatal("an empty access unit must not report a format")
	}
}

func TestVideoFormatRejectsMalformedSequenceHeaders(t *testing.T) {
	truncated := sequenceHeader(720, 480, 3)[:7]
	if _, ok := ParseVideoFormat(truncated); ok {
		t.Fatal("a truncated sequence header must not report a format")
	}
	if _, ok := ParseVideoFormat(sequenceHeader(720, 480, 0)); ok {
		t.Fatal("forbidden aspect_ratio_information must not report a format")
	}
	if _, ok := ParseVideoFormat(sequenceHeader(0, 480, 3)); ok {
		t.Fatal("a zero width must not report a format")
	}
}
