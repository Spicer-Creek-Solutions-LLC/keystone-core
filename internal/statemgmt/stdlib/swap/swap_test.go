// SPDX-License-Identifier: Apache-2.0

package swap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "swap:" + name,
		Module: "swap",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// withFstab points fstabPath at a fresh tempdir file. Callers must
// NOT t.Parallel().
func withFstab(t *testing.T, content string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fstab")
	if content != "" {
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := fstabPath
	fstabPath = p
	t.Cleanup(func() { fstabPath = old })
}

func fstabContent(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(fstabPath)
	if err != nil {
		return ""
	}
	return string(b)
}

// --- fakeProvider ------------------------------------------------------

type fakeProvider struct {
	active    map[string]*SwapInfo // source → info (Active=true)
	makeSwaps []string
	swapOns   []struct {
		path string
		prio int
	}
	swapOffs []string
	created  []struct {
		path  string
		bytes int64
	}
	lookupErr error
	makeErr   error
	onErr     error
	offErr    error
	createErr error
}

func newFake(activeSources ...string) *fakeProvider {
	f := &fakeProvider{active: map[string]*SwapInfo{}}
	for _, s := range activeSources {
		f.active[s] = &SwapInfo{Source: s, Active: true, Priority: -2}
	}
	return f
}
func (f *fakeProvider) Lookup(_ context.Context, source string) (*SwapInfo, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	if si, ok := f.active[source]; ok {
		return si, nil
	}
	return &SwapInfo{Source: source, Active: false}, nil
}
func (f *fakeProvider) MakeSwap(_ context.Context, path string) error {
	if f.makeErr != nil {
		return f.makeErr
	}
	f.makeSwaps = append(f.makeSwaps, path)
	return nil
}
func (f *fakeProvider) SwapOn(_ context.Context, path string, prio int) error {
	if f.onErr != nil {
		return f.onErr
	}
	f.swapOns = append(f.swapOns, struct {
		path string
		prio int
	}{path, prio})
	f.active[path] = &SwapInfo{Source: path, Active: true, Priority: prio}
	return nil
}
func (f *fakeProvider) SwapOff(_ context.Context, path string) error {
	if f.offErr != nil {
		return f.offErr
	}
	f.swapOffs = append(f.swapOffs, path)
	delete(f.active, path)
	return nil
}
func (f *fakeProvider) CreateSwapfile(_ context.Context, path string, bytes int64) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, struct {
		path  string
		bytes int64
	}{path, bytes})
	return nil
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("/swapfile", StateOn, map[string]any{"sizes": "1G"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParseSize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   any
		ok   bool
		want int64
	}{
		{"1024K", true, 1024 * 1024},
		{"512M", true, 512 * 1024 * 1024},
		{"2G", true, 2 * 1024 * 1024 * 1024},
		{"100m", true, 100 * 1024 * 1024},
		{"1024", false, 0}, // bare number rejected
		{"0G", false, 0},
		{"-1G", false, 0},
		{"abcM", false, 0},
		{"", false, 0},
		{1024, false, 0}, // bare YAML int rejected
	}
	for _, c := range cases {
		got, err := parseSize(c.in)
		if c.ok {
			if err != nil || got != c.want {
				t.Errorf("parseSize(%v) = %d,%v want %d", c.in, got, err, c.want)
			}
		} else if err == nil {
			t.Errorf("parseSize(%v): expected error", c.in)
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
		{"on ok", decl("/swapfile", StateOn, map[string]any{"size": "2G"}), false},
		{"on ok no size", decl("/dev/sda2", StateOn, nil), false},
		{"on ok priority", decl("/swapfile", StateOn, map[string]any{"size": "1G", "priority": 10}), false},
		{"relative source", decl("swapfile", StateOn, nil), true},
		{"whitespace source", decl("/swap file", StateOn, nil), true},
		{"priority too high", decl("/swapfile", StateOn, map[string]any{"priority": 99999}), true},
		{"priority -2", decl("/swapfile", StateOn, map[string]any{"priority": -2}), true},
		{"present ok", decl("/dev/sda2", StatePresent, map[string]any{"priority": 5}), false},
		{"present rejects size", decl("/swapfile", StatePresent, map[string]any{"size": "1G"}), true},
		{"off ok", decl("/swapfile", StateOff, nil), false},
		{"off rejects size", decl("/swapfile", StateOff, map[string]any{"size": "1G"}), true},
		{"absent ok", decl("/swapfile", StateAbsent, nil), false},
		{"absent rejects priority", decl("/swapfile", StateAbsent, map[string]any{"priority": 1}), true},
		{"bad state", decl("/swapfile", "frob", nil), true},
		{"bad size", decl("/swapfile", StateOn, map[string]any{"size": "1024"}), true},
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

// --- fstab.go (pure) --------------------------------------------------

func TestFstab(t *testing.T) {
	t.Parallel()
	base := "# /etc/fstab\nUUID=root / ext4 defaults 0 1\n/dev/sda2 none swap defaults 0 0\n"
	// findSwapEntry keyed on source among swap lines
	if o, ok := findSwapEntry(base, "/dev/sda2"); !ok || o != "defaults" {
		t.Fatalf("find: %q ok=%v", o, ok)
	}
	if _, ok := findSwapEntry(base, "/dev/sda3"); ok {
		t.Error("missing swap source should not be found")
	}
	// upsert a new swap entry with a priority
	p := &params{Source: "/swapfile", Priority: 10}
	c1 := upsertSwapEntry(base, p)
	if o, ok := findSwapEntry(c1, "/swapfile"); !ok || !optsSetEqual(o, "defaults,pri=10") {
		t.Fatalf("upsert add: %q ok=%v\n%s", o, ok, c1)
	}
	if !strings.Contains(c1, "/dev/sda2 none swap defaults 0 0") || !strings.Contains(c1, "UUID=root /") {
		t.Errorf("upsert clobbered: %s", c1)
	}
	// upsert again with no priority → opts back to "defaults", replaced in place
	c2 := upsertSwapEntry(c1, &params{Source: "/swapfile", Priority: priorityAuto})
	if o, _ := findSwapEntry(c2, "/swapfile"); o != "defaults" {
		t.Errorf("upsert replace: %q", o)
	}
	if strings.Count(c2, "/swapfile none swap") != 1 {
		t.Errorf("upsert duplicated: %s", c2)
	}
	// remove
	c3 := removeSwapEntry(c2, "/swapfile")
	if _, ok := findSwapEntry(c3, "/swapfile"); ok {
		t.Error("entry not removed")
	}
	if _, ok := findSwapEntry(c3, "/dev/sda2"); !ok {
		t.Error("removal clobbered the other swap entry")
	}
	if removeSwapEntry(c3, "/nope") != c3 {
		t.Error("removing a missing entry should be a no-op")
	}
	// a non-swap line with the same field-1 isn't matched
	weird := "/dev/sda2 /data ext4 defaults 0 2\n"
	if _, ok := findSwapEntry(weird, "/dev/sda2"); ok {
		t.Error("an ext4 line should not match as a swap entry")
	}
	// desiredOpts
	if desiredOpts(&params{Priority: priorityAuto}) != "defaults" {
		t.Error("default opts")
	}
	if desiredOpts(&params{Priority: 3}) != "defaults,pri=3" {
		t.Error("priority opts")
	}
}

// --- Check / Apply -----------------------------------------------------

func TestCheckApply_On_Swapfile(t *testing.T) {
	withFstab(t, "UUID=root / ext4 defaults 0 1\n")
	src := filepath.Join(t.TempDir(), "swapfile") // does not exist
	f := newFake()
	m := NewWithProvider(f)
	d := decl(src, StateOn, map[string]any{"size": "1G", "priority": 5})

	// nothing yet → drift
	r, err := m.Check(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches {
		t.Error("should drift")
	}

	// Apply: fstab entry + create swapfile + mkswap + swapon
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("first apply should change")
	}
	if o, ok := findSwapEntry(fstabContent(t), src); !ok || !optsSetEqual(o, "defaults,pri=5") {
		t.Fatalf("fstab: %q ok=%v", o, ok)
	}
	if len(f.created) != 1 || f.created[0].bytes != 1024*1024*1024 {
		t.Fatalf("CreateSwapfile: %+v", f.created)
	}
	if len(f.makeSwaps) != 1 || f.makeSwaps[0] != src {
		t.Fatalf("MakeSwap: %v", f.makeSwaps)
	}
	if len(f.swapOns) != 1 || f.swapOns[0].path != src || f.swapOns[0].prio != 5 {
		t.Fatalf("SwapOn: %+v", f.swapOns)
	}

	// converged
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("should match after apply, diff=%q", r.Diff)
	}
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed || sr.Comment != "already converged" {
		t.Errorf("second apply: changed=%v comment=%q", sr.Changed, sr.Comment)
	}

	// it got swapped off out of band → re-activate only (fstab is right)
	delete(f.active, src)
	makes := len(f.makeSwaps)
	r, _ = m.Check(context.Background(), d)
	if r.Matches {
		t.Error("not-active should drift")
	}
	sr, _ = m.Apply(context.Background(), d)
	if !sr.Changed || len(f.makeSwaps) != makes+1 || len(f.swapOns) != 2 {
		t.Errorf("re-activate: changed=%v makes=%d ons=%d", sr.Changed, len(f.makeSwaps), len(f.swapOns))
	}
	// the file existed by then? no — the fake didn't create it; ensureSwapArea
	// sees it missing and creates it again. That's fine for the fake.
}

func TestApply_On_MissingSwapfile_NoSize(t *testing.T) {
	withFstab(t, "")
	src := filepath.Join(t.TempDir(), "swapfile")
	if _, err := NewWithProvider(newFake()).Apply(context.Background(), decl(src, StateOn, nil)); !errors.Is(err, ErrSwapfileSizeRequired) {
		t.Fatalf("expected ErrSwapfileSizeRequired, got %v", err)
	}
}

func TestApply_On_ExistingFile_NoCreate(t *testing.T) {
	withFstab(t, "")
	src := filepath.Join(t.TempDir(), "swapfile")
	if err := os.WriteFile(src, make([]byte, 1024*1024), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFake()
	if _, err := NewWithProvider(f).Apply(context.Background(), decl(src, StateOn, map[string]any{"size": "1G"})); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 0 {
		t.Error("an existing swapfile should not be re-created")
	}
	if len(f.makeSwaps) != 1 || len(f.swapOns) != 1 {
		t.Errorf("should have mkswap'd + swapon'd: makes=%v ons=%v", f.makeSwaps, f.swapOns)
	}
}

func TestCheckApply_Present(t *testing.T) {
	withFstab(t, "")
	m := NewWithProvider(newFake())
	d := decl("/dev/sda2", StatePresent, map[string]any{"priority": 7})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("no fstab entry → drift")
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if o, ok := findSwapEntry(fstabContent(t), "/dev/sda2"); !ok || !optsSetEqual(o, "defaults,pri=7") {
		t.Fatalf("fstab: %q ok=%v", o, ok)
	}
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Error("should match after apply")
	}
}

func TestCheckApply_Off(t *testing.T) {
	withFstab(t, "/dev/sda2 none swap defaults 0 0\n")
	f := newFake("/dev/sda2")
	m := NewWithProvider(f)
	d := decl("/dev/sda2", StateOff, nil)
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("active → drift from off")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed || len(f.swapOffs) != 1 {
		t.Errorf("should swapoff: changed=%v offs=%v", sr.Changed, f.swapOffs)
	}
	if _, ok := findSwapEntry(fstabContent(t), "/dev/sda2"); !ok {
		t.Error("off should not touch the fstab entry")
	}
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("off on an inactive source should be a no-op")
	}
}

func TestCheckApply_Absent(t *testing.T) {
	withFstab(t, "UUID=root / ext4 defaults 0 1\n/swapfile none swap defaults 0 0\n")
	src := filepath.Join(t.TempDir(), "swapfile")
	if err := os.WriteFile(src, make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	// the fstab entry uses "/swapfile" but our managed source is `src`
	// — make the fstab entry actually reference `src`:
	withFstab(t, "UUID=root / ext4 defaults 0 1\n"+src+" none swap defaults 0 0\n")
	f := newFake(src)
	m := NewWithProvider(f)
	d := decl(src, StateAbsent, nil)

	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("active + in fstab + file present → drift from absent")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed || len(f.swapOffs) != 1 {
		t.Errorf("should swapoff: %v", f.swapOffs)
	}
	if _, ok := findSwapEntry(fstabContent(t), src); ok {
		t.Error("fstab entry should be removed")
	}
	if !strings.Contains(fstabContent(t), "UUID=root /") {
		t.Error("other fstab entries should survive")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("swapfile should be removed")
	}
	// already absent → no-op + match
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("absent on a clean state should be a no-op")
	}
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Error("absent should match a clean state")
	}

	// absent on a device source (no file) that's not active and not in fstab → match
	withFstab(t, "")
	r, _ = NewWithProvider(newFake()).Check(context.Background(), decl("/dev/sdz9", StateAbsent, nil))
	if !r.Matches {
		t.Error("absent on a clean device source should match")
	}
}

