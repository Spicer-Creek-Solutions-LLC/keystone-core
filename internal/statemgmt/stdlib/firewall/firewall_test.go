// SPDX-License-Identifier: Apache-2.0

package firewall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/iptables"
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
	received    *statemgmt.Declaration   // most recent (single-sub backends)
	receivedAll []*statemgmt.Declaration // every decl received (dual-stack iptables)
	checkResult *statemgmt.ModuleCheckResult
	checkErr    error
	applyResult *statemgmt.StateResult
	applyErr    error

	// errFor, when set, decides the per-decl error (overriding
	// check/applyErr). Used to make only the IPv6 iptables sub fail
	// with ErrNoIptables in the graceful-skip test.
	errFor func(*statemgmt.Declaration) error
	// resultFor, when set, decides the per-decl apply result so a test
	// can give the two iptables families distinct Changed/Diff/Comment.
	resultFor func(*statemgmt.Declaration) *statemgmt.StateResult
	// checkResultFor is the Check analogue of resultFor.
	checkResultFor func(*statemgmt.Declaration) *statemgmt.ModuleCheckResult
}

func (m *recordingModule) Name() string          { return m.name }
func (m *recordingModule) ValidStates() []string { return []string{StatePresent, StateAbsent} }
func (m *recordingModule) Check(_ context.Context, d *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	m.received = d
	m.receivedAll = append(m.receivedAll, d)
	if m.errFor != nil {
		if err := m.errFor(d); err != nil {
			return nil, err
		}
	}
	if m.checkResultFor != nil {
		return m.checkResultFor(d), nil
	}
	return m.checkResult, m.checkErr
}
func (m *recordingModule) Apply(_ context.Context, d *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	m.received = d
	m.receivedAll = append(m.receivedAll, d)
	if m.errFor != nil {
		if err := m.errFor(d); err != nil {
			return nil, err
		}
	}
	if m.resultFor != nil {
		return m.resultFor(d), nil
	}
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
	if p.Zone != "public" || p.Backend != "" || len(p.Ports) != 1 || p.Ports[0] != (portProto{"22", "tcp"}) {
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
		if err != nil || len(p.Ports) != 1 || p.Ports[0].Port != want.port || p.Ports[0].Proto != want.proto {
			t.Errorf("service %q: %+v %v", name, p, err)
		}
	}
	// unknown service mentions the catalog
	_, err := parseParams(decl("l", StatePresent, map[string]any{"service": "nope"}))
	if err == nil {
		t.Fatal("unknown service should error")
	}
	if msg := err.Error(); !contains(msg, "catalog") || !contains(msg, "ssh") {
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
		if err != nil || len(p.Ports) != 1 || p.Ports[0].Port != c.port || p.Ports[0].Proto != c.proto {
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
	got := iptablesRule(portProto{"22", "tcp"})
	want := []any{"-p", "tcp", "--dport", "22", "-j", "ACCEPT"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("iptablesRule(22/tcp) = %v, want %v", got, want)
	}
	// range — iptables uses ':'
	got = iptablesRule(portProto{"1000-2000", "udp"})
	want = []any{"-p", "udp", "--dport", "1000:2000", "-j", "ACCEPT"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("iptablesRule(1000-2000/udp) = %v, want %v", got, want)
	}
}

func TestNftablesRule(t *testing.T) {
	t.Parallel()
	if got := nftablesRule(portProto{"22", "tcp"}); got != "tcp dport 22 accept" {
		t.Errorf("nftablesRule(22/tcp) = %q", got)
	}
	if got := nftablesRule(portProto{"1000-2000", "udp"}); got != "udp dport 1000-2000 accept" {
		t.Errorf("nftablesRule(1000-2000/udp) = %q", got)
	}
}

func TestFirewalldPortValue(t *testing.T) {
	t.Parallel()
	if v := firewalldPortValue(portProto{"22", "tcp"}); v != "22/tcp" {
		t.Errorf("got %q", v)
	}
	if v := firewalldPortValue(portProto{"1000-2000", "tcp"}); v != "1000-2000/tcp" {
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
		return
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
	// Dual-stack: the iptables backend is invoked twice — once per
	// address family — with the same translated rule body.
	all := r[BackendIptables].receivedAll
	if len(all) != 2 {
		t.Fatalf("iptables backend should be invoked twice (ipv4+ipv6), got %d", len(all))
	}
	wantRule := []any{"-p", "udp", "--dport", "1000:2000", "-j", "ACCEPT"}
	gotFamilies := map[string]bool{}
	for _, got := range all {
		if got.State != StateAbsent {
			t.Errorf("state should propagate: %q", got.State)
		}
		if got.Params["table"] != "filter" || got.Params["chain"] != "INPUT" {
			t.Errorf("iptables defaults: %v", got.Params)
		}
		if !reflect.DeepEqual(got.Params["rule"], wantRule) {
			t.Errorf("rule = %v, want %v", got.Params["rule"], wantRule)
		}
		gotFamilies[got.Params["family"].(string)] = true
	}
	if !gotFamilies["ipv4"] || !gotFamilies["ipv6"] {
		t.Errorf("want both ipv4 and ipv6 sub-decls, got families %v", gotFamilies)
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

func TestBuildSubDecls_UnsupportedBackend(t *testing.T) {
	t.Parallel()
	p := &params{State: StatePresent, Zone: "public", Ports: []portProto{{"22", "tcp"}}}
	if _, err := buildSubDecls("ufw", p, decl("l", StatePresent, map[string]any{"service": "ssh"})); err == nil {
		t.Error("buildSubDecls(\"ufw\") should error")
	}
}

// --- dual-stack iptables ----------------------------------------------

func sshIptablesDecl(state string) *statemgmt.Declaration {
	return decl("allow-ssh", state, map[string]any{"service": "ssh", "backend": BackendIptables})
}

func TestDualStack_Apply_AggregatesChangedAndSuccess(t *testing.T) {
	t.Parallel()
	m, r := wiredModule(t, BackendIptables, nil)
	// IPv4 changed, IPv6 already converged. Aggregate: Changed (any),
	// Success (all).
	r[BackendIptables].resultFor = func(d *statemgmt.Declaration) *statemgmt.StateResult {
		if d.Params["family"] == iptables.FamilyIPv4 {
			return &statemgmt.StateResult{Success: true, Changed: true, Comment: "applied"}
		}
		return &statemgmt.StateResult{Success: true, Changed: false, Comment: "already converged"}
	}
	res, err := m.Apply(context.Background(), sshIptablesDecl(StatePresent))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || !res.Changed {
		t.Errorf("want Success+Changed, got %+v", res)
	}
	// Comment is family-labelled so a dual-stack result is legible.
	if !strings.Contains(res.Comment, "ipv4 22/tcp: applied") || !strings.Contains(res.Comment, "ipv6 22/tcp: already converged") {
		t.Errorf("comment should carry both families with port labels: %q", res.Comment)
	}
	if res.Duration == 0 {
		t.Error("Duration should be filled")
	}
}

func TestDualStack_Check_MatchesOnlyWhenBothMatch(t *testing.T) {
	t.Parallel()

	t.Run("both match", func(t *testing.T) {
		t.Parallel()
		m, r := wiredModule(t, BackendIptables, nil)
		res, err := m.Check(context.Background(), sshIptablesDecl(StatePresent))
		if err != nil {
			t.Fatal(err)
		}
		if !res.Matches {
			t.Errorf("both families match → want Matches, got %+v", res)
		}
		if n := len(r[BackendIptables].receivedAll); n != 2 {
			t.Errorf("Check should probe both families, got %d", n)
		}
	})

	t.Run("one family drifts", func(t *testing.T) {
		t.Parallel()
		m, r := wiredModule(t, BackendIptables, nil)
		// IPv4 matches, IPv6 drifts → aggregate must report not-match
		// with the IPv6 diff family-labelled.
		r[BackendIptables].checkResultFor = func(d *statemgmt.Declaration) *statemgmt.ModuleCheckResult {
			if d.Params["family"] == iptables.FamilyIPv6 {
				return &statemgmt.ModuleCheckResult{Matches: false, Diff: "rule absent"}
			}
			return &statemgmt.ModuleCheckResult{Matches: true}
		}
		res, err := m.Check(context.Background(), sshIptablesDecl(StatePresent))
		if err != nil {
			t.Fatal(err)
		}
		if res.Matches {
			t.Error("IPv6 drift → aggregate should not match")
		}
		if !strings.Contains(res.Diff, "ipv6 22/tcp: rule absent") {
			t.Errorf("diff should carry the family-labelled IPv6 drift: %q", res.Diff)
		}
	})
}

func TestDualStack_IPv6SkippedWhenNoIp6tables(t *testing.T) {
	t.Parallel()
	m, r := wiredModule(t, BackendIptables, nil)
	r[BackendIptables].applyResult = &statemgmt.StateResult{Success: true, Changed: true, Comment: "applied"}
	// Only the IPv6 sub fails, with the iptables "no binary" sentinel.
	r[BackendIptables].errFor = func(d *statemgmt.Declaration) error {
		if d.Params["family"] == iptables.FamilyIPv6 {
			return iptables.ErrNoIptables
		}
		return nil
	}
	res, err := m.Apply(context.Background(), sshIptablesDecl(StatePresent))
	if err != nil {
		t.Fatalf("graceful skip must not error: %v", err)
	}
	if !res.Success {
		t.Errorf("IPv4 applied → apply should succeed: %+v", res)
	}
	// The skip must be unmistakable in BOTH surfaces.
	if !strings.Contains(res.Comment, "ipv6 22/tcp NOT APPLIED") {
		t.Errorf("comment must loudly flag the IPv6 skip: %q", res.Comment)
	}
	if !strings.Contains(res.Diff, "ipv6 22/tcp: SKIPPED") {
		t.Errorf("diff must flag the IPv6 skip: %q", res.Diff)
	}
	if !strings.Contains(res.Comment, "ipv4 22/tcp: applied") {
		t.Errorf("IPv4 should still have applied: %q", res.Comment)
	}
}

func TestDualStack_Check_IPv6SkipStaysConverged(t *testing.T) {
	t.Parallel()
	// On an IPv4-only host, Check must not report perpetual drift for
	// the un-checkable IPv6 sub.
	m, r := wiredModule(t, BackendIptables, nil)
	r[BackendIptables].checkResult = &statemgmt.ModuleCheckResult{Matches: true}
	r[BackendIptables].errFor = func(d *statemgmt.Declaration) error {
		if d.Params["family"] == iptables.FamilyIPv6 {
			return iptables.ErrNoIptables
		}
		return nil
	}
	res, err := m.Check(context.Background(), sshIptablesDecl(StatePresent))
	if err != nil {
		t.Fatalf("graceful skip must not error: %v", err)
	}
	if !res.Matches {
		t.Errorf("IPv4 matches + IPv6 skipped → want Matches, got %+v", res)
	}
}

func TestDualStack_HardFailurePropagates(t *testing.T) {
	t.Parallel()
	m, r := wiredModule(t, BackendIptables, nil)
	// An IPv6 failure that is NOT "no ip6tables" (e.g. a rejected rule)
	// must fail the whole apply, not be silently skipped.
	r[BackendIptables].errFor = func(d *statemgmt.Declaration) error {
		if d.Params["family"] == iptables.FamilyIPv6 {
			return errors.New("ip6tables: rule rejected")
		}
		return nil
	}
	_, err := m.Apply(context.Background(), sshIptablesDecl(StatePresent))
	if err == nil {
		t.Fatal("a non-ErrNoIptables IPv6 failure must propagate")
	}
}

func TestDualStack_Absent_RemovesBothFamilies(t *testing.T) {
	t.Parallel()
	m, r := wiredModule(t, BackendIptables, nil)
	if _, err := m.Apply(context.Background(), sshIptablesDecl(StateAbsent)); err != nil {
		t.Fatal(err)
	}
	all := r[BackendIptables].receivedAll
	if len(all) != 2 {
		t.Fatalf("absent should touch both families, got %d", len(all))
	}
	for _, d := range all {
		if d.State != StateAbsent {
			t.Errorf("family %v sub-decl state = %q, want absent", d.Params["family"], d.State)
		}
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

// --- multi-port catalog ----------------------------------------------

func TestParse_MultiPortService(t *testing.T) {
	t.Parallel()
	cases := map[string][]portProto{
		"samba":         {{"137", "udp"}, {"138", "udp"}, {"139", "tcp"}, {"445", "tcp"}},
		"dns":           {{"53", "tcp"}, {"53", "udp"}},
		"kerberos":      {{"88", "tcp"}, {"88", "udp"}},
		"dhcpv6-client": {{"546", "udp"}},
		"cockpit":       {{"9090", "tcp"}},
	}
	for name, want := range cases {
		p, err := parseParams(decl("l", StatePresent, map[string]any{"service": name}))
		if err != nil {
			t.Errorf("service %q: %v", name, err)
			continue
		}
		if !reflect.DeepEqual(p.Ports, want) {
			t.Errorf("service %q ports = %v, want %v", name, p.Ports, want)
		}
	}
}

func TestBuildSubDecls_MultiPort_Iptables(t *testing.T) {
	t.Parallel()
	// samba (4 ports) on iptables → 4 ports × 2 families = 8 subs.
	p, err := parseParams(decl("allow-samba", StatePresent, map[string]any{"service": "samba", "backend": BackendIptables}))
	if err != nil {
		t.Fatal(err)
	}
	subs, err := buildSubDecls(BackendIptables, p, decl("allow-samba", StatePresent, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 8 {
		t.Fatalf("samba on iptables → want 8 subs, got %d", len(subs))
	}
	// Each (port, family) pair appears once with a disambiguating label.
	labels := map[string]bool{}
	for _, s := range subs {
		labels[s.label] = true
		if s.decl.Params["family"] != "ipv4" && s.decl.Params["family"] != "ipv6" {
			t.Errorf("unexpected family in %v", s.decl.Params)
		}
	}
	for _, want := range []string{"ipv4 137/udp", "ipv6 137/udp", "ipv4 445/tcp", "ipv6 445/tcp"} {
		if !labels[want] {
			t.Errorf("missing labelled sub %q (got %v)", want, labels)
		}
	}
}

func TestBuildSubDecls_MultiPort_Firewalld(t *testing.T) {
	t.Parallel()
	// samba on firewalld → 4 subs (one --add-port each), port-labelled.
	p, err := parseParams(decl("allow-samba", StatePresent, map[string]any{"service": "samba", "backend": BackendFirewalld}))
	if err != nil {
		t.Fatal(err)
	}
	subs, err := buildSubDecls(BackendFirewalld, p, decl("allow-samba", StatePresent, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 4 {
		t.Fatalf("samba on firewalld → want 4 subs, got %d", len(subs))
	}
	gotPorts := map[string]bool{}
	for _, s := range subs {
		gotPorts[s.decl.Params["port"].(string)] = true
		if s.label == "" {
			t.Error("multi-port firewalld sub should be labelled")
		}
	}
	for _, want := range []string{"137/udp", "138/udp", "139/tcp", "445/tcp"} {
		if !gotPorts[want] {
			t.Errorf("missing firewalld port sub %q (got %v)", want, gotPorts)
		}
	}
}

func TestBuildSubDecls_SinglePort_FirewalldUnlabelled(t *testing.T) {
	t.Parallel()
	// A single-port firewalld sub carries no label (nothing to
	// disambiguate) — preserves the simple "applied" comment.
	p, err := parseParams(decl("allow-ssh", StatePresent, map[string]any{"service": "ssh", "backend": BackendFirewalld}))
	if err != nil {
		t.Fatal(err)
	}
	subs, err := buildSubDecls(BackendFirewalld, p, decl("allow-ssh", StatePresent, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].label != "" {
		t.Errorf("single-port firewalld → want 1 unlabelled sub, got %d subs label=%q", len(subs), subs[0].label)
	}
}

// --- /etc/services loose lookup ---------------------------------------

const sampleServices = `# a sample /etc/services
ssh             22/tcp
domain          53/tcp     nameserver
domain          53/udp     nameserver
echo            7/ddp                      # unsupported proto, skipped
custom          9999/tcp   myalias
`

func TestLookupServicesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "services")
	if err := os.WriteFile(path, []byte(sampleServices), 0o644); err != nil {
		t.Fatal(err)
	}

	// canonical name, multi-proto → two ports (sorted)
	got, err := lookupServicesFile(path, "domain")
	if err != nil {
		t.Fatal(err)
	}
	want := []portProto{{"53", "tcp"}, {"53", "udp"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("domain = %v, want %v", got, want)
	}

	// alias match
	if got, err := lookupServicesFile(path, "myalias"); err != nil || !reflect.DeepEqual(got, []portProto{{"9999", "tcp"}}) {
		t.Errorf("alias myalias = %v, %v", got, err)
	}

	// unsupported proto (ddp) is skipped → name resolves to nothing → error
	if _, err := lookupServicesFile(path, "echo"); err == nil {
		t.Error("echo (ddp only) should not resolve")
	}

	// absent name errors
	if _, err := lookupServicesFile(path, "nope"); err == nil {
		t.Error("absent service should error")
	}

	// missing file errors
	if _, err := lookupServicesFile(filepath.Join(dir, "nofile"), "ssh"); err == nil {
		t.Error("missing services file should error")
	}
}

func TestParse_LooseService_FallsBackToEtcServices(t *testing.T) {
	// Not parallel: swaps the package-level servicesFilePath. Safe — no
	// parallel test reads it (the strict path never touches the file).
	dir := t.TempDir()
	path := filepath.Join(dir, "services")
	if err := os.WriteFile(path, []byte(sampleServices), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := servicesFilePath
	servicesFilePath = path
	defer func() { servicesFilePath = orig }()

	// strict (default): an off-catalog name errors without consulting
	// /etc/services.
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"service": "custom"})); err == nil {
		t.Error("strict_catalog default → off-catalog name should error")
	}

	// strict_catalog: false → resolves via /etc/services.
	p, err := parseParams(decl("l", StatePresent, map[string]any{"service": "custom", "strict_catalog": false}))
	if err != nil {
		t.Fatalf("loose lookup: %v", err)
	}
	if !reflect.DeepEqual(p.Ports, []portProto{{"9999", "tcp"}}) {
		t.Errorf("custom = %v, want [9999/tcp]", p.Ports)
	}

	// strict_catalog: false but still unknown → error.
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"service": "ghost", "strict_catalog": false})); err == nil {
		t.Error("loose lookup of an absent name should error")
	}

	// strict_catalog wrong type → error.
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"service": "ssh", "strict_catalog": "yes"})); err == nil {
		t.Error("non-bool strict_catalog should error")
	}
}
