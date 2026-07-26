//go:build native && cgo

package nativevideo

/*
#cgo CFLAGS: -I/usr/local/include
#cgo LDFLAGS: -ldl
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <dlfcn.h>
#include <ffnvcodec/dynlink_cuda.h>
#include <ffnvcodec/dynlink_cuviddec.h>
#include <ffnvcodec/dynlink_nvcuvid.h>

#define TV_NVDEC_MAX_OUTPUTS 16

typedef struct {
	void *cuda_library;
	void *cuvid_library;
	CUcontext context;
	CUvideoparser parser;
	CUvideodecoder decoder;
	tcuInit *cu_init;
	tcuDeviceGet *cu_device_get;
	tcuCtxCreate_v2 *cu_ctx_create;
	tcuCtxDestroy_v2 *cu_ctx_destroy;
	tcuCtxPushCurrent_v2 *cu_ctx_push;
	tcuCtxPopCurrent_v2 *cu_ctx_pop;
	tcuMemcpy2D_v2 *cu_memcpy_2d;
	tcuvidGetDecoderCaps *get_caps;
	tcuvidCreateVideoParser *create_parser;
	tcuvidParseVideoData *parse_video;
	tcuvidDestroyVideoParser *destroy_parser;
	tcuvidCreateDecoder *create_decoder;
	tcuvidDestroyDecoder *destroy_decoder;
	tcuvidDecodePicture *decode_picture;
	tcuvidMapVideoFrame64 *map_frame;
	tcuvidUnmapVideoFrame64 *unmap_frame;
	uint8_t *frames;
	uint8_t *nv12_chroma;
	size_t frame_size;
	unsigned int width;
	unsigned int height;
	unsigned int chroma_width;
	unsigned int chroma_height;
	unsigned int output_count;
	unsigned char deinterlaced[TV_NVDEC_MAX_OUTPUTS];
	CUVIDPARSERDISPINFO displays[TV_NVDEC_MAX_OUTPUTS];
	int display_fields[TV_NVDEC_MAX_OUTPUTS];
	int field_counts[TV_NVDEC_MAX_OUTPUTS];
	CUdeviceptr mapped_devices[TV_NVDEC_MAX_OUTPUTS];
	unsigned int mapped_pitches[TV_NVDEC_MAX_OUTPUTS];
	int defer_output;
	int callback_error;
} tv_nvdec;

static void *tv_symbol(void *library, const char *name) {
	return library ? dlsym(library, name) : NULL;
}

static int CUDAAPI tv_nvdec_sequence(void *opaque, CUVIDEOFORMAT *format) {
	tv_nvdec *state = opaque;
	CUVIDDECODECREATEINFO create_info;
	CUVIDDECODECAPS caps;
	unsigned int width;
	unsigned int height;
	uint8_t *frames;
	uint8_t *nv12_chroma;
	if (!state || !format || format->codec != cudaVideoCodec_MPEG2 ||
		format->chroma_format != cudaVideoChromaFormat_420 ||
		format->bit_depth_luma_minus8 != 0 || format->bit_depth_chroma_minus8 != 0) return 0;
	width = (unsigned int)(format->display_area.right - format->display_area.left);
	height = (unsigned int)(format->display_area.bottom - format->display_area.top);
	if (!width || !height || width > 4096 || height > 2160 || (width & 1) || (height & 1)) return 0;
	if (state->decoder && state->width == width && state->height == height)
		return format->min_num_decode_surfaces;
	if (state->decoder) {
		state->destroy_decoder(state->decoder);
		state->decoder = NULL;
	}
	memset(&caps, 0, sizeof(caps));
	caps.eCodecType = format->codec;
	caps.eChromaFormat = format->chroma_format;
	if (state->get_caps(&caps) != CUDA_SUCCESS || !caps.bIsSupported ||
		format->coded_width > caps.nMaxWidth || format->coded_height > caps.nMaxHeight) {
		state->callback_error = -11;
		return 0;
	}
	memset(&create_info, 0, sizeof(create_info));
	create_info.CodecType = format->codec;
	create_info.ChromaFormat = format->chroma_format;
	create_info.OutputFormat = cudaVideoSurfaceFormat_NV12;
	create_info.DeinterlaceMode = cudaVideoDeinterlaceMode_Adaptive;
	create_info.ulCreationFlags = cudaVideoCreate_PreferCUVID;
	create_info.ulWidth = format->coded_width;
	create_info.ulHeight = format->coded_height;
	create_info.ulNumDecodeSurfaces = format->min_num_decode_surfaces;
	create_info.ulNumOutputSurfaces = 4;
	create_info.display_area.left = (short)format->display_area.left;
	create_info.display_area.top = (short)format->display_area.top;
	create_info.display_area.right = (short)format->display_area.right;
	create_info.display_area.bottom = (short)format->display_area.bottom;
	create_info.ulTargetWidth = width;
	create_info.ulTargetHeight = height;
	create_info.target_rect.right = (short)width;
	create_info.target_rect.bottom = (short)height;
	if (state->create_decoder(&state->decoder, &create_info) != CUDA_SUCCESS) {
		state->callback_error = -12;
		return 0;
	}
	state->frame_size = (size_t)width * height * 3 / 2;
	frames = realloc(state->frames, state->frame_size * TV_NVDEC_MAX_OUTPUTS);
	if (!frames) {
		state->destroy_decoder(state->decoder);
		state->decoder = NULL;
		state->callback_error = -13;
		return 0;
	}
	state->frames = frames;
	nv12_chroma = realloc(state->nv12_chroma, (size_t)width * height / 2);
	if (!nv12_chroma) {
		state->destroy_decoder(state->decoder);
		state->decoder = NULL;
		state->callback_error = -14;
		return 0;
	}
	state->nv12_chroma = nv12_chroma;
	state->width = width;
	state->height = height;
	state->chroma_width = width / 2;
	state->chroma_height = height / 2;
	return format->min_num_decode_surfaces;
}

static int CUDAAPI tv_nvdec_decode_picture(void *opaque, CUVIDPICPARAMS *picture) {
	tv_nvdec *state = opaque;
	if (!state || !state->decoder || !picture) return 0;
	if (state->decode_picture(state->decoder, picture) != CUDA_SUCCESS) {
		state->callback_error = -21;
		return 0;
	}
	return 1;
}

static int tv_nvdec_copy_frame(tv_nvdec *state, CUVIDPARSERDISPINFO *display, int field) {
	CUVIDPROCPARAMS processing;
	CUDA_MEMCPY2D copy;
	CUdeviceptr device = 0;
	unsigned int pitch = 0;
	uint8_t *output;
	unsigned int row;
	if (state->output_count >= TV_NVDEC_MAX_OUTPUTS) return -31;
	memset(&processing, 0, sizeof(processing));
	processing.progressive_frame = display->progressive_frame;
	processing.second_field = field & 1;
	processing.top_field_first = display->top_field_first;
	processing.unpaired_field = display->repeat_first_field < 0;
	if (state->map_frame(state->decoder, display->picture_index,
		(unsigned long long *)&device, &pitch, &processing) != CUDA_SUCCESS) return -32;
	if (pitch < state->width) {
		state->unmap_frame(state->decoder, (unsigned long long)device);
		return -36;
	}
	output = state->frames + state->frame_size * state->output_count;
	memset(&copy, 0, sizeof(copy));
	copy.srcMemoryType = CU_MEMORYTYPE_DEVICE;
	copy.srcDevice = device;
	copy.srcPitch = pitch;
	copy.dstMemoryType = CU_MEMORYTYPE_HOST;
	copy.dstHost = output;
	copy.dstPitch = state->width;
	copy.WidthInBytes = state->width;
	copy.Height = state->height;
	if (state->cu_memcpy_2d(&copy) != CUDA_SUCCESS) {
		state->unmap_frame(state->decoder, (unsigned long long)device);
		return -33;
	}
	memset(&copy, 0, sizeof(copy));
	copy.srcMemoryType = CU_MEMORYTYPE_DEVICE;
	copy.srcDevice = device + (CUdeviceptr)pitch * state->height;
	copy.srcPitch = pitch;
	copy.dstMemoryType = CU_MEMORYTYPE_HOST;
	copy.dstHost = state->nv12_chroma;
	copy.dstPitch = state->width;
	copy.WidthInBytes = state->width;
	copy.Height = state->chroma_height;
	if (state->cu_memcpy_2d(&copy) != CUDA_SUCCESS) {
		state->unmap_frame(state->decoder, (unsigned long long)device);
		return -34;
	}
	if (state->unmap_frame(state->decoder, (unsigned long long)device) != CUDA_SUCCESS) return -35;
	// Convert interleaved NV12 chroma to the planar representation used by NVENC wrapper.
	{
		uint8_t *uv = state->nv12_chroma;
		uint8_t *u = output + (size_t)state->width * state->height;
		uint8_t *v = u + (size_t)state->chroma_width * state->chroma_height;
		for (row = state->chroma_height; row-- > 0;) {
			unsigned int col;
			uint8_t *source = uv + (size_t)row * state->width;
			uint8_t *u_row = u + (size_t)row * state->chroma_width;
			uint8_t *v_row = v + (size_t)row * state->chroma_width;
			for (col = state->chroma_width; col-- > 0;) {
				u_row[col] = source[col * 2];
				v_row[col] = source[col * 2 + 1];
			}
		}
	}
	state->deinterlaced[state->output_count] = !display->progressive_frame;
	state->output_count++;
	return 0;
}

static int CUDAAPI tv_nvdec_display_picture(void *opaque, CUVIDPARSERDISPINFO *display) {
	tv_nvdec *state = opaque;
	int field_count;
	int field;
	if (!state || !display || !state->decoder) return 0;
	field_count = display->progressive_frame ? 1 : 2 + display->repeat_first_field;
	if (field_count < 1) field_count = 1;
	for (field = 0; field < field_count; field++) {
		int result;
		if (state->defer_output) {
			CUVIDPROCPARAMS processing;
			CUdeviceptr device = 0;
			unsigned int pitch = 0;
			if (state->output_count >= TV_NVDEC_MAX_OUTPUTS) {
				state->callback_error = -31;
				return 0;
			}
			memset(&processing, 0, sizeof(processing));
			processing.progressive_frame = display->progressive_frame;
			processing.second_field = field & 1;
			processing.top_field_first = display->top_field_first;
			processing.unpaired_field = display->repeat_first_field < 0;
			if (state->map_frame(state->decoder, display->picture_index,
				(unsigned long long *)&device, &pitch, &processing) != CUDA_SUCCESS) {
				state->callback_error = -32;
				return 0;
			}
			if (!device || pitch < state->width) {
				if (device) state->unmap_frame(state->decoder, (unsigned long long)device);
				state->callback_error = -32;
				return 0;
			}
			state->displays[state->output_count] = *display;
			state->display_fields[state->output_count] = field;
			state->field_counts[state->output_count] = field_count;
			state->mapped_devices[state->output_count] = device;
			state->mapped_pitches[state->output_count] = pitch;
			state->deinterlaced[state->output_count] = !display->progressive_frame;
			state->output_count++;
			result = 0;
		} else {
			result = tv_nvdec_copy_frame(state, display, field);
		}
		if (result != 0) {
			state->callback_error = result;
			return 0;
		}
	}
	return 1;
}

static void tv_nvdec_free(tv_nvdec *state);

// The caller must have made state->context current before calling this helper.
static int tv_nvdec_release_pending_outputs(tv_nvdec *state) {
	unsigned int slot;
	int result = 0;
	if (!state || !state->decoder || !state->unmap_frame) return 0;
	for (slot = 0; slot < TV_NVDEC_MAX_OUTPUTS; slot++) {
		if (state->mapped_devices[slot]) {
			if (state->unmap_frame(state->decoder,
				(unsigned long long)state->mapped_devices[slot]) != CUDA_SUCCESS) result = -1;
			state->mapped_devices[slot] = 0;
			state->mapped_pitches[slot] = 0;
		}
	}
	state->output_count = 0;
	return result;
}

static int tv_nvdec_new(tv_nvdec **result) {
	tv_nvdec *state = calloc(1, sizeof(*state));
	CUVIDPARSERPARAMS parser_params;
	CUVIDDECODECAPS caps;
	CUdevice device;
	CUcontext old_context = NULL;
	if (!state) return -1;
	state->cuda_library = dlopen("libcuda.so.1", RTLD_NOW | RTLD_LOCAL);
	state->cuvid_library = dlopen("libnvcuvid.so.1", RTLD_NOW | RTLD_LOCAL);
	if (!state->cuda_library || !state->cuvid_library) { tv_nvdec_free(state); return -2; }
#define TV_LOAD(member, library, symbol) do { state->member = (void *)tv_symbol(state->library, symbol); if (!state->member) { tv_nvdec_free(state); return -3; } } while (0)
	TV_LOAD(cu_init, cuda_library, "cuInit");
	TV_LOAD(cu_device_get, cuda_library, "cuDeviceGet");
	TV_LOAD(cu_ctx_create, cuda_library, "cuCtxCreate_v2");
	TV_LOAD(cu_ctx_destroy, cuda_library, "cuCtxDestroy_v2");
	TV_LOAD(cu_ctx_push, cuda_library, "cuCtxPushCurrent_v2");
	TV_LOAD(cu_ctx_pop, cuda_library, "cuCtxPopCurrent_v2");
	TV_LOAD(cu_memcpy_2d, cuda_library, "cuMemcpy2D_v2");
	TV_LOAD(get_caps, cuvid_library, "cuvidGetDecoderCaps");
	TV_LOAD(create_parser, cuvid_library, "cuvidCreateVideoParser");
	TV_LOAD(parse_video, cuvid_library, "cuvidParseVideoData");
	TV_LOAD(destroy_parser, cuvid_library, "cuvidDestroyVideoParser");
	TV_LOAD(create_decoder, cuvid_library, "cuvidCreateDecoder");
	TV_LOAD(destroy_decoder, cuvid_library, "cuvidDestroyDecoder");
	TV_LOAD(decode_picture, cuvid_library, "cuvidDecodePicture");
	TV_LOAD(map_frame, cuvid_library, "cuvidMapVideoFrame64");
	TV_LOAD(unmap_frame, cuvid_library, "cuvidUnmapVideoFrame64");
#undef TV_LOAD
	if (state->cu_init(0) != CUDA_SUCCESS || state->cu_device_get(&device, 0) != CUDA_SUCCESS ||
		state->cu_ctx_create(&state->context, 0, device) != CUDA_SUCCESS) { tv_nvdec_free(state); return -4; }
	memset(&caps, 0, sizeof(caps));
	caps.eCodecType = cudaVideoCodec_MPEG2;
	caps.eChromaFormat = cudaVideoChromaFormat_420;
	if (state->get_caps(&caps) != CUDA_SUCCESS || !caps.bIsSupported) { tv_nvdec_free(state); return -5; }
	memset(&parser_params, 0, sizeof(parser_params));
	parser_params.CodecType = cudaVideoCodec_MPEG2;
	parser_params.ulMaxNumDecodeSurfaces = 8;
	parser_params.ulMaxDisplayDelay = 2;
	parser_params.ulErrorThreshold = 0;
	parser_params.pUserData = state;
	parser_params.pfnSequenceCallback = tv_nvdec_sequence;
	parser_params.pfnDecodePicture = tv_nvdec_decode_picture;
	parser_params.pfnDisplayPicture = tv_nvdec_display_picture;
	if (state->create_parser(&state->parser, &parser_params) != CUDA_SUCCESS) { tv_nvdec_free(state); return -6; }
	if (state->cu_ctx_pop(&old_context) != CUDA_SUCCESS) { tv_nvdec_free(state); return -7; }
	*result = state;
	return 0;
}

static int tv_nvdec_decode_mode(tv_nvdec *state, const uint8_t *data, size_t size, int defer_output) {
	CUVIDSOURCEDATAPACKET packet;
	CUcontext old_context = NULL;
	CUresult status;
	if (!state || !state->parser || !data || !size || size > 16 * 1024 * 1024) return -1;
	state->callback_error = 0;
	state->defer_output = defer_output;
	if (state->cu_ctx_push(state->context) != CUDA_SUCCESS) return -2;
	if (tv_nvdec_release_pending_outputs(state) != 0) {
		state->cu_ctx_pop(&old_context);
		return -5;
	}
	memset(&packet, 0, sizeof(packet));
	packet.flags = CUVID_PKT_ENDOFPICTURE;
	packet.payload = data;
	packet.payload_size = size;
	status = state->parse_video(state->parser, &packet);
	if ((state->callback_error || status != CUDA_SUCCESS) &&
		tv_nvdec_release_pending_outputs(state) != 0 && !state->callback_error) {
		state->callback_error = -6;
	}
	if (state->cu_ctx_pop(&old_context) != CUDA_SUCCESS) return -3;
	if (old_context != state->context) return -4;
	if (state->callback_error) return state->callback_error;
	if (status != CUDA_SUCCESS) return -(1000 + (int)status);
	return (int)state->output_count;
}

static int tv_nvdec_decode(tv_nvdec *state, const uint8_t *data, size_t size) {
	return tv_nvdec_decode_mode(state, data, size, 0);
}

static int tv_nvdec_decode_surfaces(tv_nvdec *state, const uint8_t *data, size_t size) {
	return tv_nvdec_decode_mode(state, data, size, 1);
}

static int tv_nvdec_map_output(tv_nvdec *state, unsigned int slot, uintptr_t *device_result, unsigned int *pitch_result) {
	if (!state || !device_result || !pitch_result || slot >= state->output_count || !state->defer_output) return -1;
	if (!state->mapped_devices[slot] || state->mapped_pitches[slot] < state->width) return -5;
	*device_result = (uintptr_t)state->mapped_devices[slot];
	*pitch_result = state->mapped_pitches[slot];
	state->mapped_devices[slot] = 0;
	state->mapped_pitches[slot] = 0;
	return 0;
}

static int tv_nvdec_unmap_output(tv_nvdec *state, uintptr_t device) {
	CUcontext old_context = NULL;
	CUresult status;
	if (!state || !device) return -1;
	if (state->cu_ctx_push(state->context) != CUDA_SUCCESS) return -2;
	status = state->unmap_frame(state->decoder, (unsigned long long)device);
	if (state->cu_ctx_pop(&old_context) != CUDA_SUCCESS) return -3;
	if (old_context != state->context) return -4;
	return status == CUDA_SUCCESS ? 0 : -5;
}

static uint8_t *tv_nvdec_plane(tv_nvdec *state, unsigned int slot, unsigned int plane) {
	uint8_t *frame;
	if (!state || slot >= state->output_count) return NULL;
	frame = state->frames + state->frame_size * slot;
	if (plane == 0) return frame;
	if (plane == 1) return frame + (size_t)state->width * state->height;
	if (plane == 2) return frame + (size_t)state->width * state->height + (size_t)state->chroma_width * state->chroma_height;
	return NULL;
}

static uintptr_t tv_nvdec_context(tv_nvdec *state) {
	return state ? (uintptr_t)state->context : 0;
}

static void tv_nvdec_free(tv_nvdec *state) {
	CUcontext old_context = NULL;
	if (!state) return;
	if (state->context && state->cu_ctx_push && state->cu_ctx_push(state->context) == CUDA_SUCCESS) {
		tv_nvdec_release_pending_outputs(state);
		if (state->parser && state->destroy_parser) state->destroy_parser(state->parser);
		if (state->decoder && state->destroy_decoder) state->destroy_decoder(state->decoder);
		if (state->cu_ctx_pop) state->cu_ctx_pop(&old_context);
	}
	if (state->context && state->cu_ctx_destroy) state->cu_ctx_destroy(state->context);
	free(state->frames);
	free(state->nv12_chroma);
	if (state->cuvid_library) dlclose(state->cuvid_library);
	if (state->cuda_library) dlclose(state->cuda_library);
	free(state);
}
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

// AdaptiveDecoder uses NVDEC's motion-adaptive deinterlacer and returns
// progressive planar 4:2:0 frames in display order.
type AdaptiveDecoder struct {
	handle    *C.tv_nvdec
	frames    [16]YUVFrame
	gpuFrames [16]GPUFrame
	mu        sync.Mutex
	leases    int
}

// DecodeSurfaces decodes into NVDEC-owned NV12 surfaces without copying them
// through host memory. Every returned surface must be encoded before DecodeSurfaces is called again.
func (d *AdaptiveDecoder) DecodeSurfaces(data []byte) ([]GPUFrame, error) {
	if d == nil {
		return nil, ErrUnavailable
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle == nil {
		return nil, ErrUnavailable
	}
	if len(data) == 0 {
		return nil, nil
	}
	result := C.tv_nvdec_decode_surfaces(d.handle, (*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)))
	if result < 0 {
		return nil, fmt.Errorf("NVDEC surface decode failed: status=%d", int(result))
	}
	count := int(result)
	for i := 0; i < count; i++ {
		d.gpuFrames[i] = GPUFrame{
			Slot: i, Width: int(d.handle.width), Height: int(d.handle.height),
			Deinterlaced: d.handle.deinterlaced[i] != 0,
			FieldIndex:   int(d.handle.display_fields[i]), FieldCount: int(d.handle.field_counts[i]),
		}
	}
	return d.gpuFrames[:count], nil
}

func (d *AdaptiveDecoder) mapSurface(frame GPUFrame) (uintptr, uint32, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle == nil || frame.Slot < 0 || frame.Slot >= int(d.handle.output_count) {
		return 0, 0, ErrUnavailable
	}
	var device C.uintptr_t
	var pitch C.uint
	if status := C.tv_nvdec_map_output(d.handle, C.uint(frame.Slot), &device, &pitch); status != 0 {
		return 0, 0, fmt.Errorf("NVDEC map output failed: status=%d", int(status))
	}
	return uintptr(device), uint32(pitch), nil
}

func (d *AdaptiveDecoder) unmapSurface(device uintptr) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle == nil {
		return ErrUnavailable
	}
	if status := C.tv_nvdec_unmap_output(d.handle, C.uintptr_t(device)); status != 0 {
		return fmt.Errorf("NVDEC unmap output failed: status=%d", int(status))
	}
	return nil
}

func NewAdaptiveDecoder() (*AdaptiveDecoder, error) {
	var handle *C.tv_nvdec
	if status := C.tv_nvdec_new(&handle); status != 0 {
		return nil, fmt.Errorf("NVDEC adaptive initialization failed: status=%d", int(status))
	}
	return &AdaptiveDecoder{handle: handle}, nil
}

func (d *AdaptiveDecoder) Decode(data []byte) ([]YUVFrame, error) {
	if d == nil {
		return nil, ErrUnavailable
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle == nil {
		return nil, ErrUnavailable
	}
	if len(data) == 0 {
		return nil, nil
	}
	result := C.tv_nvdec_decode(d.handle, (*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)))
	if result < 0 {
		return nil, fmt.Errorf("NVDEC adaptive decode failed: status=%d", int(result))
	}
	count := int(result)
	ySize := int(d.handle.width * d.handle.height)
	chromaSize := int(d.handle.chroma_width * d.handle.chroma_height)
	for i := 0; i < count; i++ {
		frame := &d.frames[i]
		frame.Y = copyPlane(frame.Y, C.tv_nvdec_plane(d.handle, C.uint(i), 0), ySize)
		frame.U = copyPlane(frame.U, C.tv_nvdec_plane(d.handle, C.uint(i), 1), chromaSize)
		frame.V = copyPlane(frame.V, C.tv_nvdec_plane(d.handle, C.uint(i), 2), chromaSize)
		frame.Width = int(d.handle.width)
		frame.Height = int(d.handle.height)
		frame.ChromaWidth = int(d.handle.chroma_width)
		frame.ChromaHeight = int(d.handle.chroma_height)
		frame.Interlaced = false
		frame.Deinterlaced = d.handle.deinterlaced[i] != 0
	}
	return d.frames[:count], nil
}

func (d *AdaptiveDecoder) Close() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.leases != 0 {
		return fmt.Errorf("cannot close NVDEC decoder with %d active encoder lease(s)", d.leases)
	}
	if d.handle != nil {
		C.tv_nvdec_free(d.handle)
		d.handle = nil
	}
	return nil
}

func (d *AdaptiveDecoder) acquireCUDAContext() (uintptr, bool) {
	if d == nil {
		return 0, false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle == nil {
		return 0, false
	}
	d.leases++
	return uintptr(C.tv_nvdec_context(d.handle)), true
}

func (d *AdaptiveDecoder) releaseCUDAContext() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.leases > 0 {
		d.leases--
	}
}
