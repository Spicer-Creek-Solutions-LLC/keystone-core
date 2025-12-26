package edge

import "time"

// OperationMode represents the agent's current operation mode
type OperationMode int

const (
	// ModeOnline - connected to control plane, full functionality
	ModeOnline OperationMode = iota
	// ModeOffline - disconnected from control plane, autonomous operation
	ModeOffline
	// ModeLightweight - resource-constrained mode with reduced features
	ModeLightweight
)

func (m OperationMode) String() string {
	switch m {
	case ModeOnline:
		return "online"
	case ModeOffline:
		return "offline"
	case ModeLightweight:
		return "lightweight"
	default:
		return "unknown"
	}
}

// Config represents edge agent configuration
type Config struct {
	// EnableOfflineMode enables autonomous operation when disconnected
	EnableOfflineMode bool

	// EnableLightweightMode reduces resource usage
	EnableLightweightMode bool

	// LocalCachePath is the path for local state cache
	LocalCachePath string

	// MaxCacheSize is the maximum cache size in bytes (0 = unlimited)
	MaxCacheSize int64

	// MaxCacheAge is how long to keep cached data
	MaxCacheAge time.Duration

	// ReconnectInterval is how often to attempt reconnection when offline
	ReconnectInterval time.Duration

	// MaxReconnectAttempts is max reconnection attempts (0 = unlimited)
	MaxReconnectAttempts int

	// HeartbeatInterval for edge devices (may be longer to save power)
	HeartbeatInterval time.Duration

	// MaxMemoryMB is max memory usage in MB for lightweight mode
	MaxMemoryMB int

	// MaxCPUPercent is max CPU usage percent for lightweight mode
	MaxCPUPercent int

	// EnableLocalStateExecution allows state execution while offline
	EnableLocalStateExecution bool

	// EnableLocalCommandExecution allows command execution while offline
	EnableLocalCommandExecution bool
}

// DefaultEdgeConfig returns default edge configuration
func DefaultEdgeConfig() *Config {
	return &Config{
		EnableOfflineMode:           true,
		EnableLightweightMode:       false,
		LocalCachePath:              "/var/lib/titananvil/cache",
		MaxCacheSize:                100 * 1024 * 1024, // 100 MB
		MaxCacheAge:                 24 * time.Hour,
		ReconnectInterval:           30 * time.Second,
		MaxReconnectAttempts:        0, // unlimited
		HeartbeatInterval:           60 * time.Second,
		MaxMemoryMB:                 512,
		MaxCPUPercent:               50,
		EnableLocalStateExecution:   true,
		EnableLocalCommandExecution: false,
	}
}

// Status represents current edge agent status
type Status struct {
	Mode                 OperationMode
	Connected            bool
	LastConnected        time.Time
	ReconnectAttempts    int
	CacheSize            int64
	CachedStatesCount    int
	CachedCommandsCount  int
	MemoryUsageMB        int
	CPUUsagePercent      int
	UptimeSeconds        int64
	ResourceConstrained  bool
}

// CacheEntry represents a cached item
type CacheEntry struct {
	ID        string
	Type      string // "state" or "command"
	Data      []byte
	CreatedAt time.Time
	ExpiresAt time.Time
	Size      int64
}

// Cache interface for local caching
type Cache interface {
	// Set stores an entry in the cache
	Set(entry *CacheEntry) error

	// Get retrieves an entry from the cache
	Get(id string) (*CacheEntry, error)

	// Delete removes an entry from the cache
	Delete(id string) error

	// List returns all cache entries
	List() ([]*CacheEntry, error)

	// Clear removes all entries from the cache
	Clear() error

	// Prune removes expired entries
	Prune() error

	// GetSize returns total cache size in bytes
	GetSize() (int64, error)

	// GetStats returns cache statistics
	GetStats() (*CacheStats, error)
}

// CacheStats represents cache statistics
type CacheStats struct {
	TotalEntries  int
	TotalSize     int64
	OldestEntry   time.Time
	NewestEntry   time.Time
	HitCount      int64
	MissCount     int64
	EvictionCount int64
}

// Manager manages edge-specific operations
type Manager interface {
	// GetMode returns current operation mode
	GetMode() OperationMode

	// SetMode sets the operation mode
	SetMode(mode OperationMode) error

	// GetStatus returns current edge status
	GetStatus() (*Status, error)

	// IsConnected checks if connected to control plane
	IsConnected() bool

	// SetConnected updates connection status
	SetConnected(connected bool)

	// GetCache returns the local cache
	GetCache() Cache

	// CheckResourceConstraints checks if resources are constrained
	CheckResourceConstraints() (bool, error)

	// HandleDisconnect handles disconnection from control plane
	HandleDisconnect() error

	// HandleReconnect handles reconnection to control plane
	HandleReconnect() error
}
