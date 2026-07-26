package mpegts

import "encoding/binary"

// Stream describes an elementary stream announced by a PMT.
type Stream struct {
	PID            uint16
	StreamType     byte
	ProgramNumbers map[uint16]struct{}
}

// HasProgram reports whether a stream is announced by the given program.
func (s Stream) HasProgram(programNumber uint16) bool {
	_, ok := s.ProgramNumbers[programNumber]
	return ok
}

type ProgramMap struct {
	Programs map[uint16]uint16 // program number -> PMT PID
	Streams  map[uint16]Stream // elementary PID -> stream metadata
}

func NewProgramMap() *ProgramMap {
	return &ProgramMap{Programs: make(map[uint16]uint16), Streams: make(map[uint16]Stream)}
}

// Analyzer extracts PAT/PMT metadata and continuity diagnostics while packets
// pass through unchanged.
type Analyzer struct {
	Map      *ProgramMap
	lastCC   map[uint16]byte
	seenCC   map[uint16]bool
	sections map[uint16][]byte
	streams  map[uint16]map[uint16]byte // program number -> elementary PID -> stream type
	Stats    Stats
}

func NewAnalyzer() *Analyzer {
	return &Analyzer{
		Map: NewProgramMap(), lastCC: make(map[uint16]byte), seenCC: make(map[uint16]bool),
		sections: make(map[uint16][]byte), streams: make(map[uint16]map[uint16]byte),
	}
}

func (a *Analyzer) Push(packet Packet) {
	if packet.Payload != nil {
		if a.seenCC[packet.PID] && (packet.ContinuityCounter != ((a.lastCC[packet.PID] + 1) & 0x0f)) {
			a.Stats.ContinuityErrors++
		}
		a.lastCC[packet.PID] = packet.ContinuityCounter
		a.seenCC[packet.PID] = true
	}
	if packet.PID != PATPID && !a.isPMTPID(packet.PID) {
		return
	}
	if packet.Payload == nil {
		return
	}
	payload := packet.Payload
	if packet.PayloadUnitStart {
		if len(payload) == 0 {
			return
		}
		pointer := int(payload[0])
		if pointer+1 > len(payload) {
			return
		}
		payload = payload[1+pointer:]
		a.sections[packet.PID] = nil
	}
	a.sections[packet.PID] = append(a.sections[packet.PID], payload...)
	for {
		section := a.sections[packet.PID]
		if len(section) < 3 {
			return
		}
		length := int(binary.BigEndian.Uint16(section[1:3]) & 0x0fff)
		total := 3 + length
		if length < 4 || total > 4096 {
			a.sections[packet.PID] = nil
			return
		}
		if len(section) < total {
			return
		}
		a.consumeSection(packet.PID, section[:total])
		a.sections[packet.PID] = section[total:]
	}
}

func (a *Analyzer) isPMTPID(pid uint16) bool {
	for _, pmtPID := range a.Map.Programs {
		if pmtPID == pid {
			return true
		}
	}
	return false
}

func (a *Analyzer) consumeSection(pid uint16, section []byte) {
	switch {
	case pid == PATPID && section[0] == 0x00:
		a.consumePAT(section)
	case pid != PATPID && section[0] == 0x02:
		a.consumePMT(pid, section)
	}
}

func (a *Analyzer) consumePAT(section []byte) {
	end := len(section) - 4 // exclude CRC32
	for pos := 8; pos+4 <= end; pos += 4 {
		program := binary.BigEndian.Uint16(section[pos : pos+2])
		pmtPID := binary.BigEndian.Uint16(section[pos+2:pos+4]) & 0x1fff
		if program != 0 {
			a.Map.Programs[program] = pmtPID
		}
	}
}

func (a *Analyzer) consumePMT(pid uint16, section []byte) {
	if len(section) < 12 {
		return
	}
	programNumber := a.programForPMTPID(pid)
	if programNumber == 0 {
		return
	}
	programInfoLength := int(binary.BigEndian.Uint16(section[10:12]) & 0x0fff)
	pos := 12 + programInfoLength
	end := len(section) - 4
	programStreams := make(map[uint16]byte)
	for pos+5 <= end {
		streamType := section[pos]
		elementaryPID := binary.BigEndian.Uint16(section[pos+1:pos+3]) & 0x1fff
		esInfoLength := int(binary.BigEndian.Uint16(section[pos+3:pos+5]) & 0x0fff)
		pos += 5
		if pos+esInfoLength > end {
			return
		}
		programStreams[elementaryPID] = streamType
		pos += esInfoLength
	}
	a.streams[programNumber] = programStreams
	a.rebuildStreams()
}

func (a *Analyzer) rebuildStreams() {
	streams := make(map[uint16]Stream)
	for programNumber, programStreams := range a.streams {
		for pid, streamType := range programStreams {
			stream := streams[pid]
			stream.PID = pid
			stream.StreamType = streamType
			if stream.ProgramNumbers == nil {
				stream.ProgramNumbers = make(map[uint16]struct{})
			}
			stream.ProgramNumbers[programNumber] = struct{}{}
			streams[pid] = stream
		}
	}
	a.Map.Streams = streams
}

func (a *Analyzer) programForPMTPID(pid uint16) uint16 {
	for programNumber, pmtPID := range a.Map.Programs {
		if pmtPID == pid {
			return programNumber
		}
	}
	return 0
}
