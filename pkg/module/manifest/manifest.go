// Package manifest defines the module manifest, per-capability
// config, and lockfile types for the Epic 14 plugin/module system,
// with a stable YAML codec and validation.
//
// Scope (Epic 14 task 1): types + codec + validation only. Capability
// enforcement, verification, resolution, the registry, the loader
// pipeline, and the Starlark runtime are later tasks.
//
// v0.1 module system is Starlark-only (PROJECT-DETAILS §4.18); the
// `wasm` type is reserved in the schema but rejected by validation
// until v1.1.
package manifest

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v3"

	"go.keystone-core.io/keystone-core/pkg/semver"
)

// ModuleType is the module runtime kind.
type ModuleType string

const (
	// TypeStarlark is the only runtime supported in v1.0.
	TypeStarlark ModuleType = "starlark"
	// TypeWASM is reserved (v1.1); validation rejects it for now.
	TypeWASM ModuleType = "wasm"
)

// Capability names — the 9 core v1.0 capabilities (PROJECT-DETAILS
// §4.18). Manifests may only request these.
const (
	CapFSRead       = "fs.read"
	CapFSWrite      = "fs.write"
	CapHTTPGet      = "http.get"
	CapHTTPPost     = "http.post"
	CapExec         = "exec"
	CapSecretsRead  = "secrets.read"
	CapSecretsWrite = "secrets.write"
	CapKV           = "kv"
	CapLog          = "log"
)

// KnownCapability reports whether name is one of the 9 core v1.0
// capabilities. Single source of truth for the capability registry
// + backends (Epic 14 tasks 2/3).
func KnownCapability(name string) bool {
	switch name {
	case CapFSRead, CapFSWrite, CapHTTPGet, CapHTTPPost, CapExec,
		CapSecretsRead, CapSecretsWrite, CapKV, CapLog:
		return true
	default:
		return false
	}
}

// CapabilityConfig is the superset of per-capability scoping knobs.
// Only the fields relevant to a given capability are meaningful;
// unset fields are omitted from YAML. Sizes/rates/durations are kept
// as their authored strings (the loader/runtime consume the parsed
// forms) but are validated here.
type CapabilityConfig struct {
	// fs.read / fs.write
	Paths       []string `yaml:"paths,omitempty"`
	DeniedPaths []string `yaml:"denied_paths,omitempty"`
	MaxFileSize string   `yaml:"max_file_size,omitempty"`

	// http.get / http.post
	Domains         []string `yaml:"domains,omitempty"`
	MaxRequestSize  string   `yaml:"max_request_size,omitempty"`
	MaxResponseSize string   `yaml:"max_response_size,omitempty"`

	// exec
	Commands   []string `yaml:"commands,omitempty"`
	WorkingDir string   `yaml:"working_dir,omitempty"`

	// secrets.read / secrets.write
	SecretPaths []string `yaml:"secret_paths,omitempty"`

	// shared
	RateLimit string `yaml:"rate_limit,omitempty"` // "<n>/<s|m|h>"
	Timeout   string `yaml:"timeout,omitempty"`    // Go duration
}

// Limits are the module-wide resource bounds.
type Limits struct {
	Timeout string  `yaml:"timeout,omitempty"` // Go duration
	Memory  string  `yaml:"memory,omitempty"`  // size string, e.g. 64MB
	CPU     float64 `yaml:"cpu,omitempty"`     // fractional cores
}

// Manifest is the module manifest (the `module.yaml` artifact).
type Manifest struct {
	Name         string                      `yaml:"name"`
	Version      string                      `yaml:"version"`
	Type         ModuleType                  `yaml:"type"`
	Entrypoint   string                      `yaml:"entrypoint"`
	Description  string                      `yaml:"description,omitempty"`
	Author       string                      `yaml:"author,omitempty"`
	License      string                      `yaml:"license,omitempty"`
	Capabilities map[string]CapabilityConfig `yaml:"capabilities,omitempty"`
	Limits       Limits                      `yaml:"limits,omitempty"`
	Dependencies map[string]string           `yaml:"dependencies,omitempty"`
}

