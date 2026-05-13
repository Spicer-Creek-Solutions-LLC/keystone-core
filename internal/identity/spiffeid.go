package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
)

// DefaultTrustDomain is the embedded-provider default per
// PROJECT-DETAILS §4.10. Operators override via the
// identity.trust_domain config key once the [Provider] wiring lands
// (task 7); v0.1 code paths use this constant directly.
const DefaultTrustDomain = "kscore.local"

// Standard path prefixes for the canonical Keystone SPIFFE IDs:
//
//   - spiffe://<td>/server/<name>  — [ServerID]
//   - spiffe://<td>/agent/<id>     — [AgentID]
//   - spiffe://<td>/service/<name> — [ServiceID]
//
// Constants rather than literals so callers grep cleanly and so the
// scheme moves in one place if the project ever picks different
// roots.
const (
	pathPrefixServer  = "server"
	pathPrefixAgent   = "agent"
	pathPrefixService = "service"
)

// ErrInvalidSPIFFEID wraps every grammar / shape rejection returned
// by this package. Callers that need the underlying upstream error
// can [errors.Unwrap] to the [github.com/spiffe/go-spiffe/v2/spiffeid]
// sentinel.
var ErrInvalidSPIFFEID = errors.New("identity: invalid SPIFFE ID")

// SPIFFEID is a SPIFFE identifier — `spiffe://<trust-domain>/<path>`.
// Every constructor validates against the SPIFFE-ID spec (delegated
// to [github.com/spiffe/go-spiffe/v2/spiffeid] for canonical
// compliance) and wraps any rejection in [ErrInvalidSPIFFEID].
//
// The zero value is invalid and round-trips as the empty string;
// [SPIFFEID.IsZero] distinguishes it. Fields are unexported so the
// only path in is through a validating call.
//
// SPIFFEID is comparable, so == works for equality (the underlying
// spiffeid.ID is canonical after parse). [SPIFFEID.Equal] is the
// preferred form when reading.
type SPIFFEID struct {
	id spiffeid.ID
}

// ParseSPIFFEID accepts a canonical SPIFFE ID string
// (`spiffe://kscore.local/agent/agent-1`) and returns the typed
// value. Errors wrap [ErrInvalidSPIFFEID].
func ParseSPIFFEID(s string) (SPIFFEID, error) {
	if s == "" {
		return SPIFFEID{}, fmt.Errorf("%w: empty string", ErrInvalidSPIFFEID)
	}
	id, err := spiffeid.FromString(s)
	if err != nil {
		return SPIFFEID{}, fmt.Errorf("%w: %v", ErrInvalidSPIFFEID, err)
	}
	return SPIFFEID{id: id}, nil
}

