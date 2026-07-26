package mpegts

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const maxPESPayloadSize = 8 * 1024 * 1024

var ErrPESPayloadTooLarge = errors.New("MPEG-TS PES payload exceeds safety limit")

// PESPacket is one complete packetized elementary stream payload.
type PESPacket struct {
	PID           uint16
	Stream        Stream
	StreamID      byte
	PTS           uint64
	DTS           uint64
	HasPTS        bool
	HasDTS        bool
	Discontinuity bool
	Payload       []byte
}

// Demuxer converts TS packets into PES units after PAT/PMT discovery. It does
// not decode or alter elementary stream bytes.
type Demuxer struct {
	Analyzer          *Analyzer
	pending           map[uint16][]byte
	pendingReset      map[uint16]bool
	lastCC            map[uint16]byte
	seenCC            map[uint16]bool
	broken            map[uint16]bool
	candidatePrograms []uint16
	selectedProgram   uint16
}

// PESHandler receives complete PES packets in transport-stream order.
// The elementary-stream payload is not decoded or modified.
type PESHandler func(PESPacket) error

// ReadPES parses a transport stream from source and emits complete PES
// packets directly from Go. It is the boundary used by native decoders so
// they do not need to consume an external transcoder pipe.
func (d *Demuxer) ReadPES(ctx context.Context, source io.Reader, handler PESHandler) (Stats, error) {
	reader := NewReader(source)
	var raw [PacketSize]byte

	for {
		select {
		case <-ctx.Done():
			stats := reader.Stats()
			stats.ContinuityErrors = d.Analyzer.Stats.ContinuityErrors
			return stats, ctx.Err()
		default:
		}

		_, err := io.ReadFull(reader, raw[:])
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				packets, flushErr := d.Flush()
				if flushErr != nil {
					return reader.Stats(), flushErr
				}
				for _, packet := range packets {
					if handler != nil {
						if handlerErr := handler(packet); handlerErr != nil {
							return reader.Stats(), handlerErr
						}
					}
				}
				stats := reader.Stats()
				stats.ContinuityErrors = d.Analyzer.Stats.ContinuityErrors
				return stats, nil
			}
			return reader.Stats(), err
		}

		packet, err := ParsePacket(raw[:])
		if err != nil {
			return reader.Stats(), err
		}
		packets, err := d.Push(packet)
		if err != nil {
			return reader.Stats(), err
		}
		for _, packet := range packets {
			if handler != nil {
				if handlerErr := handler(packet); handlerErr != nil {
					return reader.Stats(), handlerErr
				}
			}
		}
	}
}

func NewDemuxer() *Demuxer {
	return &Demuxer{
		Analyzer: NewAnalyzer(), pending: make(map[uint16][]byte), pendingReset: make(map[uint16]bool),
		lastCC: make(map[uint16]byte), seenCC: make(map[uint16]bool), broken: make(map[uint16]bool),
	}
}

// NewDemuxerForProgram limits PES output to one MPEG-TS program number.
func NewDemuxerForProgram(programNumber uint16) *Demuxer {
	return NewDemuxerForPrograms([]uint16{programNumber})
}

// NewDemuxerForPrograms selects the first candidate that has an elementary
// stream in the input. The selection remains fixed for the stream lifetime.
func NewDemuxerForPrograms(programNumbers []uint16) *Demuxer {
	demuxer := NewDemuxer()
	demuxer.candidatePrograms = append([]uint16(nil), programNumbers...)
	return demuxer
}

// SelectedProgram returns the program chosen from the candidate list.
func (d *Demuxer) SelectedProgram() uint16 {
	return d.selectedProgram
}

func (d *Demuxer) Push(packet Packet) ([]PESPacket, error) {
	d.Analyzer.Push(packet)
	if packet.Payload != nil {
		if d.seenCC[packet.PID] {
			if packet.ContinuityCounter == d.lastCC[packet.PID] {
				return nil, nil
			}
			if packet.ContinuityCounter != (d.lastCC[packet.PID]+1)&0x0f {
				delete(d.pending, packet.PID)
				delete(d.pendingReset, packet.PID)
				d.broken[packet.PID] = true
			}
		}
		d.lastCC[packet.PID] = packet.ContinuityCounter
		d.seenCC[packet.PID] = true
		if d.broken[packet.PID] {
			if !packet.PayloadUnitStart {
				return nil, nil
			}
			delete(d.broken, packet.PID)
			d.pendingReset[packet.PID] = true
		}
	}
	if len(d.candidatePrograms) > 0 && d.selectedProgram == 0 {
		for _, candidate := range d.candidatePrograms {
			if _, announced := d.Analyzer.Map.Programs[candidate]; announced {
				d.selectedProgram = candidate
				break
			}
		}
	}
	stream, ok := d.Analyzer.Map.Streams[packet.PID]
	if !ok || packet.Payload == nil {
		return nil, nil
	}
	if len(d.candidatePrograms) > 0 && (d.selectedProgram == 0 || !stream.HasProgram(d.selectedProgram)) {
		return nil, nil
	}
	if stream.StreamType == 0x02 {
		return d.pushStreamingVideo(packet, stream)
	}

	var result []PESPacket
	if packet.PayloadUnitStart {
		if previous := d.pending[packet.PID]; len(previous) > 0 {
			pes, err := parsePES(packet.PID, stream, previous)
			if err == nil {
				pes.Discontinuity = d.pendingReset[packet.PID]
				result = append(result, pes)
			}
		}
		d.pending[packet.PID] = append([]byte(nil), packet.Payload...)
		if len(result) > 0 {
			d.pendingReset[packet.PID] = false
		}
	} else if len(d.pending[packet.PID]) > 0 {
		// A live tuner stream can begin in the middle of a PES. Do not
		// prepend those orphaned TS payloads to the first complete PES.
		d.pending[packet.PID] = append(d.pending[packet.PID], packet.Payload...)
	}
	if len(d.pending[packet.PID]) > maxPESPayloadSize {
		delete(d.pending, packet.PID)
		return nil, fmt.Errorf("%w on PID %#x", ErrPESPayloadTooLarge, packet.PID)
	}
	return result, nil
}

