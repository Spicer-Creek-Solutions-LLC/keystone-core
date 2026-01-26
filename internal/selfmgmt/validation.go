// Package selfmgmt provides self-management capabilities for Keystone Core components.
package selfmgmt

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// StateValidator provides validation utilities for self-management states.
type StateValidator struct{}

// NewStateValidator creates a new state validator.
func NewStateValidator() *StateValidator {
	return &StateValidator{}
}

// ValidateURL validates a URL string.
func (v *StateValidator) ValidateURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL is empty")
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme == "" {
		return fmt.Errorf("URL missing scheme")
	}

	if u.Host == "" {
		return fmt.Errorf("URL missing host")
	}

	return nil
}

// ValidateNATSURL validates a NATS URL.
func (v *StateValidator) ValidateNATSURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("NATS URL is empty")
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid NATS URL: %w", err)
	}

	validSchemes := map[string]bool{
		"nats":  true,
		"tls":   true,
		"ws":    true,
		"wss":   true,
		"nats+ws":  true,
		"nats+wss": true,
	}

	if !validSchemes[u.Scheme] {
		return fmt.Errorf("invalid NATS scheme: %s (expected nats, tls, ws, wss)", u.Scheme)
	}

	if u.Host == "" {
		return fmt.Errorf("NATS URL missing host")
	}

	return nil
}

// ValidateHostPort validates a host:port string.
func (v *StateValidator) ValidateHostPort(hostPort string) error {
	if hostPort == "" {
		return fmt.Errorf("host:port is empty")
	}

	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return fmt.Errorf("invalid host:port format: %w", err)
	}

	if host == "" {
		return fmt.Errorf("host is empty")
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	return nil
}

// ValidatePort validates a port number.
func (v *StateValidator) ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

// ValidatePath validates a file path.
func (v *StateValidator) ValidatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}

	// Require absolute path
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute: %s", path)
	}

	// Check for directory traversal
	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains directory traversal")
	}

	// Clean the path
	cleaned := filepath.Clean(path)
	if cleaned != path && !strings.HasPrefix(path, cleaned) {
		return fmt.Errorf("path is not clean")
	}

	return nil
}

// ValidatePathExists validates that a path exists.
func (v *StateValidator) ValidatePathExists(path string) error {
	if err := v.ValidatePath(path); err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	}

	return nil
}

// ValidatePathWritable validates that a path is writable.
func (v *StateValidator) ValidatePathWritable(path string) error {
	if err := v.ValidatePath(path); err != nil {
		return err
	}

	// Check if directory exists
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("parent directory does not exist: %s", dir)
	}

	// Try to create a test file
	testFile := filepath.Join(dir, ".kscore-test-write")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("directory is not writable: %s", dir)
	}
	f.Close()
	os.Remove(testFile)

	return nil
}

// ValidateCronSchedule validates a cron schedule expression.
func (v *StateValidator) ValidateCronSchedule(schedule string) error {
	if schedule == "" {
		return fmt.Errorf("schedule is empty")
	}

	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return fmt.Errorf("cron schedule must have 5 fields (minute hour day month weekday)")
	}

	// Validate each field
	fieldNames := []string{"minute", "hour", "day", "month", "weekday"}
	fieldRanges := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}

	for i, field := range fields {
		if err := v.validateCronField(field, fieldNames[i], fieldRanges[i][0], fieldRanges[i][1]); err != nil {
			return err
		}
	}

	return nil
}

// validateCronField validates a single cron field.
func (v *StateValidator) validateCronField(field, name string, min, max int) error {
	if field == "*" {
		return nil
	}

	// Handle */n
	if strings.HasPrefix(field, "*/") {
		n, err := strconv.Atoi(field[2:])
		if err != nil {
			return fmt.Errorf("invalid %s step: %s", name, field)
		}
		if n < 1 {
			return fmt.Errorf("invalid %s step: must be >= 1", name)
		}
		return nil
	}

	// Handle comma-separated values
	parts := strings.Split(field, ",")
	for _, part := range parts {
		// Handle range (n-m)
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return fmt.Errorf("invalid %s range: %s", name, part)
			}
			start, err := strconv.Atoi(rangeParts[0])
			if err != nil {
				return fmt.Errorf("invalid %s range start: %s", name, rangeParts[0])
			}
			end, err := strconv.Atoi(rangeParts[1])
			if err != nil {
				return fmt.Errorf("invalid %s range end: %s", name, rangeParts[1])
			}
			if start < min || start > max || end < min || end > max {
				return fmt.Errorf("invalid %s range: values must be between %d and %d", name, min, max)
			}
			if start > end {
				return fmt.Errorf("invalid %s range: start must be <= end", name)
			}
			continue
		}

		// Single value
		val, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("invalid %s value: %s", name, part)
		}
		if val < min || val > max {
			return fmt.Errorf("invalid %s value: must be between %d and %d", name, min, max)
		}
	}

	return nil
}

