//go:build native && cgo

package nativecaption

/*
#cgo LDFLAGS: -laribb24
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <aribb24/aribb24.h>
#include <aribb24/parser.h>
#include <aribb24/decoder.h>

#define TV_CAPTION_MAX_REGIONS 64

// One drawn caption region. ARIB places every row - and every ruby annotation -
// as its own region on a fixed caption plane, so position and character size are
// the only way to recover line breaks, ruby and placement.
typedef struct {
	size_t text_offset;
	size_t text_length;
	int foreground_color;
	int background_color;
	int foreground_alpha;
	int background_alpha;
	int plane_width;
	int plane_height;
	int width;
	int height;
	int font_width;
	int font_height;
	int vertical_interval;
	int horizontal_interval;
	int char_left;
	int char_bottom;
	int vertical_adjust;
	int horizontal_adjust;
} tv_caption_region;

typedef struct {
	arib_instance_t *instance;
	arib_parser_t *parser;
	arib_decoder_t *decoder;
	uint8_t *input;
	size_t input_capacity;
	char *output;
	size_t output_capacity;
} tv_caption_decoder;

static tv_caption_decoder *tv_caption_new(void) {
	tv_caption_decoder *state = calloc(1, sizeof(*state));
	if (!state) return NULL;
	state->instance = arib_instance_new(NULL);
	if (!state->instance) goto fail;
	state->parser = arib_get_parser(state->instance);
	state->decoder = arib_get_decoder(state->instance);
	if (!state->parser || !state->decoder) goto fail;
	arib_initialize_decoder_a_profile(state->decoder);
	return state;
fail:
	if (state->instance) arib_instance_destroy(state->instance);
	free(state);
	return NULL;
}

static void tv_caption_free(tv_caption_decoder *state) {
	if (!state) return;
	if (state->decoder) arib_finalize_decoder(state->decoder);
	if (state->instance) arib_instance_destroy(state->instance);
	free(state->input);
	free(state->output);
	free(state);
}

static int tv_caption_collect_regions(tv_caption_decoder *state, tv_caption_region *out, int capacity) {
	const arib_buf_region_t *region;
	int count = 0;
	if (!state || !out || capacity <= 0) return 0;
	for (region = arib_decoder_get_regions(state->decoder); region && count < capacity; region = region->p_next) {
		if (!region->p_start || !region->p_end || region->p_end < region->p_start) continue;
		if (region->p_start < state->output || region->p_end > state->output + state->output_capacity) continue;
		out[count].text_offset = (size_t)(region->p_start - state->output);
		out[count].text_length = (size_t)(region->p_end - region->p_start);
		out[count].foreground_color = region->i_foreground_color;
		out[count].background_color = region->i_background_color;
		out[count].foreground_alpha = region->i_foreground_alpha;
		out[count].background_alpha = region->i_background_alpha;
		out[count].plane_width = region->i_planewidth;
		out[count].plane_height = region->i_planeheight;
		out[count].width = region->i_width;
		out[count].height = region->i_height;
		out[count].font_width = region->i_fontwidth;
		out[count].font_height = region->i_fontheight;
		out[count].vertical_interval = region->i_verint;
		out[count].horizontal_interval = region->i_horint;
		out[count].char_left = region->i_charleft;
		out[count].char_bottom = region->i_charbottom;
		out[count].vertical_adjust = region->i_veradj;
		out[count].horizontal_adjust = region->i_horadj;
		count++;
	}
	return count;
}

static int tv_caption_decode(tv_caption_decoder *state, const uint8_t *data, size_t size,
	char **output, size_t *output_size, int64_t *duration_us, int *parsed,
	tv_caption_region *regions, int region_capacity, int *region_count) {
	const unsigned char *parsed_data;
	size_t parsed_size = 0;
	size_t required;
	size_t decoded_size;
	if (!state || !data || !size || size > 8 * 1024 * 1024) return -1;
	if (state->input_capacity < size) {
		uint8_t *resized = realloc(state->input, size);
		if (!resized) return -2;
		state->input = resized;
		state->input_capacity = size;
	}
	memcpy(state->input, data, size);
	arib_parse_pes(state->parser, state->input, size);
	parsed_data = arib_parser_get_data(state->parser, &parsed_size);
	*parsed = parsed_data && parsed_size;
	*region_count = 0;
	if (!*parsed) {
		*output = NULL;
		*output_size = 0;
		*duration_us = 0;
		return 0;
	}
	if (parsed_size > 256 * 1024) return -3;
	required = parsed_size * 4 + 1;
	if (state->output_capacity < required) {
		char *resized = realloc(state->output, required);
		if (!resized) return -2;
		state->output = resized;
		state->output_capacity = required;
	}
	decoded_size = arib_decode_buffer(state->decoder, parsed_data, parsed_size,
		state->output, state->output_capacity - 1);
	if (decoded_size >= state->output_capacity) return -4;
	state->output[decoded_size] = '\0';
	*output = state->output;
	*output_size = decoded_size;
	*duration_us = (int64_t)arib_decoder_get_time(state->decoder);
	// The regions point into the decoded buffer and are released by finalize.
	*region_count = tv_caption_collect_regions(state, regions, region_capacity);
	arib_finalize_decoder(state->decoder);
	arib_initialize_decoder_a_profile(state->decoder);
	return 0;
}
*/
import "C"

