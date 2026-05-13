package lvm

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "lvm:" + name,
		Module: "lvm",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider ----------------------------------------------------

type fakeProvider struct {
	pvs    map[string]bool     // device → present
	vgs    map[string]bool     // name → present
	lvs    map[string]bool     // "vg/lv" → present
	pvSets map[string][]string // VG name → PV set (recorded by Create)

	hasErr, createErr, removeErr error

	createCalls []createCall
	removeCalls []removeCall
}

type createCall struct {
	Op      string // "pv" / "vg" / "lv"
	Device  string
	VGName  string
	VGPVs   []string
	LVName  string
	Size    string
	Extents string
}

type removeCall struct {
	Op     string
	Device string
	VGName string
	LVName string
}

func newFake() *fakeProvider {
	return &fakeProvider{
		pvs:    map[string]bool{},
		vgs:    map[string]bool{},
		lvs:    map[string]bool{},
		pvSets: map[string][]string{},
	}
}

func (f *fakeProvider) HasPV(_ context.Context, device string) (bool, error) {
	if f.hasErr != nil {
		return false, f.hasErr
	}
	return f.pvs[device], nil
}
func (f *fakeProvider) CreatePV(_ context.Context, device string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createCalls = append(f.createCalls, createCall{Op: "pv", Device: device})
	f.pvs[device] = true
	return nil
}
func (f *fakeProvider) RemovePV(_ context.Context, device string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removeCalls = append(f.removeCalls, removeCall{Op: "pv", Device: device})
	delete(f.pvs, device)
	return nil
}
func (f *fakeProvider) HasVG(_ context.Context, name string) (bool, error) {
	if f.hasErr != nil {
		return false, f.hasErr
	}
	return f.vgs[name], nil
}
func (f *fakeProvider) CreateVG(_ context.Context, name string, pvs []string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createCalls = append(f.createCalls, createCall{Op: "vg", VGName: name, VGPVs: append([]string(nil), pvs...)})
	f.vgs[name] = true
	f.pvSets[name] = append([]string(nil), pvs...)
	return nil
}
func (f *fakeProvider) RemoveVG(_ context.Context, name string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removeCalls = append(f.removeCalls, removeCall{Op: "vg", VGName: name})
	delete(f.vgs, name)
	delete(f.pvSets, name)
	return nil
}
func (f *fakeProvider) HasLV(_ context.Context, vg, lv string) (bool, error) {
	if f.hasErr != nil {
		return false, f.hasErr
	}
	return f.lvs[vg+"/"+lv], nil
}
func (f *fakeProvider) CreateLV(_ context.Context, vg, lv, size, extents string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createCalls = append(f.createCalls, createCall{Op: "lv", VGName: vg, LVName: lv, Size: size, Extents: extents})
	f.lvs[vg+"/"+lv] = true
	return nil
}
func (f *fakeProvider) RemoveLV(_ context.Context, vg, lv string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removeCalls = append(f.removeCalls, removeCall{Op: "lv", VGName: vg, LVName: lv})
	delete(f.lvs, vg+"/"+lv)
	return nil
}

