package sysctl

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// fakeProvider drives Module tests without touching /proc or sysctl.
type fakeProvider struct {
	runtime    map[string]string // key → value (absence → not exists)
	persisted  map[string]string
	setErr     error
	writeErr   error
	setCalls   []kv
	writeCalls []kv
}

type kv struct{ Key, Value string }

func newFake() *fakeProvider {
	return &fakeProvider{runtime: map[string]string{}, persisted: map[string]string{}}
}

func (f *fakeProvider) Get(key string) (string, bool, error) {
	v, ok := f.runtime[key]
	return v, ok, nil
}
func (f *fakeProvider) Set(_ context.Context, key, value string) error {
	f.setCalls = append(f.setCalls, kv{key, value})
	if f.setErr != nil {
		return f.setErr
	}
	f.runtime[key] = value
	return nil
}
func (f *fakeProvider) ReadPersist(key string) (string, bool, error) {
	v, ok := f.persisted[key]
	return v, ok, nil
}
func (f *fakeProvider) WritePersist(key, value string) error {
	f.writeCalls = append(f.writeCalls, kv{key, value})
	if f.writeErr != nil {
		return f.writeErr
	}
	f.persisted[key] = value
	return nil
}

func declFor(name string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID: "sysctl:" + name, Module: "sysctl", Name: name, State: StatePresent, Params: params,
	}
}

func newModuleWith(p Provider) *Module { return &Module{provider: p} }

// ---- params / validate -------------------------------------------

func TestParseParams_UnknownKey(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("net.ipv4.ip_forward", map[string]any{"valu": "1"}))
	if err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Errorf("err = %v, want unknown-param", err)
	}
}

func TestValidate_RequiresValue(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("net.ipv4.ip_forward", nil))
	if err := p.validate(); err == nil || !strings.Contains(err.Error(), "value is required") {
		t.Errorf("err = %v, want value-required", err)
	}
}

func TestValidate_BadKey(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "net.evil;rm", "net pipe|cat"} {
		p, _ := parseParams(declFor(bad, map[string]any{"value": "1"}))
		if err := p.validate(); err == nil {
			t.Errorf("key %q should be rejected", bad)
		}
	}
}

func TestParseParams_NormalizesSlashKey(t *testing.T) {
	t.Parallel()
	p, err := parseParams(declFor("net/ipv4/ip_forward", map[string]any{"value": "1"}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Key != "net.ipv4.ip_forward" {
		t.Errorf("Key = %q, want dotted form", p.Key)
	}
}

func TestParseParams_CoercesNonStringValue(t *testing.T) {
	t.Parallel()
	p, err := parseParams(declFor("net.ipv4.ip_forward", map[string]any{"value": 1}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Value != "1" {
		t.Errorf("Value = %q, want \"1\"", p.Value)
	}
}

func TestParseParams_PersistDefaultsTrue(t *testing.T) {
	t.Parallel()
	p, _ := parseParams(declFor("net.ipv4.ip_forward", map[string]any{"value": "1"}))
	if !p.Persist {
		t.Error("persist should default to true")
	}
}

func TestParseParams_PersistNotBool(t *testing.T) {
	t.Parallel()
	_, err := parseParams(declFor("net.ipv4.ip_forward", map[string]any{"value": "1", "persist": "yes"}))
	if err == nil {
		t.Error("non-bool persist should be rejected")
	}
}

// ---- Module surface ----------------------------------------------

func TestModule_NameAndStates(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "sysctl" {
		t.Errorf("Name = %q", m.Name())
	}
	if len(m.ValidStates()) != 1 || m.ValidStates()[0] != StatePresent {
		t.Errorf("ValidStates = %v", m.ValidStates())
	}
}

func TestModule_Interfaces(t *testing.T) {
	t.Parallel()
	var _ statemgmt.ValidatableModule = &Module{}
	var _ statemgmt.DriftSeverityModule = &Module{}
}

func TestModule_DriftSeverity(t *testing.T) {
	t.Parallel()
	if (&Module{}).DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("want medium")
	}
}

func TestNew_DefaultProvider(t *testing.T) {
	t.Parallel()
	if New() == nil {
		t.Fatal("nil")
	}
}

// ---- Check -------------------------------------------------------

func TestCheck_KeyNotFound(t *testing.T) {
	t.Parallel()
	m := newModuleWith(newFake())
	_, err := m.Check(context.Background(), declFor("net.ipv4.ip_forward", map[string]any{"value": "1"}))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("err = %v, want ErrKeyNotFound", err)
	}
}

func TestCheck_RuntimeAndPersistMatch(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runtime["net.ipv4.ip_forward"] = "1"
	f.persisted["net.ipv4.ip_forward"] = "1"
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("net.ipv4.ip_forward", map[string]any{"value": "1"}))
	if !res.Matches {
		t.Errorf("should match; diff = %q", res.Diff)
	}
}

func TestCheck_RuntimeMismatch(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runtime["net.ipv4.ip_forward"] = "0"
	f.persisted["net.ipv4.ip_forward"] = "1"
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("net.ipv4.ip_forward", map[string]any{"value": "1"}))
	if res.Matches {
		t.Error("runtime mismatch should drift")
	}
	if !strings.Contains(res.Diff, "runtime") {
		t.Errorf("diff should mention runtime; got %q", res.Diff)
	}
}

