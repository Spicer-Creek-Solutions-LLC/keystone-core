// SPDX-License-Identifier: Apache-2.0

package firewall

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "firewall:" + name,
		Module: "firewall",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakes -------------------------------------------------------------

type fakeDetector struct {
	name string
	err  error
}

func (d *fakeDetector) Detect(_ context.Context) (string, error) { return d.name, d.err }

// recordingModule is a statemgmt.Module that records the Declaration
// it received and returns canned Check/Apply results.
type recordingModule struct {
	name        string
	received    *statemgmt.Declaration
	checkResult *statemgmt.ModuleCheckResult
	checkErr    error
	applyResult *statemgmt.StateResult
	applyErr    error
}

func (m *recordingModule) Name() string          { return m.name }
func (m *recordingModule) ValidStates() []string { return []string{StatePresent, StateAbsent} }
func (m *recordingModule) Check(_ context.Context, d *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	m.received = d
	return m.checkResult, m.checkErr
}
func (m *recordingModule) Apply(_ context.Context, d *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	m.received = d
	return m.applyResult, m.applyErr
}
func (m *recordingModule) Test(_ context.Context, _ *statemgmt.Declaration) (bool, error) {
	return false, nil
}

func wiredModule(t *testing.T, detectorName string, detectorErr error) (statemgmt.Module, map[string]*recordingModule) {
	t.Helper()
	r := map[string]*recordingModule{
		BackendIptables:  {name: BackendIptables, checkResult: &statemgmt.ModuleCheckResult{Matches: true}, applyResult: &statemgmt.StateResult{Success: true}},
		BackendNftables:  {name: BackendNftables, checkResult: &statemgmt.ModuleCheckResult{Matches: true}, applyResult: &statemgmt.StateResult{Success: true}},
		BackendFirewalld: {name: BackendFirewalld, checkResult: &statemgmt.ModuleCheckResult{Matches: true}, applyResult: &statemgmt.StateResult{Success: true}},
	}
	backends := map[string]statemgmt.Module{
		BackendIptables:  r[BackendIptables],
		BackendNftables:  r[BackendNftables],
		BackendFirewalld: r[BackendFirewalld],
	}
	m := NewWithBackends(&fakeDetector{name: detectorName, err: detectorErr}, backends)
	return m, r
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"services": "ssh"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_Defaults(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("l", StatePresent, map[string]any{"service": "ssh"}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Zone != "public" || p.Backend != "" || p.Port != "22" || p.Proto != "tcp" {
		t.Errorf("defaults: %+v", p)
	}
	// type errors
	for _, bad := range []map[string]any{
		{"backend": 1, "service": "ssh"},
		{"zone": 1, "service": "ssh"},
		{"service": 1},
		{"port": 1},
	} {
		if _, err := parseParams(decl("l", StatePresent, bad)); err == nil {
			t.Errorf("parseParams(%v) should error", bad)
		}
	}
}

func TestParse_ServiceCatalog(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]struct{ port, proto string }{
		"ssh":     {"22", "tcp"},
		"http":    {"80", "tcp"},
		"ntp":     {"123", "udp"},
		"dns-tcp": {"53", "tcp"},
		"dns-udp": {"53", "udp"},
	} {
		p, err := parseParams(decl("l", StatePresent, map[string]any{"service": name}))
		if err != nil || p.Port != want.port || p.Proto != want.proto {
			t.Errorf("service %q: %+v %v", name, p, err)
		}
	}
	// unknown service mentions the catalog
	_, err := parseParams(decl("l", StatePresent, map[string]any{"service": "nope"}))
	if err == nil {
		t.Fatal("unknown service should error")
	}
	if msg := err.Error(); !contains(msg, "v1.0 catalog") || !contains(msg, "ssh") {
		t.Errorf("error should mention catalog + known names, got %q", msg)
	}
	// empty service name
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"service": "   "})); err == nil {
		t.Error("empty service should error")
	}
}

func TestKnownServiceNames_SortedNonEmpty(t *testing.T) {
	t.Parallel()
	names := KnownServiceNames()
	if len(names) == 0 {
		t.Fatal("catalog is empty")
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Errorf("names not sorted: %q >= %q", names[i-1], names[i])
		}
	}
}

