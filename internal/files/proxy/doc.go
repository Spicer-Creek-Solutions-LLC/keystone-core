// SPDX-License-Identifier: Apache-2.0

// Package proxy is the agent-side cache for file-distribution reads.
//
// Agents fetch the same blueprints / configs / packages repeatedly;
// a thin LRU + TTL cache between agent consumers and the
// transport.Client cuts the repeat traffic. PROJECT-DETAILS §4.20
// names this "proxy caching on agents".
//
// The cache wraps a [Getter] (typically a [transport.Client]) and
// exposes Get + Invalidate. Writes are not proxied — agents do
// writes via the wrapped client directly and call Invalidate(path)
// themselves so the cache does not serve a stale body after a put
// or delete.
//
// Semantics:
//
//	Hit             Get returns the cached metadata + body, moves
//	                the entry to MRU position, increments
//	                kscore_files_cache_hits_total.
//	Cold miss       No entry; falls through to the source, stores
//	                the result, increments
//	                kscore_files_cache_misses_total{reason=miss}.
//	Expired         Entry exists but Inserted + TTL < now; evicts,
//	                falls through, stores the fresh body, counts
//	                under reason=expired.
//	Bypass          Caller set GetOptions.FromChunk > 0; partial
//	                bodies are not cache-worthy, counted under
//	                reason=bypass.
//	Eviction        On insert above capacity, the least-recently-
//	                used entry is dropped.
//
// v1.0 caps the cache by entry count, not byte budget; operators
// sizing the cache should reason about capacity × expected file
// size. Byte-budget eviction is a v1.x concern when real-world
// workloads ask for it.
//
// Concurrency: safe for concurrent use; one mutex guards the map
// + LRU list. The wrapped Getter is invoked outside the lock so
// concurrent misses for different paths do not serialise.
//
// No new dependency — container/list from the standard library
// provides the LRU primitive.
package proxy
