package config

import (
	"encoding/hex"
	"fmt"
)

// SecurityConfig governs the agent-side SecurityEnforcer (Epic 06
// task 4/5) and the control-plane-side command signer. Both binaries
// read the same `security.*` section so the HMAC secret, default
// policy, and command/principal rules stay in lockstep.
//
// HMACSecret is hex-encoded so config files stay copy-pasteable;
// the binary decodes once at startup. Empty HMACSecret disables
// the HMAC check (escape hatch for fresh agents pre-bootstrap; v1.x
// can require non-empty when bootstrap-derived keys land).
//
// PROJECT-DETAILS §4.6 lists `security.{authorization,
// command_filter}` as the agent's top-level security keys; v1.0
// flattens them into the fields below.
type SecurityConfig struct {
	HMACSecret          string   `koanf:"hmacsecret"` //nolint:gosec // PSK-style hex string from operator config — flagged false-positive on field-name pattern
	PrincipalAllowlist  []string `koanf:"principalallowlist"`
	CommandAllowGlobs   []string `koanf:"commandallowglobs"`
	CommandAllowRegexes []string `koanf:"commandallowregexes"`
	CommandDenyGlobs    []string `koanf:"commanddenyglobs"`
	CommandDenyRegexes  []string `koanf:"commanddenyregexes"`
	EnvVarAllowlist     []string `koanf:"envvarallowlist"`
	MaxArgsBytes        int      `koanf:"maxargsbytes"`
	DefaultPolicy       string   `koanf:"defaultpolicy"` // "allow" | "deny" — empty defaults to "deny" at construction
}

// Validate rejects malformed config: HMACSecret hex-decodable when
// non-empty; DefaultPolicy is one of {"", "allow", "deny"};
// MaxArgsBytes non-negative.
func (s SecurityConfig) Validate() error {
	if s.HMACSecret != "" {
		if _, err := hex.DecodeString(s.HMACSecret); err != nil {
			return fmt.Errorf("hmacsecret: not hex: %w", err)
		}
	}
	switch s.DefaultPolicy {
	case "", "allow", "deny":
	default:
		return fmt.Errorf("defaultpolicy: %q (must be allow or deny)", s.DefaultPolicy)
	}
	if s.MaxArgsBytes < 0 {
		return fmt.Errorf("maxargsbytes: must not be negative, got %d", s.MaxArgsBytes)
	}
	return nil
}

// DecodedHMACSecret returns the hex-decoded secret bytes. Caller
// has already passed Validate, so a non-hex value here panics — the
// invariant holds at runtime when SecurityConfig flows from
// config.Load.
func (s SecurityConfig) DecodedHMACSecret() []byte {
	if s.HMACSecret == "" {
		return nil
	}
	b, err := hex.DecodeString(s.HMACSecret)
	if err != nil {
		// Unreachable when Validate ran first.
		panic(fmt.Sprintf("config: SecurityConfig.HMACSecret not hex: %v", err))
	}
	return b
}
