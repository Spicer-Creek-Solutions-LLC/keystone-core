package main

import (
	"bytes"
	"encoding/binary"
	"testing"

	"go.keystone-core.io/keystone-core/internal/protocol"
)

// Review of #354: this binary filled field 7 with make([]byte, NonceBytes), so
// every vector it emitted carried a zero nonce and two runs carried the same
// one. ADR-0005 § 6 requires sixteen bytes from a cryptographic random source.
//
// The defect was invisible to every other check: a zero nonce is the right
// LENGTH, so Decode accepts it, the signature covers it, and the verifier says
// accepted. Only something that looks at the value can see it, which is what
// this does.
func TestProducerGeneratesARealNonce(t *testing.T) {
	nonces := make([][]byte, 0, 3)
	for i := 0; i < 3; i++ {
		v, err := vector(protocol.ClassPresence, []byte("alive"), "", "agent-7")
		if err != nil {
			t.Fatal(err)
		}
		wire, ok := protocol.Unhex(v.EnvelopeHex)
		if !ok {
			t.Fatal("the producer emitted a non-hex envelope")
		}
		fields, err := protocol.Split(wire, protocol.DefaultMaxEnvelope)
		if err != nil {
			t.Fatalf("the producer emitted an envelope Split refuses: %v", err)
		}
		nonce := fields[6]
		if len(nonce) != protocol.NonceBytes {
			t.Fatalf("nonce is %d bytes, want %d", len(nonce), protocol.NonceBytes)
		}
		if bytes.Equal(nonce, make([]byte, protocol.NonceBytes)) {
			t.Fatal("the nonce is zero-filled; ADR-0005 § 6 requires a cryptographic random source")
		}
		nonces = append(nonces, nonce)
	}
	for i := range nonces {
		for j := i + 1; j < len(nonces); j++ {
			if bytes.Equal(nonces[i], nonces[j]) {
				t.Errorf("runs %d and %d produced the same nonce; the field bounds repetition and cannot repeat", i, j)
			}
		}
	}
}

// The timestamp is the other field a producer could quietly freeze, and it has
// the same property: a fixed value is the right length and passes every
// structural check.
func TestProducerStampsTheCurrentTime(t *testing.T) {
	v, err := vector(protocol.ClassPresence, []byte("alive"), "", "agent-7")
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := protocol.Unhex(v.EnvelopeHex)
	fields, err := protocol.Split(wire, protocol.DefaultMaxEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	if ms := int64(binary.BigEndian.Uint64(fields[5])); ms <= 0 {
		t.Errorf("timestamp is %d; want a real Unix millisecond value", ms)
	}
}
