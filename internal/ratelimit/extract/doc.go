// Package extract returns rate-limit keys from inbound requests.
// Concrete extractors cover IP, API key, and arbitrary header
// per PROJECT-DETAILS §4.20. The package is consumed by the
// rate-limit middleware (Task 18) which calls the extractor on
// each request and passes the returned key to
// [ratelimit.Registry.Allow].
//
// Each extractor exposes both an HTTP and a gRPC method. The
// middleware picks the right one for the transport it is
// instrumenting; the [Extractor] interface deliberately keeps
// the two shapes parallel rather than unifying via context — the
// HTTP path needs *http.Request (RemoteAddr + headers) and the
// gRPC path uses peer.FromContext / metadata.FromIncomingContext.
// A single context-based API would require synthetic peer
// injection that the auth interceptor already handles in
// pkg/api/auth/http_inject.go.
//
// ok=false semantics:
//
//	An extractor returns (key, false) when the request carries
//	no usable signal for that extractor (no XFF when trust is
//	enabled, no Authorization header, no matching gRPC peer).
//	The middleware decides what to do — typically: try the next
//	extractor in a [Chain], or skip rate limiting on this
//	request, or apply an "anonymous" key.
//
// API-key privacy:
//
//	[APIKey] returns the SHA-256 hash of the bearer token, not
//	the cleartext. Cleartext keys would land in the bucket map
//	(observable via memory dump) and in any log line that
//	includes the rate-limit key. The hash is drift-prevented by
//	reusing [pkg/api/auth.HashAPIKey].
//
// X-Forwarded-For trust:
//
//	[IP] reads RemoteAddr by default. Set
//	[IPConfig.TrustForwardedFor] only when kscore runs behind a
//	known reverse proxy — otherwise an unauthenticated client
//	can spoof the header to evade per-IP rate limits.
package extract
