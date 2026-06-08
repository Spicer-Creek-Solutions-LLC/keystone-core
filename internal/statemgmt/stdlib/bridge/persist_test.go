// SPDX-License-Identifier: Apache-2.0

package bridge

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
	if g := renderNetdevSection(&params{STP: true}); g != "[Bridge]\nSTP=yes\n" {
		t.Errorf("stp=true section = %q", g)
	}
	if g := renderNetdevSection(&params{STP: false}); g != "[Bridge]\nSTP=no\n" {
		t.Errorf("stp=false section = %q", g)
	}
}

func TestRenderNetplan(t *testing.T) {
	t.Parallel()
	got := renderNetplan(&params{Name: "br0", Members: []string{"eth0"}, STP: true})
	want := netpersist.ManagedHeader +
		"network:\n  version: 2\n  bridges:\n" +
		"    br0:\n" +
		"      interfaces:\n        - eth0\n" +
		"      parameters:\n        stp: true\n"
	if got != want {
		t.Errorf("netplan:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	// declared stp:false is pinned explicitly (netplan default is true)
	if g := renderNetplan(&params{Name: "br1", STP: false}); !strings.Contains(g, "stp: false") {
		t.Errorf("stp:false must be explicit: %q", g)
	}
}

// --- module persist integration --------------------------------------

func usePersistTempdir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	origND, origNP := netpersist.NetworkdDir, netpersist.NetplanDir
	netpersist.NetworkdDir = filepath.Join(dir, "systemd-network")
	netpersist.NetplanDir = filepath.Join(dir, "absent-netplan")
	t.Cleanup(func() { netpersist.NetworkdDir, netpersist.NetplanDir = origND, origNP })
}

func TestPersist_PresentWritesAndIdempotent(t *testing.T) {
	usePersistTempdir(t)
	ctx := context.Background()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("br0", "present", map[string]any{
		"name": "br0", "stp": true, "members": []any{"eth0", "eth1"}, "persist": "networkd",
	})

	if res, _ := m.Check(ctx, d); res.Matches {
		t.Error("runtime + persist both absent → want drift")
	}
	sr, err := m.Apply(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Fatalf("apply should change: %+v", sr)
	}
	if c, ok, _ := netpersist.Read(netpersist.NetdevPath("br0")); !ok || !strings.Contains(c, "Kind=bridge") || !strings.Contains(c, "STP=yes") {
		t.Errorf("netdev wrong: %q", c)
	}
	for _, mbr := range []string{"eth0", "eth1"} {
		if c, ok, _ := netpersist.Read(netpersist.NetworkDropinPath(mbr, "kscore-bridge-br0")); !ok || !strings.Contains(c, "Bridge=br0") {
			t.Errorf("enslave drop-in for %s wrong: %q", mbr, c)
		}
	}
	if res2, _ := m.Check(ctx, d); !res2.Matches {
		t.Error("second check should converge")
	}
	if sr2, _ := m.Apply(ctx, d); sr2.Changed {
		t.Errorf("second apply should be a no-op: %+v", sr2)
	}
}

func TestPersist_AbsentRemovesByGlob(t *testing.T) {
	usePersistTempdir(t)
	ctx := context.Background()
	m := NewWithProvider(newFake())
	if _, err := m.Apply(ctx, decl("br0", "present", map[string]any{
		"name": "br0", "members": []any{"eth0", "eth1"}, "persist": "networkd",
	})); err != nil {
		t.Fatal(err)
	}
	ad := decl("br0", "absent", map[string]any{"name": "br0", "persist": "networkd"})
	if res, _ := m.Check(ctx, ad); res.Matches {
		t.Error("want drift for absent")
	}
	if _, err := m.Apply(ctx, ad); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := netpersist.Read(netpersist.NetdevPath("br0")); ok {
		t.Error("netdev should be gone")
	}
	for _, mbr := range []string{"eth0", "eth1"} {
		if _, ok, _ := netpersist.Read(netpersist.NetworkDropinPath(mbr, "kscore-bridge-br0")); ok {
			t.Errorf("drop-in for %s should be gone", mbr)
		}
	}
}

func TestPersist_AutoResolvesNetplan(t *testing.T) {
	usePersistTempdir(t)
	if err := os.MkdirAll(netpersist.NetplanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewWithProvider(newFake())
	if _, err := m.Apply(context.Background(), decl("br0", "present", map[string]any{
		"name": "br0", "members": []any{"eth0"}, "persist": "auto",
	})); err != nil {
		t.Fatal(err)
	}
	if c, ok, _ := netpersist.Read(netpersist.NetplanDevicePath("bridge", "br0")); !ok || !strings.Contains(c, "bridges:") {
		t.Errorf("auto should write a netplan bridges file: %q", c)
	}
}

func TestPersist_Validation(t *testing.T) {
	t.Parallel()
	if err := (&Module{}).Validate(decl("br0", "present", map[string]any{"name": "br0", "persist": "nm"})); err == nil {
		t.Error("bad backend should error")
	}
	if err := (&Module{}).Validate(decl("br0", "present", map[string]any{"name": "br0", "persist": "networkd"})); err != nil {
		t.Errorf("valid persist should pass: %v", err)
	}
}
