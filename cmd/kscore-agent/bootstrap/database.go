package bootstrap

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func setupDatabase(cfg *Config, output io.Writer, verbose bool) ([]string, error) {
	switch strings.ToLower(cfg.Storage) {
	case "postgres":
		dsn := buildPostgresDSN(cfg)
		if err := validatePostgresDSN(dsn); err != nil {
			return nil, err
		}
		if verbose {
			fmt.Fprintln(output, "postgres configuration validated")
		}
		return nil, nil
	default:
		path := sqliteDatabasePath(cfg)
		created, err := ensureSQLiteDatabase(path)
		if err != nil {
			return nil, err
		}
		if verbose {
			fmt.Fprintf(output, "sqlite database ready at %s\n", path)
		}
		if created {
			return []string{path}, nil
		}
		return nil, nil
	}
}

func sqliteDatabasePath(cfg *Config) string {
	return "/var/lib/keystone-core/state.db"
}

func ensureSQLiteDatabase(path string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("sqlite path is required")
	}
	//nolint:gosec // G301: database directory needs to be accessible by service user
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create sqlite directory: %w", err)
	}
	existed := fileExists(path)
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o640) //nolint:gosec // G302: Database file needs group-read for service account access
	if err != nil {
		return false, fmt.Errorf("create sqlite file: %w", err)
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	return !existed, nil
}

func validatePostgresDSN(dsn string) error {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("invalid postgres dsn: %w", err)
	}
	if parsed.Scheme != "postgres" {
		return fmt.Errorf("postgres dsn must use postgres scheme")
	}
	if parsed.Host == "" {
		return fmt.Errorf("postgres dsn must include host")
	}
	return nil
}
