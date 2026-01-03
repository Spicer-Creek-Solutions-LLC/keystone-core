package capabilities

import "time"

// Default limits for capabilities (DefaultMaxFileSize is defined in fs.go)
const (
	// DefaultMaxResponseSize is the default maximum HTTP response size (10MB)
	DefaultMaxResponseSize int64 = 10 * 1024 * 1024
	// DefaultMaxRequestSize is the default maximum HTTP request size (10MB)
	DefaultMaxRequestSize int64 = 10 * 1024 * 1024
	// DefaultTimeout is the default timeout for operations
	DefaultTimeout = 30 * time.Second
)

// NewFSReadCapability creates a file system read capability
func NewFSReadCapability(ctx *CapabilityContext, allowedPaths []string, deniedPaths []string, maxFileSize int64) Capability {
	if maxFileSize <= 0 {
		maxFileSize = DefaultMaxFileSize
	}
	return &FSReadCapability{
		AllowedPaths: allowedPaths,
		DeniedPaths:  deniedPaths,
		MaxFileSize:  maxFileSize,
	}
}

// NewFSWriteCapability creates a file system write capability
func NewFSWriteCapability(ctx *CapabilityContext, allowedPaths []string, deniedPaths []string) Capability {
	return &FSWriteCapability{
		AllowedPaths: allowedPaths,
		DeniedPaths:  deniedPaths,
		MaxFileSize:  DefaultMaxFileSize,
	}
}

// NewHTTPGetCapability creates an HTTP GET capability
func NewHTTPGetCapability(ctx *CapabilityContext, allowedDomains []string, timeout time.Duration, maxResponseSize int64, maxRequests int) Capability {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if maxResponseSize <= 0 {
		maxResponseSize = DefaultMaxResponseSize
	}
	var rateLimit *RateLimit
	if maxRequests > 0 {
		rateLimit = &RateLimit{
			Requests: maxRequests,
			Period:   time.Minute,
		}
	}
	return &HTTPGetCapability{
		AllowedDomains: allowedDomains,
		TimeoutMax:     timeout,
		MaxRespSize:    maxResponseSize,
		RateLimit:      rateLimit,
	}
}

// NewHTTPPostCapability creates an HTTP POST capability
func NewHTTPPostCapability(ctx *CapabilityContext, allowedDomains []string, timeout time.Duration, maxRequestSize int64, maxResponseSize int64, maxRequests int) Capability {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if maxRequestSize <= 0 {
		maxRequestSize = DefaultMaxRequestSize
	}
	if maxResponseSize <= 0 {
		maxResponseSize = DefaultMaxResponseSize
	}
	var rateLimit *RateLimit
	if maxRequests > 0 {
		rateLimit = &RateLimit{
			Requests: maxRequests,
			Period:   time.Minute,
		}
	}
	return &HTTPPostCapability{
		AllowedDomains: allowedDomains,
		TimeoutMax:     timeout,
		MaxReqSize:     maxRequestSize,
		MaxRespSize:    maxResponseSize,
		RateLimit:      rateLimit,
	}
}

// NewExecCapability creates a command execution capability
func NewExecCapability(ctx *CapabilityContext, allowedCommands []string, timeout time.Duration, workingDir string) Capability {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &ExecCapability{
		AllowedCommands: allowedCommands,
		TimeoutMax:      timeout,
		WorkingDir:      workingDir,
	}
}

// NewSecretsReadCapability creates a secrets read capability
func NewSecretsReadCapability(ctx *CapabilityContext, allowedPrefixes []string, store SecretsStore) Capability {
	cap := &SecretsReadCapability{
		AllowedPaths: allowedPrefixes,
		AuditAll:     true,
	}
	if store != nil {
		cap.SetStore(store)
	}
	return cap
}

// NewSecretsWriteCapability creates a secrets write capability
func NewSecretsWriteCapability(ctx *CapabilityContext, allowedPrefixes []string, store SecretsStore) Capability {
	cap := &SecretsWriteCapability{
		AllowedPaths: allowedPrefixes,
		AuditAll:     true,
	}
	if store != nil {
		cap.SetStore(store)
	}
	return cap
}

// NewLogCapability creates a logging capability
func NewLogCapability(ctx *CapabilityContext, logger Logger, rateLimit int) Capability {
	var rl *RateLimit
	if rateLimit > 0 {
		rl = &RateLimit{
			Requests: rateLimit,
			Period:   time.Minute,
		}
	}
	cap := &LogCapability{
		RateLimit: rl,
	}
	if logger != nil {
		cap.SetLogger(logger)
	}
	return cap
}

// NewTimeCapability creates a time access capability
// WARNING: This capability breaks determinism!
func NewTimeCapability(ctx *CapabilityContext) Capability {
	return &TimeCapability{}
}

// NewKVCapability creates a key-value storage capability
func NewKVCapability(ctx *CapabilityContext, namespace string, store interface{}) Capability {
	cap := &KVCapability{
		Namespace: namespace,
	}
	if kvStore, ok := store.(KVStore); ok && kvStore != nil {
		cap.SetStore(kvStore)
	}
	return cap
}
