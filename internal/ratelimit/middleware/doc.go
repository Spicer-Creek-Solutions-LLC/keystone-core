// SPDX-License-Identifier: Apache-2.0

// Package middleware wires the token-bucket [ratelimit.Registry]
// + [extract.Extractor] into the HTTP server chain and the gRPC
// interceptor chain. Per PROJECT-DETAILS §4.4 the chain order is
//
//	CORS → Rate-Limit → Auth → Handler
//
// — rate-limit sits outside auth so brute-force attempts on
// expensive auth paths are bounded.
//
// Behavior summary:
//
//  1. Call [extract.Extractor.HTTP] (or .GRPC) to derive the
//     rate-limit key.
//  2. ok=false from the extractor means "no usable key" — the
//     request is allowed without consulting the bucket
//     (skip-rather-than-deny). Operators chain APIKey → IP at
//     the extractor layer if they want a fallback identity.
//  3. ok=true → call [ratelimit.Registry.AllowOrRetryAfter] for
//     the key. Allowed requests proceed; denied requests
//     short-circuit with the transport-appropriate "rate
//     limited" response.
//
// HTTP deny:
//
//	HTTP/1.1 429 Too Many Requests
//	Content-Type: application/json
//	Retry-After: <seconds, ceil>
//
//	{"error": "rate limit exceeded"}
//
// gRPC deny:
//
//	codes.ResourceExhausted + status message "rate limit exceeded"
//	+ trailer metadata "retry-after-ms" carrying the delay in
//	milliseconds. (retry-after-ms is not a formal gRPC standard;
//	we ship it for client-side parity with HTTP. Clients that
//	ignore the trailer get a generic ResourceExhausted error.)
//
// Nil-on-any → passthrough:
//
//	A middleware constructed with nil registry OR nil extractor
//	passes every request through unchanged. The metric is
//	optional; a nil Metrics emitter disables rejection counting
//	without conditional code at the call site.
//
// Boot wiring (kscore-server registration) is out of scope here;
// it lands under the existing gate-v1.0 "REST/gRPC dark-until-
// boot" deferral.
package middleware
