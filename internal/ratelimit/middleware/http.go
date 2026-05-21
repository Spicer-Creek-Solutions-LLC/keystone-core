package middleware

import (
	"encoding/json"
	"math"
	"net/http"
	"time"

	"go.keystone-core.io/keystone-core/internal/ratelimit"
	"go.keystone-core.io/keystone-core/internal/ratelimit/extract"
)

// errorBody is the JSON shape returned with a 429. The single
// "error" field matches pkg/api/secrets + pkg/api/files response
// envelopes so clients can parse one shape across all routes.
type errorBody struct {
	Error string `json:"error"`
}

// rejectedMessage is the message string emitted in 429 bodies +
// gRPC status errors. Kept as a const so future audit-log entries
// can match on a stable value.
const rejectedMessage = "rate limit exceeded"

// Middleware returns an HTTP middleware that consults reg for
// each request keyed by ext. A nil reg or nil ext disables
// limiting (passthrough). m is optional and counts rejections.
func Middleware(reg *ratelimit.Registry, ext extract.Extractor, m *Metrics) func(http.Handler) http.Handler {
	if reg == nil || ext == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, ok := ext.HTTP(r)
			if !ok {
				// No key derivable for this request — allow it.
				// Operators wanting an "anonymous" key chain
				// extractors at the extract.Chain layer.
				next.ServeHTTP(w, r)
				return
			}
			allowed, delay := reg.AllowOrRetryAfter(key)
			if allowed {
				next.ServeHTTP(w, r)
				return
			}
			writeRejected429(w, delay)
			m.RecordReject(ReasonLimitExceeded)
		})
	}
}

// writeRejected429 emits the canonical 429 response. Retry-After
// is rounded up to the next whole second and floored at 1 — some
// clients ignore a zero-value Retry-After. Per HTTP spec the
// header value is either a date or a delta-seconds integer; we
// use delta-seconds.
func writeRejected429(w http.ResponseWriter, delay time.Duration) {
	seconds := int(math.Ceil(delay.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", formatRetryAfter(seconds))
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(errorBody{Error: rejectedMessage})
}

// formatRetryAfter exists as its own function so the gRPC
// trailer can reuse the integer-seconds formatting unchanged.
func formatRetryAfter(seconds int) string {
	// strconv.Itoa avoids the fmt allocation on this hot path.
	return itoa(seconds)
}

// itoa is a small allocation-free integer-to-decimal. Avoids an
// fmt.Sprintf in the deny path which is a measurable hot spot
// under sustained 429 traffic.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
