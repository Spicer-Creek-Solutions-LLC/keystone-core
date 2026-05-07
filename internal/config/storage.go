package config

import (
	"fmt"

	"go.keystone-core.io/keystone-core/internal/state"
)

// StorageConfig configures the persistence backend.
type StorageConfig struct {
	Driver string `koanf:"driver"`
	DSN    string `koanf:"dsn"`
}

var validStorageDrivers = map[string]bool{"sqlite": true, "postgres": true}

// Validate returns an error if Driver is not recognized or DSN is empty.
func (s StorageConfig) Validate() error {
	if !validStorageDrivers[s.Driver] {
		return fmt.Errorf("driver: %q (must be sqlite or postgres)", s.Driver)
	}
	if s.DSN == "" {
		return fmt.Errorf("dsn: must not be empty")
	}
	return nil
}

// ToStateConfig maps the user-facing config to the state package's
// backend-specific Config. Driver name conventions differ — config
// uses "postgres", state uses "postgresql" — and DSN routing depends
// on the backend (file path for sqlite, libpq DSN for postgres).
func (s StorageConfig) ToStateConfig() (*state.Config, error) {
	switch s.Driver {
	case "sqlite":
		return &state.Config{
			Backend: state.BackendSQLite,
			SQLite:  state.SQLiteConfig{Path: s.DSN},
		}, nil
	case "postgres":
		return &state.Config{
			Backend:    state.BackendPostgreSQL,
			PostgreSQL: state.PostgreSQLConfig{DSN: s.DSN},
		}, nil
	default:
		return nil, fmt.Errorf("config: storage driver %q has no state mapping", s.Driver)
	}
}