func TestCheck_PersistMissing(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runtime["net.ipv4.ip_forward"] = "1"
	// no persist entry
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("net.ipv4.ip_forward", map[string]any{"value": "1"}))
	if res.Matches {
		t.Error("missing persist should drift")
	}
	if !strings.Contains(res.Diff, "persist file missing") {
		t.Errorf("diff should mention persist; got %q", res.Diff)
	}
}

func TestCheck_PersistMismatch(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runtime["net.ipv4.ip_forward"] = "1"
	f.persisted["net.ipv4.ip_forward"] = "0"
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("net.ipv4.ip_forward", map[string]any{"value": "1"}))
	if res.Matches {
		t.Error("persist mismatch should drift")
	}
}

func TestCheck_PersistFalse_IgnoresFile(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runtime["net.ipv4.ip_forward"] = "1"
	// persist:false → don't look at the file even though it's missing
	m := newModuleWith(f)
	res, _ := m.Check(context.Background(), declFor("net.ipv4.ip_forward", map[string]any{"value": "1", "persist": false}))
	if !res.Matches {
		t.Errorf("persist:false should match on runtime alone; diff = %q", res.Diff)
	}
}

func TestCheck_MultiFieldValueNormalized(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runtime["net.ipv4.tcp_rmem"] = "4096 16384 4194304"
	m := newModuleWith(f)
	// Declared with extra spaces — should still match.
	res, _ := m.Check(context.Background(), declFor("net.ipv4.tcp_rmem", map[string]any{"value": "4096   16384   4194304", "persist": false}))
	if !res.Matches {
		t.Errorf("whitespace-normalized multi-field should match; diff = %q", res.Diff)
	}
}

// ---- Apply -------------------------------------------------------

func TestApply_SetsRuntimeAndPersist(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runtime["net.ipv4.ip_forward"] = "0"
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("net.ipv4.ip_forward", map[string]any{"value": "1"}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Changed {
		t.Error("Changed should be true")
	}
	if len(f.setCalls) != 1 || f.setCalls[0] != (kv{"net.ipv4.ip_forward", "1"}) {
		t.Errorf("Set calls = %v", f.setCalls)
	}
	if len(f.writeCalls) != 1 {
		t.Errorf("WritePersist calls = %d, want 1", len(f.writeCalls))
	}
}

func TestApply_PersistFalse_SkipsWrite(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runtime["net.ipv4.ip_forward"] = "0"
	m := newModuleWith(f)
	if _, err := m.Apply(context.Background(), declFor("net.ipv4.ip_forward", map[string]any{"value": "1", "persist": false})); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(f.writeCalls) != 0 {
		t.Errorf("WritePersist should not fire with persist:false; got %v", f.writeCalls)
	}
}

func TestApply_AlreadyConverged_NoCalls(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runtime["net.ipv4.ip_forward"] = "1"
	f.persisted["net.ipv4.ip_forward"] = "1"
	m := newModuleWith(f)
	res, _ := m.Apply(context.Background(), declFor("net.ipv4.ip_forward", map[string]any{"value": "1"}))
	if res.Changed {
		t.Error("converged should be Changed=false")
	}
	if len(f.setCalls)+len(f.writeCalls) != 0 {
		t.Error("no provider calls on converged")
	}
}

func TestApply_SetError_Propagates(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runtime["net.ipv4.ip_forward"] = "0"
	f.setErr = errors.New("sysctl: permission denied")
	m := newModuleWith(f)
	res, err := m.Apply(context.Background(), declFor("net.ipv4.ip_forward", map[string]any{"value": "1"}))
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("err = %v, want provider error", err)
	}
	if res.Success {
		t.Error("Success should be false")
	}
}

func TestApply_WritePersistError_Propagates(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runtime["net.ipv4.ip_forward"] = "0"
	f.writeErr = errors.New("write /etc/sysctl.d: read-only fs")
	m := newModuleWith(f)
	_, err := m.Apply(context.Background(), declFor("net.ipv4.ip_forward", map[string]any{"value": "1"}))
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("err = %v, want write error", err)
	}
}

// ---- end-to-end --------------------------------------------------

func TestModule_EndToEnd(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runtime["net.ipv4.ip_forward"] = "0"
	m := newModuleWith(f)
	decl := declFor("net.ipv4.ip_forward", map[string]any{"value": "1"})
	if _, err := m.Apply(context.Background(), decl); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	ok, _ := m.Test(context.Background(), decl)
	if !ok {
		t.Error("Test should match after Apply")
	}
	res, _ := m.Apply(context.Background(), decl)
	if res.Changed {
		t.Error("re-Apply should be Changed=false")
	}
}

func TestSentinels(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) || !IsKeyNotFound(ErrKeyNotFound) {
		t.Error("sentinel mismatch")
	}
	if IsKeyNotFound(errors.New("x")) {
		t.Error("unrelated error matched")
	}
}
