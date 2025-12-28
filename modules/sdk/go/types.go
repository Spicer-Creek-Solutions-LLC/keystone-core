// Package kscoresdk provides the Go SDK for building Keystone Core modules that compile to WebAssembly.
package kscoresdk

// Capability represents a capability that a module can request
type Capability string

const (
	// CapabilityFsRead allows reading files
	CapabilityFsRead Capability = "fs.read"
	// CapabilityFsWrite allows writing files
	CapabilityFsWrite Capability = "fs.write"
	// CapabilityHttpGet allows HTTP GET requests
	CapabilityHttpGet Capability = "http.get"
	// CapabilityHttpPost allows HTTP POST requests
	CapabilityHttpPost Capability = "http.post"
	// CapabilityExec allows command execution
	CapabilityExec Capability = "exec"
	// CapabilitySecretsRead allows reading secrets
	CapabilitySecretsRead Capability = "secrets.read"
	// CapabilitySecretsWrite allows writing secrets
	CapabilitySecretsWrite Capability = "secrets.write"
	// CapabilityLog allows logging
	CapabilityLog Capability = "log"
	// CapabilityTime allows time access
	CapabilityTime Capability = "time"
	// CapabilityKv allows key-value storage
	CapabilityKv Capability = "kv"
)

// ModuleContext provides context information for module execution
type ModuleContext struct {
	ModuleName    string                 `json:"module_name"`
	CorrelationID string                 `json:"correlation_id"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// ModuleResult represents the result of module execution
type ModuleResult[T any] struct {
	Success bool   `json:"success"`
	Data    *T     `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// FileInfo contains file metadata
type FileInfo struct {
	Path     string `json:"path"`
	Size     uint64 `json:"size"`
	IsDir    bool   `json:"is_dir"`
	Modified uint64 `json:"modified"`
}

// HttpRequest represents an HTTP request
type HttpRequest struct {
	URL     string              `json:"url"`
	Headers map[string]string   `json:"headers,omitempty"`
	Body    []byte              `json:"body,omitempty"`
}

// HttpResponse represents an HTTP response
type HttpResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body"`
}

// ExecRequest represents a command execution request
type ExecRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Stdin   string   `json:"stdin,omitempty"`
}

// ExecResult represents command execution result
type ExecResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// LogLevel represents logging severity
type LogLevel int

const (
	// LogLevelDebug for debug messages
	LogLevelDebug LogLevel = iota
	// LogLevelInfo for informational messages
	LogLevelInfo
	// LogLevelWarn for warnings
	LogLevelWarn
	// LogLevelError for errors
	LogLevelError
)

// String returns the string representation of a log level
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "debug"
	case LogLevelInfo:
		return "info"
	case LogLevelWarn:
		return "warn"
	case LogLevelError:
		return "error"
	default:
		return "unknown"
	}
}
