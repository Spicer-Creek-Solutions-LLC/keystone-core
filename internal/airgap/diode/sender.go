package diode

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math"
	"net"
	"os"
	"time"

	airgapsync "github.com/shawnbutts/keystone-core/internal/airgap/sync"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// Sender transmits data over a UDP data diode with optional FEC.
type Sender struct {
	config  Config
	conn    *net.UDPConn
	limiter *airgapsync.BandwidthLimiter
	machine *statemachine.Machine[State, Event]
}

// NewSender creates a sender. Call Send or SendFile, then Close.
func NewSender(config Config) (*Sender, error) {
	config.Defaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	addr, err := net.ResolveUDPAddr("udp", config.Address)
	if err != nil {
		return nil, fmt.Errorf("resolve address: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("dial UDP: %w", err)
	}

	return &Sender{
		config:  config,
		conn:    conn,
		limiter: airgapsync.NewBandwidthLimiter(config.RateLimit),
		machine: buildTransferMachine("sender"),
	}, nil
}

func buildTransferMachine(name string) *statemachine.Machine[State, Event] {
	return statemachine.New[State, Event](StateQueued).
		WithName("diode-" + name).
		WithHistory(20).
		AddTransition(StateQueued, EventBeginSend, StateSending).
		AddTransition(StateSending, EventSendComplete, StateComplete).
		AddTransition(StateSending, EventSendFail, StateFailed).
		MustBuild()
}

// State returns the current transfer state.
func (s *Sender) State() State {
	return s.machine.State()
}

// Send transmits data with the given filename through the diode.
func (s *Sender) Send(ctx context.Context, data []byte, filename string) error {
	if err := s.machine.FireCtx(ctx, EventBeginSend); err != nil {
		return err
	}

	err := s.doSend(ctx, data, filename)
	if err != nil {
		_ = s.machine.FireCtx(ctx, EventSendFail)
		return err
	}
	_ = s.machine.FireCtx(ctx, EventSendComplete)
	return nil
}

// SendFile reads a file and transmits it through the diode.
func (s *Sender) SendFile(ctx context.Context, path string) error {
	data, err := os.ReadFile(path) //#nosec G304 -- path is caller-provided, validated by CLI
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	return s.Send(ctx, data, path)
}

func (s *Sender) doSend(ctx context.Context, data []byte, filename string) error {
	checksum := sha256.Sum256(data)
	payloadSize := s.config.PacketSize - 29 // subtract data packet header overhead
	if payloadSize <= 0 {
		payloadSize = 1
	}
	totalChunks := (len(data) + payloadSize - 1) / payloadSize

	var sessionID [16]byte
	if _, err := rand.Read(sessionID[:]); err != nil {
		return fmt.Errorf("generate session ID: %w", err)
	}

	totalChunksU32 := safeUint32(totalChunks)
	totalSizeU64 := safeUint64(len(data))

	header := &HeaderPacket{
		SessionID:    sessionID,
		TotalChunks:  totalChunksU32,
		TotalSize:    totalSizeU64,
		Checksum:     checksum,
		FECEnabled:   s.config.FECEnabled,
		FECGroupSize: safeUint32(s.config.FECGroupSize),
		Filename:     filename,
		Timestamp:    time.Now().UTC(),
	}

	// Send header 3 times for reliability
	headerBytes := marshalHeader(header)
	for i := 0; i < 3; i++ {
		if err := s.sendPacket(ctx, headerBytes); err != nil {
			return fmt.Errorf("send header: %w", err)
		}
	}

	var fecEncoder *FECEncoder
	if s.config.FECEnabled {
		fecEncoder = NewFECEncoder(s.config.FECGroupSize)
	}

	for seq := 0; seq < totalChunks; seq++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		start := seq * payloadSize
		end := start + payloadSize
		if end > len(data) {
			end = len(data)
		}

		dp := &DataPacket{
			SessionID: sessionID,
			Sequence:  safeUint32(seq),
			Payload:   data[start:end],
		}
		if fecEncoder != nil {
			dp.FECGroup = fecEncoder.Group()
		}

		dpBytes := marshalData(dp)
		if err := s.sendPacket(ctx, dpBytes); err != nil {
			return fmt.Errorf("send data %d: %w", seq, err)
		}

		if fecEncoder != nil {
			if parity := fecEncoder.AddPacket(data[start:end]); parity != nil {
				pp := &ParityPacket{
					SessionID: sessionID,
					FECGroup:  fecEncoder.Group() - 1,
					Parity:    parity,
				}
				if err := s.sendPacket(ctx, marshalParity(pp)); err != nil {
					return fmt.Errorf("send parity: %w", err)
				}
			}
		}
	}

	// Flush remaining FEC group
	if fecEncoder != nil {
		if parity := fecEncoder.Flush(); parity != nil {
			pp := &ParityPacket{
				SessionID: sessionID,
				FECGroup:  fecEncoder.Group() - 1,
				Parity:    parity,
			}
			if err := s.sendPacket(ctx, marshalParity(pp)); err != nil {
				return fmt.Errorf("send final parity: %w", err)
			}
		}
	}

	// Send end packet 3 times
	endPkt := &EndPacket{
		SessionID:   sessionID,
		TotalChunks: totalChunksU32,
		Checksum:    checksum,
	}
	endBytes := marshalEnd(endPkt)
	for i := 0; i < 3; i++ {
		if err := s.sendPacket(ctx, endBytes); err != nil {
			return fmt.Errorf("send end: %w", err)
		}
	}

	return nil
}

func (s *Sender) sendPacket(ctx context.Context, data []byte) error {
	if err := s.limiter.WaitN(ctx, len(data)); err != nil {
		return err
	}
	_, err := s.conn.Write(data)
	return err
}

// Close releases the sender's UDP connection.
func (s *Sender) Close() error {
	return s.conn.Close()
}

func safeUint32(n int) uint32 {
	if n < 0 {
		return 0
	}
	if n > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(n) //#nosec G115 -- bounded
}

func safeUint64(n int) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n) //#nosec G115 -- bounded to non-negative
}
