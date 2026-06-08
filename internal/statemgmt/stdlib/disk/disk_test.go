// SPDX-License-Identifier: Apache-2.0

package disk

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "disk:" + name,
		Module: "disk",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider ----------------------------------------------------

type fakeProvider struct {
	currentFS  string
	getErr     error
	mkfsErr    error
	wipeErr    error
	fills      bool
	fillsErr   error
	resizeErr  error
	mkfsCalls  []mkfsCall
	wipeCalls  []string
	resizeCall int
}

type mkfsCall struct {
	Device  string
	Fstype  string
	Options []string
}

func (f *fakeProvider) GetFilesystem(_ context.Context, _ string) (string, error) {
	return f.currentFS, f.getErr
}
func (f *fakeProvider) MakeFilesystem(_ context.Context, device, fstype string, opts []string) error {
	if f.mkfsErr != nil {
		return f.mkfsErr
	}
	f.mkfsCalls = append(f.mkfsCalls, mkfsCall{Device: device, Fstype: fstype, Options: append([]string(nil), opts...)})
	f.currentFS = fstype
	return nil
}
func (f *fakeProvider) WipeFilesystem(_ context.Context, device string) error {
	if f.wipeErr != nil {
		return f.wipeErr
	}
	f.wipeCalls = append(f.wipeCalls, device)
	f.currentFS = ""
	return nil
}
func (f *fakeProvider) FilesystemFillsDevice(_ context.Context, _, _ string) (bool, error) {
	return f.fills, f.fillsErr
}
func (f *fakeProvider) ResizeFilesystem(_ context.Context, _, _ string) error {
	if f.resizeErr != nil {
		return f.resizeErr
	}
	f.resizeCall++
	f.fills = true // after a resize, the fs fills the device
	return nil
}

// --- params / validate -----------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"devices": "/dev/sdb1"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_TypesAndDefaults(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("l", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "ext4"}))
	if err != nil || p.Device != "/dev/sdb1" || p.Fstype != "ext4" || p.Force || len(p.MkfsOptions) != 0 {
		t.Errorf("defaults: %+v %v", p, err)
	}
	// fstype lower-cased + trimmed
	p, err = parseParams(decl("l", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "  EXT4  "}))
	if err != nil || p.Fstype != "ext4" {
		t.Errorf("normalisation: %+v %v", p, err)
	}
	// mkfs_options + force
	p, err = parseParams(decl("l", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "ext4", "mkfs_options": []any{"-F", "-L", "data"}, "force": true}))
	if err != nil || !p.Force || !reflect.DeepEqual(p.MkfsOptions, []string{"-F", "-L", "data"}) {
		t.Errorf("opts: %+v %v", p, err)
	}
	// type errors
	for _, bad := range []map[string]any{
		{"device": 1, "fstype": "ext4"},
		{"device": "/dev/sdb1", "fstype": 1},
		{"device": "/dev/sdb1", "fstype": "ext4", "force": "yes"},
		{"device": "/dev/sdb1", "fstype": "ext4", "mkfs_options": "not-a-list"},
		{"device": "/dev/sdb1", "fstype": "ext4", "mkfs_options": []any{1}},
		{"device": "/dev/sdb1", "fstype": "ext4", "mkfs_options": []any{""}},
	} {
		if _, err := parseParams(decl("l", StatePresent, bad)); err == nil {
			t.Errorf("parseParams(%v) should error", bad)
		}
	}
}

func TestKnownFstypes_SortedAndComplete(t *testing.T) {
	t.Parallel()
	names := KnownFstypes()
	if len(names) != len(validFstypes) {
		t.Fatalf("len mismatch: %d vs %d", len(names), len(validFstypes))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Errorf("not sorted: %q >= %q", names[i-1], names[i])
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
		{"present ext4 ok", decl("l", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "ext4"}), false},
		{"present xfs ok", decl("l", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "xfs"}), false},
		{"present swap ok", decl("l", StatePresent, map[string]any{"device": "/dev/sdb2", "fstype": "swap"}), false},
		{"present needs fstype", decl("l", StatePresent, map[string]any{"device": "/dev/sdb1"}), true},
		{"unknown fstype", decl("l", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "ntfs"}), true},
		{"empty device", decl("l", StatePresent, map[string]any{"device": "", "fstype": "ext4"}), true},
		{"bad device path", decl("l", StatePresent, map[string]any{"device": "sdb1", "fstype": "ext4"}), true},
		{"unsafe device chars", decl("l", StatePresent, map[string]any{"device": "/dev/sda;rm", "fstype": "ext4"}), true},
		{"mkfs_options ok", decl("l", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "ext4", "mkfs_options": []any{"-F", "-L", "mylabel", "-N", "1000"}}), false},
		{"mkfs_options unsafe space", decl("l", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "ext4", "mkfs_options": []any{"-L my label"}}), true},
		{"mkfs_options shell metachar", decl("l", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "ext4", "mkfs_options": []any{"-L;rm"}}), true},
		{"absent ok", decl("l", StateAbsent, map[string]any{"device": "/dev/sdb1", "force": true}), false},
		{"absent rejects mkfs_options", decl("l", StateAbsent, map[string]any{"device": "/dev/sdb1", "mkfs_options": []any{"-F"}}), true},
		{"bad state", decl("l", "frob", map[string]any{"device": "/dev/sdb1", "fstype": "ext4"}), true},
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

