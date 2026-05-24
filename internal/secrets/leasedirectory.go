// SPDX-License-Identifier: Apache-2.0

package secrets

import "sync"

// LeaseRecord is the slim metadata the [Broker] needs to route a
// `RenewLease` / `RevokeLease` request back to the issuing backend
// and invalidate the cache on revoke. Task 6's [LeaseManager] carries
// a richer [Lease] / [LeaseInfo] record; this is the routing
// projection.
//
// Strategy, when non-zero, lets the recording caller pin a specific
// renewal cadence (the lease manager interprets [RenewStrategyUnknown]
// = 0 as "use the manager's [LeaseManagerConfig.DefaultStrategy]").
// The in-memory directory does not look at this field; it's a hint
// the persistent manager honours when it shadows the call.
type LeaseRecord struct {
	Backend  string
	Path     string
	Strategy RenewStrategy
}

// LeaseDirectory is the seam the [Broker] consults to route
// [SecretBackend.RenewLease] and [SecretBackend.RevokeLease] calls.
// Task 6's persistent `LeaseManager` will satisfy this interface as a
// sub-shape (adding scheduling, persistence, and renewal callbacks).
// Until then, [InMemoryLeaseDirectory] is the broker default — a
// process-restart wipes it, which matches the v0.1 single-CP
// posture.
//
// Record stores leaseID → (backend, path). Same-leaseID double-record
// overwrites — backends do not reuse lease IDs in v1.0 and the
// `IssueDynamicSecret` flow records exactly once.
//
// Lookup returns (record, true) on hit and (zero, false) on miss.
//
// Forget drops the entry; no error on unknown leaseID (idempotent).
type LeaseDirectory interface {
	Record(leaseID string, record LeaseRecord)
	Lookup(leaseID string) (LeaseRecord, bool)
	Forget(leaseID string)
}

// InMemoryLeaseDirectory is the broker default — a
// `sync.RWMutex`-protected map. Process-restart safe-by-omission: the
// directory is cleared, so a fresh boot has no leases to renew, which
// is the v0.1 single-CP failure mode (dynamic credentials issued
// pre-restart need re-issuing). Task 6's `LeaseManager` adds SQLite
// persistence to close the gap.
type InMemoryLeaseDirectory struct {
	mu      sync.RWMutex
	entries map[string]LeaseRecord
}

// NewInMemoryLeaseDirectory returns an empty directory ready for use.
func NewInMemoryLeaseDirectory() *InMemoryLeaseDirectory {
	return &InMemoryLeaseDirectory{entries: make(map[string]LeaseRecord)}
}

// Record stores leaseID → record.
func (d *InMemoryLeaseDirectory) Record(leaseID string, record LeaseRecord) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries[leaseID] = record
}

// Lookup returns the record + true, or zero + false.
func (d *InMemoryLeaseDirectory) Lookup(leaseID string) (LeaseRecord, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	rec, ok := d.entries[leaseID]
	return rec, ok
}

// Forget drops the entry. No-op on unknown leaseID.
func (d *InMemoryLeaseDirectory) Forget(leaseID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.entries, leaseID)
}

// Len returns the directory size. Convenience for tests + the
// /api/status endpoint (task 9) that wants to surface "tracked
// leases" alongside other broker counters.
func (d *InMemoryLeaseDirectory) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.entries)
}

// Compile-time assertion.
var _ LeaseDirectory = (*InMemoryLeaseDirectory)(nil)
