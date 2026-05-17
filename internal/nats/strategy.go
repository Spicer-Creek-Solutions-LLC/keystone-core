package nats

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"

	"github.com/nats-io/nats.go"
)

// ConnectionStrategy abstracts the mechanism used to open a connection
// to a single endpoint. v1.0 ships Direct (plain) and TLS; WebSocket
// and LeafNode are deferred to v2.x+ (PROJECT-DETAILS §4.2).
type ConnectionStrategy interface {
	// Connect dials the endpoint with the supplied client options
	// (Name, MaxReconnects, etc. from buildClientOptions). Strategy-
	// specific options (TLS) are appended internally.
	Connect(endpoint Endpoint, opts []nats.Option) (*nats.Conn, error)

	// Scheme returns the URL scheme this strategy handles. Used by
	// StrategySelector for dispatch and by tests to assert wiring.
	Scheme() string
}

// DirectStrategy connects without TLS. URL scheme: nats://.
type DirectStrategy struct{}

func (DirectStrategy) Scheme() string { return "nats" }

func (DirectStrategy) Connect(e Endpoint, opts []nats.Option) (*nats.Conn, error) {
	conn, err := nats.Connect(e.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("direct connect %q: %w", e.URL, err)
	}
	return conn, nil
}

// TLSStrategy connects with TLS. URL scheme: tls://. A nil TLSConfig
// means "use system roots" — nats.Secure() with no args. A configured
// TLSConfig (e.g., for mTLS) is passed through.
type TLSStrategy struct {
	TLSConfig *tls.Config
}

func (TLSStrategy) Scheme() string { return "tls" }

func (s TLSStrategy) Connect(e Endpoint, opts []nats.Option) (*nats.Conn, error) {
	if s.TLSConfig != nil {
		opts = append(opts, nats.Secure(s.TLSConfig))
	} else {
		opts = append(opts, nats.Secure())
	}
	conn, err := nats.Connect(e.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("tls connect %q: %w", e.URL, err)
	}
	return conn, nil
}

// StrategySelector dispatches to a ConnectionStrategy by URL scheme.
// Unknown schemes default to DirectStrategy so a misconfigured URL
// produces a clean nats-level error rather than a panic.
type StrategySelector struct {
	direct ConnectionStrategy
	tls    ConnectionStrategy
}

// NewStrategySelector builds a selector with the v1.0 strategy set.
// Pass tlsCfg = nil for system-default TLS roots.
func NewStrategySelector(tlsCfg *tls.Config) StrategySelector {
	return StrategySelector{
		direct: DirectStrategy{},
		tls:    TLSStrategy{TLSConfig: tlsCfg},
	}
}

// Select returns the strategy for the given scheme. Unknown schemes
// fall back to direct — the underlying nats.Connect surfaces the URL
// error with a clearer message than a panic here would.
func (s StrategySelector) Select(scheme string) ConnectionStrategy {
	switch strings.ToLower(scheme) {
	case "tls":
		return s.tls
	default:
		return s.direct
	}
}

// schemeFromURL returns the lowercased scheme for url. Empty on parse
// error — the caller treats unknown schemes as Direct, so an unparsable
// URL surfaces at Connect time, not here.
func schemeFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme)
}
