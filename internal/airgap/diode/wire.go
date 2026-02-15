package diode

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

// marshalHeader encodes a HeaderPacket to bytes.
func marshalHeader(h *HeaderPacket) []byte {
	fnBytes := []byte(h.Filename)
	// type(1) + session(16) + chunks(4) + size(8) + checksum(32) + fec_enabled(1) + fec_group(4) + fn_len(2) + fn + timestamp(8)
	size := 1 + 16 + 4 + 8 + 32 + 1 + 4 + 2 + len(fnBytes) + 8
	buf := make([]byte, size)

	buf[0] = byte(PacketHeader)
	copy(buf[1:17], h.SessionID[:])
	binary.BigEndian.PutUint32(buf[17:21], h.TotalChunks)
	binary.BigEndian.PutUint64(buf[21:29], h.TotalSize)
	copy(buf[29:61], h.Checksum[:])

	if h.FECEnabled {
		buf[61] = 1
	}
	binary.BigEndian.PutUint32(buf[62:66], h.FECGroupSize)

	fnLen := len(fnBytes)
	if fnLen > math.MaxUint16 {
		fnLen = math.MaxUint16
	}
	binary.BigEndian.PutUint16(buf[66:68], uint16(fnLen)) //#nosec G115 -- bounded above
	copy(buf[68:68+fnLen], fnBytes)

	tsNano := h.Timestamp.UnixNano()
	if tsNano < 0 {
		tsNano = 0
	}
	binary.BigEndian.PutUint64(buf[68+fnLen:], uint64(tsNano)) //#nosec G115 -- bounded to non-negative
	return buf
}

// unmarshalHeader decodes a HeaderPacket from bytes.
func unmarshalHeader(data []byte) (*HeaderPacket, error) {
	if len(data) < 76 { // minimum without filename
		return nil, fmt.Errorf("header too short: %d bytes", len(data))
	}
	h := &HeaderPacket{}
	copy(h.SessionID[:], data[1:17])
	h.TotalChunks = binary.BigEndian.Uint32(data[17:21])
	h.TotalSize = binary.BigEndian.Uint64(data[21:29])
	copy(h.Checksum[:], data[29:61])
	h.FECEnabled = data[61] == 1
	h.FECGroupSize = binary.BigEndian.Uint32(data[62:66])

	fnLen := binary.BigEndian.Uint16(data[66:68])
	if len(data) < 68+int(fnLen)+8 {
		return nil, fmt.Errorf("header truncated: need %d, have %d", 68+int(fnLen)+8, len(data))
	}
	h.Filename = string(data[68 : 68+fnLen])

	tsRaw := binary.BigEndian.Uint64(data[68+fnLen:])
	if tsRaw <= math.MaxInt64 {
		h.Timestamp = time.Unix(0, int64(tsRaw)) //#nosec G115 -- bounded
	}
	return h, nil
}

// marshalData encodes a DataPacket to bytes.
func marshalData(d *DataPacket) []byte {
	// type(1) + session(16) + seq(4) + group(4) + payload_len(4) + payload
	pLen := len(d.Payload)
	size := 1 + 16 + 4 + 4 + 4 + pLen
	buf := make([]byte, size)

	buf[0] = byte(PacketData)
	copy(buf[1:17], d.SessionID[:])
	binary.BigEndian.PutUint32(buf[17:21], d.Sequence)
	binary.BigEndian.PutUint32(buf[21:25], d.FECGroup)
	if pLen > math.MaxUint32 {
		pLen = math.MaxUint32
	}
	binary.BigEndian.PutUint32(buf[25:29], uint32(pLen)) //#nosec G115 -- bounded
	copy(buf[29:], d.Payload)
	return buf
}

// unmarshalData decodes a DataPacket from bytes.
func unmarshalData(data []byte) (*DataPacket, error) {
	if len(data) < 29 {
		return nil, fmt.Errorf("data packet too short: %d bytes", len(data))
	}
	d := &DataPacket{}
	copy(d.SessionID[:], data[1:17])
	d.Sequence = binary.BigEndian.Uint32(data[17:21])
	d.FECGroup = binary.BigEndian.Uint32(data[21:25])
	pLen := binary.BigEndian.Uint32(data[25:29])
	if len(data) < 29+int(pLen) {
		return nil, fmt.Errorf("data payload truncated: need %d, have %d", 29+int(pLen), len(data))
	}
	d.Payload = make([]byte, pLen)
	copy(d.Payload, data[29:29+pLen])
	return d, nil
}

// marshalParity encodes a ParityPacket to bytes.
func marshalParity(p *ParityPacket) []byte {
	// type(1) + session(16) + group(4) + parity_len(4) + parity
	pLen := len(p.Parity)
	size := 1 + 16 + 4 + 4 + pLen
	buf := make([]byte, size)

	buf[0] = byte(PacketParity)
	copy(buf[1:17], p.SessionID[:])
	binary.BigEndian.PutUint32(buf[17:21], p.FECGroup)
	if pLen > math.MaxUint32 {
		pLen = math.MaxUint32
	}
	binary.BigEndian.PutUint32(buf[21:25], uint32(pLen)) //#nosec G115 -- bounded
	copy(buf[25:], p.Parity)
	return buf
}

// unmarshalParity decodes a ParityPacket from bytes.
func unmarshalParity(data []byte) (*ParityPacket, error) {
	if len(data) < 25 {
		return nil, fmt.Errorf("parity packet too short: %d bytes", len(data))
	}
	p := &ParityPacket{}
	copy(p.SessionID[:], data[1:17])
	p.FECGroup = binary.BigEndian.Uint32(data[17:21])
	pLen := binary.BigEndian.Uint32(data[21:25])
	if len(data) < 25+int(pLen) {
		return nil, fmt.Errorf("parity payload truncated: need %d, have %d", 25+int(pLen), len(data))
	}
	p.Parity = make([]byte, pLen)
	copy(p.Parity, data[25:25+pLen])
	return p, nil
}

// marshalEnd encodes an EndPacket to bytes.
func marshalEnd(e *EndPacket) []byte {
	// type(1) + session(16) + chunks(4) + checksum(32)
	buf := make([]byte, 53)
	buf[0] = byte(PacketEnd)
	copy(buf[1:17], e.SessionID[:])
	binary.BigEndian.PutUint32(buf[17:21], e.TotalChunks)
	copy(buf[21:53], e.Checksum[:])
	return buf
}

// unmarshalEnd decodes an EndPacket from bytes.
func unmarshalEnd(data []byte) (*EndPacket, error) {
	if len(data) < 53 {
		return nil, fmt.Errorf("end packet too short: %d bytes", len(data))
	}
	e := &EndPacket{}
	copy(e.SessionID[:], data[1:17])
	e.TotalChunks = binary.BigEndian.Uint32(data[17:21])
	copy(e.Checksum[:], data[21:53])
	return e, nil
}

// parsePacketType extracts the packet type from the first byte.
func parsePacketType(data []byte) (PacketType, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("empty packet")
	}
	pt := PacketType(data[0])
	switch pt {
	case PacketHeader, PacketData, PacketParity, PacketEnd:
		return pt, nil
	default:
		return 0, fmt.Errorf("unknown packet type: 0x%02x", data[0])
	}
}
