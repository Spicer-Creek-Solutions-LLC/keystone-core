// SPDX-License-Identifier: Apache-2.0

package langpkg

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "langpkg:" + name,
		Module: "langpkg",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider -----------------------------------------------------

type fakeProvider struct {
	pip map[string]string
	npm map[string]string
	gem map[string]string

	hasErr, installErr, uninstallErr error

	installCalls   []installCall
	uninstallCalls []uninstallCall
}

type installCall struct {
	Manager string
	Name    string
	Version string
}
type uninstallCall struct {
	Manager string
	Name    string
}

func newFake() *fakeProvider {
	return &fakeProvider{
		pip: map[string]string{},
		npm: map[string]string{},
		gem: map[string]string{},
	}
}

func (f *fakeProvider) HasPipPackage(_ context.Context, name string) (bool, string, error) {
	if f.hasErr != nil {
		return false, "", f.hasErr
	}
	v, ok := f.pip[name]
	return ok, v, nil
}
func (f *fakeProvider) InstallPipPackage(_ context.Context, name, version string) error {
	if f.installErr != nil {
		return f.installErr
	}
	f.installCalls = append(f.installCalls, installCall{Manager: "pip", Name: name, Version: version})
	v := version
	if v == "" {
		v = "latest"
	}
	f.pip[name] = v
	return nil
}
func (f *fakeProvider) UninstallPipPackage(_ context.Context, name string) error {
	if f.uninstallErr != nil {
		return f.uninstallErr
	}
	f.uninstallCalls = append(f.uninstallCalls, uninstallCall{Manager: "pip", Name: name})
	delete(f.pip, name)
	return nil
}
func (f *fakeProvider) HasNpmPackage(_ context.Context, name string) (bool, string, error) {
	if f.hasErr != nil {
		return false, "", f.hasErr
	}
	v, ok := f.npm[name]
	return ok, v, nil
}
func (f *fakeProvider) InstallNpmPackage(_ context.Context, name, version string) error {
	if f.installErr != nil {
		return f.installErr
	}
	f.installCalls = append(f.installCalls, installCall{Manager: "npm", Name: name, Version: version})
	v := version
	if v == "" {
		v = "latest"
	}
	f.npm[name] = v
	return nil
}
func (f *fakeProvider) UninstallNpmPackage(_ context.Context, name string) error {
	if f.uninstallErr != nil {
		return f.uninstallErr
	}
	f.uninstallCalls = append(f.uninstallCalls, uninstallCall{Manager: "npm", Name: name})
	delete(f.npm, name)
	return nil
}
func (f *fakeProvider) HasGemPackage(_ context.Context, name string) (bool, string, error) {
	if f.hasErr != nil {
		return false, "", f.hasErr
	}
	v, ok := f.gem[name]
	return ok, v, nil
}
func (f *fakeProvider) InstallGemPackage(_ context.Context, name, version string) error {
	if f.installErr != nil {
		return f.installErr
	}
	f.installCalls = append(f.installCalls, installCall{Manager: "gem", Name: name, Version: version})
	v := version
	if v == "" {
		v = "latest"
	}
	f.gem[name] = v
	return nil
}
func (f *fakeProvider) UninstallGemPackage(_ context.Context, name string) error {
	if f.uninstallErr != nil {
		return f.uninstallErr
	}
	f.uninstallCalls = append(f.uninstallCalls, uninstallCall{Manager: "gem", Name: name})
	delete(f.gem, name)
	return nil
}

