// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDefaultDetector_Smoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1.0 platform target is Linux/Unix")
	}
	d := NewDefaultDetector(discardLogger())
	res, err := d.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if res.OS == "" {
		t.Error("OS empty")
	}
	if res.Architecture == "" {
		t.Error("Architecture empty")
	}
	// AgentInstalled depends on host state — we only assert the
	// AgentConfigPath default got set.
	if res.AgentConfigPath != defaultAgentConfigPath {
		t.Errorf("AgentConfigPath = %q, want %q", res.AgentConfigPath, defaultAgentConfigPath)
	}
}

func TestReadOSRelease_ParsesIDAndVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	body := []byte(`NAME="Ubuntu"
ID=ubuntu
VERSION_ID="24.04"
PRETTY_NAME="Ubuntu 24.04 LTS"
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	id, ver, err := readOSRelease(path)
	if err != nil {
		t.Fatalf("readOSRelease: %v", err)
	}
	if id != "ubuntu" {
		t.Errorf("id = %q, want ubuntu", id)
	}
	if ver != "24.04" {
		t.Errorf("ver = %q, want 24.04", ver)
	}
}

func TestReadOSRelease_HandlesMissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	if err := os.WriteFile(path, []byte("NAME=Custom\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	id, ver, err := readOSRelease(path)
	if err != nil {
		t.Fatalf("readOSRelease: %v", err)
	}
	if id != "" || ver != "" {
		t.Errorf("expected empty strings; got id=%q ver=%q", id, ver)
	}
}

func TestTrimQuotes(t *testing.T) {
	cases := map[string]string{
		`"ubuntu"`: "ubuntu",
		`'rhel'`:   "rhel",
		`alpine`:   "alpine",
		`"24.04"`:  "24.04",
		``:         "",
	}
	for in, want := range cases {
		if got := trimQuotes(in); got != want {
			t.Errorf("trimQuotes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDetectInitSystem_NonLinuxReturnsGOOS(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("test exercises the non-linux branch")
	}
	if got := detectInitSystem(); got != runtime.GOOS {
		t.Errorf("detectInitSystem on %s = %q, want %q", runtime.GOOS, got, runtime.GOOS)
	}
}

func TestDetectPackageManager_ReturnsKnownOrNone(t *testing.T) {
	got := detectPackageManager()
	switch got {
	case "apt-get", "dnf", "yum", "apk", "pacman", "zypper", "none":
		// expected — first found wins
	default:
		t.Errorf("detectPackageManager returned unexpected %q", got)
	}
}
