// SPDX-License-Identifier: Apache-2.0

package config

// IdentityConfig drives the embedded identity provider boot in
// kscore-server (Epic 09 task 12). All fields have v0.1 defaults
// that work for a single-server dev / external-tester install;
// production multi-CP deployments override TrustDomain + StoragePath.
type IdentityConfig struct {
	// Enabled toggles the embedded identity provider on/off. v0.1
	// defaults to true so a fresh kscore-server boots with a
	// working CA + join-token endpoint. Operators who don't want
	// identity (e.g. running their own SPIRE) set this false.
	Enabled bool `koanf:"enabled"`

	// TrustDomain is the SPIFFE trust domain. Defaults to
	// identity.DefaultTrustDomain ("kscore.local"). Validation
	// happens inside identity.NewSPIFFEID at construction time.
	TrustDomain string `koanf:"trust_domain"`

	// StoragePath is the directory that holds the CA cert + key
	// files. Defaults to "./.kscore/identity" — operators are
	// expected to override in production (e.g.
	// "/var/lib/kscore/identity").
	StoragePath string `koanf:"storage_path"`

	// KeyType selects the asymmetric algorithm for the root +
	// signing CAs. Defaults to "ecdsa-p256" (per
	// identity.KeyTypeECDSAP256). Other valid values:
	// "ecdsa-p384", "rsa-2048", "rsa-4096".
	KeyType string `koanf:"key_type"`

	// JoinTokensInMemory forces the provider to use an in-memory
	// JoinTokenStore even when a state.Store is otherwise
	// available. Defaults false — the v0.1 production path wires
	// the state-backed store so tokens survive restarts.
	// Useful for development / CI when the test harness doesn't
	// want join-token persistence to bleed across runs.
	JoinTokensInMemory bool `koanf:"join_tokens_in_memory"`
}
