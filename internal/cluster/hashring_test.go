// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"fmt"
	"testing"
)

func TestHashRing_EmptyAndBasic(t *testing.T) {
	r := NewHashRing(0) // → DefaultVirtualNodes
	if _, ok := r.Get("agent-1"); ok {
		t.Fatal("empty ring must return ok=false")
	}
	if r.Len() != 0 {
		t.Fatalf("Len = %d, want 0", r.Len())
	}

	r.Add("m1")
	m, ok := r.Get("agent-1")
	if !ok || m != "m1" {
		t.Fatalf("single-member Get = %q,%v; want m1,true", m, ok)
	}
	r.Add("m1") // idempotent
	if r.Len() != 1 {
		t.Fatalf("Len after dup Add = %d, want 1", r.Len())
	}
	r.Remove("absent") // no-op
	if r.Len() != 1 {
		t.Fatalf("Len after no-op Remove = %d, want 1", r.Len())
	}
	if !r.Has("m1") || r.Has("nope") {
		t.Fatal("Has wrong")
	}
}

func TestHashRing_Stability(t *testing.T) {
	r := NewHashRing(150)
	for _, m := range []string{"m1", "m2", "m3"} {
		r.Add(m)
	}
	first := map[string]string{}
	for i := 0; i < 2000; i++ {
		k := fmt.Sprintf("agent-%d", i)
		v, _ := r.Get(k)
		first[k] = v
	}
	// Repeated lookups never change.
	for k, want := range first {
		if got, _ := r.Get(k); got != want {
			t.Fatalf("unstable Get(%s): %q != %q", k, got, want)
		}
	}
}

func TestHashRing_DeterministicAcrossAddOrder(t *testing.T) {
	a := NewHashRing(150)
	for _, m := range []string{"m1", "m2", "m3", "m4"} {
		a.Add(m)
	}
	b := NewHashRing(150)
	for _, m := range []string{"m4", "m2", "m1", "m3"} { // different order
		b.Add(m)
	}
	for i := 0; i < 3000; i++ {
		k := fmt.Sprintf("agent-%d", i)
		av, _ := a.Get(k)
		bv, _ := b.Get(k)
		if av != bv {
			t.Fatalf("ring not order-independent at %s: %q != %q", k, av, bv)
		}
	}
}

func TestHashRing_DistributionSanity(t *testing.T) {
	r := NewHashRing(150)
	members := []string{"m1", "m2", "m3"}
	for _, m := range members {
		r.Add(m)
	}
	const n = 12000
	counts := map[string]int{}
	for i := 0; i < n; i++ {
		v, _ := r.Get(fmt.Sprintf("agent-%d", i))
		counts[v]++
	}
	// 3 members + 150 vnodes over 12k keys: each should hold a
	// meaningful share. Loose bound (>15%) to avoid flakiness.
	for _, m := range members {
		if frac := float64(counts[m]) / n; frac < 0.15 {
			t.Fatalf("member %s holds only %.1f%% of keys (counts=%v)", m, frac*100, counts)
		}
	}
}

func TestHashRing_MinimalDisruptionOnAdd(t *testing.T) {
	r := NewHashRing(150)
	for _, m := range []string{"m1", "m2", "m3"} {
		r.Add(m)
	}
	const n = 10000
	before := make(map[string]string, n)
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("agent-%d", i)
		before[k], _ = r.Get(k)
	}

	r.Add("m4")

	moved := 0
	for k, was := range before {
		now, _ := r.Get(k)
		if now != was {
			moved++
			// A moved key must land on the new member (consistent
			// hashing only steals from existing members onto the
			// newcomer; it never reshuffles among the old ones).
			if now != "m4" {
				t.Fatalf("key %s moved %s→%s, not onto the new member", k, was, now)
			}
		}
	}
	frac := float64(moved) / n
	// Ideal ≈ 1/4; allow generous headroom, but it must be far
	// below a naive hash-mod reshuffle (~3/4).
	if frac < 0.10 || frac > 0.40 {
		t.Fatalf("moved fraction %.3f out of expected ~0.25 band", frac)
	}
}

func TestHashRing_RemoveRedistributes(t *testing.T) {
	r := NewHashRing(150)
	for _, m := range []string{"m1", "m2", "m3"} {
		r.Add(m)
	}
	const n = 8000
	before := make(map[string]string, n)
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("agent-%d", i)
		before[k], _ = r.Get(k)
	}

	r.Remove("m2")
	if r.Has("m2") {
		t.Fatal("m2 still present after Remove")
	}

	for k, was := range before {
		now, _ := r.Get(k)
		if was == "m2" {
			if now == "m2" {
				t.Fatalf("key %s still on removed member", k)
			}
		} else if now != was {
			t.Fatalf("key %s on surviving member moved %s→%s (should be stable)", k, was, now)
		}
	}
	if got := r.Members(); len(got) != 2 || got[0] != "m1" || got[1] != "m3" {
		t.Fatalf("Members after remove = %v, want [m1 m3]", got)
	}
}