// --- params / validate -----------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"pvs2": []any{"/dev/sda"}})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_OpSelection(t *testing.T) {
	t.Parallel()
	// none
	if _, err := parseParams(decl("l", StatePresent, map[string]any{})); err == nil {
		t.Error("none should error")
	}
	// pv only
	p, err := parseParams(decl("l", StatePresent, map[string]any{"pv": "/dev/sdb1"}))
	if err != nil || p.Op != OpPV || p.Device != "/dev/sdb1" {
		t.Errorf("pv: %+v %v", p, err)
	}
	// vg only
	p, err = parseParams(decl("l", StatePresent, map[string]any{"vg": "myvg", "pvs": []any{"/dev/sdb1"}}))
	if err != nil || p.Op != OpVG || p.VGName != "myvg" || !reflect.DeepEqual(p.VGPVs, []string{"/dev/sdb1"}) {
		t.Errorf("vg: %+v %v", p, err)
	}
	// lv (requires vg)
	p, err = parseParams(decl("l", StatePresent, map[string]any{"lv": "home", "vg": "myvg", "size": "10G"}))
	if err != nil || p.Op != OpLV || p.LVName != "home" || p.VGName != "myvg" || p.Size != "10G" {
		t.Errorf("lv size: %+v %v", p, err)
	}
	// lv with extents
	p, err = parseParams(decl("l", StatePresent, map[string]any{"lv": "swap", "vg": "vg0", "extents": "100%FREE"}))
	if err != nil || p.Op != OpLV || p.Extents != "100%FREE" {
		t.Errorf("lv extents: %+v %v", p, err)
	}
	// invalid combos
	for _, bad := range []map[string]any{
		{"pv": "/dev/sdb1", "vg": "x"},
		{"pv": "/dev/sdb1", "lv": "h", "vg": "x"},
		{"lv": "h"}, // no vg
		{"vg": "x", "pvs": "not-a-list"},
		{"vg": "x", "size": "10G"},
		{"pv": "/dev/sdb1", "size": "10G"},
		{"lv": "h", "vg": "x", "pvs": []any{"/dev/sda"}},
	} {
		if _, err := parseParams(decl("l", StatePresent, bad)); err == nil {
			t.Errorf("parseParams(%v) should error", bad)
		}
	}
	// type errors
	for _, bad := range []map[string]any{
		{"pv": 1},
		{"vg": 1},
		{"lv": 1, "vg": "x", "size": "10G"},
		{"lv": "h", "vg": 1, "size": "10G"},
		{"lv": "h", "vg": "x", "size": 1},
		{"lv": "h", "vg": "x", "extents": 1},
		{"vg": "x", "pvs": []any{1}},
		{"vg": "x", "pvs": []any{""}},
	} {
		if _, err := parseParams(decl("l", StatePresent, bad)); err == nil {
			t.Errorf("type error parse(%v) should fail", bad)
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
		{"pv present ok", decl("l", StatePresent, map[string]any{"pv": "/dev/sdb1"}), false},
		{"pv absent ok", decl("l", StateAbsent, map[string]any{"pv": "/dev/sdb1"}), false},
		{"pv bad path", decl("l", StatePresent, map[string]any{"pv": "sdb1"}), true},
		{"pv unsafe chars", decl("l", StatePresent, map[string]any{"pv": "/dev/sda;rm"}), true},
		{"vg present ok", decl("l", StatePresent, map[string]any{"vg": "myvg", "pvs": []any{"/dev/sdb1"}}), false},
		{"vg absent without pvs ok", decl("l", StateAbsent, map[string]any{"vg": "myvg"}), false},
		{"vg present needs pvs", decl("l", StatePresent, map[string]any{"vg": "myvg"}), true},
		{"vg bad name", decl("l", StatePresent, map[string]any{"vg": "my vg", "pvs": []any{"/dev/sda"}}), true},
		{"vg bad pv path", decl("l", StatePresent, map[string]any{"vg": "myvg", "pvs": []any{"sda1"}}), true},
		{"lv size ok", decl("l", StatePresent, map[string]any{"lv": "home", "vg": "myvg", "size": "10G"}), false},
		{"lv size lowercase ok", decl("l", StatePresent, map[string]any{"lv": "home", "vg": "myvg", "size": "10g"}), false},
		{"lv size no unit ok", decl("l", StatePresent, map[string]any{"lv": "home", "vg": "myvg", "size": "1024"}), false},
		{"lv extents FREE ok", decl("l", StatePresent, map[string]any{"lv": "x", "vg": "y", "extents": "50%FREE"}), false},
		{"lv extents VG ok", decl("l", StatePresent, map[string]any{"lv": "x", "vg": "y", "extents": "100%VG"}), false},
		{"lv size+extents conflict", decl("l", StatePresent, map[string]any{"lv": "x", "vg": "y", "size": "10G", "extents": "50%FREE"}), true},
		{"lv no size or extents on present", decl("l", StatePresent, map[string]any{"lv": "x", "vg": "y"}), true},
		{"lv absent no size needed", decl("l", StateAbsent, map[string]any{"lv": "x", "vg": "y"}), false},
		{"lv bad size", decl("l", StatePresent, map[string]any{"lv": "x", "vg": "y", "size": "1.5G"}), true},
		{"lv bad extents", decl("l", StatePresent, map[string]any{"lv": "x", "vg": "y", "extents": "50%BOGUS"}), true},
		{"lv bad name", decl("l", StatePresent, map[string]any{"lv": "ho me", "vg": "y", "size": "10G"}), true},
		{"lv bad vg", decl("l", StatePresent, map[string]any{"lv": "x", "vg": "my vg", "size": "10G"}), true},
		{"bad state", decl("l", "frob", map[string]any{"pv": "/dev/sdb1"}), true},
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

func TestOpString(t *testing.T) {
	t.Parallel()
	if OpPV.String() != "pv" || OpVG.String() != "vg" || OpLV.String() != "lv" || OpUnknown.String() != "unknown" {
		t.Error("Op.String mapping wrong")
	}
}

func TestLocator(t *testing.T) {
	t.Parallel()
	if (&params{Op: OpPV, Device: "/dev/sdb1"}).locator() != "PV /dev/sdb1" {
		t.Error("PV locator")
	}
	if (&params{Op: OpVG, VGName: "myvg"}).locator() != "VG myvg" {
		t.Error("VG locator")
	}
	if (&params{Op: OpLV, VGName: "myvg", LVName: "home"}).locator() != "LV myvg/home" {
		t.Error("LV locator")
	}
	if (&params{Op: OpUnknown}).locator() != "<unknown>" {
		t.Error("unknown locator")
	}
}

// --- PV op ------------------------------------------------------------

func TestPV_PresentDrift(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("p", StatePresent, map[string]any{"pv": "/dev/sdb1"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("absent → should drift: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.createCalls) != 1 || f.createCalls[0].Op != "pv" || f.createCalls[0].Device != "/dev/sdb1" {
		t.Errorf("create call: %+v", f.createCalls)
	}
	// converged
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged check: %+v", r)
	}
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Errorf("converged apply should be no-op: %+v", sr)
	}
}

func TestPV_Absent(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.pvs["/dev/sdb1"] = true
	m := NewWithProvider(f)
	d := decl("p", StateAbsent, map[string]any{"pv": "/dev/sdb1"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("present → should drift: %+v", r)
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.removeCalls) != 1 || f.removeCalls[0].Op != "pv" {
		t.Errorf("remove: %+v", f.removeCalls)
	}
}

// --- VG op ------------------------------------------------------------

func TestVG_Present(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("v", StatePresent, map[string]any{"vg": "myvg", "pvs": []any{"/dev/sdb1", "/dev/sdc1"}})
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	c := f.createCalls[0]
	if c.Op != "vg" || c.VGName != "myvg" || !reflect.DeepEqual(c.VGPVs, []string{"/dev/sdb1", "/dev/sdc1"}) {
		t.Errorf("create: %+v", c)
	}
	// converged
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.createCalls) != 1 {
		t.Errorf("converged should not create again: %d calls", len(f.createCalls))
	}
}

func TestVG_AbsentIgnoresPVSet(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.vgs["myvg"] = true
	m := NewWithProvider(f)
	// pvs ignored for absent (not validated)
	d := decl("v", StateAbsent, map[string]any{"vg": "myvg"})
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.removeCalls) != 1 || f.removeCalls[0].VGName != "myvg" {
		t.Errorf("remove: %+v", f.removeCalls)
	}
}

// --- LV op ------------------------------------------------------------

func TestLV_PresentSize(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("h", StatePresent, map[string]any{"lv": "home", "vg": "myvg", "size": "10G"})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	c := f.createCalls[0]
	if c.Op != "lv" || c.VGName != "myvg" || c.LVName != "home" || c.Size != "10G" || c.Extents != "" {
		t.Errorf("create: %+v", c)
	}
}

func TestLV_PresentExtents(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("h", StatePresent, map[string]any{"lv": "data", "vg": "myvg", "extents": "100%FREE"})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	c := f.createCalls[0]
	if c.Size != "" || c.Extents != "100%FREE" {
		t.Errorf("create: %+v", c)
	}
}

func TestLV_Absent(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.lvs["myvg/home"] = true
	m := NewWithProvider(f)
	d := decl("h", StateAbsent, map[string]any{"lv": "home", "vg": "myvg"})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.removeCalls) != 1 || f.removeCalls[0].VGName != "myvg" || f.removeCalls[0].LVName != "home" {
		t.Errorf("remove: %+v", f.removeCalls)
	}
}