func TestApply_ErrorsPropagate(t *testing.T) {
	withFstab(t, "")
	src := filepath.Join(t.TempDir(), "sf")
	// create-swapfile error
	f := newFake()
	f.createErr = errors.New("dd: no space left")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl(src, StateOn, map[string]any{"size": "1G"})); err == nil {
		t.Error("CreateSwapfile error should propagate")
	}
	// mkswap error (file exists so no creation)
	withFstab(t, "")
	if err := os.WriteFile(src, make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	f = newFake()
	f.makeErr = errors.New("mkswap: device busy")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl(src, StateOn, nil)); err == nil {
		t.Error("mkswap error should propagate")
	}
	// lookup error
	withFstab(t, "")
	f = newFake()
	f.lookupErr = errors.New("cannot read /proc/swaps")
	if _, err := NewWithProvider(f).Check(context.Background(), decl("/dev/sda2", StateOff, nil)); err == nil {
		t.Error("lookup error should propagate")
	}
	// swapoff error
	withFstab(t, "")
	f = newFake("/dev/sda2")
	f.offErr = errors.New("swapoff: cannot")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("/dev/sda2", StateOff, nil)); err == nil {
		t.Error("swapoff error should propagate")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "swap" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 4 {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("swap should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("/swapfile", StateOn, nil), nil) != statemgmt.DriftSeverityHigh {
		t.Error("on drift → HIGH")
	}
	if dsm.DriftSeverity(decl("/swapfile", StateAbsent, nil), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift → HIGH")
	}
	if dsm.DriftSeverity(decl("/dev/sda2", StatePresent, nil), nil) != statemgmt.DriftSeverityMedium {
		t.Error("present drift → MEDIUM")
	}
	if dsm.DriftSeverity(decl("/swapfile", StateOff, nil), nil) != statemgmt.DriftSeverityMedium {
		t.Error("off drift → MEDIUM")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("/dev/sda2", StateOn, nil)); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("relative", StateOn, nil)); err == nil {
		t.Error("a relative source should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	withFstab(t, "")
	m := NewWithProvider(newFake())
	if ok, err := m.Test(context.Background(), decl("/dev/sda2", StateOff, nil)); err != nil || !ok {
		t.Errorf("Test off on an inactive source: ok=%v err=%v", ok, err)
	}
	if ok, err := m.Test(context.Background(), decl("/dev/sda2", StatePresent, nil)); err != nil || ok {
		t.Errorf("Test present with no fstab entry should be false: ok=%v err=%v", ok, err)
	}
}

func TestSentinelMatchers(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) || IsUnsupportedOS(errors.New("x")) {
		t.Error("IsUnsupportedOS")
	}
	if !IsNoSwapTools(ErrNoSwapTools) || IsNoSwapTools(errors.New("x")) {
		t.Error("IsNoSwapTools")
	}
	if !IsSwapfileSizeRequired(ErrSwapfileSizeRequired) || IsSwapfileSizeRequired(errors.New("x")) {
		t.Error("IsSwapfileSizeRequired")
	}
}
