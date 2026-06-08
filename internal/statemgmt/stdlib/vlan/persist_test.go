// SPDX-License-Identifier: Apache-2.0

package vlan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"
)

// --- pure renderers ---------------------------------------------------

func TestRenderNetdevSection(t *testing.T) {
	t.Parallel()
	if g := renderNetdevSection(&params{ID: 100}); g != "[VLAN]\nId=100\n" {
		t.Errorf("section = %q", g)
	}
}

func TestRenderNetplan(t *testing.T) {
	t.Parallel()
	got := renderNetplan(&params{Name: "eth0.10", Parent: "eth0", ID: 10})
	want := netpersist.ManagedHeader +
		"network:\n  version: 2\n  vlans:\n" +
		"    eth0.10:\n" +
		"      id: 10\n" +
		"      link: eth0\n"
	if got != want {
		t.Errorf("netplan:\n--- got ---\n%s\n--- want ---\n%s", got, want)
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

func presentDecl() *statemgmt.Declaration {
	return decl("eth0.10", "present", map[string]any{
		"name": "eth0.10", "parent": "eth0", "id": 10, "persist": "networkd",
	})
}

func TestPersist_PresentWritesAndIdempotent(t *testing.T) {
	usePersistTempdir(t)
	ctx := context.Background()
	f := newFake()
	m := NewWithProvider(f)
	d := presentDecl()

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
	if c, ok, _ := netpersist.Read(netpersist.NetdevPath("eth0.10")); !ok || !strings.Contains(c, "Kind=vlan") || !strings.Contains(c, "Id=10") {
		t.Errorf("netdev wrong: %q", c)
	}
	// parent gets the VLAN= enslave drop-in + a base
	if c, ok, _ := netpersist.Read(netpersist.NetworkDropinPath("eth0", "kscore-vlan-eth0.10")); !ok || !strings.Contains(c, "VLAN=eth0.10") {
		t.Errorf("parent enslave drop-in wrong: %q", c)
	}
	if _, ok, _ := netpersist.Read(netpersist.NetworkPath("eth0")); !ok {
		t.Error("parent base missing")
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
	if _, err := m.Apply(ctx, presentDecl()); err != nil {
		t.Fatal(err)
	}
	// absent carries no parent/id
	ad := decl("eth0.10", "absent", map[string]any{"name": "eth0.10", "persist": "networkd"})
	if res, _ := m.Check(ctx, ad); res.Matches {
		t.Error("want drift for absent")
	}
	if _, err := m.Apply(ctx, ad); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := netpersist.Read(netpersist.NetdevPath("eth0.10")); ok {
		t.Error("netdev should be gone")
	}
	if _, ok, _ := netpersist.Read(netpersist.NetworkDropinPath("eth0", "kscore-vlan-eth0.10")); ok {
		t.Error("parent drop-in should be gone")
	}
	if _, ok, _ := netpersist.Read(netpersist.NetworkPath("eth0")); !ok {
		t.Error("parent base should remain")
	}
}

func TestPersist_AutoResolvesNetplan(t *testing.T) {
	usePersistTempdir(t)
	if err := os.MkdirAll(netpersist.NetplanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewWithProvider(newFake())
	d := decl("eth0.10", "present", map[string]any{"name": "eth0.10", "parent": "eth0", "id": 10, "persist": "auto"})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if c, ok, _ := netpersist.Read(netpersist.NetplanDevicePath("vlan", "eth0.10")); !ok || !strings.Contains(c, "vlans:") {
		t.Errorf("auto should write a netplan vlans file: %q", c)
	}
}

func TestPersist_Validation(t *testing.T) {
	t.Parallel()
	if err := (&Module{}).Validate(decl("eth0.10", "present", map[string]any{"name": "eth0.10", "parent": "eth0", "id": 10, "persist": "nm"})); err == nil {
		t.Error("bad backend should error")
	}
	if err := (&Module{}).Validate(presentDecl()); err != nil {
		t.Errorf("valid persist should pass: %v", err)
	}
}
