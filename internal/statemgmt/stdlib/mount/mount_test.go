package mount

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
		ID:     "mount:" + name,
		Module: "mount",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// withFstab points fstabPath at a fresh tempdir file with the given
// content (or no file if content == ""). Callers must NOT t.Parallel().
func withFstab(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "fstab")
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
	mounted   map[string]*MountInfo // mountPoint → info (Mounted=true)
	mounts    []struct{ device, mp, fstype, opts string }
	unmounts  []string
	lookupErr error
	mountErr  error
	umountErr error
}

func newFake(mountedAt ...string) *fakeProvider {
	f := &fakeProvider{mounted: map[string]*MountInfo{}}
	for _, mp := range mountedAt {
		f.mounted[mp] = &MountInfo{MountPoint: mp, Mounted: true, Device: "/dev/sdz1", FSType: "ext4"}
	}
	return f
}
func (f *fakeProvider) Lookup(_ context.Context, mp string) (*MountInfo, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	if mi, ok := f.mounted[mp]; ok {
		return mi, nil
	}
	return &MountInfo{MountPoint: mp, Mounted: false}, nil
}
func (f *fakeProvider) Mount(_ context.Context, device, mp, fstype, opts string) error {
	if f.mountErr != nil {
		return f.mountErr
	}
	f.mounts = append(f.mounts, struct{ device, mp, fstype, opts string }{device, mp, fstype, opts})
	f.mounted[mp] = &MountInfo{MountPoint: mp, Mounted: true, Device: device, FSType: fstype}
	return nil
}
func (f *fakeProvider) Unmount(_ context.Context, mp string) error {
	if f.umountErr != nil {
		return f.umountErr
	}
	f.unmounts = append(f.unmounts, mp)
	delete(f.mounted, mp)
	return nil
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("/mnt", StateMounted, map[string]any{"devices": "/dev/x"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_TypeAndDefaults(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("/mnt", StateMounted, map[string]any{"device": "/dev/x", "fstype": "ext4"}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Opts != defaultOpts || !p.Mkmnt || p.Dump != 0 || p.Pass != 0 {
		t.Errorf("defaults: %+v", p)
	}
	for _, bad := range []map[string]any{{"device": 1}, {"fstype": 2}, {"opts": 3}, {"mkmnt": "yes"}, {"dump": "x"}} {
		if _, err := parseParams(decl("/mnt", StateMounted, bad)); err == nil {
			t.Errorf("%v: expected type error", bad)
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
		{"mounted ok", decl("/mnt/d", StateMounted, map[string]any{"device": "/dev/sdb1", "fstype": "ext4"}), false},
		{"present ok", decl("/mnt/d", StatePresent, map[string]any{"device": "UUID=abc", "fstype": "xfs", "opts": "rw,noatime"}), false},
		{"mounted needs device", decl("/mnt/d", StateMounted, map[string]any{"fstype": "ext4"}), true},
		{"mounted needs fstype", decl("/mnt/d", StateMounted, map[string]any{"device": "/dev/x"}), true},
		{"relative mount point", decl("mnt/d", StateMounted, map[string]any{"device": "/dev/x", "fstype": "ext4"}), true},
		{"whitespace in mount point", decl("/mnt/my disk", StateMounted, map[string]any{"device": "/dev/x", "fstype": "ext4"}), true},
		{"whitespace in opts", decl("/mnt/d", StateMounted, map[string]any{"device": "/dev/x", "fstype": "ext4", "opts": "rw, noatime"}), true},
		{"negative dump", decl("/mnt/d", StateMounted, map[string]any{"device": "/dev/x", "fstype": "ext4", "dump": -1}), true},
		{"unmounted ok", decl("/mnt/d", StateUnmounted, nil), false},
		{"absent ok", decl("/mnt/d", StateAbsent, nil), false},
		{"unmounted rejects device", decl("/mnt/d", StateUnmounted, map[string]any{"device": "/dev/x"}), true},
		{"absent rejects fstype", decl("/mnt/d", StateAbsent, map[string]any{"fstype": "ext4"}), true},
		{"absent allows mkmnt", decl("/mnt/d", StateAbsent, map[string]any{"mkmnt": false}), false},
		{"bad state", decl("/mnt/d", "frob", nil), true},
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

func TestFstab_ParseAndCompare(t *testing.T) {
	t.Parallel()
	e, ok := parseFstabLine("UUID=abc  /data  ext4  rw,noatime  0  2")
	if !ok || e.Device != "UUID=abc" || e.MountPoint != "/data" || e.FSType != "ext4" || e.Opts != "rw,noatime" || e.Dump != 0 || e.Pass != 2 {
		t.Fatalf("parse: %+v ok=%v", e, ok)
	}
	// 4-field line (no dump/pass)
	e, ok = parseFstabLine("tmpfs /tmp tmpfs defaults")
	if !ok || e.Dump != 0 || e.Pass != 0 {
		t.Errorf("4-field: %+v", e)
	}
	// comment / blank / short → not a line
	for _, bad := range []string{"# comment", "", "   ", "/dev/x /mnt"} {
		if _, ok := parseFstabLine(bad); ok {
			t.Errorf("%q should not parse as an entry", bad)
		}
	}
	// opts-set comparison is order-insensitive
	a := fstabEntry{Device: "d", MountPoint: "/m", FSType: "ext4", Opts: "rw,noatime,nofail"}
	b := fstabEntry{Device: "d", MountPoint: "/m", FSType: "ext4", Opts: "nofail,rw,noatime"}
	if !a.matchesDesired(b) {
		t.Error("opts should compare as a set")
	}
	if a.matchesDesired(fstabEntry{Device: "d2", MountPoint: "/m", FSType: "ext4", Opts: "rw,noatime,nofail"}) {
		t.Error("a different device should not match")
	}
}

func TestFstab_UpsertRemove(t *testing.T) {
	t.Parallel()
	base := "# /etc/fstab\nproc /proc proc defaults 0 0\nUUID=root / ext4 errors=remount-ro 0 1\n"
	// upsert a new entry → appended, others intact
	c1 := upsertEntry(base, fstabEntry{Device: "/dev/sdb1", MountPoint: "/data", FSType: "ext4", Opts: "defaults"})
	if e, ok := findEntry(c1, "/data"); !ok || e.Device != "/dev/sdb1" {
		t.Fatalf("upsert add: %+v ok=%v\n%s", e, ok, c1)
	}
	if !strings.Contains(c1, "UUID=root / ext4") || !strings.Contains(c1, "# /etc/fstab") {
		t.Errorf("upsert clobbered: %s", c1)
	}
	// upsert again with changed opts → replaces in place
	c2 := upsertEntry(c1, fstabEntry{Device: "/dev/sdb1", MountPoint: "/data", FSType: "ext4", Opts: "rw,noatime", Pass: 2})
	if e, _ := findEntry(c2, "/data"); e.Opts != "rw,noatime" || e.Pass != 2 {
		t.Errorf("upsert replace: %+v", e)
	}
	if strings.Count(c2, " /data ") != 1 {
		t.Errorf("upsert duplicated the entry: %s", c2)
	}
	// remove
	c3 := removeEntry(c2, "/data")
	if _, ok := findEntry(c3, "/data"); ok {
		t.Error("entry not removed")
	}
	if !strings.Contains(c3, "UUID=root / ext4") {
		t.Error("removal clobbered other entries")
	}
	if removeEntry(c3, "/nope") != c3 {
		t.Error("removing a missing entry should be a no-op")
	}
	// upsert into empty content
	if upsertEntry("", fstabEntry{Device: "d", MountPoint: "/m", FSType: "ext4", Opts: "defaults"}) != "d /m ext4 defaults 0 0\n" {
		t.Errorf("upsert into empty: %q", upsertEntry("", fstabEntry{Device: "d", MountPoint: "/m", FSType: "ext4", Opts: "defaults"}))
	}
}

// --- Check / Apply -----------------------------------------------------

func TestCheckApply_Mounted(t *testing.T) {
	withFstab(t, "proc /proc proc defaults 0 0\n")
	f := newFake()
	m := NewWithProvider(f)
	d := decl("/data", StateMounted, map[string]any{"device": "/dev/sdb1", "fstype": "ext4", "opts": "rw,noatime", "mkmnt": false})

	// nothing in fstab, not mounted → drift
	r, err := m.Check(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches {
		t.Error("should drift")
	}

	// Apply: adds fstab entry + mounts
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("first apply should change")
	}
	if e, ok := findEntry(fstabContent(t), "/data"); !ok || e.Device != "/dev/sdb1" || !optsSetEqual(e.Opts, "rw,noatime") {
		t.Fatalf("fstab entry wrong: %+v ok=%v", e, ok)
	}
	if len(f.mounts) != 1 || f.mounts[0].mp != "/data" || f.mounts[0].fstype != "ext4" || f.mounts[0].opts != "rw,noatime" {
		t.Fatalf("Mount call wrong: %+v", f.mounts)
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

	// fstab entry drifts (opts changed in the decl) → rewrite, but
	// it's still mounted so no re-mount
	d2 := decl("/data", StateMounted, map[string]any{"device": "/dev/sdb1", "fstype": "ext4", "opts": "rw,noatime,nofail", "mkmnt": false})
	r, _ = m.Check(context.Background(), d2)
	if r.Matches {
		t.Error("opts change should drift")
	}
	mounts := len(f.mounts)
	sr, _ = m.Apply(context.Background(), d2)
	if !sr.Changed {
		t.Error("fstab opts change should be a change")
	}
	if len(f.mounts) != mounts {
		t.Error("a fstab-only change should not re-mount")
	}
	if e, _ := findEntry(fstabContent(t), "/data"); !optsSetEqual(e.Opts, "nofail,rw,noatime") {
		t.Errorf("opts not updated: %+v", e)
	}

	// not mounted but fstab is right → mount only
	f.Unmount(context.Background(), "/data") // simulate it got unmounted out of band
	mounts = len(f.mounts)
	r, _ = m.Check(context.Background(), d2)
	if r.Matches {
		t.Error("not-mounted should drift")
	}
	sr, _ = m.Apply(context.Background(), d2)
	if !sr.Changed || len(f.mounts) != mounts+1 {
		t.Errorf("should have re-mounted: changed=%v mounts=%d", sr.Changed, len(f.mounts))
	}
}

func TestApply_Mounted_Mkmnt(t *testing.T) {
	withFstab(t, "")
	mp := filepath.Join(t.TempDir(), "newmnt") // does not exist yet
	f := newFake()
	if _, err := NewWithProvider(f).Apply(context.Background(), decl(mp, StateMounted, map[string]any{"device": "tmpfs", "fstype": "tmpfs"})); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(mp); err != nil || !fi.IsDir() {
		t.Errorf("mkmnt should have created the mount point: %v %v", fi, err)
	}
	if len(f.mounts) != 1 {
		t.Errorf("expected one Mount call, got %v", f.mounts)
	}
}

func TestCheckApply_Present(t *testing.T) {
	withFstab(t, "")
	m := NewWithProvider(newFake())
	d := decl("/backups", StatePresent, map[string]any{"device": "UUID=xyz", "fstype": "xfs", "opts": "noauto"})

	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("no fstab entry → drift")
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if e, ok := findEntry(fstabContent(t), "/backups"); !ok || e.Device != "UUID=xyz" || e.FSType != "xfs" {
		t.Fatalf("fstab entry: %+v ok=%v", e, ok)
	}
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Error("should match after apply")
	}
}

func TestCheckApply_Unmounted(t *testing.T) {
	withFstab(t, "proc /proc proc defaults 0 0\nUUID=x /data ext4 defaults 0 2\n")
	f := newFake("/data")
	m := NewWithProvider(f)
	d := decl("/data", StateUnmounted, nil)

	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("mounted → drift from unmounted")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed || len(f.unmounts) != 1 {
		t.Errorf("should have unmounted: changed=%v unmounts=%v", sr.Changed, f.unmounts)
	}
	// fstab entry must be left in place
	if _, ok := findEntry(fstabContent(t), "/data"); !ok {
		t.Error("unmounted should not touch the fstab entry")
	}
	// already unmounted → no-op
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("unmounted on a not-mounted fs should be a no-op")
	}
}

func TestCheckApply_Absent(t *testing.T) {
	withFstab(t, "proc /proc proc defaults 0 0\nUUID=x /data ext4 defaults 0 2\n")
	f := newFake("/data")
	m := NewWithProvider(f)
	d := decl("/data", StateAbsent, nil)

	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("mounted + in fstab → drift from absent")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed || len(f.unmounts) != 1 {
		t.Errorf("should have unmounted: %v", f.unmounts)
	}
	if _, ok := findEntry(fstabContent(t), "/data"); ok {
		t.Error("fstab entry should be removed")
	}
	if !strings.Contains(fstabContent(t), "/proc") {
		t.Error("other fstab entries should survive")
	}
	// already absent → no-op
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("absent on a clean state should be a no-op")
	}
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Error("absent should match a clean state")
	}

	// in fstab only (not mounted) → drift → remove the line, no umount
	withFstab(t, "UUID=x /old ext4 defaults 0 2\n")
	f2 := newFake()
	sr, _ = NewWithProvider(f2).Apply(context.Background(), decl("/old", StateAbsent, nil))
	if !sr.Changed || len(f2.unmounts) != 0 {
		t.Errorf("fstab-only absent: changed=%v unmounts=%v", sr.Changed, f2.unmounts)
	}
	if _, ok := findEntry(fstabContent(t), "/old"); ok {
		t.Error("fstab-only entry not removed")
	}
}

func TestApply_ErrorsPropagate(t *testing.T) {
	withFstab(t, "")
	// mount error
	f := newFake()
	f.mountErr = errors.New("mount: wrong fs type")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("/data", StateMounted, map[string]any{"device": "/dev/x", "fstype": "ext4", "mkmnt": false})); err == nil {
		t.Error("mount error should propagate")
	}
	// lookup error
	withFstab(t, "")
	f = newFake()
	f.lookupErr = errors.New("cannot read /proc/mounts")
	if _, err := NewWithProvider(f).Check(context.Background(), decl("/data", StateUnmounted, nil)); err == nil {
		t.Error("lookup error should propagate")
	}
	// umount error
	withFstab(t, "UUID=x /data ext4 defaults 0 2\n")
	f = newFake("/data")
	f.umountErr = errors.New("umount: target is busy")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("/data", StateAbsent, nil)); err == nil {
		t.Error("umount error should propagate")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "mount" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 4 {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("mount should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("/m", StateMounted, map[string]any{"device": "d", "fstype": "ext4"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("mounted drift → HIGH")
	}
	if dsm.DriftSeverity(decl("/m", StateAbsent, nil), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift → HIGH")
	}
	if dsm.DriftSeverity(decl("/m", StatePresent, map[string]any{"device": "d", "fstype": "ext4"}), nil) != statemgmt.DriftSeverityMedium {
		t.Error("present drift → MEDIUM")
	}
	if dsm.DriftSeverity(decl("/m", StateUnmounted, nil), nil) != statemgmt.DriftSeverityMedium {
		t.Error("unmounted drift → MEDIUM")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("/m", StateMounted, map[string]any{"device": "d", "fstype": "ext4"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("/m", StateMounted, map[string]any{"fstype": "ext4"})); err == nil {
		t.Error("mounted without device should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	withFstab(t, "")
	m := NewWithProvider(newFake())
	if ok, err := m.Test(context.Background(), decl("/data", StateUnmounted, nil)); err != nil || !ok {
		t.Errorf("Test unmounted on a not-mounted fs: ok=%v err=%v", ok, err)
	}
	if ok, err := m.Test(context.Background(), decl("/data", StatePresent, map[string]any{"device": "d", "fstype": "ext4"})); err != nil || ok {
		t.Errorf("Test present with no fstab entry should be false: ok=%v err=%v", ok, err)
	}
}

func TestSentinelMatchers(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) || IsUnsupportedOS(errors.New("x")) {
		t.Error("IsUnsupportedOS")
	}
	if !IsNoMountTools(ErrNoMountTools) || IsNoMountTools(errors.New("x")) {
		t.Error("IsNoMountTools")
	}
}
