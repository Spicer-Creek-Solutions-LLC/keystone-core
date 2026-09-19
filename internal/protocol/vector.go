package protocol

import (
	"encoding/hex"
	"encoding/json"
	"io"
)

// Vector is what one process hands another. D-C01-3 requires C01 to pay
// ARCH-COMM-003's serialization-across-processes half: real address spaces,
// production serialization, over files or stdio. At C01 there are no NATS
// subjects, so the transport half moves to C08.
//
// The public halves travel with the envelope because a verifier in another
// process has no other way to obtain them. That is a property of a test vector
// and not of the protocol: in production ADR-0003's enrollment records them.
type Vector struct {
	VerifyingKeyHex string            `json:"verifying_key_hex"`
	RecipientKeyHex string            `json:"recipient_key_hex,omitempty"`
	DecryptionHex   string            `json:"decryption_key_hex,omitempty"`
	EnvelopeHex     string            `json:"envelope_hex"`
	Headers         map[string]string `json:"headers,omitempty"`
}

func WriteVector(w io.Writer, v Vector) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func ReadVector(r io.Reader) (Vector, error) {
	var v Vector
	if err := json.NewDecoder(r).Decode(&v); err != nil {
		return Vector{}, err
	}
	return v, nil
}

// Hex helpers, so neither binary has to agree with the other about anything but
// the field names above.
func Hex(b []byte) string { return hex.EncodeToString(b) }

func Unhex(s string) ([]byte, bool) {
	b, err := hex.DecodeString(s)
	return b, err == nil
}
