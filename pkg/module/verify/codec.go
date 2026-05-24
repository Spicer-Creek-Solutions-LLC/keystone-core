// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"encoding/json"
	"fmt"
)

// sigJSON is the on-disk / on-the-wire form of a detached
// signature (the `<module>.sig` artifact). Value is base64 via the
// []byte JSON convention.
type sigJSON struct {
	KeyID     string `json:"key_id"`
	Algorithm string `json:"algorithm"`
	Value     []byte `json:"value"`
}

// MarshalSignature renders sig as the JSON `.sig` artifact.
func MarshalSignature(sig Signature) ([]byte, error) {
	return json.Marshal(sigJSON(sig))
}

// UnmarshalSignature parses a `.sig` artifact.
func UnmarshalSignature(b []byte) (Signature, error) {
	var s sigJSON
	if err := json.Unmarshal(b, &s); err != nil {
		return Signature{}, fmt.Errorf("verify: parse signature: %w", err)
	}
	if s.KeyID == "" || s.Algorithm == "" || len(s.Value) == 0 {
		return Signature{}, fmt.Errorf("%w: incomplete signature artifact", ErrInvalidKey)
	}
	return Signature(s), nil
}
