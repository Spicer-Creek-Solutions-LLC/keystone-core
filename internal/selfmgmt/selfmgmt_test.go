package selfmgmt

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestNewServerModule tests server module creation.
func TestNewServerModule(t *testing.T) {
	m := NewServerModule(nil)
	if m == nil {
		t.Fatal("expected non-nil module")
	}
	if m.Name() != "kscore_server" {
		t.Errorf("expected name 'kscore_server', got '%s'", m.Name())
	}
	if m.ComponentType() != ComponentServer {
		t.Errorf("expected component type 'server', got '%s'", m.ComponentType())
	}
	states := m.ValidStates()
	if len(states) == 0 {
		t.Error("expected valid states")
	}
}

// TestNewAgentModule tests agent module creation.
func TestNewAgentModule(t *testing.T) {
	m := NewAgentModule(nil)
	if m == nil {
		t.Fatal("expected non-nil module")
	}
	if m.Name() != "kscore_agent" {
		t.Errorf("expected name 'kscore_agent', got '%s'", m.Name())
	}
	if m.ComponentType() != ComponentAgent {
		t.Errorf("expected component type 'agent', got '%s'", m.ComponentType())
	}
}

// TestNewNATSModule tests NATS module creation.
func TestNewNATSModule(t *testing.T) {
	m := NewNATSModule(nil)
	if m == nil {
		t.Fatal("expected non-nil module")
	}
	if m.Name() != "kscore_nats" {
		t.Errorf("expected name 'kscore_nats', got '%s'", m.Name())
	}
	if m.ComponentType() != ComponentNATS {
		t.Errorf("expected component type 'nats', got '%s'", m.ComponentType())
	}
}

// TestNewDatabaseModule tests database module creation.
func TestNewDatabaseModule(t *testing.T) {
	m := NewDatabaseModule()
	if m == nil {
		t.Fatal("expected non-nil module")
	}
	if m.Name() != "kscore_database" {
		t.Errorf("expected name 'kscore_database', got '%s'", m.Name())
	}
	if m.ComponentType() != ComponentDatabase {
		t.Errorf("expected component type 'database', got '%s'", m.ComponentType())
	}
}

// TestNewBackupModule tests backup module creation.
func TestNewBackupModule(t *testing.T) {
	m := NewBackupModule()
	if m == nil {
		t.Fatal("expected non-nil module")
	}
	if m.Name() != "kscore_backup" {
		t.Errorf("expected name 'kscore_backup', got '%s'", m.Name())
	}
	if m.ComponentType() != ComponentBackup {
		t.Errorf("expected component type 'backup', got '%s'", m.ComponentType())
	}
}

