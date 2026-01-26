package blueprint

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// PlatformInfo describes the current platform for loading overrides.
type PlatformInfo struct {
	// OS is the operating system (e.g., "linux", "darwin", "windows")
	OS string

	// Family is the OS family (e.g., "debian", "rhel", "alpine")
	Family string

	// Version is the OS version (e.g., "12", "9", "3.19")
	Version string

	// Arch is the CPU architecture (e.g., "amd64", "arm64")
	Arch string
}

// DefaultsLoader loads and merges default parameter values from blueprints.
type DefaultsLoader struct {
	// Platform is the current platform info
	Platform PlatformInfo

	// baseDir is the blueprint base directory
	baseDir string
}

// NewDefaultsLoader creates a new defaults loader.
func NewDefaultsLoader(baseDir string) *DefaultsLoader {
	return &DefaultsLoader{
		Platform: DetectPlatform(),
		baseDir:  baseDir,
	}
}

// SetPlatform sets the platform for loading overrides.
func (d *DefaultsLoader) SetPlatform(platform PlatformInfo) {
	d.Platform = platform
}

// LoadDefaults loads default values from vars/defaults.yaml and platform overrides.
// Returns merged defaults in order of precedence (later overrides earlier):
// 1. Schema defaults (from blueprint.yaml parameters)
// 2. vars/defaults.yaml
// 3. vars/platforms/{family}.yaml (e.g., debian.yaml)
// 4. vars/platforms/{family}-{version}.yaml (e.g., debian-12.yaml)
// 5. vars/platforms/{os}.yaml (e.g., linux.yaml)
// 6. vars/platforms/{arch}.yaml (e.g., arm64.yaml)
func (d *DefaultsLoader) LoadDefaults(bp *Blueprint) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// 1. Extract defaults from parameter schemas
	d.extractSchemaDefaults("", bp.Parameters, result)

	// 2. Load vars/defaults.yaml
	defaults, err := d.loadYAMLFile("vars/defaults.yaml")
	if err == nil && defaults != nil {
		mergeParameters(result, defaults)
	}

	// 3. Load platform-specific overrides in order
	platformFiles := d.getPlatformFiles()
	for _, filename := range platformFiles {
		platformDefaults, err := d.loadYAMLFile(filepath.Join("vars", "platforms", filename))
		if err == nil && platformDefaults != nil {
			mergeParameters(result, platformDefaults)
		}
	}

	return result, nil
}

// LoadDefaultsFromReader loads defaults from a reader (for testing).
func (d *DefaultsLoader) LoadDefaultsFromReader(reader io.Reader) (map[string]interface{}, error) {
	var result map[string]interface{}
	decoder := yaml.NewDecoder(reader)
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse defaults YAML: %w", err)
	}
	return result, nil
}

// extractSchemaDefaults extracts default values from parameter schemas.
func (d *DefaultsLoader) extractSchemaDefaults(prefix string, schemas map[string]ParameterSchema, result map[string]interface{}) {
	for name, schema := range schemas {
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}

		if schema.Default != nil {
			setNestedValue(result, key, schema.Default)
		}

		// Recurse into object properties
		if schema.Type == "object" && schema.Properties != nil {
			nested := make(map[string]interface{})
			d.extractSchemaDefaults("", schema.Properties, nested)
			if len(nested) > 0 {
				current, _ := result[name].(map[string]interface{})
				if current == nil {
					current = make(map[string]interface{})
				}
				mergeParameters(current, nested)
				result[name] = current
			}
		}
	}
}

// getPlatformFiles returns the list of platform-specific files to check.
func (d *DefaultsLoader) getPlatformFiles() []string {
	var files []string

	// Order matters - later files override earlier ones

	// General OS file (e.g., linux.yaml)
	if d.Platform.OS != "" {
		files = append(files, d.Platform.OS+".yaml")
	}

	// Family file (e.g., debian.yaml)
	if d.Platform.Family != "" {
		files = append(files, d.Platform.Family+".yaml")
	}

	// Family with version (e.g., debian-12.yaml)
	if d.Platform.Family != "" && d.Platform.Version != "" {
		files = append(files, d.Platform.Family+"-"+d.Platform.Version+".yaml")
	}

	// Architecture file (e.g., arm64.yaml)
	if d.Platform.Arch != "" {
		files = append(files, d.Platform.Arch+".yaml")
	}

	// Combination: family-arch (e.g., debian-arm64.yaml)
	if d.Platform.Family != "" && d.Platform.Arch != "" {
		files = append(files, d.Platform.Family+"-"+d.Platform.Arch+".yaml")
	}

	return files
}

// loadYAMLFile loads a YAML file from the blueprint directory.
func (d *DefaultsLoader) loadYAMLFile(relativePath string) (map[string]interface{}, error) {
	fullPath := filepath.Join(d.baseDir, relativePath)

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File doesn't exist, not an error
		}
		return nil, err
	}
	defer file.Close()

	var result map[string]interface{}
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", relativePath, err)
	}

	return result, nil
}

