// SPDX-License-Identifier: Apache-2.0

package network

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

func renderParams() *params {
	return &params{
		Interface:    "eth0",
		Addresses:    []string{"10.0.0.5/24", "10.0.0.1/24"}, // unsorted on purpose
		HasAddresses: true,
		MTU:          1500,
		HasMTU:       true,
	}
}

func TestRenderNetworkd(t *testing.T) {
	t.Parallel()
	got := renderNetworkd(renderParams())
	want := "# Managed by keystone-core (network module). Do not edit.\n" +
		"[Match]\n" +
		"Name=eth0\n" +
		"\n[Network]\n" +
		"Address=10.0.0.1/24\n" + // sorted
		"Address=10.0.0.5/24\n" +
		"\n[Link]\n" +
		"MTUBytes=1500\n"
	if got != want {
		t.Errorf("networkd render:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderNetplan(t *testing.T) {
	t.Parallel()
	got := renderNetplan(renderParams())
	want := "# Managed by keystone-core (network module). Do not edit.\n" +
		"network:\n" +
		"  version: 2\n" +
		"  ethernets:\n" +
		"    eth0:\n" +
		"      addresses:\n" +
		"        - 10.0.0.1/24\n" +
		"        - 10.0.0.5/24\n" +
		"      mtu: 1500\n"
	if got != want {
		t.Errorf("netplan render:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRender_MTUOnly(t *testing.T) {
	t.Parallel()
	p := &params{Interface: "eth1", MTU: 9000, HasMTU: true}
	nd := renderNetworkd(p)
	if strings.Contains(nd, "[Network]") || !strings.Contains(nd, "MTUBytes=9000") {
		t.Errorf("mtu-only networkd should omit [Network]: %q", nd)
	}
	np := renderNetplan(p)
	if strings.Contains(np, "addresses:") || !strings.Contains(np, "mtu: 9000") {
		t.Errorf("mtu-only netplan should omit addresses: %q", np)
	}
}

func TestRenderPersist_UnknownBackend(t *testing.T) {
	t.Parallel()
	if _, err := renderPersist("nm", renderParams()); err == nil {
		t.Error("unknown backend should error")
	}
	if _, err := persistFilePath("nm", "eth0"); err == nil {
		t.Error("unknown backend path should error")
	}
}

// --- module persist integration --------------------------------------

// usePersistTempdir points the netpersist base dirs at a fresh tempdir
// for the duration of the test. Not parallel-safe (mutates package
// globals), so the callers must not call t.Parallel.
func usePersistTempdir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	origND, origNP := netpersist.NetworkdDir, netpersist.NetplanDir
	netpersist.NetworkdDir = filepath.Join(dir, "systemd-network")
	netpersist.NetplanDir = filepath.Join(dir, "absent-netplan") // absent by default → DetectBackend = networkd
	t.Cleanup(func() { netpersist.NetworkdDir, netpersist.NetplanDir = origND, origNP })
}

func converged(addrs ...string) *InterfaceState {
	return &InterfaceState{Name: "eth0", Up: true, MTU: 1500, Addresses: addrs}
}

func persistDecl(backend string) *statemgmt.Declaration {
	return decl("l", map[string]any{
		"interface": "eth0",
		"addresses": []any{"10.0.0.1/24"},
		"mtu":       1500,
		"persist":   backend,
	})
}

func TestPersist_WritesFileWhenAbsent(t *testing.T) {
	usePersistTempdir(t)
	// Runtime already converged, but the persist file is missing → drift.
	f := &fakeProvider{state: converged("10.0.0.1/24")}
	m := NewWithProvider(f)
	d := persistDecl("networkd")

	res, err := m.Check(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if res.Matches {
		t.Error("missing persist file → want drift even though runtime is converged")
	}

	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Fatalf("apply should write the persist file: %+v", sr)
	}
	content, ok, err := netpersist.Read(netpersist.NetworkPath("eth0"))
	if err != nil || !ok || !strings.Contains(content, "Address=10.0.0.1/24") {
		t.Errorf("written networkd file wrong: %q ok=%v err=%v", content, ok, err)
	}
	// idempotent re-apply (file now matches the render)
	sr2, _ := m.Apply(context.Background(), d)
	if sr2.Changed {
		t.Errorf("second apply should be a no-op; got %+v", sr2)
	}
}

func TestPersist_RewritesStaleFile(t *testing.T) {
	usePersistTempdir(t)
	if err := netpersist.Write(netpersist.NetworkPath("eth0"), "# stale content\n"); err != nil {
		t.Fatal(err)
	}
	f := &fakeProvider{state: converged("10.0.0.1/24")}
	m := NewWithProvider(f)
	res, _ := m.Check(context.Background(), persistDecl("networkd"))
	if res.Matches {
		t.Error("stale persist file → drift")
	}
	sr, _ := m.Apply(context.Background(), persistDecl("networkd"))
	if !sr.Changed {
		t.Error("stale file should be rewritten")
	}
	content, _, _ := netpersist.Read(netpersist.NetworkPath("eth0"))
	if strings.Contains(content, "stale") {
		t.Errorf("stale content should be gone: %q", content)
	}
}

func TestPersist_Auto_UsesDetectBackend(t *testing.T) {
	usePersistTempdir(t)
	// Make the netplan dir exist → DetectBackend resolves auto to netplan.
	if err := os.MkdirAll(netpersist.NetplanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := &fakeProvider{state: converged("10.0.0.1/24")}
	m := NewWithProvider(f)
	if _, err := m.Apply(context.Background(), persistDecl("auto")); err != nil {
		t.Fatal(err)
	}
	content, ok, err := netpersist.Read(netpersist.NetplanPath("eth0"))
	if err != nil || !ok || !strings.Contains(content, "ethernets:") {
		t.Errorf("auto should write a netplan file: %q ok=%v err=%v", content, ok, err)
	}
}

func TestPersist_RuntimeAndPersistBothApply(t *testing.T) {
	usePersistTempdir(t)
	// Runtime drifts (missing address) AND persist file absent.
	f := &fakeProvider{state: &InterfaceState{Name: "eth0", Up: true, MTU: 1500}}
	m := NewWithProvider(f)
	sr, err := m.Apply(context.Background(), persistDecl("networkd"))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.addCalls) != 1 {
		t.Errorf("runtime address should be added: %+v", f.addCalls)
	}
	if _, ok, _ := netpersist.Read(netpersist.NetworkPath("eth0")); !ok {
		t.Error("persist file should be written")
	}
	if !sr.Changed {
		t.Error("changed expected")
	}
}

func TestPersist_NoPersistParamSkipsFileOps(t *testing.T) {
	usePersistTempdir(t)
	f := &fakeProvider{state: converged("10.0.0.1/24")}
	m := NewWithProvider(f)
	// No persist param → converged runtime is a full no-op, no file ops.
	res, _ := m.Check(context.Background(), decl("l", map[string]any{"interface": "eth0", "addresses": []any{"10.0.0.1/24"}, "mtu": 1500}))
	if !res.Matches {
		t.Errorf("runtime converged + no persist → converged; got %+v", res)
	}
	if _, ok, _ := netpersist.Read(netpersist.NetworkPath("eth0")); ok {
		t.Error("no persist param → no file written")
	}
}

func TestPersist_Validation(t *testing.T) {
	t.Parallel()
	bad := []map[string]any{
		{"interface": "eth0", "addresses": []any{"10.0.0.1/24"}, "persist": "networkmanager"}, // bad backend
		{"interface": "eth0", "up": true, "persist": "networkd"},                              // up-only, nothing to render
		{"interface": "eth0", "addresses": []any{"10.0.0.1/24"}, "persist": 1},                // non-string
	}
	for _, b := range bad {
		if err := (&Module{}).Validate(decl("l", b)); err == nil {
			t.Errorf("Validate(%v) should error", b)
		}
	}
	// valid: persist with addresses
	if err := (&Module{}).Validate(persistDecl("networkd")); err != nil {
		t.Errorf("persist with addresses should validate; got %v", err)
	}
}
