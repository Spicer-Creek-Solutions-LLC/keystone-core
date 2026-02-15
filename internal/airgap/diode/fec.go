package diode

import "fmt"

// FECEncoder generates XOR parity packets for groups of data packets.
type FECEncoder struct {
	groupSize int
	current   [][]byte
	group     uint32
}

// NewFECEncoder creates an encoder with the given group size.
func NewFECEncoder(groupSize int) *FECEncoder {
	if groupSize <= 0 {
		groupSize = DefaultFECGroupSize
	}
	return &FECEncoder{
		groupSize: groupSize,
	}
}

// AddPacket adds a data packet to the current group. Returns a parity packet
// when the group is full, or nil if more packets are needed.
func (e *FECEncoder) AddPacket(data []byte) []byte {
	cp := make([]byte, len(data))
	copy(cp, data)
	e.current = append(e.current, cp)

	if len(e.current) >= e.groupSize {
		parity := e.computeParity()
		e.current = e.current[:0]
		e.group++
		return parity
	}
	return nil
}

// Flush emits a parity packet for any remaining packets in an incomplete group.
// Returns nil if the current group is empty.
func (e *FECEncoder) Flush() []byte {
	if len(e.current) == 0 {
		return nil
	}
	parity := e.computeParity()
	e.current = e.current[:0]
	e.group++
	return parity
}

// Group returns the current FEC group number.
func (e *FECEncoder) Group() uint32 {
	return e.group
}

func (e *FECEncoder) computeParity() []byte {
	maxLen := 0
	for _, p := range e.current {
		if len(p) > maxLen {
			maxLen = len(p)
		}
	}
	parity := make([]byte, maxLen)
	for _, p := range e.current {
		for i := range p {
			parity[i] ^= p[i]
		}
	}
	return parity
}

// FECDecoder recovers lost data packets using XOR parity.
type FECDecoder struct {
	groupSize int
	groups    map[uint32]*fecGroup
}

type fecGroup struct {
	packets map[uint32][]byte // sequence -> payload
	parity  []byte
	count   int
}

// NewFECDecoder creates a decoder with the given group size.
func NewFECDecoder(groupSize int) *FECDecoder {
	if groupSize <= 0 {
		groupSize = DefaultFECGroupSize
	}
	return &FECDecoder{
		groupSize: groupSize,
		groups:    make(map[uint32]*fecGroup),
	}
}

func (d *FECDecoder) getGroup(group uint32) *fecGroup {
	g, ok := d.groups[group]
	if !ok {
		g = &fecGroup{
			packets: make(map[uint32][]byte),
		}
		d.groups[group] = g
	}
	return g
}

// AddDataPacket registers a received data packet.
func (d *FECDecoder) AddDataPacket(group, sequence uint32, payload []byte) {
	g := d.getGroup(group)
	if _, exists := g.packets[sequence]; !exists {
		cp := make([]byte, len(payload))
		copy(cp, payload)
		g.packets[sequence] = cp
		g.count++
	}
}

// AddParityPacket registers the parity data for a group.
func (d *FECDecoder) AddParityPacket(group uint32, parity []byte) {
	g := d.getGroup(group)
	g.parity = make([]byte, len(parity))
	copy(g.parity, parity)
}

// Recover attempts to recover missing packets for a group. The expectedSeqs
// are the sequence numbers expected in the group. Returns recovered packets
// keyed by sequence number. Returns an error if more than one packet is missing.
func (d *FECDecoder) Recover(group uint32, expectedSeqs []uint32) (map[uint32][]byte, error) {
	g, ok := d.groups[group]
	if !ok {
		return nil, fmt.Errorf("%w: group %d not found", ErrFECRecovery, group)
	}

	var missing []uint32
	for _, seq := range expectedSeqs {
		if _, ok := g.packets[seq]; !ok {
			missing = append(missing, seq)
		}
	}

	if len(missing) == 0 {
		return nil, nil
	}

	if len(missing) > 1 {
		return nil, fmt.Errorf("%w: %d packets missing in group %d (max recoverable: 1)",
			ErrFECRecovery, len(missing), group)
	}

	if g.parity == nil {
		return nil, fmt.Errorf("%w: no parity data for group %d", ErrFECRecovery, group)
	}

	// XOR all received packets with parity to recover the missing one
	recovered := make([]byte, len(g.parity))
	copy(recovered, g.parity)
	for _, seq := range expectedSeqs {
		if data, ok := g.packets[seq]; ok {
			for i := range data {
				if i < len(recovered) {
					recovered[i] ^= data[i]
				}
			}
		}
	}

	return map[uint32][]byte{missing[0]: recovered}, nil
}
