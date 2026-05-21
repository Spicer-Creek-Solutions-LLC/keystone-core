package extract

import (
	"context"
	"net"
	"net/http"
	"strings"

	"google.golang.org/grpc/peer"
)

// IPConfig configures an [IP] extractor.
type IPConfig struct {
	// TrustForwardedFor enables reading the leftmost address in
	// the X-Forwarded-For header (HTTP) before falling back to
	// RemoteAddr. Enable only when kscore runs behind a known
	// reverse proxy; otherwise clients can spoof XFF to evade
	// per-IP limits.
	TrustForwardedFor bool
}

// IP returns an [Extractor] keyed by client IP.
func IP(cfg IPConfig) Extractor {
	return &ipExtractor{cfg: cfg}
}

type ipExtractor struct {
	cfg IPConfig
}

// HTTP reads RemoteAddr (default) or the leftmost X-Forwarded-For
// hop (when TrustForwardedFor is set). Returns the bare IP
// without the port.
func (e *ipExtractor) HTTP(r *http.Request) (string, bool) {
	if e.cfg.TrustForwardedFor {
		if v := r.Header.Get("X-Forwarded-For"); v != "" {
			ip := strings.TrimSpace(firstHop(v))
			if ip != "" {
				return ip, true
			}
		}
	}
	if r.RemoteAddr == "" {
		return "", false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr without :port (some test contexts).
		return r.RemoteAddr, true
	}
	return host, true
}

// GRPC reads peer.FromContext + strips the port. gRPC does not
// have a built-in X-Forwarded-For analogue (a proxy fronting
// gRPC would have terminated TLS and rewritten the underlying
// connection); cfg.TrustForwardedFor is ignored on this path.
func (e *ipExtractor) GRPC(ctx context.Context) (string, bool) {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return "", false
	}
	addr := p.Addr.String()
	if addr == "" {
		return "", false
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, true
	}
	return host, true
}

// firstHop returns the leftmost comma-separated value from an
// X-Forwarded-For header. RFC 7239 says the first entry is the
// originating client; intermediate proxies append themselves.
func firstHop(xff string) string {
	if i := strings.IndexByte(xff, ','); i >= 0 {
		return xff[:i]
	}
	return xff
}
