//go:build native && cgo

package encoder

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/fuba/tv-viewer/internal/mpegts"
	"github.com/fuba/tv-viewer/internal/nativeaudio"
	"github.com/fuba/tv-viewer/internal/nativecaption"
	"github.com/fuba/tv-viewer/internal/nativevideo"
	"github.com/fuba/tv-viewer/internal/streamframe"
)

const nativeVideoBitrate = 8_000_000

const broadcastFrameDuration = 1001 * time.Second / 30000

// StartNativeWebRTCEncoding starts the direct MPEG-TS -> MPEG-2 -> NVENC/Opus path.
func (e *Encoder) StartNativeWebRTCEncoding(channelID string, input io.ReadCloser, streamURL string, programNumbers []uint16, subtitlesEnabled bool, audioMode AudioMode) (*WebRTCSession, error) {
	ctx, cancel := context.WithCancel(context.Background())
	videoReader, videoWriter := io.Pipe()
	audioReader, audioWriter := io.Pipe()
	subtitleReader, subtitleWriter := newSubtitlePipe(subtitlesEnabled)
	session := &WebRTCSession{
		ID: channelID + "-native-" + time.Now().Format("150405.000"), ChannelID: channelID,
		ctx: ctx, cancel: cancel, stream: input, VideoPipe: videoReader, AudioPipe: audioReader,
		VideoRaw: true, AudioRaw: true, SubtitlePipe: subtitleReader, SubtitleRaw: subtitlesEnabled,
		AudioMode: audioMode, StreamURL: streamURL,
	}
	go func() {
		err := runNativePipeline(ctx, channelID, input, videoWriter, audioWriter, subtitleWriter, programNumbers, audioMode)
		_ = videoWriter.CloseWithError(err)
		_ = audioWriter.CloseWithError(err)
		if subtitleWriter != nil {
			_ = subtitleWriter.CloseWithError(err)
		}
		if err != nil && ctx.Err() == nil {
			log.Printf("[Native] pipeline ended: %v", err)
		}
	}()
	return session, nil
}

func newSubtitlePipe(enabled bool) (io.ReadCloser, *io.PipeWriter) {
	if !enabled {
		return nil, nil
	}
	reader, writer := io.Pipe()
	return reader, writer
}