// DetectPlatform detects the current platform information.
func DetectPlatform() PlatformInfo {
	platform := PlatformInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	// Detect Linux family
	if runtime.GOOS == "linux" {
		platform.Family, platform.Version = detectLinuxFamily()
	}

	return platform
}

// detectLinuxFamily detects the Linux distribution family and version.
func detectLinuxFamily() (family, version string) {
	// Try /etc/os-release first (modern systems)
	osRelease, err := os.ReadFile("/etc/os-release")
	if err == nil {
		return parseOSRelease(string(osRelease))
	}

	// Try /etc/lsb-release (Ubuntu/Debian)
	lsbRelease, err := os.ReadFile("/etc/lsb-release")
	if err == nil {
		return parseLSBRelease(string(lsbRelease))
	}

	// Try /etc/redhat-release (RHEL/CentOS)
	redhatRelease, err := os.ReadFile("/etc/redhat-release")
	if err == nil {
		return parseRedhatRelease(string(redhatRelease))
	}

	return "", ""
}

// parseOSRelease parses /etc/os-release content.
func parseOSRelease(content string) (family, version string) {
	var id, versionID, idLike string

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := strings.Trim(parts[1], "\"'")

		switch key {
		case "ID":
			id = value
		case "VERSION_ID":
			versionID = value
		case "ID_LIKE":
			idLike = value
		}
	}

	// Determine family from ID
	family = normalizeFamily(id)
	if family == "" && idLike != "" {
		// Try ID_LIKE (e.g., "debian" for Ubuntu)
		parts := strings.Split(idLike, " ")
		for _, like := range parts {
			f := normalizeFamily(like)
			if f != "" {
				family = f
				break
			}
		}
	}

	// Extract major version
	version = extractMajorVersion(versionID)

	return family, version
}

// parseLSBRelease parses /etc/lsb-release content.
func parseLSBRelease(content string) (family, version string) {
	var distID, release string

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := strings.Trim(parts[1], "\"'")

		switch key {
		case "DISTRIB_ID":
			distID = value
		case "DISTRIB_RELEASE":
			release = value
		}
	}

	family = normalizeFamily(strings.ToLower(distID))
	version = extractMajorVersion(release)

	return family, version
}

// parseRedhatRelease parses /etc/redhat-release content.
func parseRedhatRelease(content string) (family, version string) {
	content = strings.ToLower(strings.TrimSpace(content))

	if strings.Contains(content, "centos") {
		family = "rhel"
	} else if strings.Contains(content, "red hat") {
		family = "rhel"
	} else if strings.Contains(content, "fedora") {
		family = "fedora"
	}

	// Extract version number
	for _, word := range strings.Fields(content) {
		if len(word) > 0 && word[0] >= '0' && word[0] <= '9' {
			version = extractMajorVersion(word)
			break
		}
	}

	return family, version
}

// normalizeFamily normalizes distribution ID to a family name.
func normalizeFamily(id string) string {
	id = strings.ToLower(id)

	// Debian-based
	switch id {
	case "debian", "ubuntu", "raspbian", "linuxmint", "pop":
		return "debian"
	}

	// RHEL-based
	switch id {
	case "rhel", "centos", "rocky", "almalinux", "ol", "oracle":
		return "rhel"
	}

	// Other families
	switch id {
	case "fedora":
		return "fedora"
	case "alpine":
		return "alpine"
	case "arch", "manjaro":
		return "arch"
	case "opensuse", "suse", "sles":
		return "suse"
	case "amazon", "amzn":
		return "amazon"
	}

	return ""
}

// extractMajorVersion extracts the major version from a version string.
func extractMajorVersion(version string) string {
	if version == "" {
		return ""
	}

	// Handle versions like "12.1", "22.04", "9.2"
	parts := strings.Split(version, ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return version
}

// ApplyPlatformDefaults applies platform-specific defaults to parameter values.
// This merges defaults with user-provided values, with user values taking precedence.
func (d *DefaultsLoader) ApplyPlatformDefaults(bp *Blueprint, userParams map[string]interface{}) (map[string]interface{}, error) {
	// Load all defaults
	defaults, err := d.LoadDefaults(bp)
	if err != nil {
		return nil, err
	}

	// Start with defaults
	result := defaults

	// Merge user parameters (user values override defaults)
	if userParams != nil {
		mergeParameters(result, userParams)
	}

	return result, nil
}

// GetPlatformOverridesPath returns the path to platform-specific overrides file.
func (d *DefaultsLoader) GetPlatformOverridesPath() string {
	if d.Platform.Family != "" {
		if d.Platform.Version != "" {
			return filepath.Join("vars", "platforms", d.Platform.Family+"-"+d.Platform.Version+".yaml")
		}
		return filepath.Join("vars", "platforms", d.Platform.Family+".yaml")
	}
	return ""
}