// ValidateVersion validates a semantic version string.
func (v *StateValidator) ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version is empty")
	}

	// Simple semver pattern: vX.Y.Z or X.Y.Z
	pattern := `^v?(\d+)\.(\d+)\.(\d+)(-[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?(\+[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?$`
	re := regexp.MustCompile(pattern)
	if !re.MatchString(version) {
		return fmt.Errorf("invalid version format: %s (expected semver)", version)
	}

	return nil
}

// ValidateLabels validates a map of labels.
func (v *StateValidator) ValidateLabels(labels map[string]string) error {
	for key, value := range labels {
		if err := v.ValidateLabelKey(key); err != nil {
			return fmt.Errorf("invalid label key '%s': %w", key, err)
		}
		if err := v.ValidateLabelValue(value); err != nil {
			return fmt.Errorf("invalid label value for '%s': %w", key, err)
		}
	}
	return nil
}

// ValidateLabelKey validates a label key.
func (v *StateValidator) ValidateLabelKey(key string) error {
	if key == "" {
		return fmt.Errorf("key is empty")
	}

	if len(key) > 63 {
		return fmt.Errorf("key too long (max 63 characters)")
	}

	// Must start and end with alphanumeric, can contain -._
	pattern := `^[a-zA-Z0-9][a-zA-Z0-9._-]*[a-zA-Z0-9]$`
	if len(key) == 1 {
		pattern = `^[a-zA-Z0-9]$`
	}

	re := regexp.MustCompile(pattern)
	if !re.MatchString(key) {
		return fmt.Errorf("key must start and end with alphanumeric and contain only alphanumeric, '.', '-', '_'")
	}

	return nil
}

// ValidateLabelValue validates a label value.
func (v *StateValidator) ValidateLabelValue(value string) error {
	if len(value) > 63 {
		return fmt.Errorf("value too long (max 63 characters)")
	}

	if value == "" {
		return nil // Empty values are allowed
	}

	// Must start and end with alphanumeric, can contain -._
	pattern := `^[a-zA-Z0-9][a-zA-Z0-9._-]*[a-zA-Z0-9]$`
	if len(value) == 1 {
		pattern = `^[a-zA-Z0-9]$`
	}

	re := regexp.MustCompile(pattern)
	if !re.MatchString(value) {
		return fmt.Errorf("value must start and end with alphanumeric and contain only alphanumeric, '.', '-', '_'")
	}

	return nil
}

// ValidateDatabaseType validates a database type.
func (v *StateValidator) ValidateDatabaseType(dbType string) error {
	validTypes := map[string]bool{
		"postgresql": true,
		"sqlite":     true,
	}

	if !validTypes[dbType] {
		return fmt.Errorf("invalid database type: %s (expected postgresql or sqlite)", dbType)
	}

	return nil
}

// ValidateInstallMethod validates an installation method.
func (v *StateValidator) ValidateInstallMethod(method InstallMethod) error {
	validMethods := map[InstallMethod]bool{
		InstallPackage: true,
		InstallBinary:  true,
		InstallDocker:  true,
		InstallHelm:    true,
	}

	if !validMethods[method] {
		return fmt.Errorf("invalid install method: %s", method)
	}

	return nil
}

// ValidateComponentState validates a component state.
func (v *StateValidator) ValidateComponentState(state ComponentState, validStates []ComponentState) error {
	for _, s := range validStates {
		if state == s {
			return nil
		}
	}

	var stateStrs []string
	for _, s := range validStates {
		stateStrs = append(stateStrs, string(s))
	}

	return fmt.Errorf("invalid state: %s (expected one of: %s)", state, strings.Join(stateStrs, ", "))
}

