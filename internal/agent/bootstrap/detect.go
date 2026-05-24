// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/host"
)

// DetectionResult is the output of the Detect phase. Captured into
// State.Detection for later phases (Validate, Install) to inspect.
type DetectionResult struct {
	OS              string `json:"os"`
	Distro          string `json:"distro,omitempty"`
	DistroVersion   string `json:"distro_version,omitempty"`
	KernelVersion   string `json:"kernel_version,omitempty"`
	Architecture    string `json:"architecture,omitempty"`
	InitSystem      string `json:"init_system,omitempty"`
	PackageManager  string `json:"package_manager,omitempty"`
	AgentInstalled  bool   `json:"agent_installed"`
	AgentConfigPath string `json:"agent_config_path,omitempty"`
}

// Detector probes the host for bootstrap-relevant facts. Pluggable
// so tests can inject canned results.
type Detector interface {
	Detect(ctx context.Context) (*DetectionResult, error)
}

// defaultAgentConfigPath is the v1.0 location for the agent's
// runtime config. PROJECT-DETAILS §4.6 lists this as the on-disk
// layout default.
const defaultAgentConfigPath = "/etc/keystone-core/keystone-core-agent.yaml"

// NewDefaultDetector returns the production Detector. gopsutil
// supplies kernel + arch; /etc/os-release supplies distro family
// + version on Linux; init-system + package-manager probes look
// for marker files / binaries.
func NewDefaultDetector(log *slog.Logger) Detector {
	if log == nil {
		log = slog.Default()
	}
	return &defaultDetector{log: log}
}

type defaultDetector struct {
	log *slog.Logger
}

func (d *defaultDetector) Detect(ctx context.Context) (*DetectionResult, error) {
	out := &DetectionResult{
		OS:              runtime.GOOS,
		Architecture:    runtime.GOARCH,
		AgentConfigPath: defaultAgentConfigPath,
	}

	if hi, err := host.InfoWithContext(ctx); err != nil {
		d.log.Warn("bootstrap: host.Info", "err", err)
	} else if hi != nil {
		if hi.OS != "" {
			out.OS = hi.OS
		}
		out.Distro = hi.Platform
		out.DistroVersion = hi.PlatformVersion
		out.KernelVersion = hi.KernelVersion
		if hi.KernelArch != "" {
			out.Architecture = hi.KernelArch
		}
	}

	// /etc/os-release is the canonical Linux distro identity. gopsutil
	// already reads it on Linux, but the parsed fields don't always
	// align with what Install needs (e.g., distro family for package
	// repo selection). Re-read here so the result is explicit.
	if runtime.GOOS == "linux" {
		if os, ver, err := readOSRelease("/etc/os-release"); err == nil {
			if os != "" {
				out.Distro = os
			}
			if ver != "" {
				out.DistroVersion = ver
			}
		}
	}

	out.InitSystem = detectInitSystem()
	out.PackageManager = detectPackageManager()

	if _, err := os.Stat(defaultAgentConfigPath); err == nil {
		out.AgentInstalled = true
	} else if !errors.Is(err, os.ErrNotExist) {
		d.log.Warn("bootstrap: stat agent config",
			"path", defaultAgentConfigPath, "err", err)
	}

	return out, nil
}

// readOSRelease parses /etc/os-release for the ID + VERSION_ID
// fields. Returns ("", "", error) on read failure; ("", "", nil)
// when the file exists but the fields are absent.
func readOSRelease(path string) (string, string, error) {
	f, err := os.Open(path) //nolint:gosec // /etc/os-release is the canonical OS-distro file
	if err != nil {
		return "", "", err
	}
	defer func() { _ = f.Close() }()
	var id, versionID string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "ID="):
			id = trimQuotes(strings.TrimPrefix(line, "ID="))
		case strings.HasPrefix(line, "VERSION_ID="):
			versionID = trimQuotes(strings.TrimPrefix(line, "VERSION_ID="))
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", fmt.Errorf("scan %q: %w", path, err)
	}
	return id, versionID, nil
}

func trimQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[0] == s[len(s)-1] {
		return s[1 : len(s)-1]
	}
	return s
}

// detectInitSystem returns "systemd", "openrc", "none" (no
// recognizable init), or "unknown" (probe failed). v1.0 platform
// is Linux + systemd; non-systemd Linuxes are detected so install
// can refuse cleanly rather than producing a broken systemd unit.
func detectInitSystem() string {
	if runtime.GOOS != "linux" {
		return runtime.GOOS // "darwin", "windows", etc.
	}
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return "systemd"
	}
	if _, err := os.Stat("/sbin/openrc-run"); err == nil {
		return "openrc"
	}
	if _, err := os.Stat("/proc/1/comm"); err == nil {
		b, err := os.ReadFile("/proc/1/comm") //nolint:gosec // /proc/1/comm is canonical PID-1 lookup
		if err == nil {
			name := strings.TrimSpace(string(b))
			if name == "systemd" {
				return "systemd"
			}
			if name == "openrc-init" {
				return "openrc"
			}
		}
	}
	return "unknown"
}

// detectPackageManager returns the first matched binary by lookpath.
// Used by Install (later tasks) to pick the right repo-setup
// procedure.
func detectPackageManager() string {
	for _, candidate := range []string{"apt-get", "dnf", "yum", "apk", "pacman", "zypper"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return "none"
}

