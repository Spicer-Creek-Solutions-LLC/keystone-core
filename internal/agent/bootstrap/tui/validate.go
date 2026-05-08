package tui

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// Field-validation funcs are pure — they take user-entered text
// and return nil on accept or a UX-quality error on reject. huh
// surfaces the error inline next to the field so users self-
// correct without leaving the screen.
//
// Validation here is deliberately loose ("does this look like a
// reasonable value") — bootstrap.Configuration.Validate, the
// validator phase, and the runtime config loader all enforce
// stricter invariants. The wizard's job is to catch typos before
// the user spends 30 seconds in Validate.

const (
	// minIdentifierLen rejects empty / whitespace-only cluster
	// names + agent IDs. We accept anything reasonable — the
	// runtime invariant on these is "non-empty, parseable" and
	// the rest of the system's validation catches stricter rules.
	minIdentifierLen = 1
	// maxIdentifierLen is generous on purpose. Operators name
	// clusters whatever they want; we just protect against an
	// accidental paste of a multi-KB blob.
	maxIdentifierLen = 253
)

// validateClusterName rejects empty / overlong values and
// whitespace-only input. Format-strictness (DNS-safety) is the
// runtime config loader's job.
func validateClusterName(s string) error {
	s = strings.TrimSpace(s)
	if len(s) < minIdentifierLen {
		return errors.New("cluster name is required")
	}
	if len(s) > maxIdentifierLen {
		return fmt.Errorf("cluster name is %d chars; keep it under %d", len(s), maxIdentifierLen)
	}
	return nil
}

// validateAgentID accepts the same shape as cluster names. The
// runtime invariant is "unique within cluster, parseable" —
// uniqueness is enforced server-side at registration.
func validateAgentID(s string) error {
	s = strings.TrimSpace(s)
	if len(s) < minIdentifierLen {
		return errors.New("agent id is required")
	}
	if len(s) > maxIdentifierLen {
		return fmt.Errorf("agent id is %d chars; keep it under %d", len(s), maxIdentifierLen)
	}
	return nil
}

// validateJoinURL accepts a NATS URL — scheme nats:// or tls://,
// non-empty host. Catches the common typo `nats://::1:4222`
// (unbracketed IPv6) the same way internal/config/nats.go does.
func validateJoinURL(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("nats join url is required")
	}
	// Catch the unbracketed-IPv6 typo before url.Parse — the
	// stdlib parser rejects it with a generic "invalid port"
	// message, which is unhelpful for operators. We grep the
	// post-scheme portion for >1 colons without a leading bracket.
	if rest, ok := strings.CutPrefix(s, "nats://"); ok && unbracketedIPv6(rest) {
		return errors.New("ipv6 hosts must be bracketed: nats://[::1]:4222")
	}
	if rest, ok := strings.CutPrefix(s, "tls://"); ok && unbracketedIPv6(rest) {
		return errors.New("ipv6 hosts must be bracketed: tls://[::1]:4443")
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("not a valid url: %v", err)
	}
	switch u.Scheme {
	case "nats", "tls":
	default:
		return fmt.Errorf("scheme must be nats:// or tls:// (got %q)", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("missing host (example: nats://server:4222)")
	}
	return nil
}

// unbracketedIPv6 returns true when host[:port] looks like a
// bare IPv6 address (>1 colons) but isn't bracketed. This is the
// shape that url.Parse rejects with "invalid port" — surfacing
// the bracket hint up-front is much friendlier.
func unbracketedIPv6(hostPort string) bool {
	if strings.HasPrefix(hostPort, "[") {
		return false
	}
	// Drop trailing path/query before counting colons so we only
	// look at the host:port portion.
	if i := strings.IndexAny(hostPort, "/?#"); i >= 0 {
		hostPort = hostPort[:i]
	}
	return strings.Count(hostPort, ":") > 1
}

// validateConfigPath accepts an absolute path (the agent
// installer's atomic-write step needs an absolute path so a
// later cwd change doesn't change where the file lands).
func validateConfigPath(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("config path is required")
	}
	if !filepath.IsAbs(s) {
		return fmt.Errorf("config path must be absolute (got %q)", s)
	}
	return nil
}