import (
	"fmt"
	"strings"
	"time"
	"unsafe"
)

type Decoder struct {
	handle *C.tv_caption_decoder
}

func NewDecoder() (*Decoder, error) {
	handle := C.tv_caption_new()
	if handle == nil {
		return nil, fmt.Errorf("ARIB caption decoder initialization failed")
	}
	return &Decoder{handle: handle}, nil
}

func (d *Decoder) DecodePES(data []byte) (Caption, bool, error) {
	if d == nil || d.handle == nil || len(data) == 0 {
		return Caption{}, false, nil
	}
	var output *C.char
	var outputSize C.size_t
	var durationUS C.int64_t
	var parsed C.int
	var regionCount C.int
	regions := make([]C.tv_caption_region, C.TV_CAPTION_MAX_REGIONS)
	status := C.tv_caption_decode(d.handle, (*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		&output, &outputSize, &durationUS, &parsed, &regions[0], C.int(len(regions)), &regionCount)
	if status != 0 {
		return Caption{}, false, fmt.Errorf("ARIB caption decode failed: status=%d", int(status))
	}
	if parsed == 0 {
		return Caption{}, false, nil
	}
	decoded := ""
	if output != nil && outputSize > 0 {
		decoded = C.GoStringN(output, C.int(outputSize))
	}
	placed := collectRegions(decoded, regions[:int(regionCount)])
	rows := BuildRows(decoded, placed)
	text := PlainText(rows)
	if text == "" {
		text = strings.TrimSpace(decoded)
	}
	caption := Caption{
		Text:     text,
		Duration: time.Duration(durationUS) * time.Microsecond,
		Plane:    PlaneOf(placed),
		Rows:     rows,
	}
	return caption, true, nil
}

func collectRegions(decoded string, regions []C.tv_caption_region) []Region {
	collected := make([]Region, 0, len(regions))
	for i := range regions {
		start := int(regions[i].text_offset)
		end := start + int(regions[i].text_length)
		if start < 0 || end > len(decoded) || start >= end {
			continue
		}
		text := strings.TrimRight(decoded[start:end], "\r\n")
		if strings.TrimSpace(text) == "" {
			continue
		}
		collected = append(collected, Region{
			Text:            text,
			Start:           start,
			End:             start + len(text),
			Left:            int(regions[i].char_left),
			Bottom:          int(regions[i].char_bottom),
			FontWidth:       int(regions[i].font_width),
			FontHeight:      int(regions[i].font_height),
			HorizontalSpace: int(regions[i].horizontal_interval),
			VerticalSpace:   int(regions[i].vertical_interval),
			PlaneWidth:      int(regions[i].plane_width),
			PlaneHeight:     int(regions[i].plane_height),
			Foreground:      int(regions[i].foreground_color),
			Background:      int(regions[i].background_color),
			ForegroundAlpha: int(regions[i].foreground_alpha),
			BackgroundAlpha: int(regions[i].background_alpha),
		})
	}
	return collected
}

func (d *Decoder) Close() error {
	if d != nil && d.handle != nil {
		C.tv_caption_free(d.handle)
		d.handle = nil
	}
	return nil
}
