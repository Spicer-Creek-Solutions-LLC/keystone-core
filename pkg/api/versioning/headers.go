// SPDX-License-Identifier: Apache-2.0

package versioning

import (
	"fmt"
	"net/http"
	"time"

	"google.golang.org/grpc/metadata"
)

// HeadersFor returns the HTTP response headers to emit for method,
// or nil/empty if method isn't registered or has no advisory state.
//
// Header semantics:
//
//   - Deprecation (RFC 9745):  HTTP-date when the endpoint became
//     deprecated. Emitted whenever DeprecatedAt is set, regardless of
//     declared Status.
//   - Sunset (RFC 8594):       HTTP-date when the endpoint will be /
//     was retired. Emitted whenever SunsetAt is set.
//   - Link (RFC 8288):         <successor>; rel="successor-version".
//     Emitted whenever Replacement is non-empty.
//   - Warning:                 advisory text for older clients. Emits
//     299 (deprecated) or 199 (alpha/beta) per RFC 7234 §5.5.
func (r *Registry) HeadersFor(method string) http.Header {
	e, ok := r.Lookup(method)
	if !ok {
		return nil
	}
	h := http.Header{}
	addHeaders(h, e, r.effective(e))
	return h
}

// MetadataFor returns the gRPC response metadata for method, mirroring
// HeadersFor's semantics. Header keys are lowercased per gRPC
// metadata conventions.
func (r *Registry) MetadataFor(method string) metadata.MD {
	e, ok := r.Lookup(method)
	if !ok {
		return nil
	}
	md := metadata.MD{}
	if !e.DeprecatedAt.IsZero() {
		md.Set("deprecation", e.DeprecatedAt.UTC().Format(http.TimeFormat))
	}
	if !e.SunsetAt.IsZero() {
		md.Set("sunset", e.SunsetAt.UTC().Format(http.TimeFormat))
	}
	if e.Replacement != "" {
		md.Set("link", linkHeader(e.Replacement))
	}
	if w := warningFor(e, r.effective(e)); w != "" {
		md.Set("warning", w)
	}
	if len(md) == 0 {
		return nil
	}
	return md
}

// effective is a lock-acquiring wrapper around the registry's
// EffectiveStatus that takes a pre-fetched Endpoint to avoid a second
// map lookup.
func (r *Registry) effective(e Endpoint) Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !e.SunsetAt.IsZero() && r.now().After(e.SunsetAt) {
		return StatusRetired
	}
	return e.Status
}

// addHeaders populates h with the standard deprecation set for e.
func addHeaders(h http.Header, e Endpoint, eff Status) {
	if !e.DeprecatedAt.IsZero() {
		h.Set("Deprecation", e.DeprecatedAt.UTC().Format(http.TimeFormat))
	}
	if !e.SunsetAt.IsZero() {
		h.Set("Sunset", e.SunsetAt.UTC().Format(http.TimeFormat))
	}
	if e.Replacement != "" {
		h.Set("Link", linkHeader(e.Replacement))
	}
	if w := warningFor(e, eff); w != "" {
		h.Set("Warning", w)
	}
}

// linkHeader formats Replacement as an RFC 8288 successor-version Link.
func linkHeader(replacement string) string {
	return fmt.Sprintf(`<%s>; rel="successor-version"`, replacement)
}

// warningFor returns the Warning header text for e (empty if none).
//
//   - 299 — deprecated/retired endpoints (Warning: 299 - "...")
//   - 199 — alpha/beta endpoints (Warning: 199 - "...")
func warningFor(e Endpoint, eff Status) string {
	switch eff {
	case StatusDeprecated, StatusRetired:
		base := "deprecated"
		if eff == StatusRetired {
			base = "retired"
		}
		if !e.SunsetAt.IsZero() {
			return fmt.Sprintf(`299 - "%s; sunset on %s"`, base,
				e.SunsetAt.UTC().Format(time.DateOnly))
		}
		if e.Notes != "" {
			return fmt.Sprintf(`299 - "%s; %s"`, base, e.Notes)
		}
		return fmt.Sprintf(`299 - "%s"`, base)
	case StatusAlpha:
		return `199 - "alpha endpoint; breaking changes likely"`
	case StatusBeta:
		return `199 - "beta endpoint; breaking changes possible"`
	default:
		return ""
	}
}