// MustParseSPIFFEID panics on rejection. Test-only — production code
// should always handle the error from [ParseSPIFFEID].
func MustParseSPIFFEID(s string) SPIFFEID {
	id, err := ParseSPIFFEID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// NewSPIFFEID builds an ID from a trust-domain name + a sequence of
// path segments. Each segment is validated independently — empty
// segments and segments containing `/` are rejected. Passing zero
// segments returns the trust-domain-only ID `spiffe://<td>`.
func NewSPIFFEID(trustDomain string, segments ...string) (SPIFFEID, error) {
	td, err := spiffeid.TrustDomainFromString(trustDomain)
	if err != nil {
		return SPIFFEID{}, fmt.Errorf("%w: trust domain: %v", ErrInvalidSPIFFEID, err)
	}
	if len(segments) == 0 {
		return SPIFFEID{id: td.ID()}, nil
	}
	id, err := spiffeid.FromSegments(td, segments...)
	if err != nil {
		return SPIFFEID{}, fmt.Errorf("%w: %v", ErrInvalidSPIFFEID, err)
	}
	return SPIFFEID{id: id}, nil
}

// TrustDomain returns the trust-domain name (no scheme / no path):
// `kscore.local`, `example.org`. Empty when the ID is zero.
func (id SPIFFEID) TrustDomain() string {
	return id.id.TrustDomain().Name()
}

// Path returns the path portion of the ID — `""` for trust-domain-only
// IDs, `/agent/agent-1` otherwise. Always rooted with a leading `/`
// when non-empty.
func (id SPIFFEID) Path() string {
	return id.id.Path()
}

// Segments splits [SPIFFEID.Path] into its non-empty components.
// Returns nil (not an empty slice) for trust-domain-only IDs so that
// `len(id.Segments()) == 0` reads naturally.
func (id SPIFFEID) Segments() []string {
	p := id.id.Path()
	if p == "" {
		return nil
	}
	// Path always starts with `/` and contains no double-slashes —
	// the upstream parser canonicalises it. Trim the leading `/` and
	// split.
	return strings.Split(p[1:], "/")
}

// String returns the canonical SPIFFE-ID form. The zero value
// stringifies as `""` (not `"spiffe://"`).
func (id SPIFFEID) String() string {
	if id.id.IsZero() {
		return ""
	}
	return id.id.String()
}

// URI returns the ID as a parsed `*url.URL`. The result is suitable
// for an x509 URI SAN entry; nil for the zero value. The name
// (`URI` rather than `URL`) tracks the SPIFFE x509 SVID spec's term
// of art ("URI SAN").
func (id SPIFFEID) URI() *url.URL {
	if id.id.IsZero() {
		return nil
	}
	return id.id.URL()
}

// IsZero reports whether the receiver is the uninitialised value.
// A successful [ParseSPIFFEID] / [NewSPIFFEID] / [AgentID] /
// [ServerID] / [ServiceID] never returns a zero value.
func (id SPIFFEID) IsZero() bool {
	return id.id.IsZero()
}

// Equal reports whether two IDs name the same trust-domain and path.
// `==` is also correct (the type is comparable); Equal exists for
// readability and as the preferred call site in tests.
func (id SPIFFEID) Equal(other SPIFFEID) bool {
	return id.id == other.id
}

// MemberOf reports whether id's trust domain matches the given
// trust-domain name. A bad name (one that wouldn't parse as a
// trust domain) reports false rather than erroring — call sites are
// already inside the trust-domain check pipeline.
func (id SPIFFEID) MemberOf(trustDomain string) bool {
	td, err := spiffeid.TrustDomainFromString(trustDomain)
	if err != nil {
		return false
	}
	return id.id.MemberOf(td)
}

// MarshalText emits the canonical string form. Pairs with
// [SPIFFEID.UnmarshalText] for round-trip through `encoding/json`,
// YAML, BSON, and friends.
func (id SPIFFEID) MarshalText() ([]byte, error) {
	if id.id.IsZero() {
		return []byte{}, nil
	}
	return []byte(id.id.String()), nil
}

// UnmarshalText parses bytes (typically from `encoding/json` or a
// text-based config loader) into the receiver. Empty input decodes
// to the zero value rather than erroring so a missing field
// round-trips cleanly.
func (id *SPIFFEID) UnmarshalText(b []byte) error {
	s := string(b)
	if s == "" {
		*id = SPIFFEID{}
		return nil
	}
	parsed, err := ParseSPIFFEID(s)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// MarshalJSON emits a JSON string. The zero value emits `null` so
// `omitempty` works as operators expect — go-spiffe's upstream type
// emits `""` instead; we prefer `null` because the field semantics
// are "no identity" rather than "empty identity."
func (id SPIFFEID) MarshalJSON() ([]byte, error) {
	if id.id.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(id.id.String())
}

// UnmarshalJSON accepts a JSON string OR JSON null; null decodes to
// the zero value, mirroring [SPIFFEID.MarshalJSON].
func (id *SPIFFEID) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*id = SPIFFEID{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("%w: json: %v", ErrInvalidSPIFFEID, err)
	}
	return id.UnmarshalText([]byte(s))
}

// AgentID builds `spiffe://<trustDomain>/agent/<agentID>`. The
// canonical Keystone Core agent identity; consumed by NATS bootstrap
// (Epic 05) and the mTLS peer extractor (task 13). agentID may not
// be empty or contain `/`.
func AgentID(trustDomain, agentID string) (SPIFFEID, error) {
	if agentID == "" {
		return SPIFFEID{}, fmt.Errorf("%w: agent id is required", ErrInvalidSPIFFEID)
	}
	return NewSPIFFEID(trustDomain, pathPrefixAgent, agentID)
}

// ServerID builds `spiffe://<trustDomain>/server/<name>`. v0.1 ships
// with the canonical default `name="control-plane"` per §4.10; a
// multi-CP HA cluster (post-v1.0) assigns one per server.
func ServerID(trustDomain, name string) (SPIFFEID, error) {
	if name == "" {
		return SPIFFEID{}, fmt.Errorf("%w: server name is required", ErrInvalidSPIFFEID)
	}
	return NewSPIFFEID(trustDomain, pathPrefixServer, name)
}

// ServiceID builds `spiffe://<trustDomain>/service/<name>`. v0.1 uses
// this for internal service identities (the state runner, the
// reactor engine, the API server) when they need to authenticate to
// each other within a single CP process — usable across processes
// once Epic 13 (clustering) lands.
func ServiceID(trustDomain, name string) (SPIFFEID, error) {
	if name == "" {
		return SPIFFEID{}, fmt.Errorf("%w: service name is required", ErrInvalidSPIFFEID)
	}
	return NewSPIFFEID(trustDomain, pathPrefixService, name)
}
