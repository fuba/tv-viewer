package encoder

import (
	"log"
	"time"
)

const pipelineMetricsInterval = 5 * time.Second

type pipelineMetrics struct {
	channelID string
	started   time.Time

	sourceFrames  int
	decodedFrames int
	encodedFrames int
	encodedBytes  int
	missingPTS    int
	decodeTotal   time.Duration
	decodeMax     time.Duration
	encodeTotal   time.Duration
	encodeMax     time.Duration
	lastOutput    time.Time
	outputGapMax  time.Duration
	lastVideoPTS  uint64
	lastAudioPTS  uint64
	hasVideoPTS   bool
	hasAudioPTS   bool
	audioResyncs  int
}

func newPipelineMetrics(channelID string) *pipelineMetrics {
	return &pipelineMetrics{channelID: channelID, started: time.Now()}
}

func (m *pipelineMetrics) observeDecode(duration time.Duration, frames int) {
	m.decodedFrames += frames
	m.decodeTotal += duration
	if duration > m.decodeMax {
		m.decodeMax = duration
	}
}

func (m *pipelineMetrics) observeEncode(duration time.Duration, bytes int) {
	m.encodedFrames++
	m.encodedBytes += bytes
	m.encodeTotal += duration
	if duration > m.encodeMax {
		m.encodeMax = duration
	}
}

func (m *pipelineMetrics) observeOutput(hasPTS bool) {
	now := time.Now()
	if !m.lastOutput.IsZero() {
		if gap := now.Sub(m.lastOutput); gap > m.outputGapMax {
			m.outputGapMax = gap
		}
	}
	if !hasPTS {
		m.missingPTS++
	}
	m.lastOutput = now
}

func (m *pipelineMetrics) observeVideoPTS(pts uint64, valid bool) {
	if valid {
		m.lastVideoPTS, m.hasVideoPTS = pts, true
	}
}

func (m *pipelineMetrics) observeAudioPTS(pts uint64, valid bool) {
	if valid {
		m.lastAudioPTS, m.hasAudioPTS = pts, true
	}
}

func (m *pipelineMetrics) observeAudioResync() {
	m.audioResyncs++
}

func ptsDeltaMilliseconds(current, base uint64) float64 {
	delta := (current - base) & broadcastPTSMask
	signed := int64(delta)
	if delta > broadcastPTSMask/2 {
		signed -= int64(broadcastPTSMask + 1)
	}
	return float64(signed) / 90
}

func (m *pipelineMetrics) maybeLog() {
	now := time.Now()
	elapsed := now.Sub(m.started)
	if elapsed < pipelineMetricsInterval {
		return
	}
	seconds := elapsed.Seconds()
	decodeAverage := time.Duration(0)
	if m.sourceFrames > 0 {
		decodeAverage = m.decodeTotal / time.Duration(m.sourceFrames)
	}
	encodeAverage := time.Duration(0)
	if m.encodedFrames > 0 {
		encodeAverage = m.encodeTotal / time.Duration(m.encodedFrames)
	}
	avDelta := 0.0
	if m.hasVideoPTS && m.hasAudioPTS {
		avDelta = ptsDeltaMilliseconds(m.lastAudioPTS, m.lastVideoPTS)
	}
	log.Printf("[Native][Stats] channel=%s source_fps=%.2f decoded_fps=%.2f encoded_fps=%.2f bitrate_mbps=%.2f decode_avg_ms=%.2f decode_max_ms=%.2f encode_avg_ms=%.2f encode_max_ms=%.2f output_gap_max_ms=%.2f missing_pts=%d av_delta_ms=%.2f audio_resyncs=%d",
		m.channelID,
		float64(m.sourceFrames)/seconds,
		float64(m.decodedFrames)/seconds,
		float64(m.encodedFrames)/seconds,
		float64(m.encodedBytes*8)/seconds/1_000_000,
		float64(decodeAverage)/float64(time.Millisecond),
		float64(m.decodeMax)/float64(time.Millisecond),
		float64(encodeAverage)/float64(time.Millisecond),
		float64(m.encodeMax)/float64(time.Millisecond),
		float64(m.outputGapMax)/float64(time.Millisecond),
		m.missingPTS,
		avDelta,
		m.audioResyncs,
	)
	m.started = now
	m.sourceFrames = 0
	m.decodedFrames = 0
	m.encodedFrames = 0
	m.encodedBytes = 0
	m.missingPTS = 0
	m.decodeTotal = 0
	m.decodeMax = 0
	m.encodeTotal = 0
	m.encodeMax = 0
	m.outputGapMax = 0
	m.audioResyncs = 0
}
