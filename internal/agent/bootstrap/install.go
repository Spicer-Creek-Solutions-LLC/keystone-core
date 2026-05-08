package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"go.yaml.in/yaml/v3"
)

// InstallResult is the Install phase's output. Created/Updated
// flags let operators distinguish "first install" from "config
// converged" on a re-run — useful for the §4.6 idempotency
// promise.
type InstallResult struct {
	ConfigPath   string `json:"config_path"`
	BytesWritten int    `json:"bytes_written"`
	Created      bool   `json:"created"`
	Updated      bool   `json:"updated"`
}

// Installer applies the side effects of bootstrap (config file +,
// in production mode, systemd unit + certs). v1.0 demo mode writes
// the agent config file only.
type Installer interface {
	Install(ctx context.Context, cfg *Configuration) (*InstallResult, error)
}

// NewDefaultInstaller returns the v1.0 demo-mode installer.
// Production-mode systemd unit installation lands in Task 9 by
// adding a wrapper around this default + an InstallSystemdUnit
// step.
func NewDefaultInstaller(log *slog.Logger) Installer {
	if log == nil {
		log = slog.Default()
	}
	return &defaultInstaller{log: log}
}

type defaultInstaller struct {
	log *slog.Logger
}

// agentConfigYAML is the minimal kscore-agent config rendered from
// Configuration. Field names match internal/config.Config so
// internal/config.Load consumes the file unchanged after Install.
type agentConfigYAML struct {
	Mode  string `yaml:"mode"`
	Agent struct {
		ID                string `yaml:"id"`
		HeartbeatInterval string `yaml:"heartbeatinterval,omitempty"`
		MetadataInterval  string `yaml:"metadatainterval,omitempty"`
		CommandTimeout    string `yaml:"commandtimeout,omitempty"`
	} `yaml:"agent"`
	NATS struct {
		Mode        string   `yaml:"mode"`
		URLs        []string `yaml:"urls,omitempty"`
		ClusterName string   `yaml:"clustername"`
	} `yaml:"nats"`
	Security struct {
		DefaultPolicy string `yaml:"defaultpolicy,omitempty"`
	} `yaml:"security"`
}

func (i *defaultInstaller) Install(ctx context.Context, cfg *Configuration) (*InstallResult, error) {
	if cfg == nil {
		return nil, errors.New("bootstrap: Install: nil Configuration")
	}
	if cfg.ConfigPath == "" {
		return nil, errors.New("bootstrap: Install: ConfigPath is empty")
	}

	if cfg.DryRun {
		i.log.InfoContext(ctx, "bootstrap: Install (dry run — no side effects)",
			"config_path", cfg.ConfigPath, "mode", string(cfg.Mode))
		return &InstallResult{ConfigPath: cfg.ConfigPath}, nil
	}

	body, err := renderAgentConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: render config: %w", err)
	}

	existing, statErr := os.ReadFile(cfg.ConfigPath) //nolint:gosec // operator-controlled path
	created := errors.Is(statErr, os.ErrNotExist)
	updated := false
	switch {
	case created:
		// new install — write
	case statErr != nil:
		return nil, fmt.Errorf("bootstrap: read existing config %q: %w", cfg.ConfigPath, statErr)
	case bytes.Equal(existing, body):
		// idempotent — content unchanged
		i.log.InfoContext(ctx, "bootstrap: Install (no-op; config converged)",
			"config_path", cfg.ConfigPath)
		return &InstallResult{
			ConfigPath:   cfg.ConfigPath,
			BytesWritten: len(existing),
		}, nil
	default:
		updated = true
	}

	if err := atomicWriteFile(cfg.ConfigPath, body, 0o640); err != nil {
		return nil, fmt.Errorf("bootstrap: write config %q: %w", cfg.ConfigPath, err)
	}

	i.log.InfoContext(ctx, "bootstrap: Install wrote config",
		"config_path", cfg.ConfigPath,
		"bytes", len(body),
		"created", created,
		"updated", updated,
	)
	return &InstallResult{
		ConfigPath:   cfg.ConfigPath,
		BytesWritten: len(body),
		Created:      created,
		Updated:      updated,
	}, nil
}

// renderAgentConfig builds the YAML body for the agent config file.
// Demo-mode-friendly defaults: external NATS pointing at JoinURL
// (or empty if not set), DefaultPolicy=allow (so demo agents accept
// commands without HMAC config; operators tighten in production).
func renderAgentConfig(cfg *Configuration) ([]byte, error) {
	var doc agentConfigYAML
	doc.Mode = "development"
	doc.Agent.ID = cfg.AgentID
	doc.NATS.ClusterName = cfg.ClusterName
	doc.NATS.Mode = "external"
	if cfg.JoinURL != "" {
		doc.NATS.URLs = []string{cfg.JoinURL}
	}
	doc.Security.DefaultPolicy = "allow" // v1.0 demo; operators set deny for production

	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// atomicWriteFile = temp+rename. Same defensive pattern as
// State.Save. Pulled out so future installer extensions (systemd
// unit file, certs) reuse it.
func atomicWriteFile(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(path), err)
	}
	tmp := path + ".tmp." + strconv.Itoa(os.Getpid()) + "." + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, body, mode); err != nil {
		return fmt.Errorf("write temp %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename temp into place: %w", err)
	}
	return nil
}
