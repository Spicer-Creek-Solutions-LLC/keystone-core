package diode

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// receiveSession tracks the state of an in-progress receive.
type receiveSession struct {
	header      *HeaderPacket
	chunks      map[uint32][]byte
	fecDecoder  *FECDecoder
	receivedEnd bool
	endPacket   *EndPacket
	startedAt   time.Time
}

// Receiver listens for data diode transfers over UDP.
type Receiver struct {
	config Config
	conn   *net.UDPConn
}

// NewReceiver creates a receiver that listens on the configured address.
func NewReceiver(config Config) (*Receiver, error) {
	config.Defaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	addr, err := net.ResolveUDPAddr("udp", config.Address)
	if err != nil {
		return nil, fmt.Errorf("resolve address: %w", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen UDP: %w", err)
	}

	return &Receiver{
		config: config,
		conn:   conn,
	}, nil
}

// Receive waits for a complete transfer and returns the filename and data.
func (r *Receiver) Receive(ctx context.Context) (filename string, data []byte, err error) {
	sessions := make(map[[16]byte]*receiveSession)
	buf := make([]byte, 65535)

	for {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}

		if setErr := r.conn.SetReadDeadline(time.Now().Add(1 * time.Second)); setErr != nil {
			return "", nil, fmt.Errorf("set deadline: %w", setErr)
		}

		n, _, readErr := r.conn.ReadFromUDP(buf)
		if readErr != nil {
			var netErr net.Error
			if errors.As(readErr, &netErr) && netErr.Timeout() {
				for id, sess := range sessions {
					if time.Since(sess.startedAt) > r.config.Timeout {
						delete(sessions, id)
					}
				}
				continue
			}
			return "", nil, fmt.Errorf("read: %w", readErr)
		}

		pktData := make([]byte, n)
		copy(pktData, buf[:n])

		pt, parseErr := parsePacketType(pktData)
		if parseErr != nil {
			continue
		}

		switch pt {
		case PacketHeader:
			h, hErr := unmarshalHeader(pktData)
			if hErr != nil {
				continue
			}
			if _, exists := sessions[h.SessionID]; !exists {
				sess := &receiveSession{
					header:    h,
					chunks:    make(map[uint32][]byte),
					startedAt: time.Now(),
				}
				if h.FECEnabled {
					sess.fecDecoder = NewFECDecoder(int(h.FECGroupSize))
				}
				sessions[h.SessionID] = sess
			}

		case PacketData:
			d, dErr := unmarshalData(pktData)
			if dErr != nil {
				continue
			}
			sess, ok := sessions[d.SessionID]
			if !ok {
				continue
			}
			if _, exists := sess.chunks[d.Sequence]; !exists {
				sess.chunks[d.Sequence] = d.Payload
			}
			if sess.fecDecoder != nil {
				sess.fecDecoder.AddDataPacket(d.FECGroup, d.Sequence, d.Payload)
			}

		case PacketParity:
			p, pErr := unmarshalParity(pktData)
			if pErr != nil {
				continue
			}
			sess, ok := sessions[p.SessionID]
			if !ok {
				continue
			}
			if sess.fecDecoder != nil {
				sess.fecDecoder.AddParityPacket(p.FECGroup, p.Parity)
			}

		case PacketEnd:
			e, eErr := unmarshalEnd(pktData)
			if eErr != nil {
				continue
			}
			sess, ok := sessions[e.SessionID]
			if !ok {
				continue
			}
			sess.receivedEnd = true
			sess.endPacket = e

			assembled, aErr := r.assembleSession(sess)
			if aErr != nil {
				continue
			}
			delete(sessions, e.SessionID)
			return sess.header.Filename, assembled, nil
		}
	}
}

// ReceiveToFile waits for a transfer and writes it to the output directory.
func (r *Receiver) ReceiveToFile(ctx context.Context, outputDir string) (string, error) {
	fn, data, err := r.Receive(ctx)
	if err != nil {
		return "", err
	}

	outPath := filepath.Join(outputDir, filepath.Base(fn))
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return outPath, nil
}

func (r *Receiver) assembleSession(sess *receiveSession) ([]byte, error) {
	totalChunks := int(sess.header.TotalChunks)

	// Attempt FEC recovery for missing chunks
	if sess.fecDecoder != nil {
		groupSize := int(sess.header.FECGroupSize)
		numGroups := (totalChunks + groupSize - 1) / groupSize

		for g := 0; g < numGroups; g++ {
			var expectedSeqs []uint32
			for s := g * groupSize; s < (g+1)*groupSize && s < totalChunks; s++ {
				expectedSeqs = append(expectedSeqs, safeUint32(s))
			}

			recovered, recErr := sess.fecDecoder.Recover(safeUint32(g), expectedSeqs)
			if recErr != nil {
				continue
			}
			for seq, recData := range recovered {
				if _, exists := sess.chunks[seq]; !exists {
					sess.chunks[seq] = recData
				}
			}
		}
	}

	if len(sess.chunks) < totalChunks {
		return nil, fmt.Errorf("incomplete transfer: got %d of %d chunks", len(sess.chunks), totalChunks)
	}

	// Sort and reassemble
	seqs := make([]int, 0, len(sess.chunks))
	for seq := range sess.chunks {
		seqs = append(seqs, int(seq))
	}
	sort.Ints(seqs)

	var assembled []byte
	for _, seq := range seqs {
		assembled = append(assembled, sess.chunks[safeUint32(seq)]...)
	}

	// Verify checksum
	checksum := sha256.Sum256(assembled)
	if checksum != sess.header.Checksum {
		return nil, ErrChecksumFailed
	}

	return assembled, nil
}

// Close releases the receiver's UDP connection.
func (r *Receiver) Close() error {
	return r.conn.Close()
}
