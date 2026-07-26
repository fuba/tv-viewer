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

static int tv_caption_decode(tv_caption_decoder *state, const uint8_t *data, size_t size,
	char **output, size_t *output_size, int64_t *duration_us, int *parsed) {
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

type Caption struct {
	Text     string
	Duration time.Duration
}

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
	status := C.tv_caption_decode(d.handle, (*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		&output, &outputSize, &durationUS, &parsed)
	if status != 0 {
		return Caption{}, false, fmt.Errorf("ARIB caption decode failed: status=%d", int(status))
	}
	if parsed == 0 {
		return Caption{}, false, nil
	}
	text := ""
	if output != nil && outputSize > 0 {
		text = strings.TrimSpace(C.GoStringN(output, C.int(outputSize)))
	}
	return Caption{Text: text, Duration: time.Duration(durationUS) * time.Microsecond}, true, nil
}

func (d *Decoder) Close() error {
	if d != nil && d.handle != nil {
		C.tv_caption_free(d.handle)
		d.handle = nil
	}
	return nil
}
