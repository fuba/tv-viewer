//go:build native && cgo

package nativevideo

/*
#cgo CFLAGS: -I/usr/local/include
#cgo LDFLAGS: -ldl
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <dlfcn.h>
#include <ffnvcodec/nvEncodeAPI.h>
#include <ffnvcodec/dynlink_cuda.h>

extern CUresult cuInit(unsigned int);
extern CUresult cuDeviceGet(CUdevice *, int);
extern CUresult cuCtxCreate_v2(CUcontext *, unsigned int, CUdevice);
extern CUresult cuCtxDestroy_v2(CUcontext);
extern CUresult cuCtxPushCurrent_v2(CUcontext);
extern CUresult cuCtxPopCurrent_v2(CUcontext *);
#pragma weak cuInit
#pragma weak cuDeviceGet
#pragma weak cuCtxCreate_v2
#pragma weak cuCtxDestroy_v2
#pragma weak cuCtxPushCurrent_v2
#pragma weak cuCtxPopCurrent_v2
static int tv_nvenc_load_driver(void) {
	void *cuda = dlopen("libcuda.so.1", RTLD_LAZY | RTLD_GLOBAL);
	void *encode = dlopen("libnvidia-encode.so.1", RTLD_LAZY | RTLD_GLOBAL);
	return cuda && encode;
}

	typedef struct {
	NV_ENCODE_API_FUNCTION_LIST functions;
	void *encoder;
	CUcontext context;
	NV_ENC_INPUT_PTR input;
	NV_ENC_OUTPUT_PTR output;
	uint8_t *bitstream;
	size_t bitstream_capacity;
	unsigned int width;
	unsigned int height;
	unsigned int frame_index;
	int owns_context;
	int stage;
} tv_nvenc;

static int tv_nvenc_init(tv_nvenc **result, unsigned int width, unsigned int height, unsigned int fps, unsigned int bitrate, uintptr_t shared_context) {
	tv_nvenc *state = calloc(1, sizeof(*state));
	NVENCSTATUS status;
	NV_ENC_OPEN_ENCODE_SESSION_EX_PARAMS open_params = {0};
	NV_ENC_INITIALIZE_PARAMS init_params = {0};
	NV_ENC_PRESET_CONFIG preset_config = {0};
	CUdevice device;
	CUcontext old_context = NULL;
	int context_current = 0;
	NVENCSTATUS (*create_instance)(NV_ENCODE_API_FUNCTION_LIST *) = NULL;
	status = NV_ENC_ERR_NO_ENCODE_DEVICE;
	if (!state) return -1;
	if (!tv_nvenc_load_driver()) goto fail;
	create_instance = (NVENCSTATUS (*)(NV_ENCODE_API_FUNCTION_LIST *))dlsym(RTLD_DEFAULT, "NvEncodeAPICreateInstance");
	if (!create_instance) goto fail;
	state->width = width;
	state->height = height;
	state->bitstream_capacity = width * height * 2;
	state->bitstream = malloc(state->bitstream_capacity);
	if (!state->bitstream) goto fail;
	state->functions.version = NV_ENCODE_API_FUNCTION_LIST_VER;
	state->stage = 1;
	status = create_instance(&state->functions);
	if (status != NV_ENC_SUCCESS) goto fail;
	state->stage = 2;
	if (shared_context) {
		state->context = (CUcontext)shared_context;
		if (cuCtxPushCurrent_v2(state->context) != CUDA_SUCCESS) goto fail;
		context_current = 1;
	} else {
		if (cuInit(0) != CUDA_SUCCESS || cuDeviceGet(&device, 0) != CUDA_SUCCESS || cuCtxCreate_v2(&state->context, 0, device) != CUDA_SUCCESS) goto fail;
		state->owns_context = 1;
		context_current = 1;
	}
	state->stage = 3;
	open_params.version = NV_ENC_OPEN_ENCODE_SESSION_EX_PARAMS_VER;
	open_params.deviceType = NV_ENC_DEVICE_TYPE_CUDA;
	open_params.device = state->context;
	open_params.apiVersion = NVENCAPI_VERSION;
	status = state->functions.nvEncOpenEncodeSessionEx(&open_params, &state->encoder);
	if (status != NV_ENC_SUCCESS) goto fail;

	state->stage = 4;
	init_params.version = NV_ENC_INITIALIZE_PARAMS_VER;
	init_params.encodeGUID = NV_ENC_CODEC_H264_GUID;
	init_params.presetGUID = NV_ENC_PRESET_P4_GUID;
	init_params.encodeWidth = width;
	init_params.encodeHeight = height;
	// Japanese terrestrial broadcasts use 1440x1080 coded pixels with 16:9 display aspect.
	init_params.darWidth = 16;
	init_params.darHeight = 9;
	init_params.frameRateNum = fps;
	init_params.frameRateDen = 1;
	init_params.enablePTD = 1;
	init_params.tuningInfo = NV_ENC_TUNING_INFO_LOW_LATENCY;
	preset_config.version = NV_ENC_PRESET_CONFIG_VER;
	preset_config.presetCfg.version = NV_ENC_CONFIG_VER;
	status = state->functions.nvEncGetEncodePresetConfigEx(state->encoder, NV_ENC_CODEC_H264_GUID,
		NV_ENC_PRESET_P4_GUID, NV_ENC_TUNING_INFO_LOW_LATENCY, &preset_config);
	if (status != NV_ENC_SUCCESS) goto fail;
	preset_config.presetCfg.profileGUID = NV_ENC_H264_PROFILE_HIGH_GUID;
	preset_config.presetCfg.gopLength = fps * 2;
	preset_config.presetCfg.frameIntervalP = 1;
	preset_config.presetCfg.frameFieldMode = NV_ENC_PARAMS_FRAME_FIELD_MODE_FRAME;
	preset_config.presetCfg.rcParams.rateControlMode = NV_ENC_PARAMS_RC_VBR;
	preset_config.presetCfg.rcParams.averageBitRate = bitrate;
	preset_config.presetCfg.rcParams.maxBitRate = bitrate * 3 / 2;
	preset_config.presetCfg.rcParams.vbvBufferSize = bitrate;
	preset_config.presetCfg.rcParams.vbvInitialDelay = bitrate / 2;
	preset_config.presetCfg.rcParams.enableAQ = 1;
	preset_config.presetCfg.rcParams.aqStrength = 8;
	preset_config.presetCfg.rcParams.enableTemporalAQ = 1;
	preset_config.presetCfg.rcParams.zeroReorderDelay = 1;
	preset_config.presetCfg.encodeCodecConfig.h264Config.chromaFormatIDC = 1;
	preset_config.presetCfg.encodeCodecConfig.h264Config.repeatSPSPPS = 1;
	preset_config.presetCfg.encodeCodecConfig.h264Config.idrPeriod = fps * 2;
	init_params.encodeConfig = &preset_config.presetCfg;
	status = state->functions.nvEncInitializeEncoder(state->encoder, &init_params);
	if (status != NV_ENC_SUCCESS) goto fail;

	state->stage = 5;
	{
		NV_ENC_CREATE_INPUT_BUFFER input = {0};
		input.version = NV_ENC_CREATE_INPUT_BUFFER_VER;
		input.width = width;
		input.height = height;
		input.bufferFmt = NV_ENC_BUFFER_FORMAT_NV12;
		status = state->functions.nvEncCreateInputBuffer(state->encoder, &input);
		if (status != NV_ENC_SUCCESS) goto fail;
		state->input = input.inputBuffer;
	}
	state->stage = 6;
	{
		NV_ENC_CREATE_BITSTREAM_BUFFER output = {0};
		output.version = NV_ENC_CREATE_BITSTREAM_BUFFER_VER;
		status = state->functions.nvEncCreateBitstreamBuffer(state->encoder, &output);
		if (status != NV_ENC_SUCCESS) goto fail;
		state->output = output.bitstreamBuffer;
	}
	if (cuCtxPopCurrent_v2(&old_context) != CUDA_SUCCESS) goto fail;
	context_current = 0;
	*result = state;
	return 0;

fail:
	{
		int failure = -(1000 + state->stage * 100 + (int)status);
	if (state->encoder && state->functions.nvEncDestroyEncoder) state->functions.nvEncDestroyEncoder(state->encoder);
	if (context_current) cuCtxPopCurrent_v2(&old_context);
	if (state->context && state->owns_context) cuCtxDestroy_v2(state->context);
	free(state->bitstream);
	free(state);
		return failure;
	}
}

static int tv_nvenc_encode_inner(tv_nvenc *state, const uint8_t *y, size_t y_len, const uint8_t *u, size_t u_len, const uint8_t *v, size_t v_len, unsigned int cw, unsigned int ch, uint8_t **output, unsigned int *output_size, unsigned int *picture_type) {
	NV_ENC_LOCK_INPUT_BUFFER lock_input = {0};
	NV_ENC_PIC_PARAMS picture = {0};
	NV_ENC_LOCK_BITSTREAM lock_output = {0};
	NVENCSTATUS status;
	uint8_t *dst;
	unsigned int row;
	if (!state || !y || !u || !v || cw * 2 != state->width || ch * 2 != state->height ||
		y_len < (size_t)state->width * state->height ||
		u_len < (size_t)cw * ch || v_len < (size_t)cw * ch) return -1;
	lock_input.version = NV_ENC_LOCK_INPUT_BUFFER_VER;
	lock_input.inputBuffer = state->input;
	status = state->functions.nvEncLockInputBuffer(state->encoder, &lock_input);
	if (status != NV_ENC_SUCCESS) return -(2000 + (int)status);
	if (!lock_input.bufferDataPtr || lock_input.pitch < state->width) {
		state->functions.nvEncUnlockInputBuffer(state->encoder, state->input);
		return -2001;
	}
	dst = lock_input.bufferDataPtr;
	for (row = 0; row < state->height; row++) memcpy(dst + row * lock_input.pitch, y + row * state->width, state->width);
	for (row = 0; row < ch; row++) {
		unsigned int col;
		uint8_t *uv = dst + lock_input.pitch * state->height + row * lock_input.pitch;
		for (col = 0; col < cw; col++) { uv[col * 2] = u[row * cw + col]; uv[col * 2 + 1] = v[row * cw + col]; }
	}
	status = state->functions.nvEncUnlockInputBuffer(state->encoder, state->input);
	if (status != NV_ENC_SUCCESS) return -(2100 + (int)status);

	picture.version = NV_ENC_PIC_PARAMS_VER;
	picture.inputWidth = state->width;
	picture.inputHeight = state->height;
	picture.inputPitch = lock_input.pitch;
	picture.inputBuffer = state->input;
	picture.outputBitstream = state->output;
	picture.bufferFmt = NV_ENC_BUFFER_FORMAT_NV12;
	picture.pictureStruct = NV_ENC_PIC_STRUCT_FRAME;
	status = state->functions.nvEncEncodePicture(state->encoder, &picture);
	if (status == NV_ENC_ERR_NEED_MORE_INPUT) return 0;
	if (status != NV_ENC_SUCCESS) return -(2200 + (int)status);

	lock_output.version = NV_ENC_LOCK_BITSTREAM_VER;
	lock_output.outputBitstream = state->output;
	status = state->functions.nvEncLockBitstream(state->encoder, &lock_output);
	if (status != NV_ENC_SUCCESS) return -(2300 + (int)status);
	if (lock_output.bitstreamSizeInBytes > state->bitstream_capacity) {
		if (lock_output.bitstreamSizeInBytes > state->width * state->height * 4) {
			state->functions.nvEncUnlockBitstream(state->encoder, state->output);
			return -2301;
		}
		uint8_t *resized = realloc(state->bitstream, lock_output.bitstreamSizeInBytes);
		if (!resized) { state->functions.nvEncUnlockBitstream(state->encoder, state->output); return -2; }
		state->bitstream = resized;
		state->bitstream_capacity = lock_output.bitstreamSizeInBytes;
	}
	memcpy(state->bitstream, lock_output.bitstreamBufferPtr, lock_output.bitstreamSizeInBytes);
	*output = state->bitstream;
	*output_size = lock_output.bitstreamSizeInBytes;
	*picture_type = lock_output.pictureType;
	status = state->functions.nvEncUnlockBitstream(state->encoder, state->output);
	if (status != NV_ENC_SUCCESS) return -(2400 + (int)status);
	state->frame_index++;
	return 1;
}

static int tv_nvenc_encode(tv_nvenc *state, const uint8_t *y, size_t y_len, const uint8_t *u, size_t u_len, const uint8_t *v, size_t v_len, unsigned int cw, unsigned int ch, uint8_t **output, unsigned int *output_size, unsigned int *picture_type) {
	CUcontext old_context = NULL;
	int result;
	if (!state || !state->context || cuCtxPushCurrent_v2(state->context) != CUDA_SUCCESS) return -3;
	result = tv_nvenc_encode_inner(state, y, y_len, u, u_len, v, v_len, cw, ch, output, output_size, picture_type);
	if (cuCtxPopCurrent_v2(&old_context) != CUDA_SUCCESS && result >= 0) return -4;
	return result;
}

static int tv_nvenc_encode_device_inner(tv_nvenc *state, uintptr_t device, unsigned int pitch, uint8_t **output, unsigned int *output_size, unsigned int *picture_type) {
	NV_ENC_REGISTER_RESOURCE registration = {0};
	NV_ENC_MAP_INPUT_RESOURCE mapping = {0};
	NV_ENC_PIC_PARAMS picture = {0};
	NV_ENC_LOCK_BITSTREAM lock_output = {0};
	NVENCSTATUS status;
	int result = -1;
	if (!state || !device || pitch < state->width) return -1;
	registration.version = NV_ENC_REGISTER_RESOURCE_VER;
	registration.resourceType = NV_ENC_INPUT_RESOURCE_TYPE_CUDADEVICEPTR;
	registration.resourceToRegister = (void *)device;
	registration.width = state->width;
	registration.height = state->height;
	registration.pitch = pitch;
	registration.bufferFormat = NV_ENC_BUFFER_FORMAT_NV12;
	registration.bufferUsage = NV_ENC_INPUT_IMAGE;
	status = state->functions.nvEncRegisterResource(state->encoder, &registration);
	if (status != NV_ENC_SUCCESS) return -(3000 + (int)status);

	mapping.version = NV_ENC_MAP_INPUT_RESOURCE_VER;
	mapping.registeredResource = registration.registeredResource;
	status = state->functions.nvEncMapInputResource(state->encoder, &mapping);
	if (status != NV_ENC_SUCCESS) {
		result = -(3100 + (int)status);
		goto cleanup_registration;
	}

	picture.version = NV_ENC_PIC_PARAMS_VER;
	picture.inputWidth = state->width;
	picture.inputHeight = state->height;
	picture.inputPitch = pitch;
	picture.inputBuffer = mapping.mappedResource;
	picture.outputBitstream = state->output;
	picture.bufferFmt = NV_ENC_BUFFER_FORMAT_NV12;
	picture.pictureStruct = NV_ENC_PIC_STRUCT_FRAME;
	status = state->functions.nvEncEncodePicture(state->encoder, &picture);
	if (status == NV_ENC_ERR_NEED_MORE_INPUT) {
		result = 0;
		goto cleanup_mapping;
	}
	if (status != NV_ENC_SUCCESS) {
		result = -(3200 + (int)status);
		goto cleanup_mapping;
	}

	lock_output.version = NV_ENC_LOCK_BITSTREAM_VER;
	lock_output.outputBitstream = state->output;
	status = state->functions.nvEncLockBitstream(state->encoder, &lock_output);
	if (status != NV_ENC_SUCCESS) {
		result = -(3300 + (int)status);
		goto cleanup_mapping;
	}
	if (lock_output.bitstreamSizeInBytes > state->bitstream_capacity) {
		uint8_t *resized;
		if (lock_output.bitstreamSizeInBytes > state->width * state->height * 4) {
			state->functions.nvEncUnlockBitstream(state->encoder, state->output);
			result = -3301;
			goto cleanup_mapping;
		}
		resized = realloc(state->bitstream, lock_output.bitstreamSizeInBytes);
		if (!resized) {
			state->functions.nvEncUnlockBitstream(state->encoder, state->output);
			result = -2;
			goto cleanup_mapping;
		}
		state->bitstream = resized;
		state->bitstream_capacity = lock_output.bitstreamSizeInBytes;
	}
	memcpy(state->bitstream, lock_output.bitstreamBufferPtr, lock_output.bitstreamSizeInBytes);
	*output = state->bitstream;
	*output_size = lock_output.bitstreamSizeInBytes;
	*picture_type = lock_output.pictureType;
	status = state->functions.nvEncUnlockBitstream(state->encoder, state->output);
	if (status != NV_ENC_SUCCESS) {
		result = -(3400 + (int)status);
		goto cleanup_mapping;
	}
	state->frame_index++;
	result = 1;

cleanup_mapping:
	state->functions.nvEncUnmapInputResource(state->encoder, mapping.mappedResource);
cleanup_registration:
	state->functions.nvEncUnregisterResource(state->encoder, registration.registeredResource);
	return result;
}

static int tv_nvenc_encode_device(tv_nvenc *state, uintptr_t device, unsigned int pitch, uint8_t **output, unsigned int *output_size, unsigned int *picture_type) {
	CUcontext old_context = NULL;
	int result;
	if (!state || !state->context || cuCtxPushCurrent_v2(state->context) != CUDA_SUCCESS) return -3;
	result = tv_nvenc_encode_device_inner(state, device, pitch, output, output_size, picture_type);
	if (cuCtxPopCurrent_v2(&old_context) != CUDA_SUCCESS && result >= 0) return -4;
	return result;
}

static void tv_nvenc_free(tv_nvenc *state) {
	CUcontext old_context = NULL;
	int context_current = 0;
	if (!state) return;
	if (state->context && cuCtxPushCurrent_v2(state->context) == CUDA_SUCCESS) context_current = 1;
	if (context_current && state->encoder) {
		if (state->input) state->functions.nvEncDestroyInputBuffer(state->encoder, state->input);
		if (state->output) state->functions.nvEncDestroyBitstreamBuffer(state->encoder, state->output);
		state->functions.nvEncDestroyEncoder(state->encoder);
	}
	if (context_current) cuCtxPopCurrent_v2(&old_context);
	if (state->context && state->owns_context) cuCtxDestroy_v2(state->context);
	free(state->bitstream);
	free(state);
}
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

// NVEncoder wraps the synchronous NVENC path. Input is copied into one
// reusable NV12 surface and output is copied into Go-owned memory.
type NVEncoder struct {
	handle  *C.tv_nvenc
	width   int
	height  int
	decoder *AdaptiveDecoder
	mu      sync.Mutex
}

func NewNVEncoder(width, height, fps, bitrate int) (*NVEncoder, error) {
	return newNVEncoder(width, height, fps, bitrate, 0)
}

// NewNVEncoderForAdaptiveDecoder shares the decoder's CUDA context to avoid
// expensive cross-context synchronization between NVDEC and NVENC.
func NewNVEncoderForAdaptiveDecoder(width, height, fps, bitrate int, decoder *AdaptiveDecoder) (*NVEncoder, error) {
	context, ok := decoder.acquireCUDAContext()
	if !ok {
		return nil, ErrUnavailable
	}
	encoder, err := newNVEncoder(width, height, fps, bitrate, context)
	if err != nil {
		decoder.releaseCUDAContext()
		return nil, err
	}
	encoder.decoder = decoder
	return encoder, nil
}

func newNVEncoder(width, height, fps, bitrate int, cudaContext uintptr) (*NVEncoder, error) {
	if width <= 0 || height <= 0 || width > 4096 || height > 2160 || fps <= 0 || fps > 120 || bitrate <= 0 {
		return nil, fmt.Errorf("invalid NVENC configuration: %dx%d fps=%d bitrate=%d", width, height, fps, bitrate)
	}
	var handle *C.tv_nvenc
	status := C.tv_nvenc_init(&handle, C.uint(width), C.uint(height), C.uint(fps), C.uint(bitrate), C.uintptr_t(cudaContext))
	if status != 0 {
		return nil, fmt.Errorf("NVENC initialization failed: status=%d", int(status))
	}
	return &NVEncoder{handle: handle, width: width, height: height}, nil
}

func (e *NVEncoder) Encode(frame YUVFrame) (H264Frame, error) {
	if e == nil {
		return H264Frame{}, ErrUnavailable
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle == nil {
		return H264Frame{}, ErrUnavailable
	}
	if err := validateYUV420Frame(frame, e.width, e.height); err != nil {
		return H264Frame{}, err
	}
	var output *C.uint8_t
	var outputSize C.uint
	var pictureType C.uint
	status := C.tv_nvenc_encode(e.handle,
		(*C.uint8_t)(unsafe.Pointer(&frame.Y[0])), C.size_t(len(frame.Y)),
		(*C.uint8_t)(unsafe.Pointer(&frame.U[0])), C.size_t(len(frame.U)),
		(*C.uint8_t)(unsafe.Pointer(&frame.V[0])), C.size_t(len(frame.V)),
		C.uint(frame.ChromaWidth), C.uint(frame.ChromaHeight), &output, &outputSize, &pictureType)
	if status < 0 {
		return H264Frame{}, fmt.Errorf("NVENC encode failed: status=%d", int(status))
	}
	if status == 0 {
		return H264Frame{}, nil
	}
	return H264Frame{
		Data:     C.GoBytes(unsafe.Pointer(output), C.int(outputSize)),
		Keyframe: pictureType == C.NV_ENC_PIC_TYPE_IDR || pictureType == C.NV_ENC_PIC_TYPE_I,
	}, nil
}

// EncodeSurface feeds an NVDEC-owned NV12 surface directly to NVENC.
func (e *NVEncoder) EncodeSurface(frame GPUFrame) (result H264Frame, err error) {
	if e == nil || e.decoder == nil {
		return H264Frame{}, ErrUnavailable
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle == nil || frame.Width != e.width || frame.Height != e.height {
		return H264Frame{}, fmt.Errorf("invalid GPU frame %dx%d for encoder %dx%d", frame.Width, frame.Height, e.width, e.height)
	}
	device, pitch, err := e.decoder.mapSurface(frame)
	if err != nil {
		return H264Frame{}, err
	}
	defer func() {
		if unmapErr := e.decoder.unmapSurface(device); unmapErr != nil {
			if err == nil {
				err = unmapErr
			} else {
				err = fmt.Errorf("%v; additionally failed to release NVDEC surface: %w", err, unmapErr)
			}
		}
	}()
	var output *C.uint8_t
	var outputSize C.uint
	var pictureType C.uint
	status := C.tv_nvenc_encode_device(e.handle, C.uintptr_t(device), C.uint(pitch), &output, &outputSize, &pictureType)
	if status < 0 {
		return H264Frame{}, fmt.Errorf("NVENC device encode failed: status=%d", int(status))
	}
	if status == 0 {
		return H264Frame{}, nil
	}
	return H264Frame{
		Data:     C.GoBytes(unsafe.Pointer(output), C.int(outputSize)),
		Keyframe: pictureType == C.NV_ENC_PIC_TYPE_IDR || pictureType == C.NV_ENC_PIC_TYPE_I,
	}, nil
}

func (e *NVEncoder) Close() error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.handle != nil {
		C.tv_nvenc_free(e.handle)
		e.handle = nil
		if e.decoder != nil {
			e.decoder.releaseCUDAContext()
			e.decoder = nil
		}
	}
	return nil
}
