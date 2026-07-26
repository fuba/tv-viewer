//go:build native && cgo

package nativevideo

/*
#cgo LDFLAGS: -lmpeg2
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <mpeg2dec/mpeg2.h>

typedef struct {
	mpeg2dec_t *decoder;
	uint8_t *frames;
	uint8_t *input;
	size_t y_size;
	size_t chroma_size;
	size_t frame_size;
	size_t input_capacity;
	unsigned int width;
	unsigned int height;
	unsigned int chroma_width;
	unsigned int chroma_height;
	unsigned int source_stride;
	unsigned int source_chroma_stride;
	unsigned int picture_type[4];
	int interlaced[4];
	int top_field_first[4];
	unsigned int temporal_reference[4];
	unsigned int output_count;
} tv_mpeg2_decoder;

static tv_mpeg2_decoder *tv_mpeg2_new(void) {
	tv_mpeg2_decoder *decoder = calloc(1, sizeof(*decoder));
	if (!decoder) return NULL;
	decoder->decoder = mpeg2_init();
	if (!decoder->decoder) {
		free(decoder);
		return NULL;
	}
	return decoder;
}

static void tv_mpeg2_free(tv_mpeg2_decoder *decoder) {
	if (!decoder) return;
	mpeg2_close(decoder->decoder);
	free(decoder->frames);
	free(decoder->input);
	free(decoder);
}

static int tv_mpeg2_resize(tv_mpeg2_decoder *decoder, const mpeg2_sequence_t *sequence) {
	unsigned int width = sequence->picture_width ? sequence->picture_width : sequence->width;
	unsigned int height = sequence->picture_height ? sequence->picture_height : sequence->height;
	unsigned int chroma_width = sequence->width ? width * sequence->chroma_width / sequence->width : sequence->chroma_width;
	unsigned int chroma_height = sequence->height ? height * sequence->chroma_height / sequence->height : sequence->chroma_height;
	uint8_t *new_frames;
	if (!width || !height || width > 4096 || height > 2160 || !chroma_width || !chroma_height) return -1;
	size_t y_size = (size_t)width * height;
	size_t chroma_size = (size_t)chroma_width * chroma_height;
	size_t frame_size = y_size + 2 * chroma_size;
	if (decoder->width == width && decoder->height == height &&
		decoder->chroma_width == chroma_width && decoder->chroma_height == chroma_height) return 0;
	new_frames = malloc(frame_size * 4);
	if (!new_frames) return -1;
	free(decoder->frames);
	decoder->frames = new_frames;
	decoder->y_size = y_size;
	decoder->chroma_size = chroma_size;
	decoder->frame_size = frame_size;
	decoder->width = width;
	decoder->height = height;
	decoder->chroma_width = chroma_width;
	decoder->chroma_height = chroma_height;
	decoder->source_stride = sequence->width;
	decoder->source_chroma_stride = sequence->chroma_width;
	// Keep libmpeg2 output tightly packed for the NV12 upload path.
	mpeg2_stride(decoder->decoder, (int)sequence->width);
	return 0;
}

static int tv_mpeg2_decode(tv_mpeg2_decoder *decoder, const uint8_t *data, size_t size) {
	const mpeg2_info_t *info;
	if (!decoder || !data || size == 0 || size > 16 * 1024 * 1024) return -1;
	if (decoder->input_capacity < size) {
		uint8_t *resized = realloc(decoder->input, size);
		if (!resized) return -2;
		decoder->input = resized;
		decoder->input_capacity = size;
	}
	memcpy(decoder->input, data, size);
	decoder->output_count = 0;
	info = mpeg2_info(decoder->decoder);
	mpeg2_buffer(decoder->decoder, decoder->input, decoder->input + size);
	for (;;) {
		mpeg2_state_t state = mpeg2_parse(decoder->decoder);
		if (state == STATE_BUFFER || state == STATE_INVALID) break;
		if (!info->sequence || !info->display_fbuf) continue;
		if (tv_mpeg2_resize(decoder, info->sequence) != 0) return -2;
		// libmpeg2 documents display_fbuf as complete at these states.
		if (state != STATE_SLICE && state != STATE_END && state != STATE_INVALID_END) continue;
		if (!info->display_fbuf->buf[0] || !info->display_fbuf->buf[1] || !info->display_fbuf->buf[2]) continue;
		if (!info->display_picture) continue;
		if (decoder->output_count >= 4) return -3;
		{
			unsigned int row;
			unsigned int slot = decoder->output_count;
			uint8_t *y = decoder->frames + slot * decoder->frame_size;
			uint8_t *u = y + decoder->y_size;
			uint8_t *v = u + decoder->chroma_size;
			for (row = 0; row < decoder->height; row++)
				memcpy(y + row * decoder->width, info->display_fbuf->buf[0] + row * decoder->source_stride, decoder->width);
			for (row = 0; row < decoder->chroma_height; row++) {
				memcpy(u + row * decoder->chroma_width, info->display_fbuf->buf[1] + row * decoder->source_chroma_stride, decoder->chroma_width);
				memcpy(v + row * decoder->chroma_width, info->display_fbuf->buf[2] + row * decoder->source_chroma_stride, decoder->chroma_width);
			}
		}
		decoder->picture_type[decoder->output_count] = info->display_picture->flags & PIC_MASK_CODING_TYPE;
		decoder->interlaced[decoder->output_count] = !(info->display_picture->flags & PIC_FLAG_PROGRESSIVE_FRAME);
		decoder->top_field_first[decoder->output_count] = !!(info->display_picture->flags & PIC_FLAG_TOP_FIELD_FIRST);
		decoder->temporal_reference[decoder->output_count] = info->display_picture->temporal_reference;
		decoder->output_count++;
		if (state == STATE_END || state == STATE_INVALID_END) break;
	}
	return decoder->output_count;
}

static uint8_t *tv_mpeg2_plane(tv_mpeg2_decoder *decoder, unsigned int slot, unsigned int plane) {
	uint8_t *y;
	if (!decoder || slot >= decoder->output_count) return NULL;
	y = decoder->frames + slot * decoder->frame_size;
	if (plane == 0) return y;
	if (plane == 1) return y + decoder->y_size;
	if (plane == 2) return y + decoder->y_size + decoder->chroma_size;
	return NULL;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Decoder wraps libmpeg2 and returns copied planar frames so callers can
// safely hand them to a GPU encoder after Decode returns.
type Decoder struct {
	handle *C.tv_mpeg2_decoder
	frames [4]YUVFrame
}

func NewDecoder() (*Decoder, error) {
	handle := C.tv_mpeg2_new()
	if handle == nil {
		return nil, ErrUnavailable
	}
	return &Decoder{handle: handle}, nil
}

func (d *Decoder) Decode(data []byte) ([]YUVFrame, error) {
	if d == nil || d.handle == nil {
		return nil, ErrUnavailable
	}
	if len(data) == 0 {
		return nil, nil
	}
	result := C.tv_mpeg2_decode(d.handle, (*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)))
	if result < 0 {
		return nil, fmt.Errorf("libmpeg2 decode failed: %d", int(result))
	}
	if result == 0 {
		return nil, nil
	}
	count := int(d.handle.output_count)
	if count > len(d.frames) {
		return nil, fmt.Errorf("libmpeg2 returned too many frames: %d", count)
	}
	ySize := int(d.handle.y_size)
	chromaSize := int(d.handle.chroma_size)
	for i := 0; i < count; i++ {
		frame := &d.frames[i]
		frame.Y = copyPlane(frame.Y, C.tv_mpeg2_plane(d.handle, C.uint(i), 0), ySize)
		frame.U = copyPlane(frame.U, C.tv_mpeg2_plane(d.handle, C.uint(i), 1), chromaSize)
		frame.V = copyPlane(frame.V, C.tv_mpeg2_plane(d.handle, C.uint(i), 2), chromaSize)
		frame.Width = int(d.handle.width)
		frame.Height = int(d.handle.height)
		frame.ChromaWidth = int(d.handle.chroma_width)
		frame.ChromaHeight = int(d.handle.chroma_height)
		frame.PictureType = byte(d.handle.picture_type[i])
		frame.Interlaced = d.handle.interlaced[i] != 0
		frame.TopFieldFirst = d.handle.top_field_first[i] != 0
		frame.TemporalReference = uint16(d.handle.temporal_reference[i])
	}
	return d.frames[:count], nil
}

func copyPlane(destination []byte, source *C.uint8_t, size int) []byte {
	if cap(destination) < size {
		destination = make([]byte, size)
	} else {
		destination = destination[:size]
	}
	copy(destination, unsafe.Slice((*byte)(unsafe.Pointer(source)), size))
	return destination
}

func (d *Decoder) Close() error {
	if d != nil && d.handle != nil {
		C.tv_mpeg2_free(d.handle)
		d.handle = nil
	}
	return nil
}