func TestParse_PortSpec(t *testing.T) {
	t.Parallel()
	good := []struct {
		in    string
		port  string
		proto string
	}{
		{"22/tcp", "22", "tcp"},
		{"53/udp", "53", "udp"},
		{"1000-2000/tcp", "1000-2000", "tcp"},
		{"4444/sctp", "4444", "sctp"},
		{"5555/dccp", "5555", "dccp"},
		{"65535/tcp", "65535", "tcp"},
	}
	for _, c := range good {
		p, err := parseParams(decl("l", StatePresent, map[string]any{"port": c.in}))
		if err != nil || p.Port != c.port || p.Proto != c.proto {
			t.Errorf("port %q: %+v %v", c.in, p, err)
		}
	}
	bad := []string{
		"22",      // no proto
		"22/icmp", // bad proto
		"ssh/tcp", // non-numeric
		"0/tcp",   // out of range
		"65536/tcp",
		"2000-1000/tcp", // descending
		"100-100/tcp",   // not ascending
		"/tcp",          // missing port
		"22/",           // missing proto
	}
	for _, in := range bad {
		if _, err := parseParams(decl("l", StatePresent, map[string]any{"port": in})); err == nil {
			t.Errorf("port %q should error", in)
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
		{"present service ok", decl("l", StatePresent, map[string]any{"service": "ssh"}), false},
		{"present port ok", decl("l", StatePresent, map[string]any{"port": "8080/tcp"}), false},
		{"explicit backend ok", decl("l", StatePresent, map[string]any{"service": "ssh", "backend": "iptables"}), false},
		{"bad backend", decl("l", StatePresent, map[string]any{"service": "ssh", "backend": "ufw"}), true},
		{"needs one item", decl("l", StatePresent, map[string]any{}), true},
		{"two items rejected", decl("l", StatePresent, map[string]any{"service": "ssh", "port": "22/tcp"}), true},
		{"bad zone charset", decl("l", StatePresent, map[string]any{"service": "ssh", "zone": "pu blic"}), true},
		{"empty zone", decl("l", StatePresent, map[string]any{"service": "ssh", "zone": "  "}), true},
		{"absent ok", decl("l", StateAbsent, map[string]any{"service": "ssh"}), false},
		{"bad state", decl("l", "frob", map[string]any{"service": "ssh"}), true},
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

// --- rule translation -------------------------------------------------

func TestIptablesRule(t *testing.T) {
	t.Parallel()
	p := &params{Port: "22", Proto: "tcp"}
	got := p.iptablesRule()
	want := []any{"-p", "tcp", "--dport", "22", "-j", "ACCEPT"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("iptablesRule(22/tcp) = %v, want %v", got, want)
	}
	// range — iptables uses ':'
	p = &params{Port: "1000-2000", Proto: "udp"}
	got = p.iptablesRule()
	want = []any{"-p", "udp", "--dport", "1000:2000", "-j", "ACCEPT"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("iptablesRule(1000-2000/udp) = %v, want %v", got, want)
	}
}

func TestNftablesRule(t *testing.T) {
	t.Parallel()
	if got := (&params{Port: "22", Proto: "tcp"}).nftablesRule(); got != "tcp dport 22 accept" {
		t.Errorf("nftablesRule(22/tcp) = %q", got)
	}
	if got := (&params{Port: "1000-2000", Proto: "udp"}).nftablesRule(); got != "udp dport 1000-2000 accept" {
		t.Errorf("nftablesRule(1000-2000/udp) = %q", got)
	}
}

func TestFirewalldPortValue(t *testing.T) {
	t.Parallel()
	if v := (&params{Port: "22", Proto: "tcp"}).firewalldPortValue(); v != "22/tcp" {
		t.Errorf("got %q", v)
	}
	if v := (&params{Port: "1000-2000", Proto: "tcp"}).firewalldPortValue(); v != "1000-2000/tcp" {
		t.Errorf("got %q", v)
	}
}

// --- dispatch ---------------------------------------------------------

func TestDispatch_AutoDetect_Firewalld(t *testing.T) {
	t.Parallel()
	m, r := wiredModule(t, BackendFirewalld, nil)
	d := decl("allow-ssh", StatePresent, map[string]any{"service": "ssh"})
	if _, err := m.Check(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	// only the firewalld backend was invoked
	if r[BackendFirewalld].received == nil {
		t.Fatal("firewalld backend was not invoked")
	}
	if r[BackendIptables].received != nil || r[BackendNftables].received != nil {
		t.Errorf("only firewalld should have been invoked")
	}
	got := r[BackendFirewalld].received
	if got.Module != BackendFirewalld || got.State != StatePresent || got.Name != "allow-ssh" {
		t.Errorf("sub-decl wrong: %+v", got)
	}
	if got.Params["zone"] != "public" || got.Params["port"] != "22/tcp" {
		t.Errorf("firewalld params: %v", got.Params)
	}
}

func TestDispatch_ExplicitBackend_BypassesDetector(t *testing.T) {
	t.Parallel()
	// detector says firewalld but operator pinned nftables
	m, r := wiredModule(t, BackendFirewalld, nil)
	d := decl("allow-ssh", StatePresent, map[string]any{"service": "ssh", "backend": BackendNftables})
	if _, err := m.Check(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if r[BackendFirewalld].received != nil || r[BackendIptables].received != nil {
		t.Errorf("only nftables should have been invoked")
	}
	got := r[BackendNftables].received
	if got == nil {
		t.Fatal("nftables backend was not invoked")
	}
	if got.Params["family"] != "inet" || got.Params["table"] != "filter" || got.Params["chain"] != "input" || got.Params["rule"] != "tcp dport 22 accept" {
		t.Errorf("nftables params: %v", got.Params)
	}
}

func TestDispatch_Iptables_Translation_PortRange(t *testing.T) {
	t.Parallel()
	m, r := wiredModule(t, BackendIptables, nil)
	d := decl("allow-range", StateAbsent, map[string]any{"port": "1000-2000/udp", "backend": BackendIptables})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	got := r[BackendIptables].received
	if got == nil {
		t.Fatal("iptables backend was not invoked")
	}
	if got.State != StateAbsent {
		t.Errorf("state should propagate: %q", got.State)
	}
	if got.Params["table"] != "filter" || got.Params["chain"] != "INPUT" || got.Params["family"] != "ipv4" {
		t.Errorf("iptables defaults: %v", got.Params)
	}
	wantRule := []any{"-p", "udp", "--dport", "1000:2000", "-j", "ACCEPT"}
	if !reflect.DeepEqual(got.Params["rule"], wantRule) {
		t.Errorf("rule = %v, want %v", got.Params["rule"], wantRule)
	}
}

func TestDispatch_Firewalld_CustomZone(t *testing.T) {
	t.Parallel()
	m, r := wiredModule(t, BackendFirewalld, nil)
	d := decl("allow-8080", StatePresent, map[string]any{"port": "8080/tcp", "zone": "dmz"})
	if _, err := m.Check(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	got := r[BackendFirewalld].received
	if got.Params["zone"] != "dmz" || got.Params["port"] != "8080/tcp" {
		t.Errorf("zone/port: %v", got.Params)
	}
}

func TestDispatch_DetectorError(t *testing.T) {
	t.Parallel()
	m, _ := wiredModule(t, "", ErrNoFirewall)
	d := decl("l", StatePresent, map[string]any{"service": "ssh"})
	if _, err := m.Check(context.Background(), d); err == nil || !errors.Is(err, ErrNoFirewall) {
		t.Errorf("detector error should propagate (wrapped), got %v", err)
	}
}

func TestDispatch_UnknownBackend(t *testing.T) {
	t.Parallel()
	// detector returns a name not present in backends
	m, _ := wiredModule(t, "ufw", nil)
	d := decl("l", StatePresent, map[string]any{"service": "ssh"})
	if _, err := m.Check(context.Background(), d); err == nil {
		t.Error("an unwired backend should error")
	}
}

func TestDispatch_BackendCheckPropagates(t *testing.T) {
	t.Parallel()
	m, r := wiredModule(t, BackendFirewalld, nil)
	r[BackendFirewalld].checkResult = &statemgmt.ModuleCheckResult{Matches: false, Diff: "drift"}
	r[BackendFirewalld].checkErr = nil
	res, err := m.Check(context.Background(), decl("l", StatePresent, map[string]any{"service": "ssh"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Matches || res.Diff != "drift" {
		t.Errorf("backend result not propagated: %+v", res)
	}
	// backend Check error
	r[BackendFirewalld].checkErr = errors.New("boom")
	if _, err := m.Check(context.Background(), decl("l", StatePresent, map[string]any{"service": "ssh"})); err == nil {
		t.Error("backend error should propagate")
	}
}

func TestDispatch_BackendApplyPropagates(t *testing.T) {
	t.Parallel()
	m, r := wiredModule(t, BackendFirewalld, nil)
	r[BackendFirewalld].applyResult = &statemgmt.StateResult{Success: true, Changed: true, Comment: "applied"}
	res, err := m.Apply(context.Background(), decl("l", StatePresent, map[string]any{"service": "ssh"}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Comment != "applied" {
		t.Errorf("apply result not propagated: %+v", res)
	}
	if res.Duration == 0 {
		t.Error("Apply should fill Duration when the backend returns 0")
	}
	// backend Apply error
	r[BackendFirewalld].applyErr = errors.New("nope")
	if _, err := m.Apply(context.Background(), decl("l", StatePresent, map[string]any{"service": "ssh"})); err == nil {
		t.Error("backend apply error should propagate")
	}
}

func TestParseError_FromCheckAndApply(t *testing.T) {
	t.Parallel()
	m, _ := wiredModule(t, BackendFirewalld, nil)
	bad := decl("l", StatePresent, map[string]any{}) // no item
	if _, err := m.Check(context.Background(), bad); err == nil {
		t.Error("Check should reject an invalid declaration")
	}
	if _, err := m.Apply(context.Background(), bad); err == nil {
		t.Error("Apply should reject an invalid declaration")
	}
}

func TestBuildSubDecl_UnsupportedBackend(t *testing.T) {
	t.Parallel()
	p := &params{State: StatePresent, Zone: "public", Port: "22", Proto: "tcp"}
	if _, err := buildSubDecl("ufw", p, decl("l", StatePresent, map[string]any{"service": "ssh"})); err == nil {
		t.Error("buildSubDecl(\"ufw\") should error")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "firewall" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("firewall should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	good := decl("l", StatePresent, map[string]any{"service": "ssh"})
	if dsm.DriftSeverity(good, nil) != statemgmt.DriftSeverityHigh {
		t.Error("present drift → HIGH")
	}
	if dsm.DriftSeverity(decl("l", StateAbsent, map[string]any{"service": "ssh"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(good); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("l", StatePresent, map[string]any{})); err == nil {
		t.Error("missing item should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	m, r := wiredModule(t, BackendFirewalld, nil)
	r[BackendFirewalld].checkResult = &statemgmt.ModuleCheckResult{Matches: false}
	d := decl("l", StatePresent, map[string]any{"service": "ssh"})
	if ok, err := m.Test(context.Background(), d); err != nil || ok {
		t.Errorf("Test should reflect backend Check.Matches=false: ok=%v err=%v", ok, err)
	}
	r[BackendFirewalld].checkResult = &statemgmt.ModuleCheckResult{Matches: true}
	if ok, err := m.Test(context.Background(), d); err != nil || !ok {
		t.Errorf("Test should reflect backend Check.Matches=true: ok=%v err=%v", ok, err)
	}
}

func TestSentinelMatchers(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) || IsUnsupportedOS(errors.New("x")) {
		t.Error("IsUnsupportedOS")
	}
	if !IsNoFirewall(ErrNoFirewall) || IsNoFirewall(errors.New("x")) {
		t.Error("IsNoFirewall")
	}
}

// --- small helper ----------------------------------------------------

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
