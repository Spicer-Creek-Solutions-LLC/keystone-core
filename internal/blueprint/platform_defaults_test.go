package blueprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDefaultsLoader(t *testing.T) {
	loader := NewDefaultsLoader("/test/blueprints/myblueprint")

	if loader == nil {
		t.Fatal("NewDefaultsLoader returned nil")
	}

	if loader.baseDir != "/test/blueprints/myblueprint" {
		t.Errorf("baseDir = %s, want /test/blueprints/myblueprint", loader.baseDir)
	}

	// Platform should be auto-detected
	if loader.Platform.OS == "" {
		t.Error("Platform.OS should be auto-detected")
	}
	if loader.Platform.Arch == "" {
		t.Error("Platform.Arch should be auto-detected")
	}
}

func TestDefaultsLoader_SetPlatform(t *testing.T) {
	loader := NewDefaultsLoader("/test")

	customPlatform := PlatformInfo{
		OS:      "linux",
		Family:  "debian",
		Version: "12",
		Arch:    "amd64",
	}

	loader.SetPlatform(customPlatform)

	if loader.Platform.OS != "linux" {
		t.Errorf("Platform.OS = %s, want linux", loader.Platform.OS)
	}
	if loader.Platform.Family != "debian" {
		t.Errorf("Platform.Family = %s, want debian", loader.Platform.Family)
	}
	if loader.Platform.Version != "12" {
		t.Errorf("Platform.Version = %s, want 12", loader.Platform.Version)
	}
	if loader.Platform.Arch != "amd64" {
		t.Errorf("Platform.Arch = %s, want amd64", loader.Platform.Arch)
	}
}

func TestDefaultsLoader_GetPlatformFiles(t *testing.T) {
	tests := []struct {
		name     string
		platform PlatformInfo
		want     []string
	}{
		{
			name: "full platform info",
			platform: PlatformInfo{
				OS:      "linux",
				Family:  "debian",
				Version: "12",
				Arch:    "amd64",
			},
			want: []string{"linux.yaml", "debian.yaml", "debian-12.yaml", "amd64.yaml", "debian-amd64.yaml"},
		},
		{
			name: "no version",
			platform: PlatformInfo{
				OS:     "linux",
				Family: "rhel",
				Arch:   "arm64",
			},
			want: []string{"linux.yaml", "rhel.yaml", "arm64.yaml", "rhel-arm64.yaml"},
		},
		{
			name: "OS only",
			platform: PlatformInfo{
				OS: "darwin",
			},
			want: []string{"darwin.yaml"},
		},
		{
			name: "arch only",
			platform: PlatformInfo{
				Arch: "arm64",
			},
			want: []string{"arm64.yaml"},
		},
		{
			name:     "empty platform",
			platform: PlatformInfo{},
			want:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewDefaultsLoader("/test")
			loader.SetPlatform(tt.platform)

			got := loader.getPlatformFiles()

			if len(got) != len(tt.want) {
				t.Errorf("getPlatformFiles() returned %d files, want %d", len(got), len(tt.want))
				t.Errorf("got: %v", got)
				t.Errorf("want: %v", tt.want)
				return
			}

			for i, want := range tt.want {
				if got[i] != want {
					t.Errorf("getPlatformFiles()[%d] = %s, want %s", i, got[i], want)
				}
			}
		})
	}
}