// TestServerModuleValidate tests server module validation.
func TestServerModuleValidate(t *testing.T) {
	m := NewServerModule(nil)

	tests := []struct {
		name    string
		config  *ServerConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &ServerConfig{
				BaseConfig: BaseConfig{State: StateInstalled},
			},
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "missing state",
			config: &ServerConfig{
				BaseConfig: BaseConfig{State: ""},
			},
			wantErr: true,
		},
		{
			name: "invalid state",
			config: &ServerConfig{
				BaseConfig: BaseConfig{State: "invalid"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.config == nil {
				err = m.Validate(nil)
			} else {
				err = m.Validate(tt.config)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestAgentModuleValidate tests agent module validation.
func TestAgentModuleValidate(t *testing.T) {
	m := NewAgentModule(nil)

	tests := []struct {
		name    string
		config  *AgentConfig
		wantErr bool
	}{
		{
			name: "valid with server_url",
			config: &AgentConfig{
				BaseConfig: BaseConfig{State: StateInstalled},
				ServerURL:  "http://localhost:8080",
			},
			wantErr: false,
		},
		{
			name: "valid with nats_urls",
			config: &AgentConfig{
				BaseConfig: BaseConfig{State: StateInstalled},
				NATSURLs:   []string{"nats://localhost:4222"},
			},
			wantErr: false,
		},
		{
			name: "missing server_url and nats_urls",
			config: &AgentConfig{
				BaseConfig: BaseConfig{State: StateRunning},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.Validate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestNATSModuleValidate tests NATS module validation.
func TestNATSModuleValidate(t *testing.T) {
	m := NewNATSModule(nil)

	tests := []struct {
		name    string
		config  *NATSConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &NATSConfig{
				BaseConfig: BaseConfig{State: StateInstalled},
			},
			wantErr: false,
		},
		{
			name: "valid with ports",
			config: &NATSConfig{
				BaseConfig: BaseConfig{State: StateRunning},
				ClientPort: 4222,
				HTTPPort:   8222,
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			config: &NATSConfig{
				BaseConfig: BaseConfig{State: StateRunning},
				ClientPort: 99999,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.Validate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestDatabaseModuleValidate tests database module validation.
func TestDatabaseModuleValidate(t *testing.T) {
	m := NewDatabaseModule()

	tests := []struct {
		name    string
		config  *DatabaseConfig
		wantErr bool
	}{
		{
			name: "valid postgresql",
			config: &DatabaseConfig{
				BaseConfig: BaseConfig{State: StateInstalled},
				Type:       "postgresql",
			},
			wantErr: false,
		},
		{
			name: "valid sqlite",
			config: &DatabaseConfig{
				BaseConfig: BaseConfig{State: StateInstalled},
				Type:       "sqlite",
			},
			wantErr: false,
		},
		{
			name: "missing type",
			config: &DatabaseConfig{
				BaseConfig: BaseConfig{State: StateInstalled},
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			config: &DatabaseConfig{
				BaseConfig: BaseConfig{State: StateInstalled},
				Type:       "mysql",
			},
			wantErr: true,
		},
		{
			name: "running postgresql missing config",
			config: &DatabaseConfig{
				BaseConfig: BaseConfig{State: StateRunning},
				Type:       "postgresql",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.Validate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestBackupModuleValidate tests backup module validation.
func TestBackupModuleValidate(t *testing.T) {
	m := NewBackupModule()

	tests := []struct {
		name    string
		config  *BackupConfig
		wantErr bool
	}{
		{
			name: "valid disabled",
			config: &BackupConfig{
				BaseConfig: BaseConfig{State: StateDisabled},
			},
			wantErr: false,
		},
		{
			name: "valid enabled",
			config: &BackupConfig{
				BaseConfig: BaseConfig{State: StateEnabled},
				Schedule:   "0 0 * * *",
				Destinations: []BackupDestination{
					{Name: "local", Type: "local", Path: "/backup"},
				},
				Retention: &BackupRetention{KeepDaily: 7},
			},
			wantErr: false,
		},
		{
			name: "enabled missing schedule",
			config: &BackupConfig{
				BaseConfig: BaseConfig{State: StateEnabled},
				Destinations: []BackupDestination{
					{Name: "local", Type: "local", Path: "/backup"},
				},
			},
			wantErr: true,
		},
		{
			name: "enabled missing destination",
			config: &BackupConfig{
				BaseConfig: BaseConfig{State: StateEnabled},
				Schedule:   "0 0 * * *",
			},
			wantErr: true,
		},
		{
			name: "invalid schedule",
			config: &BackupConfig{
				BaseConfig: BaseConfig{State: StateEnabled},
				Schedule:   "invalid",
				Destinations: []BackupDestination{
					{Name: "local", Type: "local", Path: "/backup"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.Validate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestDetectInitSystem tests init system detection.
func TestDetectInitSystem(t *testing.T) {
	initSystem := DetectInitSystem()
	if initSystem == "" {
		t.Error("expected non-empty init system")
	}

	// On known systems, verify expected values
	switch runtime.GOOS {
	case "darwin":
		if initSystem != "launchd" {
			t.Errorf("expected 'launchd' on darwin, got '%s'", initSystem)
		}
	case "windows":
		if initSystem != "windows" {
			t.Errorf("expected 'windows' on windows, got '%s'", initSystem)
		}
	}
}

// TestDetectPackageManager tests package manager detection.
func TestDetectPackageManager(t *testing.T) {
	pm := DetectPackageManager()
	// On some systems package manager may be unknown, which is acceptable
	t.Logf("detected package manager: %s", pm)

	// On known systems, verify expected values
	switch runtime.GOOS {
	case "darwin":
		// On macOS, brew may or may not be installed
		if pm == "brew" || pm == "unknown" {
			// Expected
		} else {
			t.Logf("unexpected package manager on darwin: %s", pm)
		}
	case "windows":
		// Could be chocolatey, winget, or unknown
		if pm != "chocolatey" && pm != "winget" && pm != "unknown" {
			t.Logf("unexpected package manager on windows: %s", pm)
		}
	}
}

// TestGetDefaultKscoreConfigPath tests default config path.
func TestGetDefaultKscoreConfigPath(t *testing.T) {
	path := GetDefaultKscoreConfigPath()
	if path == "" {
		t.Error("expected non-empty config path")
	}

	switch runtime.GOOS {
	case "windows":
		if !filepath.IsAbs(path) {
			t.Errorf("expected absolute path, got '%s'", path)
		}
	default:
		if path != "/etc/keystone-core" {
			t.Errorf("expected '/etc/keystone-core', got '%s'", path)
		}
	}
}

// TestGetDefaultKscoreDataDir tests default data directory.
func TestGetDefaultKscoreDataDir(t *testing.T) {
	path := GetDefaultKscoreDataDir()
	if path == "" {
		t.Error("expected non-empty data dir")
	}

	switch runtime.GOOS {
	case "windows":
		if !filepath.IsAbs(path) {
			t.Errorf("expected absolute path, got '%s'", path)
		}
	default:
		if path != "/var/lib/keystone-core" {
			t.Errorf("expected '/var/lib/keystone-core', got '%s'", path)
		}
	}
}

// TestValidationErrors tests validation error handling.
func TestValidationErrors(t *testing.T) {
	errs := ValidationErrors{
		{Field: "field1", Message: "error1"},
		{Field: "field2", Message: "error2"},
	}

	errStr := errs.Error()
	if errStr == "" {
		t.Error("expected non-empty error string")
	}
}

// TestStateValidatorValidateURL tests URL validation.
func TestStateValidatorValidateURL(t *testing.T) {
	v := NewStateValidator()

	tests := []struct {
		url     string
		wantErr bool
	}{
		{"http://localhost:8080", false},
		{"https://example.com", false},
		{"", true},
		{"invalid", true},
		{"://missing-scheme", true},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := v.ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

// TestStateValidatorValidateNATSURL tests NATS URL validation.
func TestStateValidatorValidateNATSURL(t *testing.T) {
	v := NewStateValidator()

	tests := []struct {
		url     string
		wantErr bool
	}{
		{"nats://localhost:4222", false},
		{"tls://localhost:4222", false},
		{"ws://localhost:8080", false},
		{"wss://localhost:8080", false},
		{"", true},
		{"http://localhost", true},
		{"invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := v.ValidateNATSURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNATSURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

// TestStateValidatorValidatePort tests port validation.
func TestStateValidatorValidatePort(t *testing.T) {
	v := NewStateValidator()

	tests := []struct {
		port    int
		wantErr bool
	}{
		{80, false},
		{443, false},
		{4222, false},
		{65535, false},
		{0, true},
		{-1, true},
		{65536, true},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			err := v.ValidatePort(tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort(%d) error = %v, wantErr %v", tt.port, err, tt.wantErr)
			}
		})
	}
}

// TestStateValidatorValidateCronSchedule tests cron schedule validation.
func TestStateValidatorValidateCronSchedule(t *testing.T) {
	v := NewStateValidator()

	tests := []struct {
		schedule string
		wantErr  bool
	}{
		{"0 0 * * *", false},
		{"*/5 * * * *", false},
		{"0 0 1 * *", false},
		{"0 0 * * 0", false},
		{"0,30 * * * *", false},
		{"0-30 * * * *", false},
		{"", true},
		{"invalid", true},
		{"0 0 * *", true},     // Only 4 fields
		{"0 0 * * * *", true}, // 6 fields
		{"60 * * * *", true},  // Invalid minute
		{"* 24 * * *", true},  // Invalid hour
	}

	for _, tt := range tests {
		t.Run(tt.schedule, func(t *testing.T) {
			err := v.ValidateCronSchedule(tt.schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCronSchedule(%q) error = %v, wantErr %v", tt.schedule, err, tt.wantErr)
			}
		})
	}
}

// TestStateValidatorValidateVersion tests version validation.
func TestStateValidatorValidateVersion(t *testing.T) {
	v := NewStateValidator()

	tests := []struct {
		version string
		wantErr bool
	}{
		{"1.0.0", false},
		{"v1.0.0", false},
		{"1.2.3", false},
		{"1.0.0-alpha", false},
		{"1.0.0-beta.1", false},
		{"1.0.0+build", false},
		{"", true},
		{"1.0", true},
		{"invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			err := v.ValidateVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersion(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
		})
	}
}

// TestStateValidatorValidateLabels tests label validation.
func TestStateValidatorValidateLabels(t *testing.T) {
	v := NewStateValidator()

	tests := []struct {
		name    string
		labels  map[string]string
		wantErr bool
	}{
		{
			name:    "valid labels",
			labels:  map[string]string{"env": "prod", "app": "kscore"},
			wantErr: false,
		},
		{
			name:    "empty labels",
			labels:  map[string]string{},
			wantErr: false,
		},
		{
			name:    "empty key",
			labels:  map[string]string{"": "value"},
			wantErr: true,
		},
		{
			name:    "key too long",
			labels:  map[string]string{strings.Repeat("a", 100): "value"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateLabels(tt.labels)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLabels() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestServerModuleCheck tests server module check.
func TestServerModuleCheck(t *testing.T) {
	m := NewServerModule(nil)
	ctx := context.Background()

	config := &ServerConfig{
		BaseConfig: BaseConfig{State: StateInstalled},
	}

	result, err := m.Check(ctx, config)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.DesiredState != StateInstalled {
		t.Errorf("expected desired state 'installed', got '%s'", result.DesiredState)
	}
}

// TestBackupModuleBuildScript tests backup script generation.
func TestBackupModuleBuildScript(t *testing.T) {
	m := NewBackupModule()

	config := &BackupConfig{
		BaseConfig: BaseConfig{State: StateEnabled},
		Schedule:   "0 0 * * *",
		Destinations: []BackupDestination{
			{Name: "local", Type: "local", Path: "/backup"},
		},
		Retention: &BackupRetention{KeepDaily: 7},
	}

	script := m.buildUnixBackupScript(config)
	if script == "" {
		t.Error("expected non-empty script")
	}

	if !strings.Contains(script, "#!/bin/bash") {
		t.Error("expected bash shebang")
	}

	if !strings.Contains(script, "RETENTION_DAYS=7") {
		t.Error("expected retention days in script")
	}
}

// TestBackupModuleListBackups tests listing backups.
func TestBackupModuleListBackups(t *testing.T) {
	m := NewBackupModule()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "backup-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test backup files
	timestamp := time.Now().Format("20060102_150405")
	backupFile := filepath.Join(tmpDir, "kscore_backup_"+timestamp+".tar.gz")
	if err := os.WriteFile(backupFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// List backups
	backups, err := m.ListBackups(tmpDir)
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}

	if len(backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(backups))
	}

	if len(backups) > 0 && !strings.Contains(backups[0].Name, "kscore_backup_") {
		t.Errorf("expected backup name prefix, got '%s'", backups[0].Name)
	}
}

// TestNATSModuleBuildConfig tests NATS config generation.
func TestNATSModuleBuildConfig(t *testing.T) {
	m := NewNATSModule(nil)

	config := &NATSConfig{
		BaseConfig:       BaseConfig{State: StateRunning},
		ClientPort:       4222,
		HTTPPort:         8222,
		JetStreamEnabled: true,
		ClusterName:      "kscore",
		ClusterPort:      6222,
	}

	configStr := m.buildConfig(config)
	if configStr == "" {
		t.Error("expected non-empty config")
	}

	if !strings.Contains(configStr, "port: 4222") {
		t.Error("expected port in config")
	}

	if !strings.Contains(configStr, "jetstream") {
		t.Error("expected jetstream in config")
	}
}

// TestDatabaseModulePostgreSQLHelpers tests PostgreSQL helpers.
func TestDatabaseModulePostgreSQLHelpers(t *testing.T) {
	m := NewDatabaseModule()

	config := &DatabaseConfig{
		BaseConfig: BaseConfig{State: StateRunning},
		Type:       "postgresql",
		Host:       "localhost",
		Port:       5432,
		Name:       "kscore",
		DBUser:     "kscore",
		Password:   "secret",
		SSLMode:    "disable",
	}

	connStr := m.buildPsqlConnString(config, "postgres")
	if connStr == "" {
		t.Error("expected non-empty connection string")
	}

	if !strings.Contains(connStr, "host=localhost") {
		t.Error("expected host in connection string")
	}

	if !strings.Contains(connStr, "port=5432") {
		t.Error("expected port in connection string")
	}
}

// TestComponentTypes tests component type constants.
func TestComponentTypes(t *testing.T) {
	types := []ComponentType{
		ComponentServer,
		ComponentAgent,
		ComponentNATS,
		ComponentDatabase,
		ComponentBackup,
	}

	for _, ct := range types {
		if ct == "" {
			t.Error("component type should not be empty")
		}
	}
}

// TestComponentStates tests component state constants.
func TestComponentStates(t *testing.T) {
	states := []ComponentState{
		StateInstalled,
		StateUninstalled,
		StateRunning,
		StateStopped,
		StateConfigured,
		StateEnabled,
		StateDisabled,
	}

	for _, s := range states {
		if s == "" {
			t.Error("state should not be empty")
		}
	}
}

// TestInstallMethods tests install method constants.
func TestInstallMethods(t *testing.T) {
	methods := []InstallMethod{
		InstallPackage,
		InstallBinary,
		InstallDocker,
		InstallHelm,
	}

	for _, m := range methods {
		if m == "" {
			t.Error("install method should not be empty")
		}
	}
}

// TestGetDefaultConfigPath tests GetDefaultConfigPath for components.
func TestGetDefaultConfigPath(t *testing.T) {
	components := []ComponentType{
		ComponentServer,
		ComponentAgent,
		ComponentNATS,
		ComponentDatabase,
		ComponentBackup,
	}

	for _, c := range components {
		path := GetDefaultConfigPath(c)
		if path == "" {
			t.Errorf("expected non-empty config path for %s", c)
		}
	}
}

// TestGetDefaultDataDir tests GetDefaultDataDir for components.
func TestGetDefaultDataDir(t *testing.T) {
	components := []ComponentType{
		ComponentServer,
		ComponentAgent,
		ComponentNATS,
		ComponentDatabase,
		ComponentBackup,
	}

	for _, c := range components {
		path := GetDefaultDataDir(c)
		if path == "" {
			t.Errorf("expected non-empty data dir for %s", c)
		}
	}
}

// TestGetDefaultBinaryPath tests GetDefaultBinaryPath for components.
func TestGetDefaultBinaryPath(t *testing.T) {
	components := []ComponentType{
		ComponentServer,
		ComponentAgent,
		ComponentNATS,
	}

	for _, c := range components {
		path := GetDefaultBinaryPath(c)
		if path == "" {
			t.Errorf("expected non-empty binary path for %s", c)
		}
	}
}

// TestServiceName tests ServiceName for components.
func TestServiceName(t *testing.T) {
	tests := []struct {
		component ComponentType
		expected  string
	}{
		{ComponentServer, "kscore-server"},
		{ComponentAgent, "kscore-agent"},
		{ComponentNATS, "nats-server"},
	}

	for _, tt := range tests {
		name := ServiceName(tt.component)
		if name != tt.expected {
			t.Errorf("ServiceName(%s) = %s, want %s", tt.component, name, tt.expected)
		}
	}
}

// TestFileExists tests the FileExists helper.
func TestFileExists(t *testing.T) {
	// Test with a file that exists
	tmpFile, err := os.CreateTemp("", "test-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	if !FileExists(tmpFile.Name()) {
		t.Error("expected file to exist")
	}

	// Test with a file that doesn't exist
	if FileExists("/nonexistent/path/file") {
		t.Error("expected file to not exist")
	}
}

// TestEnsureDir tests the EnsureDir helper.
func TestEnsureDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	newDir := filepath.Join(tmpDir, "new", "nested", "dir")
	if err := EnsureDir(newDir, 0755); err != nil {
		t.Fatalf("EnsureDir() error = %v", err)
	}

	if !FileExists(newDir) {
		t.Error("expected directory to exist")
	}
}

// TestWriteFile tests the WriteFile helper.
func TestWriteFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "new", "file.txt")
	content := []byte("test content")

	if err := WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if !FileExists(filePath) {
		t.Error("expected file to exist")
	}

	readContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(readContent) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", readContent, content)
	}
}

// TestBackupConfigHelpers tests BackupConfig helper methods.
func TestBackupConfigHelpers(t *testing.T) {
	// Test with destinations
	cfg := &BackupConfig{
		Destinations: []BackupDestination{
			{Name: "local", Type: "local", Path: "/backup"},
			{Name: "s3", Type: "s3", Bucket: "my-bucket"},
		},
		Retention: &BackupRetention{KeepDaily: 7, KeepWeekly: 4},
	}

	dest := cfg.GetDestination()
	if dest != "/backup" {
		t.Errorf("GetDestination() = %s, want /backup", dest)
	}

	days := cfg.GetRetentionDays()
	if days != 7 {
		t.Errorf("GetRetentionDays() = %d, want 7", days)
	}

	// Test with no retention
	cfg2 := &BackupConfig{}
	if cfg2.GetRetentionDays() != 0 {
		t.Error("expected 0 retention days for nil retention")
	}

	// Test with no destinations
	if cfg2.GetDestination() != "" {
		t.Error("expected empty destination for no destinations")
	}
}
