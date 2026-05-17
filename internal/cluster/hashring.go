package cluster

import (
	"hash/fnv"
	"sort"
	"strconv"
	"sync"
)

// DefaultVirtualNodes is the §4.15 default ring points per member.
// More vnodes ⇒ smoother key distribution and less reassignment on
// topology change, at a higher rebuild cost.
const DefaultVirtualNodes = 150

// HashRing is a consistent-hash ring with virtual nodes mapping a
// key (agent ID) to a member (kscore-server). It is the pure
// primitive Epic 13's ShardManager composes with the etcd-backed
// ShardStore (Task 5) and the rebalance algorithm (Task 6); it has
// no etcd dependency itself.
//
// The ring is rebuilt deterministically from the sorted member set,
// so every server computes an identical key→member mapping for the
// same membership regardless of Add/Remove order — required for the
// cluster to agree on agent ownership and for Task 6's
// minimal-migration rebalancing.
//
// Safe for concurrent use: lookups take a read lock, topology
// changes a write lock.
type HashRing struct {
	mu      sync.RWMutex
	vnodes  int
	members map[string]struct{}

	// points is the sorted ring; owners[i] owns points[i].
	points []uint64
	owners []string
}

// NewHashRing returns an empty ring. vnodes ≤ 0 uses
// DefaultVirtualNodes.
func NewHashRing(vnodes int) *HashRing {
	if vnodes <= 0 {
		vnodes = DefaultVirtualNodes
	}
	return &HashRing{vnodes: vnodes, members: make(map[string]struct{})}
}

func hashKey(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s)) // hash.Hash64.Write never errors
	return h.Sum64()
}

func vnodeKey(member string, i int) string {
	return member + "#" + strconv.Itoa(i)
}

// Add inserts a member (idempotent) and rebuilds the ring.
func (r *HashRing) Add(member string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.members[member]; ok {
		return
	}
	r.members[member] = struct{}{}
	r.rebuild()
}

// Remove deletes a member (no-op if absent) and rebuilds the ring.
func (r *HashRing) Remove(member string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.members[member]; !ok {
		return
	}
	delete(r.members, member)
	r.rebuild()
}

// rebuild reconstructs the ring from the sorted member set. Sorted
// iteration makes vnode-hash collisions resolve to the
// lexicographically-smallest member deterministically (first writer
// wins, and the smallest member is processed first), so the result
// is independent of mutation order.
func (r *HashRing) rebuild() {
	members := make([]string, 0, len(r.members))
	for m := range r.members {
		members = append(members, m)
	}
	sort.Strings(members)

	owner := make(map[uint64]string, len(members)*r.vnodes)
	for _, m := range members {
		for i := 0; i < r.vnodes; i++ {
			p := hashKey(vnodeKey(m, i))
			if _, taken := owner[p]; !taken {
				owner[p] = m
			}
		}
	}

	pts := make([]uint64, 0, len(owner))
	for p := range owner {
		pts = append(pts, p)
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i] < pts[j] })

	owners := make([]string, len(pts))
	for i, p := range pts {
		owners[i] = owner[p]
	}
	r.points = pts
	r.owners = owners
}

// Get returns the member owning key, or ("", false) if the ring is
// empty. The owner is the first ring point clockwise from
// hash(key), wrapping past the end.
func (r *HashRing) Get(key string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.points) == 0 {
		return "", false
	}
	h := hashKey(key)
	idx := sort.Search(len(r.points), func(i int) bool { return r.points[i] >= h })
	if idx == len(r.points) {
		idx = 0 // wrap around the ring
	}
	return r.owners[idx], true
}

// Members returns the current member set, sorted.
func (r *HashRing) Members() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.members))
	for m := range r.members {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// Has reports whether member is in the ring.
func (r *HashRing) Has(member string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.members[member]
	return ok
}

// Len returns the number of members (not ring points).
func (r *HashRing) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.members)
}