func TestDefaultsLoader_LoadDefaults(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	varsDir := filepath.Join(tmpDir, "vars")
	platformsDir := filepath.Join(varsDir, "platforms")

	if err := os.MkdirAll(platformsDir, 0755); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	// Create defaults.yaml
	defaultsContent := `
port: 8080
log_level: info
max_connections: 100
`
	if err := os.WriteFile(filepath.Join(varsDir, "defaults.yaml"), []byte(defaultsContent), 0644); err != nil {
		t.Fatalf("Failed to write defaults.yaml: %v", err)
	}

	// Create debian.yaml (family override)
	debianContent := `
package_manager: apt
log_level: debug
`
	if err := os.WriteFile(filepath.Join(platformsDir, "debian.yaml"), []byte(debianContent), 0644); err != nil {
		t.Fatalf("Failed to write debian.yaml: %v", err)
	}

	// Create debian-12.yaml (version-specific override)
	debian12Content := `
systemd_version: 252
max_connections: 200
`
	if err := os.WriteFile(filepath.Join(platformsDir, "debian-12.yaml"), []byte(debian12Content), 0644); err != nil {
		t.Fatalf("Failed to write debian-12.yaml: %v", err)
	}

	// Create blueprint with schema defaults
	bp := &Blueprint{
		Parameters: map[string]ParameterSchema{
			"port": {
				Type:    "integer",
				Default: 80,
			},
			"timeout": {
				Type:    "integer",
				Default: 30,
			},
		},
	}

	loader := NewDefaultsLoader(tmpDir)
	loader.SetPlatform(PlatformInfo{
		OS:      "linux",
		Family:  "debian",
		Version: "12",
		Arch:    "amd64",
	})

	defaults, err := loader.LoadDefaults(bp)
	if err != nil {
		t.Fatalf("LoadDefaults() error = %v", err)
	}

	// Check schema defaults
	if defaults["timeout"] != 30 {
		t.Errorf("timeout = %v, want 30 (from schema)", defaults["timeout"])
	}

	// Check defaults.yaml overrides schema
	if defaults["port"] != 8080 {
		t.Errorf("port = %v, want 8080 (from defaults.yaml)", defaults["port"])
	}

	// Check debian.yaml overrides defaults.yaml
	if defaults["package_manager"] != "apt" {
		t.Errorf("package_manager = %v, want apt (from debian.yaml)", defaults["package_manager"])
	}
	if defaults["log_level"] != "debug" {
		t.Errorf("log_level = %v, want debug (from debian.yaml)", defaults["log_level"])
	}

	// Check debian-12.yaml overrides earlier values
	if defaults["max_connections"] != 200 {
		t.Errorf("max_connections = %v, want 200 (from debian-12.yaml)", defaults["max_connections"])
	}
	if defaults["systemd_version"] != 252 {
		t.Errorf("systemd_version = %v, want 252 (from debian-12.yaml)", defaults["systemd_version"])
	}
}

func TestDefaultsLoader_LoadDefaultsFromReader(t *testing.T) {
	loader := NewDefaultsLoader("/test")

	yamlContent := `
port: 8080
database:
  host: localhost
  port: 5432
features:
  - auth
  - logging
`
	reader := strings.NewReader(yamlContent)
	defaults, err := loader.LoadDefaultsFromReader(reader)
	if err != nil {
		t.Fatalf("LoadDefaultsFromReader() error = %v", err)
	}

	if defaults["port"] != 8080 {
		t.Errorf("port = %v, want 8080", defaults["port"])
	}

	db, ok := defaults["database"].(map[string]interface{})
	if !ok {
		t.Fatalf("database is not a map")
	}
	if db["host"] != "localhost" {
		t.Errorf("database.host = %v, want localhost", db["host"])
	}

	features, ok := defaults["features"].([]interface{})
	if !ok {
		t.Fatalf("features is not a slice")
	}
	if len(features) != 2 {
		t.Errorf("len(features) = %d, want 2", len(features))
	}
}

func TestDefaultsLoader_LoadDefaultsFromReader_InvalidYAML(t *testing.T) {
	loader := NewDefaultsLoader("/test")

	reader := strings.NewReader("invalid: yaml: [")
	_, err := loader.LoadDefaultsFromReader(reader)
	if err == nil {
		t.Error("LoadDefaultsFromReader() should return error for invalid YAML")
	}
}

func TestDefaultsLoader_ApplyPlatformDefaults(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	varsDir := filepath.Join(tmpDir, "vars")

	if err := os.MkdirAll(varsDir, 0755); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	// Create defaults.yaml
	defaultsContent := `
port: 8080
log_level: info
`
	if err := os.WriteFile(filepath.Join(varsDir, "defaults.yaml"), []byte(defaultsContent), 0644); err != nil {
		t.Fatalf("Failed to write defaults.yaml: %v", err)
	}

	bp := &Blueprint{
		Parameters: map[string]ParameterSchema{
			"port": {
				Type:    "integer",
				Default: 80,
			},
		},
	}

	loader := NewDefaultsLoader(tmpDir)

	// User provides some values
	userParams := map[string]interface{}{
		"port":   3000,
		"custom": "value",
	}

	result, err := loader.ApplyPlatformDefaults(bp, userParams)
	if err != nil {
		t.Fatalf("ApplyPlatformDefaults() error = %v", err)
	}

	// User value should override defaults
	if result["port"] != 3000 {
		t.Errorf("port = %v, want 3000 (user override)", result["port"])
	}

	// User custom value should be preserved
	if result["custom"] != "value" {
		t.Errorf("custom = %v, want value", result["custom"])
	}

	// Default from file should be applied if not overridden
	if result["log_level"] != "info" {
		t.Errorf("log_level = %v, want info (from defaults.yaml)", result["log_level"])
	}
}

