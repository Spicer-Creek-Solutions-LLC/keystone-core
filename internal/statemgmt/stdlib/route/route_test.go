// SPDX-License-Identifier: Apache-2.0

package route

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "route:" + name,
		Module: "route",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider -----------------------------------------------------

type fakeProvider struct {
	store      map[string]*RouteEntry // key = "<dest>|<metric>|<table>"
	getErr     error
	replaceErr error
	delErr     error

	replaceCalls []RouteSpec
	delCalls     []RouteQuery
}

func key(dest, table string, metric int, hasMetric bool) string {
	m := "-"
	if hasMetric {
		m = ""
		for v := metric; v != 0; v /= 10 {
		}
	}
	_ = m
	if hasMetric {
		return dest + "|" + intToString(metric) + "|" + table
	}
	return dest + "||" + table
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func newFake() *fakeProvider {
	return &fakeProvider{store: map[string]*RouteEntry{}}
}

func (f *fakeProvider) GetRoute(_ context.Context, q RouteQuery) (*RouteEntry, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	tbl := q.Table
	if tbl == "" {
		tbl = "main"
	}
	return f.store[key(q.Destination, tbl, q.Metric, q.HasMetric)], nil
}
func (f *fakeProvider) ReplaceRoute(_ context.Context, s RouteSpec) error {
	if f.replaceErr != nil {
		return f.replaceErr
	}
	f.replaceCalls = append(f.replaceCalls, s)
	tbl := s.Table
	if tbl == "" {
		tbl = "main"
	}
	f.store[key(s.Destination, tbl, s.Metric, s.HasMetric)] = &RouteEntry{
		Destination: s.Destination, Gateway: s.Gateway, Interface: s.Interface,
		Metric: s.Metric, HasMetric: s.HasMetric, Table: tbl,
	}
	return nil
}
func (f *fakeProvider) DelRoute(_ context.Context, q RouteQuery) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.delCalls = append(f.delCalls, q)
	tbl := q.Table
	if tbl == "" {
		tbl = "main"
	}
	delete(f.store, key(q.Destination, tbl, q.Metric, q.HasMetric))
	return nil
}

