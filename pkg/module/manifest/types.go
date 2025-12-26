package manifest

import (
	"time"
)

// Manifest represents a module.yaml file
type Manifest struct {
	SchemaVersion int    `yaml:"schemaVersion"`
	Name          string `yaml:"name"`     // e.g., "vendor/pkg_apt"
	Version       string `yaml:"version"` // e.g., "v1.2.3"

	// Dependencies on other modules
	Dependencies []Dependency `yaml:"dependencies,omitempty"`

	// Required capabilities
	Capabilities Capabilities `yaml:"capabilities,omitempty"`

	// Resource limits
	Limits ResourceLimits `yaml:"limits,omitempty"`

	// Starlark entrypoints
	Starlark *StarlarkConfig `yaml:"starlark,omitempty"`

	// WASM configuration
	WASM *WASMConfig `yaml:"wasm,omitempty"`

	// Cryptographic signatures
	Signatures []Signature `yaml:"signatures,omitempty"`

	// Build metadata
	Build *BuildInfo `yaml:"build,omitempty"`
}

// Dependency represents a module dependency
type Dependency struct {
	Module  string `yaml:"module"`  // e.g., "std/files"
	Version string `yaml:"version"` // SemVer constraint, e.g., ">=1.0 <2.0"
}

// Capabilities defines host capabilities requested by the module
type Capabilities struct {
	// Filesystem capabilities
	FSRead  *FSCapability  `yaml:"fs.read,omitempty"`
	FSWrite *FSCapability  `yaml:"fs.write,omitempty"`

	// HTTP capabilities
	HTTPGet  *HTTPCapability `yaml:"http.get,omitempty"`
	HTTPPost *HTTPCapability `yaml:"http.post,omitempty"`

	// Execution capability
	Exec *ExecCapability `yaml:"exec,omitempty"`

	// Secrets capabilities
	SecretsRead  *SecretsCapability `yaml:"secrets.read,omitempty"`
	SecretsWrite *SecretsCapability `yaml:"secrets.write,omitempty"`

	// Logging capability
	Log *LogCapability `yaml:"log,omitempty"`

	// Time capability (breaks determinism!)
	Time bool `yaml:"time,omitempty"`

	// Key-value storage capability
	KV bool `yaml:"kv,omitempty"`
}

// FSCapability defines filesystem access
type FSCapability struct {
	AllowedPaths []string `yaml:"allowed_paths,omitempty"`
	DeniedPaths  []string `yaml:"denied_paths,omitempty"`
	MaxFileSize  string   `yaml:"max_file_size,omitempty"` // e.g., "100MB"
}

// HTTPCapability defines HTTP access
type HTTPCapability struct {
	AllowedDomains []string `yaml:"allowed_domains,omitempty"`
	DeniedDomains  []string `yaml:"denied_domains,omitempty"`
	TimeoutMax     string   `yaml:"timeout_max,omitempty"` // e.g., "30s"
	RateLimit      string   `yaml:"rate_limit,omitempty"`  // e.g., "100/minute"
}

// ExecCapability defines command execution access
type ExecCapability struct {
	AllowedCommands []string `yaml:"allowed_commands,omitempty"`
	TimeoutMax      string   `yaml:"timeout_max,omitempty"`
	CPULimit        string   `yaml:"cpu_limit,omitempty"`    // e.g., "100m"
	MemoryLimit     string   `yaml:"memory_limit,omitempty"` // e.g., "50MB"
}

// SecretsCapability defines secrets access
type SecretsCapability struct {
	AllowedPaths []string `yaml:"allowed_paths,omitempty"`
	AuditAll     bool     `yaml:"audit_all,omitempty"`
}

// LogCapability defines logging access
type LogCapability struct {
	RateLimit string `yaml:"rate_limit,omitempty"` // e.g., "100/minute"
}

// ResourceLimits defines execution resource constraints
type ResourceLimits struct {
	TimeMS    int    `yaml:"time_ms,omitempty"`    // Max execution time in milliseconds
	MemPages  int    `yaml:"mem_pages,omitempty"`  // WASM memory pages (64KB each)
	CPUShares int    `yaml:"cpu_shares,omitempty"` // CPU weight
	MaxSteps  uint64 `yaml:"max_steps,omitempty"`  // Max bytecode instructions
}

