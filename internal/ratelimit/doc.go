// SPDX-License-Identifier: Apache-2.0

// Package ratelimit is the basic per-key token-bucket layer used
// by the kscore HTTP + gRPC middleware (Epic 18 tasks 17-19).
//
// One [Bucket] is one token bucket. A [Registry] holds many
// Buckets keyed by an arbitrary string (the key extractor decides
// — per-IP, per-API-key, per-header value all share this shape).
//
// Capacity bound — important for v1.0:
//
//	A sloppy key extractor (e.g., per-IP under a botnet) can
//	create unbounded keyed buckets. Registry caps the number of
//	live keys via LRU eviction so memory usage stays bounded.
//	Default capacity is [DefaultCapacity] (10 000 keys, ~320 KB).
//	Operators size the cap to match their threat model.
//
// Algorithm: thin wrapper over [golang.org/x/time/rate.Limiter],
// already used by [internal/tracing.RateLimitingSampler]. The
// wrapper:
//
//	- Exposes [Bucket.AllowOrRetryAfter] so Task 19 can populate
//	  the HTTP Retry-After header without importing x/time/rate.
//	- Treats RPM=0 as "passthrough" (always allow) — operators
//	  who want the limiter wired but disabled set RPM to 0.
//	  Bare rate.NewLimiter(0, 0) would deny every request.
//	- Hides the rate.Limiter import from middleware so the public
//	  ratelimit API is stable.
//
// Out of scope for v1.0 (defer to v1.x as ROADMAP entries):
//
//	- Per-namespace quotas (kscore-policy territory).
//	- Sliding-window / fixed-window strategies.
//	- Distributed counters (multi-replica shared limit).
//
// [Bucket.Allow] is non-blocking; HTTP middleware needs that
// shape. A blocking [Bucket.Wait] is a v1.x addition if a job-
// dispatch use case asks for it.
package ratelimit