// --- params / validate -----------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"dst": "0.0.0.0/0"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_TypesAndCanonicalisation(t *testing.T) {
	t.Parallel()
	// CIDR destination
	p, err := parseParams(decl("l", StatePresent, map[string]any{"destination": "10.0.0.0/24", "gateway": "192.168.1.1"}))
	if err != nil || p.Destination != "10.0.0.0/24" || p.Gateway != "192.168.1.1" {
		t.Errorf("parse: %+v %v", p, err)
	}
	// bare IPv4 → /32 host route
	p, _ = parseParams(decl("l", StatePresent, map[string]any{"destination": "10.0.0.5", "gateway": "192.168.1.1"}))
	if p.Destination != "10.0.0.5/32" {
		t.Errorf("host route v4: %q", p.Destination)
	}
	// bare IPv6 → /128
	p, _ = parseParams(decl("l", StatePresent, map[string]any{"destination": "2001:db8::1", "gateway": "fe80::1"}))
	if p.Destination != "2001:db8::1/128" || p.Gateway != "fe80::1" {
		t.Errorf("host route v6: %+v", p)
	}
	// default route 0.0.0.0/0
	p, _ = parseParams(decl("l", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1"}))
	if p.Destination != "0.0.0.0/0" {
		t.Errorf("default: %q", p.Destination)
	}
	// table as int
	p, _ = parseParams(decl("l", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1", "table": 100}))
	if p.Table != "100" {
		t.Errorf("table int: %q", p.Table)
	}
	// metric coercion
	for _, v := range []any{10, int64(10), float64(10), "10"} {
		p, err := parseParams(decl("l", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1", "metric": v}))
		if err != nil || p.Metric != 10 || !p.HasMetric {
			t.Errorf("metric=%v: %+v %v", v, p, err)
		}
	}
	// type errors
	for _, bad := range []map[string]any{
		{"destination": 1, "gateway": "192.168.1.1"},
		{"destination": "0.0.0.0/0", "gateway": 1},
		{"destination": "0.0.0.0/0", "gateway": "192.168.1.1", "interface": 1},
		{"destination": "0.0.0.0/0", "gateway": "192.168.1.1", "metric": "x"},
		{"destination": "0.0.0.0/0", "gateway": "192.168.1.1", "metric": 1.5},
		{"destination": "0.0.0.0/0", "gateway": "192.168.1.1", "table": []any{}},
		{"destination": "not-a-cidr", "gateway": "192.168.1.1"},
		{"destination": "0.0.0.0/0", "gateway": "not-an-ip"},
	} {
		if _, err := parseParams(decl("l", StatePresent, bad)); err == nil {
			t.Errorf("parseParams(%v) should error", bad)
		}
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		d       *statemgmt.Declaration
		wantErr bool
	}{
		{"present via gateway", decl("l", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1"}), false},
		{"present via interface only", decl("l", StatePresent, map[string]any{"destination": "10.0.0.0/24", "interface": "eth0"}), false},
		{"present needs nexthop", decl("l", StatePresent, map[string]any{"destination": "0.0.0.0/0"}), true},
		{"absent doesn't need nexthop", decl("l", StateAbsent, map[string]any{"destination": "0.0.0.0/0"}), false},
		{"with metric + table", decl("l", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1", "metric": 100, "table": "vpn"}), false},
		{"missing destination", decl("l", StatePresent, map[string]any{"gateway": "192.168.1.1"}), true},
		{"bad interface name", decl("l", StatePresent, map[string]any{"destination": "10.0.0.0/24", "interface": "eth 0"}), true},
		{"bad table charset", decl("l", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1", "table": "v p n"}), true},
		{"bad state", decl("l", "frob", map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1"}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := parseParams(tc.d)
			if err == nil {
				err = p.validate()
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

// --- module logic ----------------------------------------------------

func TestPresent_AddNew(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("def", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("absent → drift: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.replaceCalls) != 1 || f.replaceCalls[0].Gateway != "192.168.1.1" {
		t.Errorf("replace: %+v", f.replaceCalls)
	}
	// converged
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged check: %+v", r)
	}
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Errorf("converged apply: %+v", sr)
	}
}

func TestPresent_ReplaceOnGatewayChange(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.store[key("0.0.0.0/0", "main", 0, false)] = &RouteEntry{Destination: "0.0.0.0/0", Gateway: "10.0.0.1", Table: "main"}
	m := NewWithProvider(f)
	d := decl("def", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("gateway differs → drift: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.replaceCalls) != 1 || f.replaceCalls[0].Gateway != "192.168.1.1" {
		t.Errorf("replace: %+v", f.replaceCalls)
	}
}

func TestPresent_InterfaceMatch(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.store[key("10.0.0.0/24", "main", 0, false)] = &RouteEntry{Destination: "10.0.0.0/24", Interface: "eth0", Table: "main"}
	m := NewWithProvider(f)
	// matches (only interface declared)
	d := decl("net", StatePresent, map[string]any{"destination": "10.0.0.0/24", "interface": "eth0"})
	r, _ := m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("match: %+v", r)
	}
	// different interface
	d2 := decl("net", StatePresent, map[string]any{"destination": "10.0.0.0/24", "interface": "eth1"})
	r, _ = m.Check(context.Background(), d2)
	if r.Matches {
		t.Errorf("interface differs → drift: %+v", r)
	}
}

func TestAbsent(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.store[key("0.0.0.0/0", "main", 0, false)] = &RouteEntry{Destination: "0.0.0.0/0", Gateway: "10.0.0.1", Table: "main"}
	m := NewWithProvider(f)
	d := decl("def", StateAbsent, map[string]any{"destination": "0.0.0.0/0"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("present → drift: %+v", r)
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.delCalls) != 1 {
		t.Errorf("del: %+v", f.delCalls)
	}
	// already absent
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged check: %+v", r)
	}
	sr, _ := m.Apply(context.Background(), d)
	if sr.Changed {
		t.Errorf("converged apply: %+v", sr)
	}
}

func TestRouteMatches_FieldByField(t *testing.T) {
	t.Parallel()
	live := &RouteEntry{Gateway: "10.0.0.1", Interface: "eth0"}
	// both fields declared and match
	if !routeMatches(&params{Gateway: "10.0.0.1", Interface: "eth0"}, live) {
		t.Error("both match")
	}
	// only gateway declared, matches
	if !routeMatches(&params{Gateway: "10.0.0.1"}, live) {
		t.Error("gateway-only match")
	}
	// only interface declared, matches
	if !routeMatches(&params{Interface: "eth0"}, live) {
		t.Error("interface-only match")
	}
	// gateway mismatch
	if routeMatches(&params{Gateway: "10.0.0.2"}, live) {
		t.Error("gateway mismatch should be drift")
	}
	// interface mismatch
	if routeMatches(&params{Interface: "eth1"}, live) {
		t.Error("interface mismatch should be drift")
	}
}

// --- errors ----------------------------------------------------------

func TestErrorsPropagate(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{getErr: errors.New("get")})
	if _, err := m.Check(context.Background(), decl("l", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1"})); err == nil {
		t.Error("Get error should propagate")
	}
	// Replace error
	f := newFake()
	f.replaceErr = errors.New("replace")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1"})); err == nil {
		t.Error("Replace error should propagate")
	}
	// Del error
	f = newFake()
	f.store[key("0.0.0.0/0", "main", 0, false)] = &RouteEntry{Destination: "0.0.0.0/0", Gateway: "10.0.0.1"}
	f.delErr = errors.New("del")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StateAbsent, map[string]any{"destination": "0.0.0.0/0"})); err == nil {
		t.Error("Del error should propagate")
	}
}

func TestParseError_FromCheckAndApply(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(newFake())
	bad := decl("l", StatePresent, map[string]any{}) // no destination
	if _, err := m.Check(context.Background(), bad); err == nil {
		t.Error("Check should reject an invalid declaration")
	}
	if _, err := m.Apply(context.Background(), bad); err == nil {
		t.Error("Apply should reject an invalid declaration")
	}
}

// --- module surface --------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "route" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("route should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("l", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("any decl → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("l", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("l", StatePresent, map[string]any{})); err == nil {
		t.Error("missing destination should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("l", StatePresent, map[string]any{"destination": "0.0.0.0/0", "gateway": "192.168.1.1"})
	if ok, err := m.Test(context.Background(), d); err != nil || ok {
		t.Errorf("Test before apply: ok=%v err=%v", ok, err)
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.Test(context.Background(), d); err != nil || !ok {
		t.Errorf("Test after apply: ok=%v err=%v", ok, err)
	}
}

func TestSentinelMatchers(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) || IsUnsupportedOS(errors.New("x")) {
		t.Error("IsUnsupportedOS")
	}
	if !IsNoIP(ErrNoIP) || IsNoIP(errors.New("x")) {
		t.Error("IsNoIP")
	}
}