// StarlarkConfig defines Starlark module configuration
type StarlarkConfig struct {
	Entrypoints map[string]string `yaml:"entrypoints,omitempty"` // e.g., {"check": "states/verify.star:check"}
}

// WASMConfig defines WASM module configuration
type WASMConfig struct {
	Binary  string   `yaml:"binary"`            // Path to .wasm file, e.g., "providers/executor.wasm"
	Exports []string `yaml:"exports,omitempty"` // Exported functions, e.g., ["check", "apply"]
}

// Signature represents a cryptographic signature
type Signature struct {
	KeyID     string `yaml:"keyid"`               // Signing key identifier
	Algorithm string `yaml:"algorithm"`           // e.g., "cosign"
	Signature string `yaml:"signature,omitempty"` // Signature bytes (base64)
}

// BuildInfo contains module build metadata
type BuildInfo struct {
	Timestamp    time.Time `yaml:"timestamp,omitempty"`
	Reproducible bool      `yaml:"reproducible,omitempty"`
	Builder      string    `yaml:"builder,omitempty"` // e.g., "github.com/vendor/pkg_apt@v1.2.3"
}

// LockFile represents a module.lock file
type LockFile struct {
	SchemaVersion int            `yaml:"schemaVersion"`
	Modules       []LockedModule `yaml:"modules"`
}

// LockedModule represents a pinned module version
type LockedModule struct {
	Name     string    `yaml:"name"`     // e.g., "vendor/pkg_apt"
	Version  string    `yaml:"version"`  // Exact version, e.g., "v1.2.3"
	Hash     string    `yaml:"hash"`     // SHA256 hash
	Resolved time.Time `yaml:"resolved"` // When this was resolved
}

// Validate validates the manifest
func (m *Manifest) Validate() error {
	if m.SchemaVersion != 1 {
		return ErrInvalidSchemaVersion
	}
	if m.Name == "" {
		return ErrMissingName
	}
	if m.Version == "" {
		return ErrMissingVersion
	}

	// Validate dependencies
	for _, dep := range m.Dependencies {
		if dep.Module == "" {
			return ErrInvalidDependency
		}
		if dep.Version == "" {
			return ErrInvalidDependency
		}
	}

	// At least one runtime must be specified
	if m.Starlark == nil && m.WASM == nil {
		return ErrNoRuntime
	}

	return nil
}

// GetDependencyNames returns all dependency module names
func (m *Manifest) GetDependencyNames() []string {
	names := make([]string, len(m.Dependencies))
	for i, dep := range m.Dependencies {
		names[i] = dep.Module
	}
	return names
}

// HasCapability checks if a capability is requested
func (c *Capabilities) HasCapability(name string) bool {
	switch name {
	case "fs.read":
		return c.FSRead != nil
	case "fs.write":
		return c.FSWrite != nil
	case "http.get":
		return c.HTTPGet != nil
	case "http.post":
		return c.HTTPPost != nil
	case "exec":
		return c.Exec != nil
	case "secrets.read":
		return c.SecretsRead != nil
	case "secrets.write":
		return c.SecretsWrite != nil
	case "log":
		return c.Log != nil
	case "time":
		return c.Time
	case "kv":
		return c.KV
	default:
		return false
	}
}

// ListCapabilities returns all requested capabilities
func (c *Capabilities) ListCapabilities() []string {
	var caps []string

	if c.FSRead != nil {
		caps = append(caps, "fs.read")
	}
	if c.FSWrite != nil {
		caps = append(caps, "fs.write")
	}
	if c.HTTPGet != nil {
		caps = append(caps, "http.get")
	}
	if c.HTTPPost != nil {
		caps = append(caps, "http.post")
	}
	if c.Exec != nil {
		caps = append(caps, "exec")
	}
	if c.SecretsRead != nil {
		caps = append(caps, "secrets.read")
	}
	if c.SecretsWrite != nil {
		caps = append(caps, "secrets.write")
	}
	if c.Log != nil {
		caps = append(caps, "log")
	}
	if c.Time {
		caps = append(caps, "time")
	}
	if c.KV {
		caps = append(caps, "kv")
	}

	return caps
}