// nameRE is the namespaced `vendor/pkg` form: two lowercase
// dns-ish segments separated by a single slash (registry enforces
// namespacing — guards namespace squatting).
var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._-]*$`)

// ValidModuleName reports whether name is a well-formed namespaced
// `vendor/pkg` module name. Single source of truth for the registry
// + publish path (Epic 14 task 8), shared with [Manifest.Validate].
func ValidModuleName(name string) bool { return nameRE.MatchString(name) }

// MarshalManifest renders m as YAML.
func MarshalManifest(m *Manifest) ([]byte, error) {
	if m == nil {
		return nil, fmt.Errorf("manifest: nil manifest")
	}
	return yaml.Marshal(m)
}

// UnmarshalManifest parses a manifest from YAML (no validation —
// call Validate separately).
func UnmarshalManifest(b []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("manifest: parse: %w", err)
	}
	return &m, nil
}

// Validate checks structural + semantic invariants. It does not
// resolve dependencies (that is the resolver's job) — only that
// each constraint string is well-formed.
func (m *Manifest) Validate() error {
	if m == nil {
		return fmt.Errorf("manifest: nil manifest")
	}
	if !nameRE.MatchString(m.Name) {
		return fmt.Errorf("manifest: name %q must be namespaced lowercase vendor/pkg", m.Name)
	}
	if _, err := semver.Parse(m.Version); err != nil {
		return fmt.Errorf("manifest: version %q is not valid semver: %w", m.Version, err)
	}
	switch m.Type {
	case TypeStarlark:
		// ok
	case TypeWASM:
		return fmt.Errorf("manifest: type %q is reserved for v1.1; v1.0 is starlark-only", m.Type)
	default:
		return fmt.Errorf("manifest: type %q unknown (want %q)", m.Type, TypeStarlark)
	}
	if strings.TrimSpace(m.Entrypoint) == "" {
		return fmt.Errorf("manifest: entrypoint is required")
	}
	for name, cc := range m.Capabilities {
		if !KnownCapability(name) {
			return fmt.Errorf("manifest: unknown capability %q", name)
		}
		if err := cc.validate(name); err != nil {
			return fmt.Errorf("manifest: capability %q: %w", name, err)
		}
	}
	if err := m.Limits.validate(); err != nil {
		return fmt.Errorf("manifest: limits: %w", err)
	}
	for dep, constraint := range m.Dependencies {
		if !nameRE.MatchString(dep) {
			return fmt.Errorf("manifest: dependency name %q must be namespaced vendor/pkg", dep)
		}
		if _, err := semver.NewConstraint(constraint); err != nil {
			return fmt.Errorf("manifest: dependency %q constraint %q invalid: %w", dep, constraint, err)
		}
	}
	return nil
}

func (c CapabilityConfig) validate(_ string) error {
	for label, v := range map[string]string{
		"max_file_size":     c.MaxFileSize,
		"max_request_size":  c.MaxRequestSize,
		"max_response_size": c.MaxResponseSize,
	} {
		if v == "" {
			continue
		}
		if _, err := ParseSize(v); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
	}
	if c.RateLimit != "" {
		if _, _, err := ParseRate(c.RateLimit); err != nil {
			return fmt.Errorf("rate_limit: %w", err)
		}
	}
	if c.Timeout != "" {
		if _, err := time.ParseDuration(c.Timeout); err != nil {
			return fmt.Errorf("timeout: %w", err)
		}
	}
	return nil
}

func (l Limits) validate() error {
	if l.Timeout != "" {
		if _, err := time.ParseDuration(l.Timeout); err != nil {
			return fmt.Errorf("timeout: %w", err)
		}
	}
	if l.Memory != "" {
		if _, err := ParseSize(l.Memory); err != nil {
			return fmt.Errorf("memory: %w", err)
		}
	}
	if l.CPU < 0 {
		return fmt.Errorf("cpu %g must be >= 0", l.CPU)
	}
	return nil
}
