// SPDX-License-Identifier: Apache-2.0

package systemd

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
)

// DefaultUnitName is the canonical filename. Operators override
// via Options.UnitPath if they're staging multiple agents on one
// host (rare but supported).
const DefaultUnitName = "kscore-agent.service"

// DefaultUnitDir is where systemd reads operator-installed units.
// /etc/systemd/system overrides /lib/systemd/system, so we install
// here regardless of distro.
const DefaultUnitDir = "/etc/systemd/system"

// DefaultBinaryPath is where the kscore-agent .deb / .rpm installs
// the agent binary (FHS-canonical /usr/bin for distro packages).
// Operators can override via Params.BinaryPath when staging from a
// different directory.
const DefaultBinaryPath = "/usr/bin/kscore-agent"

// DefaultConfigPath matches the bootstrap installer's render
// target (bootstrap.DefaultAgentConfigPath) so a `kscore-agent
// bootstrap` followed by `kscore-agent service install` lines up by
// default. The two packages stay decoupled; the equality is enforced
// by bootstrap/paths_invariant_test.go.
const DefaultConfigPath = "/etc/kscore/agent.yaml"

// DefaultReadWritePaths is the set systemd's ProtectSystem=strict
// hardening exempts. The agent writes its state dir + log dir
// under /var; everything else stays read-only.
var DefaultReadWritePaths = []string{
	"/var/lib/kscore-agent",
	"/var/log/kscore-agent",
}

// Params captures every operator-tunable field on the rendered
// unit. Zero values fall back to the Default* constants above —
// see fillDefaults.
type Params struct {
	// BinaryPath is the absolute path to the kscore-agent binary.
	// Defaults to /usr/bin/kscore-agent.
	BinaryPath string

	// ConfigPath is the --config flag passed to the daemon.
	// Defaults to /etc/kscore/agent.yaml.
	ConfigPath string

	// User and Group are optional. When empty, systemd runs the
	// service as root. v1.0 documents `keystone-core` as the
	// recommended dedicated user once the operator creates one
	// (no auto-creation in v1.0; tracked in ROADMAP).
	User  string
	Group string

	// Description overrides the default Description= line.
	Description string

	// ReadWritePaths overrides the default RW exemption list.
	// Useful when operators relocate state/log dirs.
	ReadWritePaths []string

	// ExtraEnv adds Environment= lines. Each entry should be
	// "KEY=VALUE". Operators wire systemd-style secrets via
	// EnvironmentFile (Params.EnvironmentFile below) instead of
	// embedding plaintext here.
	ExtraEnv []string

	// EnvironmentFile points at a key=value file systemd reads
	// before launching. Optional. The file must exist before
	// systemctl start, otherwise systemd refuses with NRES.
	EnvironmentFile string
}

// fillDefaults applies the Default* constants to zero-valued
// fields. Returns a copy so the caller's Params is untouched.
func (p Params) fillDefaults() Params {
	if p.BinaryPath == "" {
		p.BinaryPath = DefaultBinaryPath
	}
	if p.ConfigPath == "" {
		p.ConfigPath = DefaultConfigPath
	}
	if p.Description == "" {
		p.Description = "Keystone Core Agent"
	}
	if len(p.ReadWritePaths) == 0 {
		p.ReadWritePaths = append([]string(nil), DefaultReadWritePaths...)
	}
	return p
}

// validate enforces invariants Render must respect. Both paths
// must be absolute — systemd refuses to load units with relative
// ExecStart anyway, but failing here gives a friendlier error.
func (p Params) validate() error {
	if !filepath.IsAbs(p.BinaryPath) {
		return fmt.Errorf("systemd: BinaryPath must be absolute (got %q)", p.BinaryPath)
	}
	if !filepath.IsAbs(p.ConfigPath) {
		return fmt.Errorf("systemd: ConfigPath must be absolute (got %q)", p.ConfigPath)
	}
	if (p.User == "") != (p.Group == "") {
		return errors.New("systemd: User and Group must be set together (or both empty for root)")
	}
	for _, e := range p.ExtraEnv {
		if !strings.Contains(e, "=") {
			return fmt.Errorf("systemd: ExtraEnv entry %q must be KEY=VALUE", e)
		}
	}
	return nil
}

// Render builds the kscore-agent.service body. Caller
// writes the bytes to disk via Install (which handles
// atomic-write + daemon-reload).
func Render(p Params) ([]byte, error) {
	p = p.fillDefaults()
	if err := p.validate(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := unitTmpl.Execute(&buf, p); err != nil {
		return nil, fmt.Errorf("systemd: render unit: %w", err)
	}
	return buf.Bytes(), nil
}

// unitTmpl is the canonical v1.0 unit. Hardening is aggressive
// by default (ProtectSystem=strict + ProtectHome + PrivateTmp +
// kernel + cgroup protections); operators override via
// Params.ReadWritePaths when state dirs land outside /var/lib.
//
// Type=exec (not notify) — sd_notify integration would require
// the daemon to call SdNotifyReady. Tracked in ROADMAP; flip
// when sd_notify lands.
var unitTmpl = template.Must(template.New("unit").Parse(`[Unit]
Description={{.Description}}
Documentation=https://keystone-core.io
After=network-online.target
Wants=network-online.target

[Service]
Type=exec
ExecStart={{.BinaryPath}} --config {{.ConfigPath}}
Restart=on-failure
RestartSec=5s
{{- if .User}}
User={{.User}}
Group={{.Group}}
{{- end}}
{{- if .EnvironmentFile}}
EnvironmentFile={{.EnvironmentFile}}
{{- end}}
{{- range .ExtraEnv}}
Environment={{.}}
{{- end}}
StateDirectory=kscore-agent
WorkingDirectory=/var/lib/kscore-agent

# Hardening (v1.0 secure-by-default).
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
{{- if .ReadWritePaths}}
ReadWritePaths={{range $i, $p := .ReadWritePaths}}{{if $i}} {{end}}{{$p}}{{end}}
{{- end}}

[Install]
WantedBy=multi-user.target
`))