func TestDefaultsLoader_ExtractSchemaDefaults(t *testing.T) {
	loader := NewDefaultsLoader("/test")

	schemas := map[string]ParameterSchema{
		"port": {
			Type:    "integer",
			Default: 8080,
		},
		"enabled": {
			Type:    "boolean",
			Default: true,
		},
		"name": {
			Type: "string",
			// No default
		},
		"database": {
			Type: "object",
			Properties: map[string]ParameterSchema{
				"host": {
					Type:    "string",
					Default: "localhost",
				},
				"port": {
					Type:    "integer",
					Default: 5432,
				},
			},
		},
	}

	result := make(map[string]interface{})
	loader.extractSchemaDefaults("", schemas, result)

	if result["port"] != 8080 {
		t.Errorf("port = %v, want 8080", result["port"])
	}

	if result["enabled"] != true {
		t.Errorf("enabled = %v, want true", result["enabled"])
	}

	if _, ok := result["name"]; ok {
		t.Error("name should not be in result (no default)")
	}

	// Check nested defaults
	db, ok := result["database"].(map[string]interface{})
	if !ok {
		t.Fatal("database is not a map")
	}
	if db["host"] != "localhost" {
		t.Errorf("database.host = %v, want localhost", db["host"])
	}
	if db["port"] != 5432 {
		t.Errorf("database.port = %v, want 5432", db["port"])
	}
}

func TestDefaultsLoader_GetPlatformOverridesPath(t *testing.T) {
	tests := []struct {
		name     string
		platform PlatformInfo
		want     string
	}{
		{
			name: "family and version",
			platform: PlatformInfo{
				Family:  "debian",
				Version: "12",
			},
			want: filepath.Join("vars", "platforms", "debian-12.yaml"),
		},
		{
			name: "family only",
			platform: PlatformInfo{
				Family: "rhel",
			},
			want: filepath.Join("vars", "platforms", "rhel.yaml"),
		},
		{
			name:     "no family",
			platform: PlatformInfo{OS: "linux"},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewDefaultsLoader("/test")
			loader.SetPlatform(tt.platform)

			got := loader.GetPlatformOverridesPath()
			if got != tt.want {
				t.Errorf("GetPlatformOverridesPath() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestDetectPlatform(t *testing.T) {
	platform := DetectPlatform()

	// These should always be set from runtime
	if platform.OS == "" {
		t.Error("OS should be detected")
	}
	if platform.Arch == "" {
		t.Error("Arch should be detected")
	}
}

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantFamily  string
		wantVersion string
	}{
		{
			name: "Debian 12",
			content: `PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"
NAME="Debian GNU/Linux"
VERSION_ID="12"
VERSION="12 (bookworm)"
VERSION_CODENAME=bookworm
ID=debian
HOME_URL="https://www.debian.org/"`,
			wantFamily:  "debian",
			wantVersion: "12",
		},
		{
			name: "Ubuntu 22.04",
			content: `PRETTY_NAME="Ubuntu 22.04.3 LTS"
NAME="Ubuntu"
VERSION_ID="22.04"
VERSION="22.04.3 LTS (Jammy Jellyfish)"
VERSION_CODENAME=jammy
ID=ubuntu
ID_LIKE=debian
HOME_URL="https://www.ubuntu.com/"`,
			wantFamily:  "debian",
			wantVersion: "22",
		},
		{
			name: "Rocky Linux 9",
			content: `NAME="Rocky Linux"
VERSION="9.2 (Blue Onyx)"
ID="rocky"
ID_LIKE="rhel centos fedora"
VERSION_ID="9.2"
PLATFORM_ID="platform:el9"`,
			wantFamily:  "rhel",
			wantVersion: "9",
		},
		{
			name: "Alpine 3.19",
			content: `NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.19.0
PRETTY_NAME="Alpine Linux v3.19"`,
			wantFamily:  "alpine",
			wantVersion: "3",
		},
		{
			name: "Fedora 39",
			content: `NAME="Fedora Linux"
VERSION="39 (Workstation Edition)"
ID=fedora
VERSION_ID=39`,
			wantFamily:  "fedora",
			wantVersion: "39",
		},
		{
			name: "Amazon Linux 2023",
			content: `NAME="Amazon Linux"
VERSION="2023"
ID="amzn"
ID_LIKE="fedora"
VERSION_ID="2023"`,
			wantFamily:  "amazon",
			wantVersion: "2023",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			family, version := parseOSRelease(tt.content)
			if family != tt.wantFamily {
				t.Errorf("family = %s, want %s", family, tt.wantFamily)
			}
			if version != tt.wantVersion {
				t.Errorf("version = %s, want %s", version, tt.wantVersion)
			}
		})
	}
}

func TestParseLSBRelease(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantFamily  string
		wantVersion string
	}{
		{
			name: "Ubuntu",
			content: `DISTRIB_ID=Ubuntu
DISTRIB_RELEASE=22.04
DISTRIB_CODENAME=jammy
DISTRIB_DESCRIPTION="Ubuntu 22.04.3 LTS"`,
			wantFamily:  "debian",
			wantVersion: "22",
		},
		{
			name: "Debian",
			content: `DISTRIB_ID=Debian
DISTRIB_RELEASE=12.0
DISTRIB_CODENAME=bookworm
DISTRIB_DESCRIPTION="Debian GNU/Linux 12 (bookworm)"`,
			wantFamily:  "debian",
			wantVersion: "12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			family, version := parseLSBRelease(tt.content)
			if family != tt.wantFamily {
				t.Errorf("family = %s, want %s", family, tt.wantFamily)
			}
			if version != tt.wantVersion {
				t.Errorf("version = %s, want %s", version, tt.wantVersion)
			}
		})
	}
}

