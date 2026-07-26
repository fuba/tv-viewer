//go:build native && cgo

package nativeaudio

/*
#cgo LDFLAGS: -lfaad -lopus
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <faad.h>
#include <opus/opus.h>

typedef struct {
	NeAACDecHandle handle;
	unsigned long sample_rate;
	unsigned char channels;
	int initialized;
} tv_aac_decoder;

static tv_aac_decoder *tv_aac_new(void) {
	tv_aac_decoder *d = calloc(1, sizeof(*d));
	if (!d) return NULL;
	d->handle = NeAACDecOpen();
	if (!d->handle) { free(d); return NULL; }
	NeAACDecConfigurationPtr config = NeAACDecGetCurrentConfiguration(d->handle);
	config->outputFormat = FAAD_FMT_16BIT;
	config->dontUpSampleImplicitSBR = 1;
	NeAACDecSetConfiguration(d->handle, config);
	return d;
}

static int tv_aac_decode(tv_aac_decoder *d, const uint8_t *data, unsigned int size, int16_t **pcm, unsigned int *samples, unsigned int *rate, unsigned int *channels) {
	NeAACDecFrameInfo info;
	void *decoded;
	if (!d || !data || !size) return -1;
	memset(&info, 0, sizeof(info));
	if (!d->initialized) {
		unsigned long rate0 = 0;
		unsigned char channels0 = 0;
		long init = NeAACDecInit(d->handle, (unsigned char *)data, size, &rate0, &channels0);
		if (init < 0) return -2;
		d->sample_rate = rate0;
		d->channels = channels0;
		d->initialized = 1;
	}
	decoded = NeAACDecDecode(d->handle, &info, (unsigned char *)data, size);
	if (info.error || !decoded) return info.error ? -(int)info.error : -3;
	*pcm = (int16_t *)decoded;
	*samples = info.samples;
	*rate = info.samplerate;
	*channels = info.channels;
	return 0;
}

static void tv_aac_free(tv_aac_decoder *d) {
	if (!d) return;
	if (d->handle) NeAACDecClose(d->handle);
	free(d);
}

typedef struct {
	OpusEncoder *handle;
	int channels;
	} tv_opus_encoder;

static tv_opus_encoder *tv_opus_new(int channels) {
	int error = OPUS_OK;
	tv_opus_encoder *e = calloc(1, sizeof(*e));
	if (!e || (channels != 1 && channels != 2)) { free(e); return NULL; }
	e->channels = channels;
	e->handle = opus_encoder_create(48000, channels, OPUS_APPLICATION_AUDIO, &error);
	if (!e->handle || error != OPUS_OK) { free(e); return NULL; }
	opus_encoder_ctl(e->handle, OPUS_SET_BITRATE(128000));
	opus_encoder_ctl(e->handle, OPUS_SET_VBR(1));
	opus_encoder_ctl(e->handle, OPUS_SET_COMPLEXITY(5));
	return e;
}

static int tv_opus_encode(tv_opus_encoder *e, const int16_t *pcm, unsigned int samples, uint8_t *out, unsigned int capacity) {
	int frames = (int)(samples / 960);
	if (!e || !pcm || !out || frames < 1 || capacity < 4096) return -1;
	return opus_encode(e->handle, pcm, 960, out, capacity);
}

static void tv_opus_free(tv_opus_encoder *e) {
	if (!e) return;
	if (e->handle) opus_encoder_destroy(e->handle);
	free(e);
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type Decoder struct{ handle *C.tv_aac_decoder }

func NewDecoder() (*Decoder, error) {
	h := C.tv_aac_new()
	if h == nil {
		return nil, ErrUnavailable
	}
	return &Decoder{handle: h}, nil
}

func (d *Decoder) DecodeADTS(data []byte) ([]PCMFrame, error) {
	if d == nil || d.handle == nil {
		return nil, ErrUnavailable
	}
	if len(data) == 0 {
		return nil, nil
	}
	var pcm *C.int16_t
	var samples, rate, channels C.uint
	if status := C.tv_aac_decode(d.handle, (*C.uint8_t)(unsafe.Pointer(&data[0])), C.uint(len(data)), &pcm, &samples, &rate, &channels); status != 0 {
		return nil, fmt.Errorf("AAC decode failed: status=%d", int(status))
	}
	values := unsafe.Slice((*int16)(unsafe.Pointer(pcm)), int(samples))
	copyValues := append([]int16(nil), values...)
	return []PCMFrame{{Data: copyValues, SampleRate: int(rate), Channels: int(channels)}}, nil
}

// Reset recreates FAAD so it can accept a broadcast AAC configuration change.
func (d *Decoder) Reset() error {
	if d == nil {
		return ErrUnavailable
	}
	if d.handle != nil {
		C.tv_aac_free(d.handle)
	}
	d.handle = C.tv_aac_new()
	if d.handle == nil {
		return ErrUnavailable
	}
	return nil
}

func (d *Decoder) Close() error {
	if d != nil && d.handle != nil {
		C.tv_aac_free(d.handle)
		d.handle = nil
	}
	return nil
}

type Encoder struct {
	handle   *C.tv_opus_encoder
	channels int
	output   []byte
}

func NewEncoder(channels int) (*Encoder, error) {
	h := C.tv_opus_new(C.int(channels))
	if h == nil {
		return nil, fmt.Errorf("Opus encoder unavailable for %d channels", channels)
	}
	return &Encoder{handle: h, channels: channels, output: make([]byte, 4096)}, nil
}

func (e *Encoder) Encode(pcm []int16) (OpusFrame, error) {
	if e == nil || e.handle == nil {
		return OpusFrame{}, ErrUnavailable
	}
	if len(pcm) < 960*e.channels {
		return OpusFrame{}, nil
	}
	size := C.tv_opus_encode(e.handle, (*C.int16_t)(unsafe.Pointer(&pcm[0])), C.uint(len(pcm)/e.channels), (*C.uint8_t)(unsafe.Pointer(&e.output[0])), C.uint(len(e.output)))
	if size < 0 {
		return OpusFrame{}, fmt.Errorf("Opus encode failed: status=%d", int(size))
	}
	return OpusFrame{Data: append([]byte(nil), e.output[:int(size)]...), Duration: 20}, nil
}

func (e *Encoder) Close() error {
	if e != nil && e.handle != nil {
		C.tv_opus_free(e.handle)
		e.handle = nil
	}
	return nil
}
