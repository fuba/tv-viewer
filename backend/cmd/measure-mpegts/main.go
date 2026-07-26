package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/fuba/tv-viewer/internal/mirakurun"
	"github.com/fuba/tv-viewer/internal/mpegts"
	"github.com/fuba/tv-viewer/internal/nativeaudio"
	"github.com/fuba/tv-viewer/internal/nativecaption"
	"github.com/fuba/tv-viewer/internal/nativevideo"
)

func main() {
	serviceID := flag.Int64("service", 0, "Mirakurun service ID")
	programNumber := flag.Uint("program", 0, "MPEG-TS program number to select")
	duration := flag.Duration("duration", 10*time.Second, "measurement duration")
	baseURL := flag.String("mirakurun", "http://tuner:40772", "Mirakurun base URL")
	inputPath := flag.String("input", "", "read an MPEG-TS capture instead of Mirakurun")
	dumpVideo := flag.String("dump-video", "", "write assembled MPEG-2 elementary video to this file")
	decodeNative := flag.Bool("decode-native", false, "decode MPEG-2 frames with the native libmpeg2 backend")
	decodeAdaptive := flag.Bool("decode-adaptive", false, "decode MPEG-2 and deinterlace with NVDEC adaptive mode")
	encodeNative := flag.Bool("encode-native", false, "encode decoded frames with the native NVENC backend")
	encodeAudioNative := flag.Bool("encode-audio-native", false, "decode AAC and encode Opus with native codecs")
	decodeCaptions := flag.Bool("decode-captions", false, "decode ARIB captions with libaribb24")
	dumpH264 := flag.String("dump-h264", "", "write native NVENC Annex-B output to this file")
	dumpYUV := flag.String("dump-yuv", "", "write the first decoded frame as a PPM image")
	traceVideo := flag.Bool("trace-video", false, "print decoded MPEG-2 temporal references")
	flag.Parse()
	if *serviceID == 0 {
		log.Fatal("-service is required")
	}

	var stream io.ReadCloser
	var err error
	if *inputPath != "" {
		stream, err = os.Open(*inputPath)
	} else {
		client := mirakurun.NewClient(*baseURL)
		stream, err = client.GetServiceStream(*serviceID)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.AfterFunc(*duration, func() {
		_ = stream.Close()
		cancel()
	})
	defer timer.Stop()

	video := mpegts.NewMPEG2VideoAssembler()
	deinterlacer := new(nativevideo.BobDeinterlacer)
	audio := mpegts.NewAACADTSAssembler()
	var videoOutput *os.File
	if *dumpVideo != "" {
		videoOutput, err = os.Create(*dumpVideo)
		if err != nil {
			log.Fatal(err)
		}
		defer videoOutput.Close()
	}
	var decoder interface {
		Decode([]byte) ([]nativevideo.YUVFrame, error)
		Close() error
	}
	var adaptiveDecoder *nativevideo.AdaptiveDecoder
	if *decodeAdaptive {
		adaptiveDecoder, err = nativevideo.NewAdaptiveDecoder()
		decoder = adaptiveDecoder
	} else if *decodeNative || *encodeNative {
		decoder, err = nativevideo.NewDecoder()
	}
	if decoder != nil {
		if err != nil {
			log.Fatal(err)
		}
		defer decoder.Close()
	}
	var audioDecoder *nativeaudio.Decoder
	var audioEncoder *nativeaudio.Encoder
	var audioPCM []int16
	if *encodeAudioNative {
		audioDecoder, err = nativeaudio.NewDecoder()
		if err != nil {
			log.Fatal(err)
		}
		defer audioDecoder.Close()
	}
	defer func() {
		if audioEncoder != nil {
			_ = audioEncoder.Close()
		}
	}()
	var nvEncoder *nativevideo.NVEncoder
	defer func() {
		if nvEncoder != nil {
			_ = nvEncoder.Close()
		}
	}()
	var h264Output *os.File
	if *dumpH264 != "" {
		h264Output, err = os.Create(*dumpH264)
		if err != nil {
			log.Fatal(err)
		}
		defer h264Output.Close()
	}
	started := time.Now()
	var pesCount uint64
	var videoPES uint64
	var audioPES uint64
	var videoFrames uint64
	var audioFrames uint64
	var videoBytes uint64
	var decodedFrames uint64
	var encodedFrames uint64
	var encodedBytes uint64
	var audioBytes uint64
	var decodedAudioFrames uint64
	var encodedAudioFrames uint64
	var firstVideoPTS uint64
	var lastVideoPTS uint64
	var hasVideoPTS bool
	var dumpedYUV bool
	var videoTrace []string
	captionDecoders := make(map[uint16]*nativecaption.Decoder)
	var captionCount uint64
	defer func() {
		for _, captionDecoder := range captionDecoders {
			_ = captionDecoder.Close()
		}
	}()

	demuxer := mpegts.NewDemuxer()
	if *programNumber != 0 {
		if *programNumber < 0 || *programNumber > 0xffff {
			log.Fatalf("invalid MPEG-TS program number: %d", *programNumber)
		}
		demuxer = mpegts.NewDemuxerForProgram(uint16(*programNumber)) // #nosec G115 -- range checked above
	}
	stats, readErr := demuxer.ReadPES(ctx, stream, func(packet mpegts.PESPacket) error {
		atomic.AddUint64(&pesCount, 1)
		switch packet.Stream.StreamType {
		case 0x02: // MPEG-2 video
			atomic.AddUint64(&videoPES, 1)
			frames, err := video.Push(packet)
			if err != nil {
				return err
			}
			for _, frame := range frames {
				if frame.HasPTS {
					if !hasVideoPTS {
						firstVideoPTS = frame.PTS
						hasVideoPTS = true
					}
					lastVideoPTS = frame.PTS
				}
				if decoder != nil {
					decoded, err := decoder.Decode(frame.Data)
					if err != nil {
						return err
					}
					atomic.AddUint64(&decodedFrames, uint64(len(decoded)))
					if *traceVideo && len(videoTrace) < 120 {
						for _, decodedFrame := range decoded {
							videoTrace = append(videoTrace, fmt.Sprintf("%d:%d:%t:%t",
								decodedFrame.TemporalReference, decodedFrame.PictureType,
								decodedFrame.Interlaced, decodedFrame.TopFieldFirst))
						}
					}
					if *encodeNative {
						if nvEncoder == nil && len(decoded) > 0 {
							fps := 30
							if decoded[0].Interlaced || decoded[0].Deinterlaced {
								fps = 60
							}
							if adaptiveDecoder != nil {
								nvEncoder, err = nativevideo.NewNVEncoderForAdaptiveDecoder(decoded[0].Width, decoded[0].Height, fps, 8_000_000, adaptiveDecoder)
							} else {
								nvEncoder, err = nativevideo.NewNVEncoder(decoded[0].Width, decoded[0].Height, fps, 8_000_000)
							}
							if err != nil {
								return err
							}
						}
						for _, decodedFrame := range decoded {
							progressiveFrames := []nativevideo.YUVFrame{decodedFrame}
							if !*decodeAdaptive {
								progressiveFrames = deinterlacer.Frames(decodedFrame)
							}
							if *dumpYUV != "" && !dumpedYUV && atomic.LoadUint64(&decodedFrames) >= 10 {
								if err := writePPM(*dumpYUV, progressiveFrames[0]); err != nil {
									return err
								}
								dumpedYUV = true
							}
							for _, progressiveFrame := range progressiveFrames {
								encoded, err := nvEncoder.Encode(progressiveFrame)
								if err != nil {
									return err
								}
								if len(encoded.Data) == 0 {
									continue
								}
								if h264Output != nil {
									if _, err := h264Output.Write(encoded.Data); err != nil {
										return err
									}
								}
								atomic.AddUint64(&encodedFrames, 1)
								atomic.AddUint64(&encodedBytes, uint64(len(encoded.Data)))
							}
						}
					}
				}
				if videoOutput != nil {
					if _, err := videoOutput.Write(frame.Data); err != nil {
						return err
					}
				}
				atomic.AddUint64(&videoFrames, 1)
				atomic.AddUint64(&videoBytes, uint64(len(frame.Data)))
			}
		case 0x0f: // AAC ADTS
			atomic.AddUint64(&audioPES, 1)
			frames, err := audio.Push(packet)
			if err != nil {
				return err
			}
			for _, frame := range frames {
				if *encodeAudioNative {
					decodedPCM, err := audioDecoder.DecodeADTS(frame.Data)
					if err != nil {
						return err
					}
					for _, pcm := range decodedPCM {
						if audioEncoder == nil {
							if pcm.SampleRate != 48000 {
								return fmt.Errorf("unsupported broadcast audio sample rate: %d", pcm.SampleRate)
							}
							audioEncoder, err = nativeaudio.NewEncoder(pcm.Channels)
							if err != nil {
								return err
							}
						}
						audioPCM = append(audioPCM, pcm.Data...)
						frameSamples := 960 * pcm.Channels
						for len(audioPCM) >= frameSamples {
							opus, err := audioEncoder.Encode(audioPCM[:frameSamples])
							if err != nil {
								return err
							}
							if len(opus.Data) > 0 {
								encodedAudioFrames++
							}
							audioPCM = audioPCM[frameSamples:]
						}
						decodedAudioFrames++
					}
				}
				atomic.AddUint64(&audioFrames, 1)
				atomic.AddUint64(&audioBytes, uint64(len(frame.Data)))
			}
		case 0x06: // ARIB caption and other private data
			if !*decodeCaptions {
				break
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
				return err
			}
			if parsed && caption.Text != "" {
				captionCount++
				if captionCount <= 10 {
					fmt.Printf("caption pid=%#x duration=%s text=%q\n", packet.PID, caption.Duration, caption.Text)
				}
			}
		}
		return nil
	})
	elapsed := time.Since(started)
	if readErr != nil && ctx.Err() == nil {
		log.Printf("stream ended: %v", readErr)
	}

	fmt.Printf("duration=%s elapsed=%s packets=%d invalid=%d null=%d transport_errors=%d continuity_errors=%d\n",
		*duration, elapsed.Round(time.Millisecond), stats.Packets, stats.InvalidPackets, stats.NullPackets, stats.TransportErrors, stats.ContinuityErrors)
	fmt.Printf("pes=%d video_pes=%d video_frames=%d decoded_frames=%d video_bytes=%s audio_pes=%d audio_frames=%d audio_bytes=%s\n",
		pesCount, videoPES, videoFrames, decodedFrames, formatBytes(videoBytes), audioPES, audioFrames, formatBytes(audioBytes))
	fmt.Printf("programs=%v selected_program=%d\n", demuxer.Analyzer.Map.Programs, demuxer.SelectedProgram())
	fmt.Printf("encoded_frames=%d encoded_bytes=%s\n", encodedFrames, formatBytes(encodedBytes))
	if hasVideoPTS && lastVideoPTS >= firstVideoPTS {
		fmt.Printf("video_pts_span=%.3fs\n", float64(lastVideoPTS-firstVideoPTS)/90000.0)
	}
	if *encodeAudioNative {
		fmt.Printf("decoded_audio_frames=%d encoded_audio_frames=%d\n", decodedAudioFrames, encodedAudioFrames)
	}
	if *traceVideo {
		fmt.Printf("video_trace(ref:type:interlaced:top_first)=%v\n", videoTrace)
	}
	if *decodeCaptions {
		fmt.Printf("decoded_captions=%d caption_pids=%d\n", captionCount, len(captionDecoders))
	}
	if elapsed > 0 {
		fmt.Printf("video_fps=%.2f audio_fps=%.2f video_mbps=%.3f\n",
			float64(videoFrames)/elapsed.Seconds(), float64(audioFrames)/elapsed.Seconds(), float64(videoBytes*8)/elapsed.Seconds()/1e6)
	}
}

func writePPM(path string, frame nativevideo.YUVFrame) error {
	file, err := os.Create(path) // #nosec G304 -- this CLI writes to the explicit operator-provided output path
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := fmt.Fprintf(file, "P6\n%d %d\n255\n", frame.Width, frame.Height); err != nil {
		return err
	}
	for y := 0; y < frame.Height; y++ {
		for x := 0; x < frame.Width; x++ {
			Y := int(frame.Y[y*frame.Width+x])
			U := int(frame.U[(y/2)*frame.ChromaWidth+x/2]) - 128
			V := int(frame.V[(y/2)*frame.ChromaWidth+x/2]) - 128
			r := clamp(Y + (V*1436)/1024)
			g := clamp(Y - (U*352+V*731)/1024)
			b := clamp(Y + (U*1814)/1024)
			// Values are bounded to [0, 255] by clamp above.
			if _, err := file.Write([]byte{byte(r), byte(g), byte(b)}); err != nil { // #nosec G115
				return err
			}
		}
	}
	return nil
}

func clamp(value int) int {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return value
}

func formatBytes(value uint64) string {
	if value < 1024 {
		return strconv.FormatUint(value, 10) + "B"
	}
	return fmt.Sprintf("%.2fMiB", float64(value)/(1024*1024))
}
