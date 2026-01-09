package webrtc

import (
	"time"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

const (
	// H264 clock rate (90kHz)
	H264ClockRate = 90000
	// Opus clock rate (48kHz)
	OpusClockRate = 48000
	// Max RTP payload size (MTU - IP header - UDP header - RTP header)
	MaxRTPPayloadSize = 1200
	// Default H264 payload type
	H264PayloadType = 96
	// Default Opus payload type
	OpusPayloadType = 111
)

// RTPPacketizer handles RTP packetization for H.264 and Opus
type RTPPacketizer struct {
	h264Payloader *codecs.H264Payloader
	opusPayloader *codecs.OpusPayloader

	videoSSRC      uint32
	audioSSRC      uint32
	videoSeq       uint16
	audioSeq       uint16
	videoTimestamp uint32
	audioTimestamp uint32

	// Frame timing
	videoFrameDuration time.Duration
	audioFrameDuration time.Duration

	// Real-time timestamp tracking
	startTime      time.Time
	lastFrameTime  time.Time
	useRealTime    bool
}

// NewRTPPacketizer creates a new RTP packetizer
func NewRTPPacketizer(videoSSRC, audioSSRC uint32) *RTPPacketizer {
	now := time.Now()
	return &RTPPacketizer{
		h264Payloader:      &codecs.H264Payloader{},
		opusPayloader:      &codecs.OpusPayloader{},
		videoSSRC:          videoSSRC,
		audioSSRC:          audioSSRC,
		videoSeq:           0,
		audioSeq:           0,
		videoTimestamp:     0,
		audioTimestamp:     0,
		videoFrameDuration: time.Second / 30, // 30fps default
		audioFrameDuration: 20 * time.Millisecond, // 20ms Opus frames
		startTime:          now,
		lastFrameTime:      now,
		useRealTime:        true,
	}
}

// PacketizeH264 converts H.264 NAL units to RTP packets
func (p *RTPPacketizer) PacketizeH264(nalUnits []NALUnit) ([]*rtp.Packet, error) {
	var packets []*rtp.Packet

	// Use real-time based timestamp
	if p.useRealTime {
		elapsed := time.Since(p.startTime)
		p.videoTimestamp = uint32(elapsed.Seconds() * H264ClockRate)
	}

	// Filter out SEI NAL units - some browsers have trouble with them
	filteredNALs := make([]NALUnit, 0, len(nalUnits))
	for _, nal := range nalUnits {
		if nal.Type != NALTypeSEI {
			filteredNALs = append(filteredNALs, nal)
		}
	}

	for i, nal := range filteredNALs {
		// Use H264 payloader for ALL NAL units
		// Pion H264Payloader handles SPS/PPS/AUD correctly as single NAL packets
		payloads := p.h264Payloader.Payload(MaxRTPPayloadSize, nal.Data)
		if len(payloads) == 0 {
			continue
		}

		// Check if this is the last NAL unit in this batch
		isLastNAL := i == len(filteredNALs)-1

		for j, payload := range payloads {
			// Set marker bit on the last packet of the last NAL unit in the batch
			// This signals end of access unit to the decoder
			isLastPacket := j == len(payloads)-1

			packet := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    H264PayloadType,
					SequenceNumber: p.videoSeq,
					Timestamp:      p.videoTimestamp,
					SSRC:           p.videoSSRC,
					// Set marker bit on last packet of this access unit
					Marker: isLastNAL && isLastPacket,
				},
				Payload: payload,
			}
			packets = append(packets, packet)
			p.videoSeq++
		}
	}

	return packets, nil
}

// PacketizeH264Single converts a single NAL unit to RTP packets
func (p *RTPPacketizer) PacketizeH264Single(nalData []byte, isLastInFrame bool) []*rtp.Packet {
	payloads := p.h264Payloader.Payload(MaxRTPPayloadSize, nalData)

	var packets []*rtp.Packet
	for i, payload := range payloads {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    H264PayloadType,
				SequenceNumber: p.videoSeq,
				Timestamp:      p.videoTimestamp,
				SSRC:           p.videoSSRC,
				Marker:         i == len(payloads)-1 && isLastInFrame,
			},
			Payload: payload,
		}
		packets = append(packets, packet)
		p.videoSeq++
	}

	return packets
}

// IncrementVideoTimestamp advances video timestamp (call after frame)
func (p *RTPPacketizer) IncrementVideoTimestamp() {
	p.videoTimestamp += uint32(p.videoFrameDuration.Seconds() * H264ClockRate)
}

// PacketizeOpus converts Opus frames to RTP packets
func (p *RTPPacketizer) PacketizeOpus(opusFrame []byte) (*rtp.Packet, error) {
	// Opus frames typically fit in single RTP packet
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    OpusPayloadType,
			SequenceNumber: p.audioSeq,
			Timestamp:      p.audioTimestamp,
			SSRC:           p.audioSSRC,
			Marker:         true, // Opus always sets marker
		},
		Payload: opusFrame,
	}

	p.audioSeq++
	// Increment timestamp (48kHz * 20ms = 960 samples per frame)
	p.audioTimestamp += uint32(p.audioFrameDuration.Seconds() * OpusClockRate)

	return packet, nil
}

// SetVideoFrameRate sets the video frame rate for timestamp calculation
func (p *RTPPacketizer) SetVideoFrameRate(fps float64) {
	if fps > 0 {
		p.videoFrameDuration = time.Duration(float64(time.Second) / fps)
	}
}

// SetAudioFrameDuration sets the audio frame duration for timestamp calculation
func (p *RTPPacketizer) SetAudioFrameDuration(d time.Duration) {
	p.audioFrameDuration = d
}

// GetVideoSequence returns the current video sequence number
func (p *RTPPacketizer) GetVideoSequence() uint16 {
	return p.videoSeq
}

// GetAudioSequence returns the current audio sequence number
func (p *RTPPacketizer) GetAudioSequence() uint16 {
	return p.audioSeq
}

// Reset resets all sequence numbers and timestamps
func (p *RTPPacketizer) Reset() {
	p.videoSeq = 0
	p.audioSeq = 0
	p.videoTimestamp = 0
	p.audioTimestamp = 0
}
