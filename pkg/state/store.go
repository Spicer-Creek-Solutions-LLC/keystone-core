package state

import (
	"fmt"
)

// NewStore creates a new state store based on configuration
func NewStore(config *Config) (Store, error) {
	switch config.Backend {
	case "sqlite":
		return NewSQLiteStore(config)
	case "postgresql":
		return nil, fmt.Errorf("PostgreSQL backend not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported backend: %s", config.Backend)
	}
}
