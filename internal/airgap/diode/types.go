// Package diode provides UDP data diode support for one-way transfers
// in classified air-gapped environments. It includes forward error correction
// (FEC) for recovering lost packets without retransmission.
package diode

import (
	"errors"
	"fmt"
	"time"
)

// PacketType identifies the kind of packet on the wire.
type PacketType byte

// Wire protocol packet types.
const (
	PacketHeader PacketType = 0x01
	PacketData   PacketType = 0x02
	PacketParity PacketType = 0x03
	PacketEnd    PacketType = 0x04
)

// State represents the lifecycle state of a diode transfer.
type State string

// Transfer states.
const (
	StateQueued   State = "queued"
	StateSending  State = "sending"
	StateComplete State = "complete"
	StateFailed   State = "failed"
)

// Event triggers state transitions in a diode transfer.
type Event string

// Transfer events.
const (
	EventBeginSend   Event = "begin_send"
	EventSendComplete Event = "send_complete"
	EventSendFail    Event = "send_fail"
)

// HeaderPacket starts a transfer session.
type HeaderPacket struct {
	SessionID    [16]byte
	TotalChunks  uint32
	TotalSize    uint64
	Checksum     [32]byte // SHA-256
	FECEnabled   bool
	FECGroupSize uint32
	Filename     string
	Timestamp    time.Time
}

// DataPacket carries a chunk of the file.
type DataPacket struct {
	SessionID [16]byte
	Sequence  uint32
	FECGroup  uint32
	Payload   []byte
}

// ParityPacket carries FEC recovery data for a group.
type ParityPacket struct {
	SessionID [16]byte
	FECGroup  uint32
	Parity    []byte
}

// EndPacket signals the end of a transfer.
type EndPacket struct {
	SessionID   [16]byte
	TotalChunks uint32
	Checksum    [32]byte
}

// Config holds sender/receiver configuration.
type Config struct {
	Address       string        `json:"address" yaml:"address"`
	PacketSize    int           `json:"packet_size" yaml:"packet_size"`
	RateLimit     int64         `json:"rate_limit" yaml:"rate_limit"`
	FECEnabled    bool          `json:"fec_enabled" yaml:"fec_enabled"`
	FECGroupSize  int           `json:"fec_group_size" yaml:"fec_group_size"`
	Timeout       time.Duration `json:"timeout" yaml:"timeout"`
}

// DefaultPacketSize is the default UDP payload size.
const DefaultPacketSize = 1400

// DefaultFECGroupSize is the default number of data packets per FEC group.
const DefaultFECGroupSize = 5

// Errors returned by diode operations.
var (
	ErrInvalidConfig  = errors.New("invalid diode configuration")
	ErrSessionTimeout = errors.New("session timed out")
	ErrChecksumFailed = errors.New("checksum verification failed")
	ErrFECRecovery    = errors.New("FEC recovery failed")
)

// Validate checks that the Config is well-formed.
func (c *Config) Validate() error {
	if c.Address == "" {
		return fmt.Errorf("%w: address is required", ErrInvalidConfig)
	}
	if c.PacketSize < 0 {
		return fmt.Errorf("%w: packet_size must be non-negative", ErrInvalidConfig)
	}
	if c.FECGroupSize < 0 {
		return fmt.Errorf("%w: fec_group_size must be non-negative", ErrInvalidConfig)
	}
	return nil
}

// Defaults fills in zero-valued fields with defaults.
func (c *Config) Defaults() {
	if c.PacketSize == 0 {
		c.PacketSize = DefaultPacketSize
	}
	if c.FECGroupSize == 0 {
		c.FECGroupSize = DefaultFECGroupSize
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
}
