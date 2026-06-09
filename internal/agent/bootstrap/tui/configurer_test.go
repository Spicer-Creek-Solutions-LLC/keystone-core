// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/agent/bootstrap"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewConfigurer_FillsDefaults(t *testing.T) {
	c := NewConfigurer(Defaults{}, quietLogger())
	if c == nil {
		t.Fatal("NewConfigurer returned nil")
		return
	}
	if c.defaults.NodeRole != defaultNodeRole {
		t.Errorf("NodeRole = %q, want %q", c.defaults.NodeRole, defaultNodeRole)
	}
	if c.defaults.ConfigPath != defaultConfigPath {
		t.Errorf("ConfigPath = %q, want %q", c.defaults.ConfigPath, defaultConfigPath)
	}
	if c.defaults.JoinURL != defaultJoinURL {
		t.Errorf("JoinURL = %q, want %q", c.defaults.JoinURL, defaultJoinURL)
	}
	if c.defaults.AgentID == "" {
		t.Error("AgentID empty; expected hostname fallback")
	}
}

func TestNewConfigurer_PreservesProvidedDefaults(t *testing.T) {
	d := Defaults{
		ClusterName: "prod",
		AgentID:     "agent-7",
		NodeRole:    "db",
		JoinURL:     "tls://nats.example:4443",
		ConfigPath:  "/srv/agent.yaml",
	}
	c := NewConfigurer(d, quietLogger())
	if c.defaults != d {
		t.Errorf("defaults = %+v, want %+v", c.defaults, d)
	}
}

func TestNewConfigurer_NilLoggerFallsBackToDefault(t *testing.T) {
	c := NewConfigurer(Defaults{}, nil)
	if c.log == nil {
		t.Error("log nil after construction")
	}
}

func TestSeedValues_StartsAtDemo(t *testing.T) {
	c := NewConfigurer(Defaults{ClusterName: "default", AgentID: "a"}, quietLogger())
	v := c.seedValues(nil)
	if v.Mode != string(bootstrap.ModeDemo) {
		t.Errorf("Mode = %q, want %q", v.Mode, bootstrap.ModeDemo)
	}
	if v.ClusterName != "default" {
		t.Errorf("ClusterName seed = %q, want default", v.ClusterName)
	}
}

func TestBuildForm_NotNil(t *testing.T) {
	c := NewConfigurer(Defaults{ClusterName: "x", AgentID: "a"}, quietLogger())
	v := c.seedValues(nil)
	if got := c.buildForm(nil, &v); got == nil {
		t.Fatal("buildForm returned nil")
	}
}

func TestBuildIntroNote(t *testing.T) {
	if buildIntroNote(nil) == nil {
		t.Error("nil detection should still produce a note")
	}
	full := &bootstrap.DetectionResult{
		OS:             "linux",
		Distro:         "ubuntu",
		DistroVersion:  "24.04",
		KernelVersion:  "6.8.0",
		Architecture:   "amd64",
		InitSystem:     "systemd",
		PackageManager: "apt-get",
	}
	if buildIntroNote(full) == nil {
		t.Error("populated detection produced nil note")
	}
}

func TestFallback(t *testing.T) {
	if fallback("", "fb") != "fb" {
		t.Error("empty -> fallback failed")
	}
	if fallback("real", "fb") != "real" {
		t.Error("non-empty -> fallback should be no-op")
	}
}

// configurationFromValues is the mode-gate + Configuration
// validation Configure runs after the form returns. Tested
// directly so we don't need to drive the interactive form to
// verify production / enterprise rejection — the wizard's
// acceptance bullet is verified manually per the epic.
func TestConfigurationFromValues_DemoModeAccepted(t *testing.T) {
	v := formValues{
		Mode:        string(bootstrap.ModeDemo),
		ClusterName: "default",
		AgentID:     "agent-1",
		NodeRole:    "worker",
		JoinURL:     "nats://server:4222",
		ConfigPath:  "/etc/keystone-core/keystone-core-agent.yaml",
	}
	cfg, err := configurationFromValues(v)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if cfg.Mode != bootstrap.ModeDemo {
		t.Errorf("Mode = %q, want %q", cfg.Mode, bootstrap.ModeDemo)
	}
}

func TestConfigurationFromValues_NonDemoModesRejected(t *testing.T) {
	for _, mode := range []string{
		string(bootstrap.ModeProduction),
		string(bootstrap.ModeEnterprise),
	} {
		t.Run(mode, func(t *testing.T) {
			v := formValues{
				Mode:        mode,
				ClusterName: "x",
				AgentID:     "a",
				ConfigPath:  "/p",
			}
			_, err := configurationFromValues(v)
			if err == nil {
				t.Fatalf("mode %q accepted; want rejection", mode)
			}
			if !strings.Contains(err.Error(), "v1.x") {
				t.Errorf("err = %v, want it to mention v1.x deferral", err)
			}
		})
	}
}

func TestConfigurationFromValues_InvalidConfigBubblesUp(t *testing.T) {
	// Demo mode but missing ClusterName — Configuration.Validate
	// rejects it. The wizard's per-field validators catch this
	// up-front in the interactive flow; this asserts the safety
	// net for a programmatic caller.
	v := formValues{
		Mode:       string(bootstrap.ModeDemo),
		AgentID:    "a",
		ConfigPath: "/p",
	}
	if _, err := configurationFromValues(v); err == nil {
		t.Error("missing cluster name accepted; want rejection")
	}
}
