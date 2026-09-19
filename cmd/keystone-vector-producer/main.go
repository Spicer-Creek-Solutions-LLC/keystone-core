// Command keystone-vector-producer emits one protocol vector on stdout.
//
// It exists for D-C01-3: C01 ships a producer and a verifier run as separate OS
// processes, so production serialization crosses a real address-space boundary
// the day it merges. No broker is involved and none is needed -- the protocol
// precedes the transport.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"go.keystone-core.io/keystone-core/internal/protocol"
)

func main() {
	class := flag.String("class", string(protocol.ClassCommand), "message class token")
	payload := flag.String("payload", "argv", "payload bytes")
	jobID := flag.String("job", "job-01hx9yq", "job identifier")
	sender := flag.String("sender", protocol.SenderCommandPublisher, "sender identifier")
	flag.Parse()

	if err := produce(protocol.Class(*class), []byte(*payload), *jobID, *sender); err != nil {
		fmt.Fprintln(os.Stderr, "keystone-vector-producer:", err)
		os.Exit(1)
	}
}

func produce(class protocol.Class, payload []byte, jobID, sender string) error {
	v, err := vector(class, payload, jobID, sender)
	if err != nil {
		return err
	}
	return protocol.WriteVector(os.Stdout, v)
}

// vector is produce without the writing, so a test can inspect what a run
// actually emits rather than trust that it emits the right thing.
func vector(class protocol.Class, payload []byte, jobID, sender string) (protocol.Vector, error) {
	signing, err := protocol.GenerateSigningKey()
	if err != nil {
		return protocol.Vector{}, err
	}
	nonce, err := protocol.NewNonce()
	if err != nil {
		return protocol.Vector{}, err
	}
	e := protocol.Envelope{
		Version:   protocol.Version,
		Class:     class,
		JobID:     jobID,
		Sender:    sender,
		Timestamp: time.UnixMilli(time.Now().UnixMilli()).UTC(),
		Nonce:     nonce,
		Payload:   payload,
	}

	v := protocol.Vector{VerifyingKeyHex: protocol.Hex(signing.Verifying().Bytes())}

	if protocol.Encrypted(class) {
		recipient, err := protocol.GenerateDecryptionKey()
		if err != nil {
			return protocol.Vector{}, err
		}
		if e, err = protocol.SealEnvelope(recipient.Recipient(), e); err != nil {
			return protocol.Vector{}, err
		}
		v.RecipientKeyHex = protocol.Hex(recipient.Recipient().Bytes())
		// The private half travels too, because the verifier is a separate
		// process with no enrollment to have recorded it. A vector is not a
		// deployment.
		v.DecryptionHex = protocol.Hex(protocol.MarshalDecryptionKey(recipient))
	}

	signed, err := protocol.SignEnvelope(signing, e)
	if err != nil {
		return protocol.Vector{}, err
	}
	wire, ok := signed.Encode()
	if !ok {
		return protocol.Vector{}, fmt.Errorf("encode refused a signed envelope")
	}
	v.EnvelopeHex = protocol.Hex(wire)
	v.Headers = protocol.Headers(signed, "")
	return v, nil
}