// ValidatePostgreSQLConfig validates PostgreSQL configuration.
func (v *StateValidator) ValidatePostgreSQLConfig(cfg *PostgreSQLConfig) ValidationErrors {
	var errs ValidationErrors

	if cfg.Host == "" {
		errs = append(errs, ValidationError{Field: "host", Message: "host is required"})
	}

	if cfg.Port != 0 {
		if err := v.ValidatePort(cfg.Port); err != nil {
			errs = append(errs, ValidationError{Field: "port", Message: err.Error()})
		}
	}

	if cfg.Database == "" {
		errs = append(errs, ValidationError{Field: "database", Message: "database name is required"})
	}

	if cfg.User == "" {
		errs = append(errs, ValidationError{Field: "user", Message: "user is required"})
	}

	validSSLModes := map[string]bool{
		"":             true,
		"disable":      true,
		"allow":        true,
		"prefer":       true,
		"require":      true,
		"verify-ca":    true,
		"verify-full":  true,
	}

	if !validSSLModes[cfg.SSLMode] {
		errs = append(errs, ValidationError{Field: "sslmode", Message: "invalid sslmode"})
	}

	return errs
}

// ValidateSQLiteConfig validates SQLite configuration.
func (v *StateValidator) ValidateSQLiteConfig(cfg *SQLiteConfig) ValidationErrors {
	var errs ValidationErrors

	if cfg.Path == "" {
		errs = append(errs, ValidationError{Field: "path", Message: "path is required"})
	} else if err := v.ValidatePath(cfg.Path); err != nil {
		errs = append(errs, ValidationError{Field: "path", Message: err.Error()})
	}

	return errs
}

// ValidateServerConfig validates server configuration.
func (v *StateValidator) ValidateServerConfig(cfg *ServerConfig) ValidationErrors {
	var errs ValidationErrors

	// Validate state
	module := NewServerModule(nil)
	if err := v.ValidateComponentState(cfg.State, module.ValidStates()); err != nil {
		errs = append(errs, ValidationError{Field: "state", Message: err.Error()})
	}

	// Validate install method
	if cfg.InstallMethod != "" {
		if err := v.ValidateInstallMethod(cfg.InstallMethod); err != nil {
			errs = append(errs, ValidationError{Field: "install_method", Message: err.Error()})
		}
	}

	// Validate version
	if cfg.Version != "" {
		if err := v.ValidateVersion(cfg.Version); err != nil {
			errs = append(errs, ValidationError{Field: "version", Message: err.Error()})
		}
	}

	// Validate listen address
	if cfg.ListenAddress != "" {
		if err := v.ValidateHostPort(cfg.ListenAddress); err != nil {
			errs = append(errs, ValidationError{Field: "listen_address", Message: err.Error()})
		}
	}

	// Validate NATS URLs
	for i, natsURL := range cfg.NATSURLs {
		if err := v.ValidateNATSURL(natsURL); err != nil {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("nats_urls[%d]", i),
				Message: err.Error(),
			})
		}
	}

	return errs
}

// ValidateAgentConfig validates agent configuration.
func (v *StateValidator) ValidateAgentConfig(cfg *AgentConfig) ValidationErrors {
	var errs ValidationErrors

	// Validate state
	module := NewAgentModule(nil)
	if err := v.ValidateComponentState(cfg.State, module.ValidStates()); err != nil {
		errs = append(errs, ValidationError{Field: "state", Message: err.Error()})
	}

	// Validate install method
	if cfg.InstallMethod != "" {
		if err := v.ValidateInstallMethod(cfg.InstallMethod); err != nil {
			errs = append(errs, ValidationError{Field: "install_method", Message: err.Error()})
		}
	}

	// Must have server_url or nats_urls
	if cfg.ServerURL == "" && len(cfg.NATSURLs) == 0 {
		errs = append(errs, ValidationError{Field: "server_url", Message: "either server_url or nats_urls is required"})
	}

	// Validate server URL
	if cfg.ServerURL != "" {
		if err := v.ValidateURL(cfg.ServerURL); err != nil {
			errs = append(errs, ValidationError{Field: "server_url", Message: err.Error()})
		}
	}

	// Validate NATS URLs
	for i, natsURL := range cfg.NATSURLs {
		if err := v.ValidateNATSURL(natsURL); err != nil {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("nats_urls[%d]", i),
				Message: err.Error(),
			})
		}
	}

	// Validate labels
	if err := v.ValidateLabels(cfg.Labels); err != nil {
		errs = append(errs, ValidationError{Field: "labels", Message: err.Error()})
	}

	return errs
}