func TestParseRedhatRelease(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantFamily  string
		wantVersion string
	}{
		{
			name:        "CentOS 7",
			content:     "CentOS Linux release 7.9.2009 (Core)",
			wantFamily:  "rhel",
			wantVersion: "7",
		},
		{
			name:        "Red Hat Enterprise Linux 8",
			content:     "Red Hat Enterprise Linux release 8.8 (Ootpa)",
			wantFamily:  "rhel",
			wantVersion: "8",
		},
		{
			name:        "Fedora 39",
			content:     "Fedora release 39 (Thirty Nine)",
			wantFamily:  "fedora",
			wantVersion: "39",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			family, version := parseRedhatRelease(tt.content)
			if family != tt.wantFamily {
				t.Errorf("family = %s, want %s", family, tt.wantFamily)
			}
			if version != tt.wantVersion {
				t.Errorf("version = %s, want %s", version, tt.wantVersion)
			}
		})
	}
}

func TestNormalizeFamily(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		// Debian-based
		{"debian", "debian"},
		{"ubuntu", "debian"},
		{"raspbian", "debian"},
		{"linuxmint", "debian"},
		{"pop", "debian"},

		// RHEL-based
		{"rhel", "rhel"},
		{"centos", "rhel"},
		{"rocky", "rhel"},
		{"almalinux", "rhel"},
		{"ol", "rhel"},
		{"oracle", "rhel"},

		// Others
		{"fedora", "fedora"},
		{"alpine", "alpine"},
		{"arch", "arch"},
		{"manjaro", "arch"},
		{"opensuse", "suse"},
		{"suse", "suse"},
		{"sles", "suse"},
		{"amazon", "amazon"},
		{"amzn", "amazon"},

		// Unknown
		{"unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := normalizeFamily(tt.id)
			if got != tt.want {
				t.Errorf("normalizeFamily(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestExtractMajorVersion(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{"12", "12"},
		{"12.1", "12"},
		{"22.04", "22"},
		{"9.2.1", "9"},
		{"3.19.0", "3"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := extractMajorVersion(tt.version)
			if got != tt.want {
				t.Errorf("extractMajorVersion(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestDefaultsLoader_MissingFiles(t *testing.T) {
	tmpDir := t.TempDir()

	bp := &Blueprint{
		Parameters: map[string]ParameterSchema{
			"port": {
				Type:    "integer",
				Default: 8080,
			},
		},
	}

	loader := NewDefaultsLoader(tmpDir)
	loader.SetPlatform(PlatformInfo{
		OS:     "linux",
		Family: "debian",
	})

	// Should succeed even without any files (just use schema defaults)
	defaults, err := loader.LoadDefaults(bp)
	if err != nil {
		t.Fatalf("LoadDefaults() error = %v", err)
	}

	if defaults["port"] != 8080 {
		t.Errorf("port = %v, want 8080 (from schema)", defaults["port"])
	}
}
