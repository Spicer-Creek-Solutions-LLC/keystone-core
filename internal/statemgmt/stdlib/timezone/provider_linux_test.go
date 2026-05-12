//go:build linux

package timezone

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxProvider_Current_AbsoluteSymlink(t *testing.T) {
	dir := t.TempDir()
	zi := filepath.Join(dir, "zoneinfo")
	if err := os.MkdirAll(filepath.Join(zi, "America"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	zoneFile := filepath.Join(zi, "America", "New_York")
	if err := os.WriteFile(zoneFile, []byte("TZif"), 0o644); err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	lt := filepath.Join(dir, "localtime")
	if err := os.Symlink(zoneFile, lt); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	oldLT, oldZI := localtimePath, zoneinfoRoot
	localtimePath, zoneinfoRoot = lt, zi
	defer func() { localtimePath, zoneinfoRoot = oldLT, oldZI }()

	p := &linuxProvider{}
	cur, set, err := p.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !set || cur != "America/New_York" {
		t.Errorf("cur=%q set=%v, want America/New_York/true", cur, set)
	}
}

func TestLinuxProvider_Current_RelativeSymlink(t *testing.T) {
	dir := t.TempDir()
	zi := filepath.Join(dir, "zoneinfo")
	if err := os.MkdirAll(zi, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(zi, "UTC"), []byte("TZif"), 0o644); err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	lt := filepath.Join(dir, "localtime")
	// Relative target as a real /etc/localtime often is.
	if err := os.Symlink("zoneinfo/UTC", lt); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	oldLT, oldZI := localtimePath, zoneinfoRoot
	localtimePath, zoneinfoRoot = lt, zi
	defer func() { localtimePath, zoneinfoRoot = oldLT, oldZI }()

	p := &linuxProvider{}
	cur, set, err := p.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !set || cur != "UTC" {
		t.Errorf("cur=%q set=%v, want UTC/true", cur, set)
	}
}

func TestLinuxProvider_Current_NotASymlink(t *testing.T) {
	dir := t.TempDir()
	lt := filepath.Join(dir, "localtime")
	if err := os.WriteFile(lt, []byte("TZif"), 0o644); err != nil { // a copied zone file, not a symlink
		t.Fatalf("seed: %v", err)
	}
	oldLT := localtimePath
	localtimePath = lt
	defer func() { localtimePath = oldLT }()
	p := &linuxProvider{}
	_, set, err := p.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if set {
		t.Error("non-symlink localtime should report set=false")
	}
}

func TestLinuxProvider_Current_SymlinkOutsideZoneinfo(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "elsewhere")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	lt := filepath.Join(dir, "localtime")
	if err := os.Symlink(other, lt); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	oldLT, oldZI := localtimePath, zoneinfoRoot
	localtimePath, zoneinfoRoot = lt, filepath.Join(dir, "zoneinfo")
	defer func() { localtimePath, zoneinfoRoot = oldLT, oldZI }()
	p := &linuxProvider{}
	_, set, _ := p.Current()
	if set {
		t.Error("symlink outside zoneinfo should report set=false")
	}
}

func TestLinuxProvider_Current_Missing(t *testing.T) {
	oldLT := localtimePath
	localtimePath = filepath.Join(t.TempDir(), "nope")
	defer func() { localtimePath = oldLT }()
	p := &linuxProvider{}
	_, set, err := p.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if set {
		t.Error("missing localtime should report set=false")
	}
}

func TestLinuxProvider_Set_ZoneNotFound(t *testing.T) {
	t.Parallel()
	oldZI := zoneinfoRoot
	zoneinfoRoot = t.TempDir() // empty — no zones
	defer func() { zoneinfoRoot = oldZI }()
	p := &linuxProvider{runner: func(context.Context, string, []string) error { return nil }}
	err := p.Set(context.Background(), "Made/Up")
	if !errors.Is(err, ErrZoneNotFound) {
		t.Errorf("err = %v, want ErrZoneNotFound", err)
	}
}

func TestLinuxProvider_Set_PrefersTimedatectl(t *testing.T) {
	dir := t.TempDir()
	zi := filepath.Join(dir, "zoneinfo")
	if err := os.MkdirAll(zi, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(zi, "UTC"), []byte("TZif"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	oldZI := zoneinfoRoot
	zoneinfoRoot = zi
	defer func() { zoneinfoRoot = oldZI }()

	oldLook := lookPath
	lookPath = func(name string) (string, error) {
		if name == "timedatectl" {
			return "/usr/bin/timedatectl", nil
		}
		return "", errors.New("not found")
	}
	defer func() { lookPath = oldLook }()

	var captured []string
	p := &linuxProvider{runner: func(_ context.Context, bin string, args []string) error {
		captured = append([]string{bin}, args...)
		return nil
	}}
	if err := p.Set(context.Background(), "UTC"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"/usr/bin/timedatectl", "set-timezone", "UTC"}
	if len(captured) != len(want) {
		t.Fatalf("captured = %v", captured)
	}
	for i := range want {
		if captured[i] != want[i] {
			t.Errorf("captured[%d] = %q, want %q", i, captured[i], want[i])
		}
	}
}

func TestLinuxProvider_Set_FallbackSymlinkAndTimezoneFile(t *testing.T) {
	dir := t.TempDir()
	zi := filepath.Join(dir, "zoneinfo")
	if err := os.MkdirAll(filepath.Join(zi, "America"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	zoneFile := filepath.Join(zi, "America", "New_York")
	if err := os.WriteFile(zoneFile, []byte("TZif"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	lt := filepath.Join(dir, "localtime")
	tzf := filepath.Join(dir, "timezone")
	oldLT, oldZI, oldTZ := localtimePath, zoneinfoRoot, etcTimezone
	localtimePath, zoneinfoRoot, etcTimezone = lt, zi, tzf
	defer func() { localtimePath, zoneinfoRoot, etcTimezone = oldLT, oldZI, oldTZ }()

	oldLook := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") } // no timedatectl
	defer func() { lookPath = oldLook }()

	p := &linuxProvider{runner: func(context.Context, string, []string) error { return nil }}
	if err := p.Set(context.Background(), "America/New_York"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// /etc/localtime should now be a symlink to the zone file.
	target, err := os.Readlink(lt)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != zoneFile {
		t.Errorf("symlink target = %q, want %q", target, zoneFile)
	}
	// /etc/timezone should carry the zone name.
	data, _ := os.ReadFile(tzf)
	if strings.TrimSpace(string(data)) != "America/New_York" {
		t.Errorf("/etc/timezone = %q, want America/New_York", data)
	}
}

func TestLinuxProvider_Set_FallbackReplacesExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	zi := filepath.Join(dir, "zoneinfo")
	if err := os.MkdirAll(zi, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, z := range []string{"UTC", "GMT"} {
		if err := os.WriteFile(filepath.Join(zi, z), []byte("TZif"), 0o644); err != nil {
			t.Fatalf("seed %s: %v", z, err)
		}
	}
	lt := filepath.Join(dir, "localtime")
	if err := os.Symlink(filepath.Join(zi, "UTC"), lt); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	tzf := filepath.Join(dir, "timezone")
	oldLT, oldZI, oldTZ := localtimePath, zoneinfoRoot, etcTimezone
	localtimePath, zoneinfoRoot, etcTimezone = lt, zi, tzf
	defer func() { localtimePath, zoneinfoRoot, etcTimezone = oldLT, oldZI, oldTZ }()
	oldLook := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = oldLook }()

	p := &linuxProvider{runner: func(context.Context, string, []string) error { return nil }}
	if err := p.Set(context.Background(), "GMT"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	target, _ := os.Readlink(lt)
	if target != filepath.Join(zi, "GMT") {
		t.Errorf("symlink target = %q, want .../GMT", target)
	}
}

func TestExecRun_ExitError(t *testing.T) {
	t.Parallel()
	if err := execRun(context.Background(), "/bin/false", nil); err == nil {
		t.Fatal("expected exit-1 error")
	}
}

func TestExecRun_BinaryNotFound(t *testing.T) {
	t.Parallel()
	if err := execRun(context.Background(), "/no/such/bin", nil); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestDefaultProvider_ReturnsProvider(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("nil")
	}
}