// --- error propagation -----------------------------------------------

func TestErrorsPropagate(t *testing.T) {
	t.Parallel()
	mk := func(err error, in *fakeProvider) statemgmt.Module {
		if in == nil {
			in = newFake()
		}
		in.hasErr = err
		return NewWithProvider(in)
	}
	for _, op := range []map[string]any{
		{"pv": "/dev/sda"},
		{"vg": "x", "pvs": []any{"/dev/sda"}},
		{"lv": "h", "vg": "x", "size": "10G"},
	} {
		if _, err := mk(errors.New("has"), nil).Check(context.Background(), decl("l", StatePresent, op)); err == nil {
			t.Errorf("Has error should propagate for %v", op)
		}
	}
	// Create error
	f := newFake()
	f.createErr = errors.New("create")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StatePresent, map[string]any{"pv": "/dev/sda"})); err == nil {
		t.Error("Create error should propagate")
	}
	// Remove error
	f = newFake()
	f.pvs["/dev/sda"] = true
	f.removeErr = errors.New("remove")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StateAbsent, map[string]any{"pv": "/dev/sda"})); err == nil {
		t.Error("Remove error should propagate")
	}
}

func TestParseError_FromCheckAndApply(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(newFake())
	bad := decl("l", StatePresent, map[string]any{}) // no op
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
	if m.Name() != "lvm" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("lvm should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("l", StatePresent, map[string]any{"pv": "/dev/sda"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("any decl → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("l", StatePresent, map[string]any{"pv": "/dev/sda"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("l", StatePresent, map[string]any{})); err == nil {
		t.Error("missing op should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	f := newFake()
	m := NewWithProvider(f)
	d := decl("l", StatePresent, map[string]any{"pv": "/dev/sda"})
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
	if !IsNoLVM(ErrNoLVM) || IsNoLVM(errors.New("x")) {
		t.Error("IsNoLVM")
	}
}
