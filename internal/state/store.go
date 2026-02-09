// Package state provides persistent state storage backends including SQLite
// and PostgreSQL with migration support.
package state

import (
	"fmt"
)

// NewStore creates a new state store based on configuration
func NewStore(config *Config) (Store, error) {
	switch config.Backend {
	case "sqlite":
		return NewSQLiteStore(config)
	case "postgresql", "postgres":
		return NewPostgreSQLStore(config)
	default:
		return nil, fmt.Errorf("unsupported backend: %s", config.Backend)
	}
}
