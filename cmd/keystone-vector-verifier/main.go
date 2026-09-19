// Command keystone-vector-verifier reads one protocol vector on stdin and says
// whether it is acceptable.
//
// It prints a single line -- "accepted", or one of ADR-0005 § 9's seven refusal
// codes -- and exits zero or one. That vocabulary is deliberate: what a separate
// process can observe about a refusal is exactly what an outside party can
// observe, so the boundary is the right place to check that the codes stay
// coarse.
package main

import (
	"fmt"
	"os"

	"go.keystone-core.io/keystone-core/internal/protocol"
)

func main() {
	line, ok := verify(os.Stdin)
	fmt.Println(line)
	if !ok {
		os.Exit(1)
	}
}

func verify(r *os.File) (string, bool) {
	v, err := protocol.ReadVector(r)
	if err != nil {
		return protocol.MalformedEnvelope.Error(), false
	}
	wire, ok := protocol.Unhex(v.EnvelopeHex)
	if !ok {
		return protocol.MalformedEnvelope.Error(), false
	}
	keyBytes, ok := protocol.Unhex(v.VerifyingKeyHex)
	if !ok {
		return protocol.SignatureInvalid.Error(), false
	}

	envelope, err := protocol.Decode(wire, protocol.DefaultMaxEnvelope)
	if err != nil {
		return err.Error(), false
	}
	if err := protocol.CheckHeaders(envelope, v.Headers); err != nil {
		return err.Error(), false
	}
	verifying, err := protocol.ParseVerifyingKey(keyBytes)
	if err != nil {
		return err.Error(), false
	}
	if err := protocol.VerifyEnvelope(verifying, envelope); err != nil {
		return err.Error(), false
	}

	if protocol.Encrypted(envelope.Class) && v.DecryptionHex != "" {
		secret, ok := protocol.Unhex(v.DecryptionHex)
		if !ok {
			return protocol.DecryptionFailed.Error(), false
		}
		decryption, err := protocol.ParseDecryptionKey(secret)
		if err != nil {
			return err.Error(), false
		}
		if _, err := protocol.OpenEnvelope(decryption, envelope); err != nil {
			return err.Error(), false
		}
	}
	return "accepted", true
}