func TestDisplayFS(t *testing.T) {
	t.Parallel()
	if displayFS("") != "<none>" || displayFS("ext4") != "ext4" {
		t.Error("displayFS mapping")
	}
}

// --- present op ------------------------------------------------------

func TestPresent_CreateOnEmpty(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{currentFS: ""}
	m := NewWithProvider(f)
	d := decl("d", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "ext4"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("empty → drift: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.mkfsCalls) != 1 || f.mkfsCalls[0].Device != "/dev/sdb1" || f.mkfsCalls[0].Fstype != "ext4" {
		t.Errorf("mkfs: %+v", f.mkfsCalls)
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

func TestPresent_MismatchRequiresForce(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{currentFS: "xfs"}
	m := NewWithProvider(f)
	// without force → error
	d := decl("d", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "ext4"})
	if _, err := m.Apply(context.Background(), d); err == nil {
		t.Error("mismatch without force should error")
	}
	if len(f.mkfsCalls) != 0 {
		t.Error("mkfs should not have been called")
	}
	// with force → reformats
	f = &fakeProvider{currentFS: "xfs"}
	m = NewWithProvider(f)
	d = decl("d", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "ext4", "force": true})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if len(f.mkfsCalls) != 1 || f.mkfsCalls[0].Fstype != "ext4" {
		t.Errorf("force should reformat: %+v", f.mkfsCalls)
	}
}

func TestPresent_OptionsPassedThrough(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{}
	m := NewWithProvider(f)
	d := decl("d", StatePresent, map[string]any{
		"device": "/dev/sdb1", "fstype": "ext4",
		"mkfs_options": []any{"-F", "-L", "mylabel", "-N", "1000"},
	})
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	want := []string{"-F", "-L", "mylabel", "-N", "1000"}
	if !reflect.DeepEqual(f.mkfsCalls[0].Options, want) {
		t.Errorf("options: %+v", f.mkfsCalls[0].Options)
	}
}

// --- absent op -------------------------------------------------------

func TestAbsent_EmptyNoOp(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{currentFS: ""}
	m := NewWithProvider(f)
	d := decl("d", StateAbsent, map[string]any{"device": "/dev/sdb1", "force": true})
	r, _ := m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("empty + absent should match: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || sr.Changed {
		t.Errorf("no-op: %+v %v", sr, err)
	}
	if len(f.wipeCalls) != 0 {
		t.Error("wipe should not have been called")
	}
}

func TestAbsent_RequiresForce(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{currentFS: "ext4"}
	m := NewWithProvider(f)
	// without force → error
	d := decl("d", StateAbsent, map[string]any{"device": "/dev/sdb1"})
	if _, err := m.Apply(context.Background(), d); err == nil {
		t.Error("wipe without force should error")
	}
	if len(f.wipeCalls) != 0 {
		t.Error("wipe should not have been called")
	}
	// with force → wipes
	f = &fakeProvider{currentFS: "ext4"}
	m = NewWithProvider(f)
	d = decl("d", StateAbsent, map[string]any{"device": "/dev/sdb1", "force": true})
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.wipeCalls) != 1 || f.wipeCalls[0] != "/dev/sdb1" {
		t.Errorf("wipe: %+v", f.wipeCalls)
	}
	// converged
	r, _ := m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged check after wipe: %+v", r)
	}
}

// --- errors ----------------------------------------------------------

func TestErrorsPropagate(t *testing.T) {
	t.Parallel()
	// Get error
	m := NewWithProvider(&fakeProvider{getErr: errors.New("blkid")})
	if _, err := m.Check(context.Background(), decl("l", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "ext4"})); err == nil {
		t.Error("get error should propagate")
	}
	// MakeFilesystem error
	m = NewWithProvider(&fakeProvider{mkfsErr: errors.New("mkfs")})
	if _, err := m.Apply(context.Background(), decl("l", StatePresent, map[string]any{"device": "/dev/sdb1", "fstype": "ext4"})); err == nil {
		t.Error("mkfs error should propagate")
	}
	// WipeFilesystem error
	m = NewWithProvider(&fakeProvider{currentFS: "ext4", wipeErr: errors.New("wipefs")})
	if _, err := m.Apply(context.Background(), decl("l", StateAbsent, map[string]any{"device": "/dev/sdb1", "force": true})); err == nil {
		t.Error("wipefs error should propagate")
	}
}

