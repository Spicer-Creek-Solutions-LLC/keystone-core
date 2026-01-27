// Package injection provides secret injection mechanisms for delivering secrets
// to applications through files, environment variables, and templates.
package injection

import (
	"context"
	"os"
	"time"
)

// InjectionType represents the type of secret injection.
type InjectionType string

const (
	// InjectionTypeFile injects secrets as files.
	InjectionTypeFile InjectionType = "file"

	// InjectionTypeEnv injects secrets as environment variables.
	InjectionTypeEnv InjectionType = "env"

	// InjectionTypeTemplate renders secrets into templates.
	InjectionTypeTemplate InjectionType = "template"
)

// SecretSource provides secrets for injection.
type SecretSource interface {
	// GetSecret retrieves a secret by path.
	GetSecret(ctx context.Context, path string) (*Secret, error)

	// WatchSecret watches for secret updates.
	WatchSecret(ctx context.Context, path string) (<-chan *Secret, error)
}

// Secret represents a secret to be injected.
type Secret struct {
	// Path is the secret path in the backend.
	Path string

	// Data contains the secret data as key-value pairs.
	Data map[string]interface{}

	// Version is the secret version.
	Version int

	// ExpiresAt is when the secret expires (for dynamic secrets).
	ExpiresAt time.Time

	// Metadata contains additional secret metadata.
	Metadata map[string]string
}

// GetString retrieves a string value from the secret data.
func (s *Secret) GetString(key string) string {
	if s.Data == nil {
		return ""
	}
	if v, ok := s.Data[key]; ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return ""
}

// GetBytes retrieves binary data from the secret.
func (s *Secret) GetBytes(key string) []byte {
	if s.Data == nil {
		return nil
	}
	if v, ok := s.Data[key]; ok {
		switch val := v.(type) {
		case []byte:
			return val
		case string:
			return []byte(val)
		}
	}
	return nil
}

// InjectionConfig is the base configuration for all injection types.
type InjectionConfig struct {
	// Enabled indicates if injection is enabled.
	Enabled bool `yaml:"enabled"`

	// RefreshInterval is how often to check for secret updates.
	RefreshInterval time.Duration `yaml:"refresh_interval"`
}

// FileInjectionConfig configures file-based secret injection.
type FileInjectionConfig struct {
	InjectionConfig `yaml:",inline"`

	// BasePath is the base directory for secret files.
	BasePath string `yaml:"base_path"`

	// DefaultMode is the default file permissions.
	DefaultMode os.FileMode `yaml:"mode"`

	// DefaultOwner is the default file owner (user).
	DefaultOwner string `yaml:"owner"`

	// DefaultGroup is the default file group.
	DefaultGroup string `yaml:"group"`

	// AtomicWrite enables atomic file writes (temp + rename).
	AtomicWrite bool `yaml:"atomic_write"`

	// Files is the list of file injection rules.
	Files []FileRule `yaml:"files"`
}

// FileRule defines how a secret is injected into a file.
type FileRule struct {
	// SecretPath is the path to the secret in the backend.
	SecretPath string `yaml:"secret_path"`

	// SecretKey is the specific key within the secret (optional).
	SecretKey string `yaml:"secret_key"`

	// FilePath is the destination file path.
	FilePath string `yaml:"file_path"`

	// Mode is the file permissions (overrides default).
	Mode os.FileMode `yaml:"mode"`

	// Owner is the file owner (overrides default).
	Owner string `yaml:"owner"`

	// Group is the file group (overrides default).
	Group string `yaml:"group"`

	// Binary indicates the secret should be written as binary.
	Binary bool `yaml:"binary"`

	// Encoding specifies the encoding for string secrets.
	// Supported: "none", "base64".
	Encoding string `yaml:"encoding"`
}

// EnvInjectionConfig configures environment variable injection.
type EnvInjectionConfig struct {
	InjectionConfig `yaml:",inline"`

	// Prefix is the prefix for injected environment variables.
	Prefix string `yaml:"prefix"`

	// Uppercase converts keys to uppercase.
	Uppercase bool `yaml:"uppercase"`

	// Secrets is the list of secrets to inject.
	Secrets []EnvRule `yaml:"secrets"`
}

// EnvRule defines how a secret is injected as an environment variable.
type EnvRule struct {
	// SecretPath is the path to the secret in the backend.
	SecretPath string `yaml:"secret_path"`

	// SecretKey is the specific key within the secret.
	SecretKey string `yaml:"secret_key"`

	// EnvVar is the environment variable name.
	EnvVar string `yaml:"env_var"`
}

// TemplateInjectionConfig configures template-based secret injection.
type TemplateInjectionConfig struct {
	InjectionConfig `yaml:",inline"`

	// Templates is the list of template injection rules.
	Templates []TemplateRule `yaml:"templates"`
}

// TemplateRule defines how secrets are rendered into a template.
type TemplateRule struct {
	// Source is the template source file path.
	Source string `yaml:"source"`

	// Destination is the rendered output file path.
	Destination string `yaml:"destination"`

	// Mode is the output file permissions.
	Mode os.FileMode `yaml:"mode"`

	// Owner is the output file owner.
	Owner string `yaml:"owner"`

	// Group is the output file group.
	Group string `yaml:"group"`

	// LeftDelim is the left template delimiter (default "{{").
	LeftDelim string `yaml:"left_delim"`

	// RightDelim is the right template delimiter (default "}}").
	RightDelim string `yaml:"right_delim"`
}

// NotifyConfig configures process notification after secret updates.
type NotifyConfig struct {
	// Signal is the signal to send (e.g., "SIGHUP", "SIGUSR1").
	Signal string `yaml:"signal"`

	// PIDFile is the path to the PID file.
	PIDFile string `yaml:"pid_file"`

	// Command is a command to run after injection.
	Command []string `yaml:"command"`

	// Timeout is the command execution timeout.
	Timeout time.Duration `yaml:"timeout"`
}

// InjectionResult represents the result of an injection operation.
type InjectionResult struct {
	// Type is the injection type.
	Type InjectionType

	// Target is the injection target (file path, env var, etc.).
	Target string

	// Success indicates if injection succeeded.
	Success bool

	// Error is the error if injection failed.
	Error error

	// Timestamp is when the injection occurred.
	Timestamp time.Time

	// SecretVersion is the version of the injected secret.
	SecretVersion int
}

// Injector is the interface for secret injectors.
type Injector interface {
	// Inject performs the injection.
	Inject(ctx context.Context) ([]InjectionResult, error)

	// Watch starts watching for secret changes and re-injecting.
	Watch(ctx context.Context) error

	// Stop stops the injector.
	Stop() error
}
