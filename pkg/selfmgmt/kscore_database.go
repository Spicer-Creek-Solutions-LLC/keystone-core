// Package selfmgmt provides self-management capabilities for Keystone Core components.
package selfmgmt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// DatabaseModule manages database state for Keystone Core.
type DatabaseModule struct {
	name        string
	validStates []ComponentState
}

// NewDatabaseModule creates a new database state module.
func NewDatabaseModule() *DatabaseModule {
	return &DatabaseModule{
		name: "kscore_database",
		validStates: []ComponentState{
			StateInstalled,
			StateUninstalled,
			StateRunning,
			StateStopped,
			StateConfigured,
		},
	}
}

// Name returns the module name.
func (m *DatabaseModule) Name() string {
	return m.name
}

// ComponentType returns the component type.
func (m *DatabaseModule) ComponentType() ComponentType {
	return ComponentDatabase
}

// ValidStates returns valid states for the database.
func (m *DatabaseModule) ValidStates() []ComponentState {
	return m.validStates
}

// Validate validates the database configuration.
func (m *DatabaseModule) Validate(config interface{}) error {
	cfg, ok := config.(*DatabaseConfig)
	if !ok {
		return fmt.Errorf("invalid config type: expected *DatabaseConfig")
	}

	var errs ValidationErrors

	// Validate database type
	if cfg.Type == "" {
		errs = append(errs, ValidationError{Field: "type", Message: "type is required"})
	} else if cfg.Type != "postgresql" && cfg.Type != "sqlite" {
		errs = append(errs, ValidationError{Field: "type", Message: "type must be 'postgresql' or 'sqlite'"})
	}

	// Validate state
	if cfg.State == "" {
		errs = append(errs, ValidationError{Field: "state", Message: "state is required"})
	} else {
		valid := false
		for _, s := range m.validStates {
			if cfg.State == s {
				valid = true
				break
			}
		}
		if !valid {
			errs = append(errs, ValidationError{
				Field:   "state",
				Message: fmt.Sprintf("invalid state: %s", cfg.State),
			})
		}
	}

	// Type-specific validation
	if cfg.Type == "postgresql" {
		if cfg.State == StateRunning || cfg.State == StateConfigured {
			if cfg.Host == "" {
				errs = append(errs, ValidationError{Field: "host", Message: "host is required for postgresql"})
			}
			if cfg.Name == "" {
				errs = append(errs, ValidationError{Field: "name", Message: "database name is required"})
			}
			if cfg.DBUser == "" {
				errs = append(errs, ValidationError{Field: "db_user", Message: "user is required"})
			}
		}
	}

	if cfg.Type == "sqlite" {
		if cfg.State == StateRunning || cfg.State == StateConfigured {
			if cfg.SQLitePath == "" {
				errs = append(errs, ValidationError{Field: "sqlite_path", Message: "path is required for sqlite"})
			}
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// Check checks the current state of the database.
func (m *DatabaseModule) Check(ctx context.Context, config interface{}) (*CheckResult, error) {
	cfg, ok := config.(*DatabaseConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected *DatabaseConfig")
	}

	if err := m.Validate(cfg); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	result := &CheckResult{
		Component:    ComponentDatabase,
		CurrentState: StateUninstalled,
		DesiredState: cfg.State,
		Matches:      false,
	}

	switch cfg.Type {
	case "postgresql":
		return m.checkPostgreSQL(ctx, cfg, result)
	case "sqlite":
		return m.checkSQLite(ctx, cfg, result)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
}

// checkPostgreSQL checks PostgreSQL state.
func (m *DatabaseModule) checkPostgreSQL(ctx context.Context, cfg *DatabaseConfig, result *CheckResult) (*CheckResult, error) {
	// Check if PostgreSQL is installed
	installed := m.isPostgreSQLInstalled()
	if !installed {
		result.CurrentState = StateUninstalled
		result.Matches = (cfg.State == StateUninstalled)
		return result, nil
	}

	result.CurrentState = StateInstalled
	result.Present = true

	// Check if running
	if m.isPostgreSQLRunning() {
		result.CurrentState = StateRunning
		result.Running = true
	} else {
		result.CurrentState = StateStopped
	}

	result.Matches = (result.CurrentState == cfg.State)
	return result, nil
}

// checkSQLite checks SQLite state.
func (m *DatabaseModule) checkSQLite(ctx context.Context, cfg *DatabaseConfig, result *CheckResult) (*CheckResult, error) {
	// Check if SQLite is installed (usually available by default)
	installed := m.isSQLiteInstalled()
	if !installed {
		result.CurrentState = StateUninstalled
		result.Matches = (cfg.State == StateUninstalled)
		return result, nil
	}

	result.CurrentState = StateInstalled
	result.Present = true

	// Check if database file exists
	if cfg.SQLitePath != "" {
		if _, err := os.Stat(cfg.SQLitePath); err == nil {
			result.CurrentState = StateConfigured
		}
	}

	result.Matches = (result.CurrentState == cfg.State)
	return result, nil
}

// Apply applies the desired database state.
func (m *DatabaseModule) Apply(ctx context.Context, config interface{}, dryRun bool) (*ApplyResult, error) {
	cfg, ok := config.(*DatabaseConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected *DatabaseConfig")
	}

	checkResult, err := m.Check(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("check failed: %w", err)
	}

	result := &ApplyResult{
		Component:     ComponentDatabase,
		Success:       true,
		Changed:       false,
		Changes:       make(map[string]interface{}),
		PreviousState: checkResult.CurrentState,
		NewState:      checkResult.CurrentState,
	}

	if checkResult.Matches {
		return result, nil
	}

	if dryRun {
		result.Changes["action"] = fmt.Sprintf("Would change from %s to %s", checkResult.CurrentState, cfg.State)
		result.Changed = true
		result.NewState = cfg.State
		return result, nil
	}

	switch cfg.Type {
	case "postgresql":
		return m.applyPostgreSQL(ctx, cfg, result)
	case "sqlite":
		return m.applySQLite(ctx, cfg, result)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
}

// applyPostgreSQL applies PostgreSQL state changes.
func (m *DatabaseModule) applyPostgreSQL(ctx context.Context, cfg *DatabaseConfig, result *ApplyResult) (*ApplyResult, error) {
	switch cfg.State {
	case StateInstalled:
		if !m.isPostgreSQLInstalled() {
			if err := m.installPostgreSQL(); err != nil {
				result.Success = false
				result.Error = err
				return result, nil
			}
			result.Changes["installed"] = "postgresql"
			result.Changed = true
		}
	case StateUninstalled:
		if m.isPostgreSQLInstalled() {
			if err := m.uninstallPostgreSQL(); err != nil {
				result.Success = false
				result.Error = err
				return result, nil
			}
			result.Changes["uninstalled"] = "postgresql"
			result.Changed = true
		}
	case StateRunning:
		if !m.isPostgreSQLInstalled() {
			if err := m.installPostgreSQL(); err != nil {
				result.Success = false
				result.Error = err
				return result, nil
			}
			result.Changes["installed"] = "postgresql"
			result.Changed = true
		}
		if !m.isPostgreSQLRunning() {
			if err := m.startPostgreSQL(); err != nil {
				result.Success = false
				result.Error = err
				return result, nil
			}
			result.Changes["started"] = "postgresql"
			result.Changed = true
		}
		// Create database and user if needed
		if cfg.DBUser != "" && cfg.DBUser != "postgres" {
			if !m.postgreSQLUserExists(ctx, cfg) {
				if err := m.createPostgreSQLUser(ctx, cfg); err != nil {
					result.Success = false
					result.Error = err
					return result, nil
				}
				result.Changes["created_user"] = cfg.DBUser
				result.Changed = true
			}
		}
		if cfg.Name != "" && !m.postgreSQLDatabaseExists(ctx, cfg) {
			if err := m.createPostgreSQLDatabase(ctx, cfg); err != nil {
				result.Success = false
				result.Error = err
				return result, nil
			}
			result.Changes["created_database"] = cfg.Name
			result.Changed = true
		}
	case StateStopped:
		if m.isPostgreSQLRunning() {
			if err := m.stopPostgreSQL(); err != nil {
				result.Success = false
				result.Error = err
				return result, nil
			}
			result.Changes["stopped"] = "postgresql"
			result.Changed = true
		}
	case StateConfigured:
		// Ensure installed and running
		if !m.isPostgreSQLInstalled() {
			if err := m.installPostgreSQL(); err != nil {
				result.Success = false
				result.Error = err
				return result, nil
			}
			result.Changes["installed"] = "postgresql"
			result.Changed = true
		}
		if !m.isPostgreSQLRunning() {
			if err := m.startPostgreSQL(); err != nil {
				result.Success = false
				result.Error = err
				return result, nil
			}
			result.Changes["started"] = "postgresql"
			result.Changed = true
		}
		// Configure database
		if err := m.configurePostgreSQL(cfg); err != nil {
			result.Success = false
			result.Error = err
			return result, nil
		}
		result.Changes["configured"] = true
		result.Changed = true
	}

	result.NewState = cfg.State
	return result, nil
}

// applySQLite applies SQLite state changes.
func (m *DatabaseModule) applySQLite(ctx context.Context, cfg *DatabaseConfig, result *ApplyResult) (*ApplyResult, error) {
	switch cfg.State {
	case StateInstalled:
		if !m.isSQLiteInstalled() {
			if err := m.installSQLite(); err != nil {
				result.Success = false
				result.Error = err
				return result, nil
			}
			result.Changes["installed"] = "sqlite"
			result.Changed = true
		}
	case StateUninstalled:
		if cfg.SQLitePath != "" {
			if _, err := os.Stat(cfg.SQLitePath); err == nil {
				if err := os.Remove(cfg.SQLitePath); err != nil {
					result.Success = false
					result.Error = err
					return result, nil
				}
				result.Changes["removed"] = cfg.SQLitePath
				result.Changed = true
			}
		}
	case StateConfigured, StateRunning:
		if !m.isSQLiteInstalled() {
			if err := m.installSQLite(); err != nil {
				result.Success = false
				result.Error = err
				return result, nil
			}
			result.Changes["installed"] = "sqlite"
			result.Changed = true
		}
		// Create database directory
		if cfg.SQLitePath != "" {
			dir := filepath.Dir(cfg.SQLitePath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				result.Success = false
				result.Error = err
				return result, nil
			}
			// Create empty database file if it doesn't exist
			if _, err := os.Stat(cfg.SQLitePath); os.IsNotExist(err) {
				f, err := os.Create(cfg.SQLitePath)
				if err != nil {
					result.Success = false
					result.Error = err
					return result, nil
				}
				f.Close()
				result.Changes["created"] = cfg.SQLitePath
				result.Changed = true
			}
		}
	}

	result.NewState = cfg.State
	return result, nil
}

// isPostgreSQLInstalled checks if PostgreSQL is installed.
func (m *DatabaseModule) isPostgreSQLInstalled() bool {
	_, err := exec.LookPath("psql")
	return err == nil
}

// isPostgreSQLRunning checks if PostgreSQL is running.
func (m *DatabaseModule) isPostgreSQLRunning() bool {
	initSystem := DetectInitSystem()
	var cmd *exec.Cmd

	switch initSystem {
	case "systemd":
		cmd = exec.Command("systemctl", "is-active", "--quiet", "postgresql")
	case "launchd":
		cmd = exec.Command("brew", "services", "list")
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(output), "postgresql") && strings.Contains(string(output), "started")
	default:
		cmd = exec.Command("pg_isready")
	}

	err := cmd.Run()
	return err == nil
}

// installPostgreSQL installs PostgreSQL.
func (m *DatabaseModule) installPostgreSQL() error {
	pm := DetectPackageManager()

	var cmd *exec.Cmd
	switch pm {
	case "apt":
		cmd = exec.Command("apt-get", "install", "-y", "postgresql", "postgresql-contrib")
	case "dnf":
		cmd = exec.Command("dnf", "install", "-y", "postgresql-server", "postgresql")
	case "yum":
		cmd = exec.Command("yum", "install", "-y", "postgresql-server", "postgresql")
	case "apk":
		cmd = exec.Command("apk", "add", "postgresql")
	case "brew":
		cmd = exec.Command("brew", "install", "postgresql")
	case "chocolatey":
		cmd = exec.Command("choco", "install", "-y", "postgresql")
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// uninstallPostgreSQL uninstalls PostgreSQL.
func (m *DatabaseModule) uninstallPostgreSQL() error {
	pm := DetectPackageManager()

	var cmd *exec.Cmd
	switch pm {
	case "apt":
		cmd = exec.Command("apt-get", "remove", "-y", "postgresql", "postgresql-contrib")
	case "dnf":
		cmd = exec.Command("dnf", "remove", "-y", "postgresql-server", "postgresql")
	case "yum":
		cmd = exec.Command("yum", "remove", "-y", "postgresql-server", "postgresql")
	case "apk":
		cmd = exec.Command("apk", "del", "postgresql")
	case "brew":
		cmd = exec.Command("brew", "uninstall", "postgresql")
	case "chocolatey":
		cmd = exec.Command("choco", "uninstall", "-y", "postgresql")
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// startPostgreSQL starts the PostgreSQL service.
func (m *DatabaseModule) startPostgreSQL() error {
	initSystem := DetectInitSystem()

	var cmd *exec.Cmd
	switch initSystem {
	case "systemd":
		cmd = exec.Command("systemctl", "start", "postgresql")
	case "launchd":
		cmd = exec.Command("brew", "services", "start", "postgresql")
	case "openrc":
		cmd = exec.Command("rc-service", "postgresql", "start")
	default:
		cmd = exec.Command("service", "postgresql", "start")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// stopPostgreSQL stops the PostgreSQL service.
func (m *DatabaseModule) stopPostgreSQL() error {
	initSystem := DetectInitSystem()

	var cmd *exec.Cmd
	switch initSystem {
	case "systemd":
		cmd = exec.Command("systemctl", "stop", "postgresql")
	case "launchd":
		cmd = exec.Command("brew", "services", "stop", "postgresql")
	case "openrc":
		cmd = exec.Command("rc-service", "postgresql", "stop")
	default:
		cmd = exec.Command("service", "postgresql", "stop")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// postgreSQLDatabaseExists checks if a database exists.
func (m *DatabaseModule) postgreSQLDatabaseExists(ctx context.Context, cfg *DatabaseConfig) bool {
	connStr := m.buildPsqlConnString(cfg, "postgres")
	cmd := exec.CommandContext(ctx, "psql", connStr, "-tAc",
		fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname='%s'", cfg.Name))
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(output)) == "1"
}

// postgreSQLUserExists checks if a user exists.
func (m *DatabaseModule) postgreSQLUserExists(ctx context.Context, cfg *DatabaseConfig) bool {
	connStr := m.buildPsqlConnString(cfg, "postgres")
	cmd := exec.CommandContext(ctx, "psql", connStr, "-tAc",
		fmt.Sprintf("SELECT 1 FROM pg_roles WHERE rolname='%s'", cfg.DBUser))
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(output)) == "1"
}

// createPostgreSQLDatabase creates a database.
func (m *DatabaseModule) createPostgreSQLDatabase(ctx context.Context, cfg *DatabaseConfig) error {
	connStr := m.buildPsqlConnString(cfg, "postgres")

	sql := fmt.Sprintf("CREATE DATABASE %s", cfg.Name)
	if cfg.DBUser != "" && cfg.DBUser != "postgres" {
		sql += fmt.Sprintf(" OWNER %s", cfg.DBUser)
	}

	cmd := exec.CommandContext(ctx, "psql", connStr, "-c", sql)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// createPostgreSQLUser creates a user.
func (m *DatabaseModule) createPostgreSQLUser(ctx context.Context, cfg *DatabaseConfig) error {
	connStr := m.buildPsqlConnString(cfg, "postgres")

	sql := fmt.Sprintf("CREATE USER %s", cfg.DBUser)
	if cfg.Password != "" {
		sql += fmt.Sprintf(" WITH PASSWORD '%s'", cfg.Password)
	}

	cmd := exec.CommandContext(ctx, "psql", connStr, "-c", sql)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// configurePostgreSQL configures PostgreSQL.
func (m *DatabaseModule) configurePostgreSQL(cfg *DatabaseConfig) error {
	// Find PostgreSQL config directory
	configDir := m.findPostgreSQLConfigDir()
	if configDir == "" {
		return fmt.Errorf("could not find PostgreSQL config directory")
	}

	// Update pg_hba.conf for local connections
	pgHbaPath := filepath.Join(configDir, "pg_hba.conf")
	if _, err := os.Stat(pgHbaPath); err == nil {
		content, err := os.ReadFile(pgHbaPath)
		if err != nil {
			return fmt.Errorf("failed to read pg_hba.conf: %w", err)
		}

		// Check if kscore entry exists
		if !strings.Contains(string(content), "# kscore") {
			f, err := os.OpenFile(pgHbaPath, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed to open pg_hba.conf: %w", err)
			}
			defer f.Close()

			entry := fmt.Sprintf("\n# kscore\nhost    %s    %s    127.0.0.1/32    md5\n",
				cfg.Name, cfg.DBUser)
			if _, err := f.WriteString(entry); err != nil {
				return fmt.Errorf("failed to write pg_hba.conf: %w", err)
			}
		}
	}

	return m.reloadPostgreSQL()
}

// reloadPostgreSQL reloads PostgreSQL configuration.
func (m *DatabaseModule) reloadPostgreSQL() error {
	initSystem := DetectInitSystem()

	var cmd *exec.Cmd
	switch initSystem {
	case "systemd":
		cmd = exec.Command("systemctl", "reload", "postgresql")
	case "launchd":
		cmd = exec.Command("brew", "services", "restart", "postgresql")
	default:
		cmd = exec.Command("pg_ctl", "reload")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// findPostgreSQLConfigDir finds the PostgreSQL config directory.
func (m *DatabaseModule) findPostgreSQLConfigDir() string {
	possiblePaths := []string{
		"/etc/postgresql",
		"/var/lib/pgsql/data",
		"/var/lib/postgres/data",
		"/usr/local/var/postgres",
		"/opt/homebrew/var/postgres",
	}

	for _, path := range possiblePaths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			if path == "/etc/postgresql" {
				entries, err := os.ReadDir(path)
				if err == nil && len(entries) > 0 {
					for i := len(entries) - 1; i >= 0; i-- {
						versionDir := filepath.Join(path, entries[i].Name(), "main")
						if _, err := os.Stat(versionDir); err == nil {
							return versionDir
						}
					}
				}
			}
			return path
		}
	}

	return ""
}

// buildPsqlConnString builds a psql connection string.
func (m *DatabaseModule) buildPsqlConnString(cfg *DatabaseConfig, database string) string {
	parts := []string{}

	if cfg.Host != "" {
		parts = append(parts, fmt.Sprintf("host=%s", cfg.Host))
	}
	if cfg.Port > 0 {
		parts = append(parts, fmt.Sprintf("port=%d", cfg.Port))
	}
	parts = append(parts, fmt.Sprintf("dbname=%s", database))
	if cfg.DBUser != "" {
		parts = append(parts, fmt.Sprintf("user=%s", cfg.DBUser))
	}
	if cfg.Password != "" {
		parts = append(parts, fmt.Sprintf("password=%s", cfg.Password))
	}
	if cfg.SSLMode != "" {
		parts = append(parts, fmt.Sprintf("sslmode=%s", cfg.SSLMode))
	}

	return strings.Join(parts, " ")
}

// isSQLiteInstalled checks if SQLite is installed.
func (m *DatabaseModule) isSQLiteInstalled() bool {
	_, err := exec.LookPath("sqlite3")
	if err == nil {
		return true
	}

	switch runtime.GOOS {
	case "linux":
		paths := []string{
			"/usr/lib/x86_64-linux-gnu/libsqlite3.so",
			"/usr/lib/libsqlite3.so",
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		}
	case "darwin":
		return true // SQLite is built into macOS
	case "windows":
		paths := []string{
			"C:\\Windows\\System32\\sqlite3.dll",
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		}
	}

	return false
}

// installSQLite installs SQLite.
func (m *DatabaseModule) installSQLite() error {
	pm := DetectPackageManager()

	var cmd *exec.Cmd
	switch pm {
	case "apt":
		cmd = exec.Command("apt-get", "install", "-y", "sqlite3")
	case "dnf":
		cmd = exec.Command("dnf", "install", "-y", "sqlite")
	case "yum":
		cmd = exec.Command("yum", "install", "-y", "sqlite")
	case "apk":
		cmd = exec.Command("apk", "add", "sqlite")
	case "brew":
		cmd = exec.Command("brew", "install", "sqlite")
	case "chocolatey":
		cmd = exec.Command("choco", "install", "-y", "sqlite")
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