// ValidateNATSConfig validates NATS configuration.
func (v *StateValidator) ValidateNATSConfig(cfg *NATSConfig) ValidationErrors {
	var errs ValidationErrors

	// Validate state
	module := NewNATSModule(nil)
	if err := v.ValidateComponentState(cfg.State, module.ValidStates()); err != nil {
		errs = append(errs, ValidationError{Field: "state", Message: err.Error()})
	}

	// Validate ports
	if cfg.ClientPort != 0 {
		if err := v.ValidatePort(cfg.ClientPort); err != nil {
			errs = append(errs, ValidationError{Field: "client_port", Message: err.Error()})
		}
	}

	if cfg.HTTPPort != 0 {
		if err := v.ValidatePort(cfg.HTTPPort); err != nil {
			errs = append(errs, ValidationError{Field: "http_port", Message: err.Error()})
		}
	}

	if cfg.ClusterPort != 0 {
		if err := v.ValidatePort(cfg.ClusterPort); err != nil {
			errs = append(errs, ValidationError{Field: "cluster_port", Message: err.Error()})
		}
	}

	// Validate routes
	for i, route := range cfg.Routes {
		if err := v.ValidateNATSURL(route); err != nil {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("routes[%d]", i),
				Message: err.Error(),
			})
		}
	}

	// Validate JetStream store path
	if cfg.JetStreamStorePath != "" {
		if err := v.ValidatePath(cfg.JetStreamStorePath); err != nil {
			errs = append(errs, ValidationError{Field: "jetstream_store_path", Message: err.Error()})
		}
	}

	return errs
}

// ValidateDatabaseConfig validates database configuration.
func (v *StateValidator) ValidateDatabaseConfig(cfg *DatabaseConfig) ValidationErrors {
	var errs ValidationErrors

	// Validate type
	if err := v.ValidateDatabaseType(cfg.Type); err != nil {
		errs = append(errs, ValidationError{Field: "type", Message: err.Error()})
	}

	// Validate state
	module := NewDatabaseModule()
	if err := v.ValidateComponentState(cfg.State, module.ValidStates()); err != nil {
		errs = append(errs, ValidationError{Field: "state", Message: err.Error()})
	}

	// Type-specific validation using inline fields
	if cfg.Type == "postgresql" {
		// Use helper method to get PostgreSQLConfig
		pgCfg := cfg.GetPostgreSQLConfig()
		pgErrs := v.ValidatePostgreSQLConfig(pgCfg)
		errs = append(errs, pgErrs...)
	}

	if cfg.Type == "sqlite" {
		// Use helper method to get SQLiteConfig
		sqliteCfg := cfg.GetSQLiteConfig()
		sqliteErrs := v.ValidateSQLiteConfig(sqliteCfg)
		errs = append(errs, sqliteErrs...)
	}

	return errs
}

// ValidateBackupConfig validates backup configuration.
func (v *StateValidator) ValidateBackupConfig(cfg *BackupConfig) ValidationErrors {
	var errs ValidationErrors

	// Validate state
	module := NewBackupModule()
	if err := v.ValidateComponentState(cfg.State, module.ValidStates()); err != nil {
		errs = append(errs, ValidationError{Field: "state", Message: err.Error()})
	}

	// Validate schedule
	if cfg.State == StateEnabled {
		if cfg.Schedule == "" {
			errs = append(errs, ValidationError{Field: "schedule", Message: "schedule is required for enabled state"})
		} else if err := v.ValidateCronSchedule(cfg.Schedule); err != nil {
			errs = append(errs, ValidationError{Field: "schedule", Message: err.Error()})
		}

		// Get destination using helper method
		destination := cfg.GetDestination()
		if destination == "" && len(cfg.Destinations) == 0 {
			errs = append(errs, ValidationError{Field: "destinations", Message: "at least one destination is required"})
		} else if destination != "" {
			if err := v.ValidatePath(destination); err != nil {
				errs = append(errs, ValidationError{Field: "destinations", Message: err.Error()})
			}
		}
	}

	// Validate retention using helper method
	retentionDays := cfg.GetRetentionDays()
	if retentionDays < 0 {
		errs = append(errs, ValidationError{Field: "retention.keep_daily", Message: "must be >= 0"})
	}

	return errs
}
