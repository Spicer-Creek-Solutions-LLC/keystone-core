// SPDX-License-Identifier: Apache-2.0

//go:build linux

package hostname

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxProvider_Current_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostname")
	if err := os.WriteFile(path, []byte("web-1\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	old := etcHostnamePath
	etcHostnamePath = path
	defer func() { etcHostnamePath = old }()

	p := &linuxProvider{}
	cur, set, err := p.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !set || cur != "web-1" {
		t.Errorf("cur=%q set=%v, want \"web-1\"/true", cur, set)
	}
}

func TestLinuxProvider_Current_Missing(t *testing.T) {
	old := etcHostnamePath
	etcHostnamePath = filepath.Join(t.TempDir(), "nope")
	defer func() { etcHostnamePath = old }()
	p := &linuxProvider{}
	_, set, err := p.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if set {
		t.Error("missing file should report set=false")
	}
}

func TestLinuxProvider_Set_PrefersHostnamectl(t *testing.T) {
	t.Parallel()
	oldLook := lookPath
	lookPath = func(name string) (string, error) {
		if name == "hostnamectl" {
			return "/usr/bin/hostnamectl", nil
		}
		return "", errors.New("not found")
	}
	defer func() { lookPath = oldLook }()

	var captured []string
	p := &linuxProvider{runner: func(_ context.Context, bin string, args []string) error {
		captured = append([]string{bin}, args...)
		return nil
	}}
	if err := p.Set(context.Background(), "web-1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"/usr/bin/hostnamectl", "set-hostname", "web-1"}
	if len(captured) != len(want) {
		t.Fatalf("captured = %v", captured)
	}
	for i := range want {
		if captured[i] != want[i] {
			t.Errorf("captured[%d] = %q, want %q", i, captured[i], want[i])
		}
	}
}

func TestLinuxProvider_Set_FallbackWritesFileAndCallsHostname(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostname")
	old := etcHostnamePath
	etcHostnamePath = path
	defer func() { etcHostnamePath = old }()

	oldLook := lookPath
	lookPath = func(name string) (string, error) {
		if name == "hostname" {
			return "/bin/hostname", nil
		}
		return "", errors.New("not found") // no hostnamectl
	}
	defer func() { lookPath = oldLook }()

	var captured []string
	p := &linuxProvider{runner: func(_ context.Context, bin string, args []string) error {
		captured = append([]string{bin}, args...)
		return nil
	}}
	if err := p.Set(context.Background(), "web-1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.TrimSpace(string(data)) != "web-1" {
		t.Errorf("/etc/hostname content = %q, want web-1", data)
	}
	want := []string{"/bin/hostname", "web-1"}
	if len(captured) != len(want) || captured[0] != want[0] || captured[1] != want[1] {
		t.Errorf("captured = %v, want %v", captured, want)
	}
}

func TestLinuxProvider_Set_NoToolsAtAll(t *testing.T) {
	dir := t.TempDir()
	old := etcHostnamePath
	etcHostnamePath = filepath.Join(dir, "hostname")
	defer func() { etcHostnamePath = old }()
	oldLook := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = oldLook }()
	p := &linuxProvider{runner: func(context.Context, string, []string) error { return nil }}
	err := p.Set(context.Background(), "web-1")
	if err == nil || !strings.Contains(err.Error(), "neither hostnamectl nor hostname") {
		t.Errorf("err = %v, want no-tools error", err)
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
