package capabilities

import "time"

// Stub capability implementations for module loader
// These will be fully implemented in Epic 9 Phase 3

// StubCapability is a simple capability implementation
type StubCapability struct {
	name string
}

// Name returns the capability name
func (c *StubCapability) Name() string {
	return c.name
}

// NewFSReadCapability creates a file system read capability
func NewFSReadCapability(ctx *CapabilityContext, allowedPaths []string, deniedPaths []string, maxFileSize int64) Capability {
	return &StubCapability{name: "fs.read"}
}

// NewFSWriteCapability creates a file system write capability
func NewFSWriteCapability(ctx *CapabilityContext, allowedPaths []string, deniedPaths []string) Capability {
	return &StubCapability{name: "fs.write"}
}

// NewHTTPGetCapability creates an HTTP GET capability
func NewHTTPGetCapability(ctx *CapabilityContext, allowedDomains []string, timeout time.Duration, maxResponseSize int64, maxRequests int) Capability {
	return &StubCapability{name: "http.get"}
}

// NewHTTPPostCapability creates an HTTP POST capability
func NewHTTPPostCapability(ctx *CapabilityContext, allowedDomains []string, timeout time.Duration, maxRequestSize int64, maxResponseSize int64, maxRequests int) Capability {
	return &StubCapability{name: "http.post"}
}

// NewExecCapability creates a command execution capability
func NewExecCapability(ctx *CapabilityContext, allowedCommands []string, timeout time.Duration, workingDir string) Capability {
	return &StubCapability{name: "exec"}
}

// NewSecretsReadCapability creates a secrets read capability
func NewSecretsReadCapability(ctx *CapabilityContext, allowedPrefixes []string, store SecretsStore) Capability {
	return &StubCapability{name: "secrets.read"}
}

// NewSecretsWriteCapability creates a secrets write capability
func NewSecretsWriteCapability(ctx *CapabilityContext, allowedPrefixes []string, store SecretsStore) Capability {
	return &StubCapability{name: "secrets.write"}
}

// NewLogCapability creates a logging capability
func NewLogCapability(ctx *CapabilityContext, logger Logger, rateLimit int) Capability {
	return &StubCapability{name: "log"}
}

// NewTimeCapability creates a time access capability
func NewTimeCapability(ctx *CapabilityContext) Capability {
	return &StubCapability{name: "time"}
}

// NewKVCapability creates a key-value storage capability
func NewKVCapability(ctx *CapabilityContext, namespace string, store interface{}) Capability {
	return &StubCapability{name: "kv"}
}
