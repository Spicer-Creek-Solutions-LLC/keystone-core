// SPDX-License-Identifier: Apache-2.0

package route

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"
)

// --- pure renderers / slug -------------------------------------------

func TestRenderNetworkdRoute(t *testing.T) {
	t.Parallel()
	p := &params{Destination: "10.0.0.0/24", Gateway: "192.168.1.1", Interface: "eth0", Table: defaultTable}
	got := renderNetworkdRoute(p)
	want := "# Managed by keystone-core (route module). Do not edit.\n" +
		"[Route]\n" +
		"Destination=10.0.0.0/24\n" +
		"Gateway=192.168.1.1\n"
	if got != want {
		t.Errorf("networkd route render:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderNetworkdRoute_MetricTable(t *testing.T) {
	t.Parallel()
	p := &params{Destination: "0.0.0.0/0", Gateway: "10.0.0.1", Interface: "eth0", Metric: 100, HasMetric: true, Table: "200"}
	got := renderNetworkdRoute(p)
	want := "# Managed by keystone-core (route module). Do not edit.\n" +
		"[Route]\n" +
		"Destination=0.0.0.0/0\n" +
		"Gateway=10.0.0.1\n" +
		"Metric=100\n" +
		"Table=200\n"
	if got != want {
		t.Errorf("networkd route render:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderNetplanRoute(t *testing.T) {
	t.Parallel()
	p := &params{Destination: "10.0.0.0/24", Gateway: "192.168.1.1", Interface: "eth0", Table: defaultTable}
	got := renderNetplanRoute(p)
	want := "# Managed by keystone-core (route module). Do not edit.\n" +
		"network:\n" +
		"  version: 2\n" +
		"  ethernets:\n" +
		"    eth0:\n" +
		"      routes:\n" +
		"        - to: 10.0.0.0/24\n" +
		"          via: 192.168.1.1\n"
	if got != want {
		t.Errorf("netplan route render:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderNetplanRoute_MetricTable(t *testing.T) {
	t.Parallel()
	p := &params{Destination: "10.0.0.0/24", Interface: "eth1", Metric: 50, HasMetric: true, Table: "200"}
	got := renderNetplanRoute(p)
	if !strings.Contains(got, "metric: 50") || !strings.Contains(got, "table: 200") || strings.Contains(got, "via:") {
		t.Errorf("netplan metric/table/no-gateway render wrong:\n%s", got)
	}
}

func TestRouteSlug(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p    *params
		want string
	}{
		{&params{Destination: "10.0.0.0/24", Table: "main"}, "10-0-0-0_24-main"},
		{&params{Destination: "0.0.0.0/0", Table: "main", Metric: 100, HasMetric: true}, "0-0-0-0_0-main-m100"},
		{&params{Destination: "::/0", Table: "main"}, "--_0-main"},
		{&params{Destination: "2001:db8::/32", Table: "200"}, "2001-db8--_32-200"},
	}
	for _, c := range cases {
		if got := routeSlug(c.p); got != c.want {
			t.Errorf("routeSlug(%+v) = %q, want %q", c.p, got, c.want)
		}
	}
}

func TestRenderRoute_UnknownBackend(t *testing.T) {
	t.Parallel()
	if _, err := renderRoute("nm", &params{Destination: "10.0.0.0/24"}); err == nil {
		t.Error("unknown backend render should error")
	}
	if _, err := routePersistPath("nm", &params{Destination: "10.0.0.0/24"}); err == nil {
		t.Error("unknown backend path should error")
	}
}

// --- module persist integration --------------------------------------

// usePersistTempdir points the netpersist base dirs at a fresh tempdir.
// Mutates package globals, so callers must not call t.Parallel.
func usePersistTempdir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	origND, origNP := netpersist.NetworkdDir, netpersist.NetplanDir
	netpersist.NetworkdDir = filepath.Join(dir, "systemd-network")
	netpersist.NetplanDir = filepath.Join(dir, "absent-netplan") // absent → DetectBackend = networkd
	t.Cleanup(func() { netpersist.NetworkdDir, netpersist.NetplanDir = origND, origNP })
}

func persistRouteDecl(backend string) *statemgmt.Declaration {
	return decl("r", "present", map[string]any{
		"destination": "10.0.0.0/24",
		"gateway":     "192.168.1.1",
		"interface":   "eth0",
		"persist":     backend,
	})
}

func TestPersist_PresentWritesAndIdempotent(t *testing.T) {
	usePersistTempdir(t)
	ctx := context.Background()
	f := newFake()
	m := NewWithProvider(f)
	d := persistRouteDecl("networkd")

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
	if !sr.Changed {
		t.Fatalf("apply should change: %+v", sr)
	}
	dropin := netpersist.NetworkDropinPath("eth0", "10-0-0-0_24-main")
	c, ok, err := netpersist.Read(dropin)
	if err != nil || !ok || !strings.Contains(c, "Destination=10.0.0.0/24") {
		t.Errorf("drop-in wrong: %q ok=%v err=%v", c, ok, err)
	}
	if _, baseOK, _ := netpersist.Read(netpersist.NetworkPath("eth0")); !baseOK {
		t.Error("networkd base unit should be created for the drop-in")
	}

	// idempotent: runtime now stored + drop-in matches → no-op
	res2, _ := m.Check(ctx, d)
	if !res2.Matches {
		t.Errorf("second check should converge: %+v", res2)
	}
	sr2, _ := m.Apply(ctx, d)
	if sr2.Changed {
		t.Errorf("second apply should be a no-op: %+v", sr2)
	}
}

func TestPersist_BaseNotOverwritten(t *testing.T) {
	usePersistTempdir(t)
	// Simulate the network module having written a fuller base unit.
	fuller := "# network module\n[Match]\nName=eth0\n[Network]\nAddress=10.0.0.5/24\n"
	if err := netpersist.Write(netpersist.NetworkPath("eth0"), fuller); err != nil {
		t.Fatal(err)
	}
	m := NewWithProvider(newFake())
	if _, err := m.Apply(context.Background(), persistRouteDecl("networkd")); err != nil {
		t.Fatal(err)
	}
	base, _, _ := netpersist.Read(netpersist.NetworkPath("eth0"))
	if base != fuller {
		t.Errorf("create-if-absent must not overwrite the existing base:\n%s", base)
	}
	if _, ok, _ := netpersist.Read(netpersist.NetworkDropinPath("eth0", "10-0-0-0_24-main")); !ok {
		t.Error("drop-in should still be written")
	}
}

func TestPersist_AbsentRemovesDropin(t *testing.T) {
	usePersistTempdir(t)
	ctx := context.Background()
	m := NewWithProvider(newFake())
	// establish a persisted present route
	if _, err := m.Apply(ctx, persistRouteDecl("networkd")); err != nil {
		t.Fatal(err)
	}
	dropin := netpersist.NetworkDropinPath("eth0", "10-0-0-0_24-main")
	if _, ok, _ := netpersist.Read(dropin); !ok {
		t.Fatal("precondition: drop-in should exist")
	}

	ad := decl("r", "absent", map[string]any{
		"destination": "10.0.0.0/24",
		"interface":   "eth0",
		"persist":     "networkd",
	})
	res, _ := m.Check(ctx, ad)
	if res.Matches {
		t.Error("route + drop-in present → want drift for absent")
	}
	sr, err := m.Apply(ctx, ad)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("absent apply should remove the drop-in")
	}
	if _, ok, _ := netpersist.Read(dropin); ok {
		t.Error("drop-in should be gone")
	}
	// base is left alone (other routes / addresses may need it)
	if _, ok, _ := netpersist.Read(netpersist.NetworkPath("eth0")); !ok {
		t.Error("base unit should not be removed")
	}
	// idempotent
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
	if _, err := m.Apply(context.Background(), persistRouteDecl("auto")); err != nil {
		t.Fatal(err)
	}
	c, ok, err := netpersist.Read(netpersist.NetplanRoutePath("10-0-0-0_24-main"))
	if err != nil || !ok || !strings.Contains(c, "routes:") {
		t.Errorf("auto should write a netplan route file: %q ok=%v err=%v", c, ok, err)
	}
}

func TestPersist_MultiRouteDropinsCoexist(t *testing.T) {
	usePersistTempdir(t)
	ctx := context.Background()
	m := NewWithProvider(newFake())
	for _, dest := range []string{"10.0.0.0/24", "10.0.1.0/24"} {
		d := decl("r", "present", map[string]any{
			"destination": dest, "gateway": "192.168.1.1", "interface": "eth0", "persist": "networkd",
		})
		if _, err := m.Apply(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	for _, slug := range []string{"10-0-0-0_24-main", "10-0-1-0_24-main"} {
		if _, ok, _ := netpersist.Read(netpersist.NetworkDropinPath("eth0", slug)); !ok {
			t.Errorf("drop-in %s should coexist", slug)
		}
	}
}

func TestPersist_NoParamSkipsFileOps(t *testing.T) {
	usePersistTempdir(t)
	ctx := context.Background()
	f := newFake()
	// runtime already converged
	if err := f.ReplaceRoute(ctx, RouteSpec{Destination: "10.0.0.0/24", Gateway: "192.168.1.1", Interface: "eth0", Table: "main"}); err != nil {
		t.Fatal(err)
	}
	f.replaceCalls = nil
	m := NewWithProvider(f)
	d := decl("r", "present", map[string]any{"destination": "10.0.0.0/24", "gateway": "192.168.1.1", "interface": "eth0"})
	res, _ := m.Check(ctx, d)
	if !res.Matches {
		t.Errorf("runtime converged + no persist → converged: %+v", res)
	}
	if _, ok, _ := netpersist.Read(netpersist.NetworkDropinPath("eth0", "10-0-0-0_24-main")); ok {
		t.Error("no persist param → no file written")
	}
}

func TestPersist_Validation(t *testing.T) {
	t.Parallel()
	bad := []map[string]any{
		{"destination": "10.0.0.0/24", "gateway": "192.168.1.1", "persist": "networkd"}, // no interface
		{"destination": "10.0.0.0/24", "gateway": "192.168.1.1", "interface": "eth0", "persist": "nm"}, // bad backend
		{"destination": "10.0.0.0/24", "gateway": "192.168.1.1", "interface": "eth0", "persist": 1},    // non-string
	}
	for _, b := range bad {
		if err := (&Module{}).Validate(decl("r", "present", b)); err == nil {
			t.Errorf("Validate(%v) should error", b)
		}
	}
	if err := (&Module{}).Validate(persistRouteDecl("networkd")); err != nil {
		t.Errorf("valid persist decl should pass: %v", err)
	}
}
