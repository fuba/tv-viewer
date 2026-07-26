// Package mpegts provides a small, streaming MPEG-2 transport stream parser.
// It deliberately keeps elementary stream payloads untouched; decoding is a
// separate concern owned by the media pipeline.
package mpegts

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
)

const (
	PacketSize = 188
	SyncByte   = 0x47
	PATPID     = 0x0000
	NullPID    = 0x1fff
)

var ErrInvalidPacket = errors.New("invalid MPEG-TS packet")

// Packet is one 188-byte MPEG-TS packet.
type Packet struct {
	Raw               [PacketSize]byte
	PID               uint16
	PayloadUnitStart  bool
	TransportError    bool
	AdaptationField   bool
	Payload           []byte
	ContinuityCounter byte
	HasPCR            bool
	PCR               uint64
}

func ParsePacket(raw []byte) (Packet, error) {
	var p Packet
	if len(raw) != PacketSize || raw[0] != SyncByte {
		return p, ErrInvalidPacket
	}
	copy(p.Raw[:], raw)
	p.TransportError = raw[1]&0x80 != 0
	p.PayloadUnitStart = raw[1]&0x40 != 0
	p.PID = uint16(raw[1]&0x1f)<<8 | uint16(raw[2])
	p.ContinuityCounter = raw[3] & 0x0f
	control := (raw[3] >> 4) & 0x03
	if control == 0 {
		return p, fmt.Errorf("%w: reserved adaptation field control", ErrInvalidPacket)
	}

	pos := 4
	if control == 2 || control == 3 {
		if pos >= len(raw) {
			return p, fmt.Errorf("%w: missing adaptation field", ErrInvalidPacket)
		}
		length := int(raw[pos])
		if pos+1+length > len(raw) {
			return p, fmt.Errorf("%w: adaptation field exceeds packet", ErrInvalidPacket)
		}
		p.AdaptationField = true
		if length >= 7 && raw[pos+1]&0x10 != 0 {
			base := pos + 2
			p.PCR = uint64(raw[base])<<25 |
				uint64(raw[base+1])<<17 |
				uint64(raw[base+2])<<9 |
				uint64(raw[base+3])<<1 |
				uint64(raw[base+4]>>7)
			p.HasPCR = true
		}
		pos += 1 + length
	}
	if control == 1 || control == 3 {
		p.Payload = raw[pos:]
	}
	return p, nil
}

// Reader aligns an arbitrary byte stream to MPEG-TS packets. It preserves
// valid packets byte-for-byte and drops only leading garbage and null packets.
type Reader struct {
	source   io.Reader
	buf      []byte
	out      []byte
	stats    Stats
	onPacket func(Packet)
	mu       sync.RWMutex
}

type Stats struct {
	Packets          uint64
	InvalidPackets   uint64
	NullPackets      uint64
	TransportErrors  uint64
	ResyncBytes      uint64
	ContinuityErrors uint64
	LastPCR          uint64
}

func NewReader(source io.Reader) *Reader {
	return &Reader{source: source, buf: make([]byte, 0, 64*1024)}
}

// SetPacketHandler registers a synchronous callback invoked for each parsed
// packet. It must be called before concurrent reads begin.
func (r *Reader) SetPacketHandler(handler func(Packet)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onPacket = handler
}

func (r *Reader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(r.out) < len(p) {
		packet, err := r.nextPacket()
		if err != nil {
			if len(r.out) > 0 {
				break
			}
			return 0, err
		}
		if packet.PID == NullPID {
			r.record(func(s *Stats) { s.NullPackets++ })
			continue
		}
		if packet.TransportError {
			r.record(func(s *Stats) { s.TransportErrors++ })
			continue
		}
		r.out = append(r.out, packet.Raw[:]...)
	}
	n := copy(p, r.out)
	r.out = r.out[n:]
	return n, nil
}

func (r *Reader) nextPacket() (Packet, error) {
	for {
		for len(r.buf) < PacketSize {
			chunk := make([]byte, 32*1024)
			n, err := r.source.Read(chunk)
			if n > 0 {
				r.buf = append(r.buf, chunk[:n]...)
			}
			if err != nil {
				if len(r.buf) < PacketSize {
					return Packet{}, err
				}
				break
			}
			if n == 0 {
				return Packet{}, io.ErrNoProgress
			}
		}

		candidate := -1
		for i := 0; i < len(r.buf); i++ {
			if r.buf[i] != SyncByte || len(r.buf)-i < PacketSize {
				continue
			}
			if len(r.buf)-i < PacketSize*2 || r.buf[i+PacketSize] == SyncByte {
				candidate = i
				break
			}
		}
		if candidate < 0 {
			keep := PacketSize - 1
			if len(r.buf) > keep {
				r.record(func(s *Stats) { s.ResyncBytes += uint64(len(r.buf) - keep) })
				r.buf = append([]byte(nil), r.buf[len(r.buf)-keep:]...)
			}
			continue
		}
		if candidate > 0 {
			r.record(func(s *Stats) { s.ResyncBytes += uint64(candidate) })
			r.buf = r.buf[candidate:]
		}
		raw := append([]byte(nil), r.buf[:PacketSize]...)
		r.buf = r.buf[PacketSize:]
		packet, err := ParsePacket(raw)
		if err != nil {
			r.record(func(s *Stats) { s.InvalidPackets++ })
			continue
		}
		r.record(func(s *Stats) {
			s.Packets++
			if packet.HasPCR {
				s.LastPCR = packet.PCR
			}
		})
		r.mu.RLock()
		handler := r.onPacket
		r.mu.RUnlock()
		if handler != nil {
			handler(packet)
		}
		return packet, nil
	}
}

func (r *Reader) Stats() Stats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stats
}

func (r *Reader) record(fn func(*Stats)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(&r.stats)
}

// ReadAllPackets is useful for bounded fixtures and parser tests.
func ReadAllPackets(data []byte) ([]Packet, Stats, error) {
	r := NewReader(bytes.NewReader(data))
	var packets []Packet
	for {
		p, err := r.nextPacket()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return packets, r.Stats(), nil
			}
			return packets, r.Stats(), err
		}
		packets = append(packets, p)
	}
}
