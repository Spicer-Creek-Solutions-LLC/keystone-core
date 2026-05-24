// SPDX-License-Identifier: Apache-2.0

//go:build linux

package kmod

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	t.Parallel()
	if normalizeName("br-netfilter") != "br_netfilter" {
		t.Errorf("got %q", normalizeName("br-netfilter"))
	}
}

func TestLinuxProvider_Loaded_ParsesProcModules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "modules")
	content := "br_netfilter 36864 0 - Live 0xffffffffc0a00000\n" +
		"overlay 151552 1 - Live 0xffffffffc0900000\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	old := procModules
	procModules = path
	defer func() { procModules = old }()

	p := &linuxProvider{}
	loaded, err := p.Loaded("br_netfilter")
	if err != nil {
		t.Fatalf("Loaded: %v", err)
	}
	if !loaded {
		t.Error("br_netfilter should be reported loaded")
	}
	// Dashed form maps to the same underscored name.
	loaded2, _ := p.Loaded("br-netfilter")
	if !loaded2 {
		t.Error("dashed-form lookup should match underscored entry")
	}
	notLoaded, _ := p.Loaded("nonexistent_mod")
	if notLoaded {
		t.Error("unknown module should be reported not loaded")
	}
}

func TestLinuxProvider_Loaded_MissingProcModules(t *testing.T) {
	old := procModules
	procModules = "/no/such/proc/modules"
	defer func() { procModules = old }()
	p := &linuxProvider{}
	_, err := p.Loaded("anything")
	if err == nil {
		t.Error("expected error when /proc/modules unreadable")
	}
}

func TestLinuxProvider_PersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	old := modulesLoadDir
	modulesLoadDir = dir
	defer func() { modulesLoadDir = old }()

	p := &linuxProvider{}
	exists, _ := p.PersistExists("br_netfilter")
	if exists {
		t.Fatal("should not exist before AddPersist")
	}
	if err := p.AddPersist("br-netfilter"); err != nil { // dashed form
		t.Fatalf("AddPersist: %v", err)
	}
	want := filepath.Join(dir, "keystone-br_netfilter.conf")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("persist file not created at expected path: %v", err)
	}
	exists, _ = p.PersistExists("br_netfilter")
	if !exists {
		t.Error("PersistExists should be true after AddPersist")
	}
	// File content should carry the module name.
	data, _ := os.ReadFile(want)
	if !strings.Contains(string(data), "br_netfilter") {
		t.Errorf("persist file content = %q, want module name", data)
	}
	if err := p.RemovePersist("br_netfilter"); err != nil {
		t.Fatalf("RemovePersist: %v", err)
	}
	exists, _ = p.PersistExists("br_netfilter")
	if exists {
		t.Error("PersistExists should be false after RemovePersist")
	}
	// RemovePersist on a missing file is a no-op.
	if err := p.RemovePersist("never_existed"); err != nil {
		t.Errorf("RemovePersist on missing should be nil; got %v", err)
	}
}

func TestLinuxProvider_Load_NoBinary(t *testing.T) {
	t.Parallel()
	p := &linuxProvider{modprobeBin: ""}
	if err := p.Load(context.Background(), "br_netfilter"); err == nil || !strings.Contains(err.Error(), "binary not found") {
		t.Errorf("err = %v, want binary-not-found", err)
	}
	if err := p.Unload(context.Background(), "br_netfilter"); err == nil {
		t.Error("Unload with no binary should error")
	}
}

func TestLinuxProvider_Load_UsesRunner(t *testing.T) {
	t.Parallel()
	var captured [][]string
	p := &linuxProvider{
		modprobeBin: "/sbin/modprobe",
		runner: func(_ context.Context, bin string, args []string) error {
			captured = append(captured, append([]string{bin}, args...))
			return nil
		},
	}
	ctx := context.Background()
	if err := p.Load(ctx, "br-netfilter"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := p.Unload(ctx, "br-netfilter"); err != nil {
		t.Fatalf("Unload: %v", err)
	}
	want := [][]string{
		{"/sbin/modprobe", "br_netfilter"},
		{"/sbin/modprobe", "-r", "br_netfilter"},
	}
	if len(captured) != len(want) {
		t.Fatalf("captured = %v", captured)
	}
	for i := range want {
		if len(captured[i]) != len(want[i]) {
			t.Errorf("call %d = %v, want %v", i, captured[i], want[i])
			continue
		}
		for j := range want[i] {
			if captured[i][j] != want[i][j] {
				t.Errorf("call %d arg %d = %q, want %q", i, j, captured[i][j], want[i][j])
			}
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

func TestPersistFilePath(t *testing.T) {
	t.Parallel()
	old := modulesLoadDir
	modulesLoadDir = "/etc/modules-load.d"
	defer func() { modulesLoadDir = old }()
	if got := persistFilePath("br-netfilter"); got != "/etc/modules-load.d/keystone-br_netfilter.conf" {
		t.Errorf("got %q", got)
	}
}

func TestWriteFileAtomic_BadParent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := writeFileAtomic(filepath.Join(blocker, "child"), []byte("data")); err == nil {
		t.Error("expected error writing under a non-dir")
	}
}
