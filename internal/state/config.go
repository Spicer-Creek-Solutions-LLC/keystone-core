package state

import (
	"errors"
	"fmt"
	"time"
)

// Backend selects the persistence implementation.
type Backend string

const (
	BackendSQLite     Backend = "sqlite"
	BackendPostgreSQL Backend = "postgresql"
)

// Config bundles backend selection, per-backend settings, and connection
// pool tuning. Pool defaults are applied per-backend by applyDefaults
// when the field is zero-valued.
type Config struct {
	Backend Backend

	SQLite     SQLiteConfig
	PostgreSQL PostgreSQLConfig

	// Connection pool tuning. Zero values get backend-appropriate
	// defaults via applyDefaults (called by NewStore).
	MaxOpenConns int
	MaxIdleConns int
	ConnMaxLife  time.Duration
}

// SQLiteConfig configures the modernc.org/sqlite-backed Store.
type SQLiteConfig struct {
	Path        string        // default "./data/keystone.db"
	WAL         bool          // default true
	BusyTimeout time.Duration // default 5s
}

// PostgreSQLConfig configures the lib/pq-backed Store.
//
// If DSN is non-empty it takes precedence; otherwise the connection
// string is built from Host/Port/Database/User/Password/SSLMode using
// the IPv6-safe builder added in epic 02 task 6.
//
// DSN and Password carry json:"-" tags: this struct must never be
// serialized in a way that exposes credentials to logs, telemetry, or
// API responses.
type PostgreSQLConfig struct {
	DSN      string `json:"-"`
	Host     string
	Port     int
	Database string
	User     string
	Password string `json:"-"`
	SSLMode  string // disable | require | verify-ca | verify-full
}

// applyDefaults fills in zero-valued fields with backend-appropriate
// defaults. Idempotent.
func (c *Config) applyDefaults() {
	switch c.Backend {
	case BackendSQLite:
		if c.SQLite.Path == "" {
			c.SQLite.Path = "./data/keystone.db"
		}
		if c.SQLite.BusyTimeout == 0 {
			c.SQLite.BusyTimeout = 5 * time.Second
		}
		// SQLite: single writer (PROJECT-DETAILS §4.3).
		if c.MaxOpenConns == 0 {
			c.MaxOpenConns = 1
		}
		if c.MaxIdleConns == 0 {
			c.MaxIdleConns = 1
		}
	case BackendPostgreSQL:
		if c.PostgreSQL.SSLMode == "" {
			c.PostgreSQL.SSLMode = "require"
		}
		if c.MaxOpenConns == 0 {
			c.MaxOpenConns = 25
		}
		if c.MaxIdleConns == 0 {
			c.MaxIdleConns = 5
		}
		if c.ConnMaxLife == 0 {
			c.ConnMaxLife = 30 * time.Minute
		}
	}
}

// Validate returns an error if the config is missing required fields or
// contains invalid values. Called by NewStore before applyDefaults so
// validation messages reference the user-supplied values.
func (c *Config) Validate() error {
	switch c.Backend {
	case BackendSQLite:
		// SQLite needs no required fields up front; Path defaults are
		// applied separately. Pool overrides for SQLite must be 1 if
		// supplied, since concurrent writers are a SQLite anti-pattern.
		if c.MaxOpenConns > 1 {
			return fmt.Errorf("state: sqlite MaxOpenConns must be 0 or 1; got %d",
				c.MaxOpenConns)
		}
	case BackendPostgreSQL:
		if c.PostgreSQL.DSN == "" {
			if c.PostgreSQL.Host == "" || c.PostgreSQL.Database == "" || c.PostgreSQL.User == "" {
				return errors.New("state: postgresql requires DSN or Host+Database+User")
			}
		}
	case "":
		return errors.New("state: Backend is required (sqlite or postgresql)")
	default:
		return fmt.Errorf("state: unknown Backend %q (want sqlite or postgresql)", c.Backend)
	}

	if c.MaxOpenConns < 0 {
		return fmt.Errorf("state: MaxOpenConns must be non-negative; got %d", c.MaxOpenConns)
	}
	if c.MaxIdleConns < 0 {
		return fmt.Errorf("state: MaxIdleConns must be non-negative; got %d", c.MaxIdleConns)
	}
	if c.ConnMaxLife < 0 {
		return fmt.Errorf("state: ConnMaxLife must be non-negative; got %s", c.ConnMaxLife)
	}
	return nil
}