func TestParseError_FromCheckAndApply(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{})
	bad := decl("l", StatePresent, map[string]any{}) // no device or fstype
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
	if m.Name() != "disk" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("disk should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("l", StatePresent, map[string]any{"device": "/dev/sda", "fstype": "ext4"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("any decl → HIGH")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("l", StatePresent, map[string]any{"device": "/dev/sda", "fstype": "ext4"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("l", StatePresent, map[string]any{"device": "/dev/sda"})); err == nil {
		t.Error("missing fstype should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{}
	m := NewWithProvider(f)
	d := decl("l", StatePresent, map[string]any{"device": "/dev/sda", "fstype": "ext4"})
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
	if !IsNoBlkid(ErrNoBlkid) || IsNoBlkid(errors.New("x")) {
		t.Error("IsNoBlkid")
	}
	if !IsNoWipefs(ErrNoWipefs) || IsNoWipefs(errors.New("x")) {
		t.Error("IsNoWipefs")
	}
	if !IsNoMkfs(ErrNoMkfs) || IsNoMkfs(errors.New("x")) {
		t.Error("IsNoMkfs")
	}
}

// --- fs resize ---------------------------------------------------------

func resizeDecl(fstype string) *statemgmt.Declaration {
	return decl("l", StatePresent, map[string]any{"device": "/dev/vg0/data", "fstype": fstype, "resize_fs": true})
}

func TestResize_GrowsWhenNotFilling(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{currentFS: "ext4", fills: false}
	m := NewWithProvider(f)

	// Check reports drift (fs doesn't fill the device).
	res, err := m.Check(context.Background(), resizeDecl("ext4"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Matches {
		t.Error("fs not filling device → want drift")
	}

	// Apply resizes.
	ar, err := m.Apply(context.Background(), resizeDecl("ext4"))
	if err != nil {
		t.Fatal(err)
	}
	if !ar.Changed || ar.Comment != "resized" || f.resizeCall != 1 {
		t.Errorf("apply should resize: %+v (resizeCall=%d)", ar, f.resizeCall)
	}
	// idempotent re-apply (now fills)
	ar2, _ := m.Apply(context.Background(), resizeDecl("ext4"))
	if ar2.Changed || f.resizeCall != 1 {
		t.Errorf("second apply should be a no-op; resizeCall=%d", f.resizeCall)
	}
}

func TestResize_ConvergedWhenFilling(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{currentFS: "ext4", fills: true}
	m := NewWithProvider(f)
	res, err := m.Check(context.Background(), resizeDecl("ext4"))
	if err != nil || !res.Matches {
		t.Errorf("fs fills device → converged; got %+v %v", res, err)
	}
	ar, _ := m.Apply(context.Background(), resizeDecl("ext4"))
	if ar.Changed || f.resizeCall != 0 {
		t.Errorf("filling fs must not resize; got %+v", ar)
	}
}

func TestResize_NoResizeFsParamLeavesConverged(t *testing.T) {
	t.Parallel()
	// Without resize_fs, a matching fstype is converged regardless of
	// device fill (the fs-fill check never runs).
	f := &fakeProvider{currentFS: "ext4", fills: false}
	res, err := NewWithProvider(f).Check(context.Background(), decl("l", StatePresent, map[string]any{"device": "/dev/sda1", "fstype": "ext4"}))
	if err != nil || !res.Matches {
		t.Errorf("no resize_fs → matching fstype converged; got %+v %v", res, err)
	}
}

func TestResize_FillsErrorPropagates(t *testing.T) {
	t.Parallel()
	f := &fakeProvider{currentFS: "ext4", fillsErr: errors.New("dumpe2fs boom")}
	if _, err := NewWithProvider(f).Check(context.Background(), resizeDecl("ext4")); err == nil {
		t.Error("FilesystemFillsDevice error should propagate")
	}
}

func TestResize_Validation(t *testing.T) {
	t.Parallel()
	bad := []map[string]any{
		{"device": "/dev/sda1", "fstype": "vfat", "resize_fs": true},  // not resizable
		{"device": "/dev/sda1", "fstype": "swap", "resize_fs": true},  // not resizable
		{"device": "/dev/sda1", "resize_fs": "yes", "fstype": "ext4"}, // non-bool
	}
	for _, b := range bad {
		if err := (&Module{}).Validate(decl("l", StatePresent, b)); err == nil {
			t.Errorf("Validate(%v) should error", b)
		}
	}
	// resize_fs with state=absent
	if err := (&Module{}).Validate(decl("l", StateAbsent, map[string]any{"device": "/dev/sda1", "resize_fs": true})); err == nil {
		t.Error("resize_fs + absent should error")
	}
	// ext / xfs / btrfs / f2fs + resize_fs are all valid
	for _, fstype := range []string{"ext4", "xfs", "btrfs", "f2fs"} {
		if err := (&Module{}).Validate(resizeDecl(fstype)); err != nil {
			t.Errorf("%s + resize_fs should validate; got %v", fstype, err)
		}
	}
}