// pushStreamingVideo removes only the PES header and forwards elementary bytes
// per TS packet. Waiting for the next PES start can otherwise batch hundreds of
// milliseconds of live video and produce periodic WebRTC freezes.
func (d *Demuxer) pushStreamingVideo(packet Packet, stream Stream) ([]PESPacket, error) {
	if packet.PayloadUnitStart {
		delete(d.pending, packet.PID)
		d.pending[packet.PID] = append([]byte(nil), packet.Payload...)
	} else if pending := d.pending[packet.PID]; len(pending) > 0 {
		d.pending[packet.PID] = append(pending, packet.Payload...)
	} else {
		return []PESPacket{{
			PID: packet.PID, Stream: stream,
			StreamID: 0xe0, Payload: append([]byte(nil), packet.Payload...),
		}}, nil
	}

	pending := d.pending[packet.PID]
	if len(pending) < 9 {
		return nil, nil
	}
	payloadStart := 9 + int(pending[8])
	if payloadStart > len(pending) {
		if len(pending) > maxPESPayloadSize {
			delete(d.pending, packet.PID)
			return nil, fmt.Errorf("%w on PID %#x", ErrPESPayloadTooLarge, packet.PID)
		}
		return nil, nil
	}
	pes, err := parsePES(packet.PID, stream, pending)
	delete(d.pending, packet.PID)
	if err != nil {
		return nil, err
	}
	if len(pes.Payload) == 0 {
		return nil, nil
	}
	pes.Discontinuity = d.pendingReset[packet.PID]
	delete(d.pendingReset, packet.PID)
	return []PESPacket{pes}, nil
}

func (d *Demuxer) Flush() ([]PESPacket, error) {
	var result []PESPacket
	for pid, payload := range d.pending {
		stream, ok := d.Analyzer.Map.Streams[pid]
		if !ok || len(payload) == 0 {
			continue
		}
		pes, err := parsePES(pid, stream, payload)
		if err == nil {
			result = append(result, pes)
		}
	}
	d.pending = make(map[uint16][]byte)
	d.pendingReset = make(map[uint16]bool)
	return result, nil
}

func parsePES(pid uint16, stream Stream, data []byte) (PESPacket, error) {
	if len(data) < 9 || data[0] != 0x00 || data[1] != 0x00 || data[2] != 0x01 {
		preview := data
		if len(preview) > 16 {
			preview = preview[:16]
		}
		return PESPacket{}, fmt.Errorf("invalid PES start code on PID %#x: %x", pid, preview)
	}
	p := PESPacket{PID: pid, Stream: stream, StreamID: data[3]}
	packetLength := int(binary.BigEndian.Uint16(data[4:6]))
	if packetLength > 0 && 6+packetLength < len(data) {
		data = data[:6+packetLength]
	}
	headerLength := int(data[8])
	payloadStart := 9 + headerLength
	if payloadStart > len(data) {
		return PESPacket{}, fmt.Errorf("PES header exceeds packet on PID %#x", pid)
	}
	flags := data[7]
	if flags&0x80 != 0 && headerLength >= 5 {
		p.PTS = decodeTimestamp(data[9:14])
		p.HasPTS = true
	}
	if flags&0x40 != 0 && headerLength >= 10 {
		p.DTS = decodeTimestamp(data[14:19])
		p.HasDTS = true
	}
	p.Payload = append([]byte(nil), data[payloadStart:]...)
	return p, nil
}

func decodeTimestamp(data []byte) uint64 {
	if len(data) < 5 {
		return 0
	}
	return (uint64(data[0]>>1&0x07) << 30) |
		(uint64(binary.BigEndian.Uint16(data[1:3])>>1) << 15) |
		uint64(binary.BigEndian.Uint16(data[3:5])>>1)
}
