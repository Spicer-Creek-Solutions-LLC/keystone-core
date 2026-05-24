// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"crypto/rand"
	"fmt"
)

// base62Alphabet is the URL-safe alphanumeric alphabet used by
// the cleartext-token generator. The dash + underscore in
// base64url are deliberately omitted — the `-` would visually
// collide with the `kscore-join-` separator and `_` is awkward
// in shells.
const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// joinTokenBodyLen is the random-body length appended to
// [JoinTokenScheme] when [EmbeddedProvider.CreateJoinToken]
// generates a cleartext token. Full token length is
// `len(JoinTokenScheme) + joinTokenBodyLen` = 12 + 40 = 52 chars.
// Entropy ≈ log2(62^40) ≈ 238 bits — far above the §4.10 ≥ 32
// minimum.
const joinTokenBodyLen = 40

// joinTokenSaltLen is the salt length each generated token gets.
// 16 bytes = 128 bits — well above the SHA-256 birthday bound
// for a per-token-distinct salt.
const joinTokenSaltLen = 16

// randomBase62 returns n base62 characters drawn from crypto/rand.
//
// Uses byte % 62, which carries a small bias (256 mod 62 = 8, so
// values 0-7 are slightly more likely — ≈4.3% vs ≈3.5% for the
// rest). At joinTokenBodyLen = 40 the residual entropy is ≈ 235
// bits, far more than v0.1 needs for one-time tokens; an
// unbiased rejection-sampling variant is logged under the v1.x
// ROADMAP entry "Unbiased base62 for join-token generation."
func randomBase62(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("identity: randomBase62 n must be > 0, got %d", n)
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("identity: random source: %w", err)
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = base62Alphabet[int(b)%62]
	}
	return string(out), nil
}

// randomSalt returns joinTokenSaltLen bytes of crypto/rand. Pulled
// into its own function so tests can pin the exact source path
// CreateJoinToken uses (and so a future swap to e.g. a counter +
// per-cluster prefix is a single-site edit).
func randomSalt() ([]byte, error) {
	buf := make([]byte, joinTokenSaltLen)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("identity: random salt: %w", err)
	}
	return buf, nil
}
