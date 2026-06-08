// SPDX-License-Identifier: Apache-2.0

package bond

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"
)

// --- pure renderers ---------------------------------------------------

func TestRenderNetdevSection(t *testing.T) {
	t.Parallel()
	got := renderNetdevSection(&params{Mode: "active-backup", Miimon: 100, HasMiimon: true})
	if got != "[Bond]\nMode=active-backup\nMIIMonitorSec=100ms\n" {
		t.Errorf("section = %q", got)
	}
	// no miimon → no MIIMonitorSec line
	if g := renderNetdevSection(&params{Mode: "balance-rr"}); g != "[Bond]\nMode=balance-rr\n" {
		t.Errorf("section without miimon = %q", g)
	}
}

func TestRenderNetplan(t *testing.T) {
	t.Parallel()
	got := renderNetplan(&params{Name: "bond0", Mode: "active-backup", Members: []string{"eth0", "eth1"}, Miimon: 100, HasMiimon: true})
	want := netpersist.ManagedHeader +
		"network:\n  version: 2\n  bonds:\n" +
		"    bond0:\n" +
		"      interfaces:\n        - eth0\n        - eth1\n" +
		"      parameters:\n        mode: active-backup\n        mii-monitor-interval: 100\n"
	if got != want {
		t.Errorf("netplan:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	// member-less bond omits the interfaces: block
	if g := renderNetplan(&params{Name: "bond1", Mode: "balance-rr"}); strings.Contains(g, "interfaces:") {
		t.Errorf("member-less bond should omit interfaces: %q", g)
	}
}

// --- module persist integration --------------------------------------

func usePersistTempdir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	origND, origNP := netpersist.NetworkdDir, netpersist.NetplanDir
	netpersist.NetworkdDir = filepath.Join(dir, "systemd-network")
	netpersist.NetplanDir = filepath.Join(dir, "absent-netplan") // absent → DetectBackend = networkd
	t.Cleanup(func() { netpersist.NetworkdDir, netpersist.NetplanDir = origND, origNP })
}

func TestPersist_PresentWritesAndIdempotent(t *testing.T) {
	usePersistTempdir(t)
	ctx := context.Background()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("bond0", "present", map[string]any{
		"name": "bond0", "mode": "active-backup", "members": []any{"eth0", "eth1"}, "persist": "networkd",
	})

	res, err := m.Check(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if res.Matches {
		t.Error("runtime + persist both absent → want drift")
	}

	sr, err := m.Apply(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed || len(f.createCalls) != 1 || len(f.masterCalls) != 2 {
		t.Fatalf("apply should create bond + enslave 2 members: %+v creates=%v masters=%v", sr, f.createCalls, f.masterCalls)
	}
	if c, ok, _ := netpersist.Read(netpersist.NetdevPath("bond0")); !ok || !strings.Contains(c, "Kind=bond") || !strings.Contains(c, "Mode=active-backup") {
		t.Errorf("netdev wrong: %q", c)
	}
	for _, mbr := range []string{"eth0", "eth1"} {
		if c, ok, _ := netpersist.Read(netpersist.NetworkDropinPath(mbr, "kscore-bond-bond0")); !ok || !strings.Contains(c, "Bond=bond0") {
			t.Errorf("enslave drop-in for %s wrong: %q", mbr, c)
		}
		if _, ok, _ := netpersist.Read(netpersist.NetworkPath(mbr)); !ok {
			t.Errorf("member base for %s missing", mbr)
		}
	}

	// idempotent
	res2, _ := m.Check(ctx, d)
	if !res2.Matches {
		t.Errorf("second check should converge: %+v", res2)
	}
	sr2, _ := m.Apply(ctx, d)
	if sr2.Changed {
		t.Errorf("second apply should be a no-op: %+v", sr2)
	}
}

func TestPersist_AbsentRemovesByGlob(t *testing.T) {
	usePersistTempdir(t)
	ctx := context.Background()
	m := NewWithProvider(newFake())
	// establish present
	if _, err := m.Apply(ctx, decl("bond0", "present", map[string]any{
		"name": "bond0", "members": []any{"eth0", "eth1"}, "persist": "networkd",
	})); err != nil {
		t.Fatal(err)
	}
	// absent decl carries no members
	ad := decl("bond0", "absent", map[string]any{"name": "bond0", "persist": "networkd"})
	res, _ := m.Check(ctx, ad)
	if res.Matches {
		t.Error("bond + persist files present → want drift for absent")
	}
	sr, err := m.Apply(ctx, ad)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("absent apply should remove files")
	}
	if _, ok, _ := netpersist.Read(netpersist.NetdevPath("bond0")); ok {
		t.Error("netdev should be gone")
	}
	for _, mbr := range []string{"eth0", "eth1"} {
		if _, ok, _ := netpersist.Read(netpersist.NetworkDropinPath(mbr, "kscore-bond-bond0")); ok {
			t.Errorf("drop-in for %s should be gone", mbr)
		}
		if _, ok, _ := netpersist.Read(netpersist.NetworkPath(mbr)); !ok {
			t.Errorf("member base for %s should remain", mbr)
		}
	}
	sr2, _ := m.Apply(ctx, ad)
	if sr2.Changed {
		t.Errorf("second absent apply should be a no-op: %+v", sr2)
	}
}

func TestPersist_AutoResolvesNetplan(t *testing.T) {
	usePersistTempdir(t)
	if err := os.MkdirAll(netpersist.NetplanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewWithProvider(newFake())
	if _, err := m.Apply(context.Background(), decl("bond0", "present", map[string]any{
		"name": "bond0", "members": []any{"eth0"}, "persist": "auto",
	})); err != nil {
		t.Fatal(err)
	}
	if c, ok, _ := netpersist.Read(netpersist.NetplanDevicePath("bond", "bond0")); !ok || !strings.Contains(c, "bonds:") {
		t.Errorf("auto should write a netplan bonds file: %q", c)
	}
}

func TestPersist_PresentDriftWithLiveBond(t *testing.T) {
	usePersistTempdir(t)
	ctx := context.Background()
	f := newFake()
	f.links["bond0"] = &LinkInfo{Name: "bond0", Kind: "bond"} // already live
	m := NewWithProvider(f)
	d := decl("bond0", "present", map[string]any{"name": "bond0", "persist": "networkd"})
	// runtime converged but persist file missing → drift, and Apply writes
	// persist without re-creating the bond (no in-place reconcile).
	res, _ := m.Check(ctx, d)
	if res.Matches {
		t.Error("live bond + missing persist → want drift")
	}
	sr, _ := m.Apply(ctx, d)
	if !sr.Changed || len(f.createCalls) != 0 {
		t.Errorf("should write persist only, not re-create: changed=%v creates=%v", sr.Changed, f.createCalls)
	}
	if _, ok, _ := netpersist.Read(netpersist.NetdevPath("bond0")); !ok {
		t.Error("netdev should be written")
	}
}

func TestPersist_Validation(t *testing.T) {
	t.Parallel()
	if err := (&Module{}).Validate(decl("bond0", "present", map[string]any{"name": "bond0", "persist": "nm"})); err == nil {
		t.Error("bad backend should error")
	}
	if err := (&Module{}).Validate(decl("bond0", "present", map[string]any{"name": "bond0", "persist": 1})); err == nil {
		t.Error("non-string persist should error")
	}
	if err := (&Module{}).Validate(decl("bond0", "present", map[string]any{"name": "bond0", "persist": "networkd"})); err != nil {
		t.Errorf("valid persist should pass: %v", err)
	}
}