// --- params / validate -----------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"names": "x"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_Defaults(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("l", StatePresent, map[string]any{"name": "requests", "manager": "pip"}))
	if err != nil || p.Name != "requests" || p.Manager != "pip" || p.Version != "" {
		t.Errorf("defaults: %+v %v", p, err)
	}
	// manager normalised (lower-cased + trimmed)
	p, err = parseParams(decl("l", StatePresent, map[string]any{"name": "x", "manager": "  PIP  "}))
	if err != nil || p.Manager != "pip" {
		t.Errorf("normalise: %+v %v", p, err)
	}
	// type errors
	for _, bad := range []map[string]any{
		{"name": 1, "manager": "pip"},
		{"name": "x", "manager": 1},
		{"name": "x", "manager": "pip", "version": 1},
	} {
		if _, err := parseParams(decl("l", StatePresent, bad)); err == nil {
			t.Errorf("parseParams(%v) should error", bad)
		}
	}
}

func TestKnownManagers_Sorted(t *testing.T) {
	t.Parallel()
	got := KnownManagers()
	if len(got) != len(validManagers) {
		t.Fatalf("len mismatch: %d vs %d", len(got), len(validManagers))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("not sorted: %q >= %q", got[i-1], got[i])
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
		{"pip present ok", decl("l", StatePresent, map[string]any{"name": "requests", "manager": "pip"}), false},
		{"npm present ok", decl("l", StatePresent, map[string]any{"name": "pm2", "manager": "npm"}), false},
		{"gem present ok", decl("l", StatePresent, map[string]any{"name": "bundler", "manager": "gem"}), false},
		{"npm scoped ok", decl("l", StatePresent, map[string]any{"name": "@types/node", "manager": "npm"}), false},
		{"version pin ok", decl("l", StatePresent, map[string]any{"name": "requests", "manager": "pip", "version": "2.31.0"}), false},
		{"version with pre-release", decl("l", StatePresent, map[string]any{"name": "x", "manager": "pip", "version": "1.0.0-rc1"}), false},
		{"name required", decl("l", StatePresent, map[string]any{"manager": "pip"}), true},
		{"manager required", decl("l", StatePresent, map[string]any{"name": "x"}), true},
		{"unknown manager", decl("l", StatePresent, map[string]any{"name": "x", "manager": "cargo"}), true},
		{"bad name charset", decl("l", StatePresent, map[string]any{"name": "ev il", "manager": "pip"}), true},
		{"bad name leading dash", decl("l", StatePresent, map[string]any{"name": "-x", "manager": "pip"}), true},
		{"bad version charset", decl("l", StatePresent, map[string]any{"name": "x", "manager": "pip", "version": "1.0 0"}), true},
		{"absent rejects version", decl("l", StateAbsent, map[string]any{"name": "x", "manager": "pip", "version": "1.0"}), true},
		{"absent ok", decl("l", StateAbsent, map[string]any{"name": "x", "manager": "pip"}), false},
		{"bad state", decl("l", "frob", map[string]any{"name": "x", "manager": "pip"}), true},
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

func TestDisplayVer(t *testing.T) {
	t.Parallel()
	if displayVer("") != "<none>" || displayVer("1.2.3") != "1.2.3" {
		t.Error("displayVer mapping")
	}
}

// --- present op -------------------------------------------------------

func TestPresent_Installs_Pip(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("r", StatePresent, map[string]any{"name": "requests", "manager": "pip"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("absent → drift: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.installCalls) != 1 || f.installCalls[0].Manager != "pip" || f.installCalls[0].Name != "requests" || f.installCalls[0].Version != "" {
		t.Errorf("install call: %+v", f.installCalls)
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

func TestPresent_VersionPin(t *testing.T) {
	t.Parallel()
	// already installed at a different version → drift, re-install
	f := newFake()
	f.pip["requests"] = "2.30.0"
	m := NewWithProvider(f)
	d := decl("r", StatePresent, map[string]any{"name": "requests", "manager": "pip", "version": "2.31.0"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("version mismatch should drift: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if f.installCalls[0].Version != "2.31.0" {
		t.Errorf("install pin: %+v", f.installCalls)
	}
	// exact match → no-op
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("exact match should converge: %+v", r)
	}
}

func TestPresent_NoVersionAnyOK(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.npm["pm2"] = "5.0.0"
	m := NewWithProvider(f)
	d := decl("p", StatePresent, map[string]any{"name": "pm2", "manager": "npm"})
	r, _ := m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("any version satisfies when no version: pin: %+v", r)
	}
	sr, _ := m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("no-op when installed and no pin")
	}
}

func TestPresent_DispatchesByManager(t *testing.T) {
	t.Parallel()
	// npm
	f := newFake()
	m := NewWithProvider(f)
	if _, err := m.Apply(context.Background(), decl("a", StatePresent, map[string]any{"name": "pm2", "manager": "npm", "version": "5.0.0"})); err != nil {
		t.Fatal(err)
	}
	if f.installCalls[0].Manager != "npm" || f.installCalls[0].Version != "5.0.0" {
		t.Errorf("npm install: %+v", f.installCalls)
	}
	// gem
	f = newFake()
	m = NewWithProvider(f)
	if _, err := m.Apply(context.Background(), decl("a", StatePresent, map[string]any{"name": "bundler", "manager": "gem"})); err != nil {
		t.Fatal(err)
	}
	if f.installCalls[0].Manager != "gem" || f.installCalls[0].Name != "bundler" {
		t.Errorf("gem install: %+v", f.installCalls)
	}
}

// --- absent op --------------------------------------------------------

func TestAbsent(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.gem["bundler"] = "2.4.0"
	m := NewWithProvider(f)
	d := decl("b", StateAbsent, map[string]any{"name": "bundler", "manager": "gem"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("present → drift: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.uninstallCalls) != 1 || f.uninstallCalls[0].Manager != "gem" {
		t.Errorf("uninstall: %+v", f.uninstallCalls)
	}
	// already absent → no-op
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged check: %+v", r)
	}
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Errorf("converged apply: %+v", sr)
	}
}

// --- errors -----------------------------------------------------------

func TestErrorsPropagate(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{hasErr: errors.New("has")})
	if _, err := m.Check(context.Background(), decl("l", StatePresent, map[string]any{"name": "x", "manager": "pip"})); err == nil {
		t.Error("Has error should propagate")
	}
	// install error
	f := newFake()
	f.installErr = errors.New("install")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StatePresent, map[string]any{"name": "x", "manager": "pip"})); err == nil {
		t.Error("install error should propagate")
	}
	// uninstall error
	f = newFake()
	f.pip["x"] = "1.0"
	f.uninstallErr = errors.New("uninstall")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StateAbsent, map[string]any{"name": "x", "manager": "pip"})); err == nil {
		t.Error("uninstall error should propagate")
	}
}

func TestParseError_FromCheckAndApply(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(newFake())
	bad := decl("l", StatePresent, map[string]any{}) // no name/manager
	if _, err := m.Check(context.Background(), bad); err == nil {
		t.Error("Check should reject an invalid declaration")
	}
	if _, err := m.Apply(context.Background(), bad); err == nil {
		t.Error("Apply should reject an invalid declaration")
	}
}

// --- module surface ---------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "langpkg" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("langpkg should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	good := decl("l", StatePresent, map[string]any{"name": "x", "manager": "pip"})
	if dsm.DriftSeverity(good, nil) != statemgmt.DriftSeverityHigh {
		t.Error("any decl → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(good); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("l", StatePresent, map[string]any{})); err == nil {
		t.Error("missing name/manager should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("l", StatePresent, map[string]any{"name": "x", "manager": "pip"})
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
	if !IsNoPip(ErrNoPip) || IsNoPip(errors.New("x")) {
		t.Error("IsNoPip")
	}
	if !IsNoNpm(ErrNoNpm) || IsNoNpm(errors.New("x")) {
		t.Error("IsNoNpm")
	}
	if !IsNoGem(ErrNoGem) || IsNoGem(errors.New("x")) {
		t.Error("IsNoGem")
	}
}