func runNativePipeline(ctx context.Context, channelID string, input io.Reader, videoOut, audioOut, subtitleOut *io.PipeWriter, programNumbers []uint16, audioMode AudioMode) error {
	decoder, err := nativevideo.NewAdaptiveDecoder()
	if err != nil {
		return err
	}
	defer decoder.Close()
	audioDecoder, err := nativeaudio.NewDecoder()
	if err != nil {
		return err
	}
	defer func() { _ = audioDecoder.Close() }()
	var videoEncoder *nativevideo.NVEncoder
	var encoderWidth, encoderHeight, encoderFPS int
	var encoderAspect nativevideo.DisplayAspect
	var sourceAspect nativevideo.DisplayAspect
	var videoFormat videoFormatAnnouncer
	var opusEncoder *nativeaudio.Encoder
	var pcm []int16
	var pcmChannels int
	var nextAudioPTS uint64
	var hasAudioPTS bool
	var audioDecodeErrors int
	var lastAudioDecoderReset time.Time
	video := mpegts.NewMPEG2VideoAssembler()
	audioAssemblers := make(map[uint16]*mpegts.AACADTSAssembler)
	var selectedAudioPID uint16
	captionDecoders := make(map[uint16]*nativecaption.Decoder)
	var selectedCaptionPID uint16
	var timeline presentationTimeline
	metrics := newPipelineMetrics(channelID)
	defer func() {
		if videoEncoder != nil {
			_ = videoEncoder.Close()
		}
		if opusEncoder != nil {
			_ = opusEncoder.Close()
		}
		for _, captionDecoder := range captionDecoders {
			_ = captionDecoder.Close()
		}
	}()
	demuxer := mpegts.NewDemuxer()
	if len(programNumbers) > 0 {
		demuxer = mpegts.NewDemuxerForPrograms(programNumbers)
		log.Printf("[Native] Selecting MPEG-TS program from candidates %v", programNumbers)
	}
	var loggedProgram bool
	_, err = demuxer.ReadPES(ctx, input, func(packet mpegts.PESPacket) error {
		if !loggedProgram && demuxer.SelectedProgram() != 0 {
			loggedProgram = true
			log.Printf("[Native] Selected MPEG-TS program %d", demuxer.SelectedProgram())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		switch packet.Stream.StreamType {
		case 0x02:
			if packet.Discontinuity {
				timeline = presentationTimeline{}
			}
			frames, err := video.Push(packet)
			if err != nil {
				return err
			}
			for _, frame := range frames {
				metrics.sourceFrames++
				// The sequence header is the only place the broadcast states how the
				// coded picture must be shaped, so read it before decoding the frame.
				if format, ok := mpegts.ParseVideoFormat(frame.Data); ok {
					if videoFormat.changed(format) {
						log.Printf("[Native] Source video format %dx%d displayed at %d:%d",
							format.Width, format.Height, format.AspectNum, format.AspectDen)
					}
					if videoFormat.shouldAnnounce(format, time.Now()) && subtitleOut != nil {
						if err := writeDataChannelJSON(subtitleOut, videoFormatMessage(format)); err != nil {
							return err
						}
					}
					sourceAspect = nativevideo.DisplayAspect{Num: format.AspectNum, Den: format.AspectDen}
				}
				timeline.Add(frame.PTS, frame.HasPTS)
				frameDuration := broadcastFrameDuration
				decodeStarted := time.Now()
				decoded, err := decoder.DecodeSurfaces(frame.Data)
				metrics.observeDecode(time.Since(decodeStarted), len(decoded))
				if err != nil {
					return err
				}
				var picturePTS uint64
				var hasPicturePTS bool
				for _, gpuFrame := range decoded {
					encodeFPS := 30
					outputDuration := frameDuration
					if gpuFrame.Deinterlaced {
						encodeFPS = 60
						outputDuration /= 2
					}
					if !gpuFrame.Deinterlaced || gpuFrame.FieldIndex == 0 {
						picturePTS, hasPicturePTS = timeline.Pop()
					}
					outputPTS := picturePTS
					if gpuFrame.Deinterlaced && gpuFrame.FieldIndex > 0 {
						outputPTS += uint64(gpuFrame.FieldIndex) * broadcastPictureTicks / 2
					}
					if videoEncoder == nil || encoderWidth != gpuFrame.Width ||
						encoderHeight != gpuFrame.Height || encoderFPS != encodeFPS ||
						encoderAspect != sourceAspect {
						if videoEncoder != nil {
							if closeErr := videoEncoder.Close(); closeErr != nil {
								return closeErr
							}
						}
						videoEncoder, err = nativevideo.NewNVEncoderForAdaptiveDecoder(gpuFrame.Width, gpuFrame.Height, encodeFPS, nativeVideoBitrate, sourceAspect, decoder)
						if err != nil {
							return err
						}
						encoderWidth, encoderHeight, encoderFPS = gpuFrame.Width, gpuFrame.Height, encodeFPS
						encoderAspect = sourceAspect
					}
					encodeStarted := time.Now()
					encoded, err := videoEncoder.EncodeSurface(gpuFrame)
					metrics.observeEncode(time.Since(encodeStarted), len(encoded.Data))
					if err != nil {
						return err
					}
					if len(encoded.Data) > 0 {
						if err := streamframe.WriteVideo(videoOut, streamframe.Video{
							Data: encoded.Data, Duration: outputDuration,
							PTS: outputPTS, HasPTS: hasPicturePTS,
						}); err != nil {
							return err
						}
						metrics.observeVideoPTS(outputPTS, hasPicturePTS)
						metrics.observeOutput(hasPicturePTS)
					}
				}
				metrics.maybeLog()
			}
		case 0x0f:
			if selectedAudioPID == 0 {
				selectedAudioPID = primaryAudioPID(demuxer.Analyzer.Map.Streams, demuxer.SelectedProgram())
				if selectedAudioPID == 0 {
					return nil
				}
				log.Printf("[Native] Selected primary AAC PID %#x", selectedAudioPID)
			}
			if selectedAudioPID != packet.PID {
				return nil
			}
			if packet.Discontinuity {
				pcm = pcm[:0]
				hasAudioPTS = false
			}
			assembler := audioAssemblers[packet.PID]
			if assembler == nil {
				assembler = mpegts.NewAACADTSAssembler()
				audioAssemblers[packet.PID] = assembler
			}
			frames, err := assembler.Push(packet)
			if err != nil {
				return err
			}
			for _, frame := range frames {
				decoded, err := audioDecoder.DecodeADTS(frame.Data)
				if err != nil {
					audioDecodeErrors++
					if audioDecodeErrors == 1 || audioDecodeErrors%100 == 0 {
						log.Printf("[Native] Dropping malformed AAC frame (%d): %v", audioDecodeErrors, err)
					}
					now := time.Now()
					if shouldResetAudioDecoder(audioDecodeErrors, lastAudioDecoderReset, now) {
						if resetErr := audioDecoder.Reset(); resetErr != nil {
							return fmt.Errorf("reset AAC decoder: %w", resetErr)
						}
						lastAudioDecoderReset = now
						pcm = pcm[:0]
						pcmChannels = 0
						hasAudioPTS = false
						if opusEncoder != nil {
							_ = opusEncoder.Close()
							opusEncoder = nil
						}
						log.Printf("[Native] Reset AAC decoder after broadcast configuration change")
					}
					continue
				}
				if audioDecodeErrors > 0 {
					log.Printf("[Native] AAC decoding recovered after %d dropped frames", audioDecodeErrors)
					audioDecodeErrors = 0
				}
				for _, pcmFrame := range decoded {
					if pcmFrame.SampleRate != 48000 {
						return fmt.Errorf("unsupported audio sample rate %d", pcmFrame.SampleRate)
					}
					if pcmFrame.Channels != 1 && pcmFrame.Channels != 2 {
						return fmt.Errorf("unsupported audio channel count %d", pcmFrame.Channels)
					}
					if pcmChannels == 0 {
						pcmChannels = pcmFrame.Channels
						opusEncoder, err = nativeaudio.NewEncoder(2)
						if err != nil {
							return err
						}
					}
					wasAligned := hasAudioPTS
					alignedPTS, aligned, resetPCM := alignAudioPTS(
						nextAudioPTS, hasAudioPTS, len(pcm), frame.PTS, frame.HasPTS,
					)
					if resetPCM {
						pcm = pcm[:0]
						if wasAligned {
							metrics.observeAudioResync()
						}
					}
					nextAudioPTS, hasAudioPTS = alignedPTS, aligned
					pcm = append(pcm, selectAudioChannels(pcmFrame.Data, pcmFrame.Channels, audioMode)...)
					for len(pcm) >= 960*2 {
						packet, err := opusEncoder.Encode(pcm[:960*2])
						if err != nil {
							return err
						}
						if len(packet.Data) > 0 {
							if err := streamframe.WriteAudio(audioOut, streamframe.Audio{
								Data: packet.Data, Duration: 20 * time.Millisecond,
								PTS: nextAudioPTS, HasPTS: hasAudioPTS,
							}); err != nil {
								return err
							}
							metrics.observeAudioPTS(nextAudioPTS, hasAudioPTS)
							if hasAudioPTS {
								nextAudioPTS = (nextAudioPTS + 1800) & ((1 << 33) - 1)
							}
						}
						pcm = pcm[960*2:]
					}
				}
			}
		case 0x06:
			if subtitleOut == nil || (selectedCaptionPID != 0 && selectedCaptionPID != packet.PID) {
				return nil
			}
			captionDecoder := captionDecoders[packet.PID]
			if captionDecoder == nil {
				captionDecoder, err = nativecaption.NewDecoder()
				if err != nil {
					return err
				}
				captionDecoders[packet.PID] = captionDecoder
			}
			caption, parsed, err := captionDecoder.DecodePES(packet.Payload)
			if err != nil {
				log.Printf("[Native] Dropping malformed ARIB caption on PID %#x: %v", packet.PID, err)
				return nil
			}
			if !parsed {
				return nil
			}
			if selectedCaptionPID == 0 {
				selectedCaptionPID = packet.PID
				log.Printf("[Native] Selected ARIB caption PID %#x", selectedCaptionPID)
			}
			if caption.Text == "" {
				return writeDataChannelJSON(subtitleOut, map[string]any{"type": "clear"})
			}
			duration := caption.Duration
			if duration <= 0 || duration > 10*time.Second {
				duration = 5 * time.Second
			}
			return writeDataChannelJSON(subtitleOut, map[string]any{
				"type": "show", "id": "arib-caption",
				"text": caption.Text, "endTime": duration.Seconds(),
			})
		}
		return nil
	})
	return err
}

// writeDataChannelJSON sends one JSON message to every viewer of this session
// over the peer data channel that also carries captions.
func writeDataChannelJSON(output io.Writer, message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > 64*1024 {
		return fmt.Errorf("invalid subtitle message size %d", len(data))
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(data)))
	if _, err := output.Write(size[:]); err != nil {
		return err
	}
	_, err = output.Write(data)
	return err
}

func selectAudioChannels(input []int16, channels int, mode AudioMode) []int16 {
	if channels == 1 {
		output := make([]int16, 0, len(input)*2)
		for _, value := range input {
			output = append(output, value, value)
		}
		return output
	}
	if channels != 2 || mode == AudioModeBoth || mode == "" {
		return input
	}
	output := make([]int16, 0, len(input))
	for i := 0; i+1 < len(input); i += 2 {
		value := input[i]
		if mode == AudioModeSub {
			value = input[i+1]
		}
		output = append(output, value, value)
	}
	return output
}
