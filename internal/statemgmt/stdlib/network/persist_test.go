// SPDX-License-Identifier: Apache-2.0

package network

import (
	"context"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
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
}

// --- module persist integration --------------------------------------

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
	t.Parallel()
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
	if !sr.Changed || len(f.setPersCalls) != 1 {
		t.Fatalf("apply should write the persist file: %+v calls=%+v", sr, f.setPersCalls)
	}
	got := f.setPersCalls[0]
	if got.Backend != "networkd" || got.Iface != "eth0" || !strings.Contains(got.Content, "Address=10.0.0.1/24") {
		t.Errorf("persist write wrong: %+v", got)
	}
	// idempotent re-apply (file now matches the render)
	sr2, _ := m.Apply(context.Background(), d)
	if sr2.Changed {
		t.Errorf("second apply should be a no-op; got %+v", sr2)
	}
}

func TestPersist_RewritesStaleFile(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{
		state:     converged("10.0.0.1/24"),
		persisted: map[string]string{"networkd/eth0": "# stale content\n"},
	}
	m := NewWithProvider(f)
	res, _ := m.Check(context.Background(), persistDecl("networkd"))
	if res.Matches {
		t.Error("stale persist file → drift")
	}
	sr, _ := m.Apply(context.Background(), persistDecl("networkd"))
	if !sr.Changed || len(f.setPersCalls) != 1 {
		t.Errorf("stale file should be rewritten: %+v", f.setPersCalls)
	}
}

func TestPersist_Auto_UsesDetectBackend(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{state: converged("10.0.0.1/24"), detect: "netplan"}
	m := NewWithProvider(f)
	sr, err := m.Apply(context.Background(), persistDecl("auto"))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.setPersCalls) != 1 || f.setPersCalls[0].Backend != "netplan" {
		t.Errorf("auto should resolve via DetectBackend (netplan): %+v", f.setPersCalls)
	}
	if !strings.Contains(f.setPersCalls[0].Content, "ethernets:") {
		t.Errorf("netplan content expected: %q", f.setPersCalls[0].Content)
	}
	_ = sr
}

func TestPersist_RuntimeAndPersistBothApply(t *testing.T) {
	t.Parallel()
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
	if len(f.setPersCalls) != 1 {
		t.Errorf("persist file should be written: %+v", f.setPersCalls)
	}
	if !sr.Changed {
		t.Error("changed expected")
	}
}

func TestPersist_NoPersistParamSkipsFileOps(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{state: converged("10.0.0.1/24")}
	m := NewWithProvider(f)
	// No persist param → converged runtime is a full no-op, no file ops.
	res, _ := m.Check(context.Background(), decl("l", map[string]any{"interface": "eth0", "addresses": []any{"10.0.0.1/24"}, "mtu": 1500}))
	if !res.Matches {
		t.Errorf("runtime converged + no persist → converged; got %+v", res)
	}
	if len(f.setPersCalls) != 0 {
		t.Errorf("no persist param → no file ops; got %+v", f.setPersCalls)
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

func TestPersist_GetPersistedErrorPropagates(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{state: converged("10.0.0.1/24"), getPersErr: context.DeadlineExceeded}
	if _, err := NewWithProvider(f).Check(context.Background(), persistDecl("networkd")); err == nil {
		t.Error("GetPersisted error should propagate")
	}
}
