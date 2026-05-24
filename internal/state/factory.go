// SPDX-License-Identifier: Apache-2.0

package state

import "fmt"

// NewStore validates cfg, applies backend-appropriate pool defaults, and
// returns the concrete Store implementation. Returns an error if the
// config is invalid or the backend is unrecognized.
func NewStore(cfg *Config) (Store, error) {
	if cfg == nil {
		return nil, fmt.Errorf("state: Config is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.applyDefaults()

	switch cfg.Backend {
	case BackendSQLite:
		return newSQLiteStore(cfg)
	case BackendPostgreSQL:
		return newPostgreSQLStore(cfg)
	default:
		// Validate already rejected unknown backends; this is defensive.
		return nil, fmt.Errorf("state: unhandled Backend %q", cfg.Backend)
	}
}
