package server

import (
	"net/http"
	"strings"

	"go.keystone-core.io/keystone-core/internal/config"
)

// corsMiddleware returns a net/http middleware that enforces CORS per
// PROJECT-DETAILS §4.4: outermost in the chain so OPTIONS preflight
// returns 204 BEFORE rate-limit / auth runs. The "preflight bypasses
// rate-limit" acceptance criterion is satisfied by structure — the
// preflight response never reaches the auth middleware below.
//
// For a request whose Origin header is in the allow-list:
//   - the response carries Access-Control-Allow-Origin (echoed) +
//     Vary: Origin + credentials
//   - OPTIONS preflight is short-circuited with 204 + the
//     Allow-Methods / Allow-Headers / Max-Age headers
//   - non-OPTIONS proceeds to next
//
// For a request whose Origin is missing or not in the allow-list:
//   - no CORS headers are added
//   - the request falls through to next unchanged (browsers will
//     reject the response themselves on the disallowed origin)
//
// Wildcard "*" matches any origin. When AllowedOrigins is exactly
// ["*"], the response uses "*" instead of echoing the request origin
// (per CORS spec: "*" is incompatible with Allow-Credentials, so we
// also drop the credentials header in that case).
func corsMiddleware(cfg config.CORSConfig) func(http.Handler) http.Handler {
	allowedMethods := strings.Join(cfg.AllowedMethods, ", ")
	allowedHeaders := strings.Join(cfg.AllowedHeaders, ", ")
	wildcard := len(cfg.AllowedOrigins) == 1 && cfg.AllowedOrigins[0] == "*"

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && originAllowed(cfg.AllowedOrigins, origin) {
				if wildcard {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				if r.Method == http.MethodOptions {
					w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
					w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
					w.Header().Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// originAllowed returns true if origin is in the allow-list. "*"
// matches everything. Comparison is exact otherwise (CORS allow-lists
// don't support glob/scheme-loose matching at v1.0).
func originAllowed(allowed []string, origin string) bool {
	for _, a := range allowed {
		if a == "*" || a == origin {
			return true
		}
	}
	return false
}
