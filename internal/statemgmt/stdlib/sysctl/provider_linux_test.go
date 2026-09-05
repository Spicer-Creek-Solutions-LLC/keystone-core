// SPDX-License-Identifier: Apache-2.0

//go:build linux

package sysctl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyToPath(t *testing.T) {
	t.Parallel()
	if keyToPath("net.ipv4.ip_forward") != "net/ipv4/ip_forward" {
		t.Errorf("got %q", keyToPath("net.ipv4.ip_forward"))
	}
}

func TestNormalizeValue(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"1\n":                    "1",
		"  4096  16384  4096 ":   "4096 16384 4096",
		"4096\t16384\t4194304\n": "4096 16384 4194304",
	}
	for in, want := range cases {
		if got := normalizeValue(in); got != want {
			t.Errorf("normalizeValue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLinuxProvider_Get_FromProcSys(t *testing.T) {
	dir := t.TempDir()
	// Lay out a fake /proc/sys: net/ipv4/ip_forward → "1\n"
	keyPath := filepath.Join(dir, "net", "ipv4", "ip_forward")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	old := procSysRoot
	procSysRoot = dir
	defer func() { procSysRoot = old }()

	p := &linuxProvider{}
	val, exists, err := p.Get("net.ipv4.ip_forward")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !exists || val != "1" {
		t.Errorf("val=%q exists=%v, want \"1\"/true", val, exists)
	}
}

func TestLinuxProvider_Get_Missing(t *testing.T) {
	old := procSysRoot
	procSysRoot = t.TempDir()
	defer func() { procSysRoot = old }()
	p := &linuxProvider{}
	_, exists, err := p.Get("net.ipv4.nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if exists {
		t.Error("missing key should report exists=false")
	}
}

func TestLinuxProvider_PersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	old := sysctlConfDir
	sysctlConfDir = dir
	defer func() { sysctlConfDir = old }()

	p := &linuxProvider{}
	if err := p.WritePersist("net.ipv4.ip_forward", "1"); err != nil {
		t.Fatalf("WritePersist: %v", err)
	}
	// The file should exist at the expected path.
	want := filepath.Join(dir, "99-keystone-net.ipv4.ip_forward.conf")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("persist file not created: %v", err)
	}
	val, ok, err := p.ReadPersist("net.ipv4.ip_forward")
	if err != nil {
		t.Fatalf("ReadPersist: %v", err)
	}
	if !ok || val != "1" {
		t.Errorf("ReadPersist = %q/%v, want \"1\"/true", val, ok)
	}
	// Slashed form maps to the same file → reads the same value.
	val2, ok2, _ := p.ReadPersist("net/ipv4/ip_forward")
	if !ok2 || val2 != "1" {
		t.Errorf("slashed-form ReadPersist = %q/%v, want \"1\"/true", val2, ok2)
	}
}

func TestLinuxProvider_ReadPersist_Missing(t *testing.T) {
	old := sysctlConfDir
	sysctlConfDir = t.TempDir()
	defer func() { sysctlConfDir = old }()
	p := &linuxProvider{}
	_, ok, err := p.ReadPersist("net.ipv4.never")
	if err != nil {
		t.Fatalf("ReadPersist: %v", err)
	}
	if ok {
		t.Error("missing file should report ok=false")
	}
}

func TestLinuxProvider_ReadPersist_IgnoresCommentsAndOtherKeys(t *testing.T) {
	dir := t.TempDir()
	old := sysctlConfDir
	sysctlConfDir = dir
	defer func() { sysctlConfDir = old }()
	path := filepath.Join(dir, "99-keystone-net.ipv4.ip_forward.conf")
	content := "# header\n; another comment\nnet.ipv4.other = 9\nnet.ipv4.ip_forward = 1\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	p := &linuxProvider{}
	val, ok, err := p.ReadPersist("net.ipv4.ip_forward")
	if err != nil {
		t.Fatalf("ReadPersist: %v", err)
	}
	if !ok || val != "1" {
		t.Errorf("got %q/%v, want \"1\"/true", val, ok)
	}
}

func TestLinuxProvider_Set_NoBinary(t *testing.T) {
	t.Parallel()
	p := &linuxProvider{sysctlBin: ""} // simulate missing binary
	err := p.Set(context.Background(), "net.ipv4.ip_forward", "1")
	if err == nil || !strings.Contains(err.Error(), "binary not found") {
		t.Errorf("err = %v, want binary-not-found", err)
	}
}

func TestLinuxProvider_Set_UsesRunner(t *testing.T) {
	t.Parallel()
	var captured []string
	p := &linuxProvider{
		sysctlBin: "/sbin/sysctl",
		runner: func(_ context.Context, bin string, args []string) error {
			captured = append([]string{bin}, args...)
			return nil
		},
	}
	if err := p.Set(context.Background(), "net.ipv4.ip_forward", "1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"/sbin/sysctl", "-w", "net.ipv4.ip_forward=1"}
	if len(captured) != len(want) {
		t.Fatalf("captured = %v", captured)
	}
	for i := range want {
		if captured[i] != want[i] {
			t.Errorf("captured[%d] = %q, want %q", i, captured[i], want[i])
		}
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

// Sanity: WritePersist failure surfaces (write into a path whose
// parent is a file, not a dir).
func TestWriteFileAtomic_BadParent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	notADir := filepath.Join(dir, "blocker")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := writeFileAtomic(filepath.Join(notADir, "child"), []byte("data"))
	if err == nil {
		t.Error("expected error writing under a non-dir")
	}
}
