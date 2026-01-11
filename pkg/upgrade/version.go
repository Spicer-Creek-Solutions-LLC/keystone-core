package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrVersionNotFound indicates the requested version was not found.
	ErrVersionNotFound = errors.New("version not found")

	// ErrInvalidVersion indicates an invalid version string.
	ErrInvalidVersion = errors.New("invalid version string")

	// ErrIncompatibleVersion indicates versions are not compatible.
	ErrIncompatibleVersion = errors.New("incompatible version")

	// ErrVerificationFailed indicates version verification failed.
	ErrVerificationFailed = errors.New("version verification failed")
)

// semverRegex matches semantic version strings.
var semverRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

// ParseVersion parses a version string into a Version struct.
func ParseVersion(s string) (Version, error) {
	if s == "" {
		return Version{}, ErrInvalidVersion
	}

	matches := semverRegex.FindStringSubmatch(s)
	if matches == nil {
		return Version{}, fmt.Errorf("%w: %s", ErrInvalidVersion, s)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	return Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: matches[4],
		Build:      matches[5],
	}, nil
}

// MustParseVersion parses a version string and panics on error.
func MustParseVersion(s string) Version {
	v, err := ParseVersion(s)
	if err != nil {
		panic(err)
	}
	return v
}

// VersionRange represents a range of versions.
type VersionRange struct {
	Min          *Version
	Max          *Version
	IncludeMin   bool
	IncludeMax   bool
	ExcludeRange []*Version // Specific versions to exclude
}

// Contains checks if a version is within the range.
func (r *VersionRange) Contains(v Version) bool {
	// Check excluded versions
	for _, excluded := range r.ExcludeRange {
		if excluded != nil && v.Compare(*excluded) == 0 {
			return false
		}
	}

	// Check minimum
	if r.Min != nil {
		cmp := v.Compare(*r.Min)
		if r.IncludeMin {
			if cmp < 0 {
				return false
			}
		} else {
			if cmp <= 0 {
				return false
			}
		}
	}

	// Check maximum
	if r.Max != nil {
		cmp := v.Compare(*r.Max)
		if r.IncludeMax {
			if cmp > 0 {
				return false
			}
		} else {
			if cmp >= 0 {
				return false
			}
		}
	}

	return true
}

// CompatibilityMatrix defines version compatibility between components.
type CompatibilityMatrix struct {
	Component ComponentType              `json:"component" yaml:"component"`
	Entries   []CompatibilityEntry       `json:"entries" yaml:"entries"`
	SkipRules []VersionSkipRule          `json:"skip_rules,omitempty" yaml:"skip_rules,omitempty"`
}

// CompatibilityEntry defines compatibility between two versions.
type CompatibilityEntry struct {
	Version      Version   `json:"version" yaml:"version"`
	MinUpgrade   *Version  `json:"min_upgrade,omitempty" yaml:"min_upgrade,omitempty"`
	MaxUpgrade   *Version  `json:"max_upgrade,omitempty" yaml:"max_upgrade,omitempty"`
	Dependencies []string  `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Breaking     bool      `json:"breaking,omitempty" yaml:"breaking,omitempty"`
	MigrationURL string    `json:"migration_url,omitempty" yaml:"migration_url,omitempty"`
}

// VersionSkipRule defines when version skipping is allowed.
type VersionSkipRule struct {
	From     *Version `json:"from,omitempty" yaml:"from,omitempty"`
	To       *Version `json:"to,omitempty" yaml:"to,omitempty"`
	MaxJump  int      `json:"max_jump,omitempty" yaml:"max_jump,omitempty"`  // Maximum minor versions to skip
	Allowed  bool     `json:"allowed" yaml:"allowed"`
	Reason   string   `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// VersionChecker checks version compatibility.
type VersionChecker struct {
	matrices map[ComponentType]*CompatibilityMatrix
	logger   Logger
}

// NewVersionChecker creates a new version checker.
func NewVersionChecker(logger Logger) *VersionChecker {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &VersionChecker{
		matrices: make(map[ComponentType]*CompatibilityMatrix),
		logger:   logger,
	}
}

// LoadMatrix loads a compatibility matrix for a component.
func (c *VersionChecker) LoadMatrix(component ComponentType, matrix *CompatibilityMatrix) {
	c.matrices[component] = matrix
}

// CheckCompatibility checks if upgrading from one version to another is compatible.
func (c *VersionChecker) CheckCompatibility(component ComponentType, from, to Version) (*CompatibilityResult, error) {
	result := &CompatibilityResult{
		Compatible: true,
		From:       from,
		To:         to,
		Component:  component,
	}

	// Basic version comparison
	cmp := from.Compare(to)
	if cmp == 0 {
		result.Warnings = append(result.Warnings, "Source and target versions are the same")
		return result, nil
	}
	if cmp > 0 {
		result.IsDowngrade = true
		result.Warnings = append(result.Warnings, "This is a downgrade operation")
	}

	// Check matrix if available
	matrix, ok := c.matrices[component]
	if !ok {
		result.Warnings = append(result.Warnings, "No compatibility matrix available, proceeding with caution")
		return result, nil
	}

	// Find target version entry
	var targetEntry *CompatibilityEntry
	for i := range matrix.Entries {
		if matrix.Entries[i].Version.Compare(to) == 0 {
			targetEntry = &matrix.Entries[i]
			break
		}
	}

	if targetEntry == nil {
		result.Compatible = false
		result.Blockers = append(result.Blockers, fmt.Sprintf(
			"Target version %s not in compatibility matrix", to,
		))
		return result, nil
	}

	// Check minimum upgrade version
	if targetEntry.MinUpgrade != nil {
		if from.Compare(*targetEntry.MinUpgrade) < 0 {
			result.Compatible = false
			result.Blockers = append(result.Blockers, fmt.Sprintf(
				"Cannot upgrade from %s to %s: minimum supported upgrade version is %s",
				from, to, targetEntry.MinUpgrade,
			))
		}
	}

	// Check maximum upgrade version
	if targetEntry.MaxUpgrade != nil {
		if from.Compare(*targetEntry.MaxUpgrade) > 0 {
			result.Compatible = false
			result.Blockers = append(result.Blockers, fmt.Sprintf(
				"Cannot upgrade from %s to %s: maximum supported upgrade version is %s",
				from, to, targetEntry.MaxUpgrade,
			))
		}
	}

	// Check for breaking changes
	if targetEntry.Breaking {
		result.Warnings = append(result.Warnings, "This version contains breaking changes")
		if targetEntry.MigrationURL != "" {
			result.RequiredSteps = append(result.RequiredSteps, fmt.Sprintf(
				"Review migration guide: %s", targetEntry.MigrationURL,
			))
		}
	}

	// Check skip rules
	for _, rule := range matrix.SkipRules {
		if rule.MaxJump > 0 && !result.IsDowngrade {
			minorDiff := to.Minor - from.Minor
			if to.Major == from.Major && minorDiff > rule.MaxJump {
				if !rule.Allowed {
					result.Compatible = false
					result.Blockers = append(result.Blockers, fmt.Sprintf(
						"Cannot skip %d minor versions (max allowed: %d): %s",
						minorDiff, rule.MaxJump, rule.Reason,
					))
				} else {
					result.Warnings = append(result.Warnings, fmt.Sprintf(
						"Skipping %d minor versions: %s", minorDiff, rule.Reason,
					))
				}
			}
		}
	}

	return result, nil
}

// CompatibilityResult contains the result of a compatibility check.
type CompatibilityResult struct {
	Compatible    bool          `json:"compatible" yaml:"compatible"`
	From          Version       `json:"from" yaml:"from"`
	To            Version       `json:"to" yaml:"to"`
	Component     ComponentType `json:"component" yaml:"component"`
	IsDowngrade   bool          `json:"is_downgrade" yaml:"is_downgrade"`
	Warnings      []string      `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Blockers      []string      `json:"blockers,omitempty" yaml:"blockers,omitempty"`
	RequiredSteps []string      `json:"required_steps,omitempty" yaml:"required_steps,omitempty"`
}

// HTTPVersionProvider retrieves version information from an HTTP endpoint.
type HTTPVersionProvider struct {
	baseURL    string
	httpClient *http.Client
	logger     Logger
	cacheDir   string
}

// NewHTTPVersionProvider creates a new HTTP version provider.
func NewHTTPVersionProvider(baseURL string, logger Logger) *HTTPVersionProvider {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &HTTPVersionProvider{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:   logger,
		cacheDir: filepath.Join(os.TempDir(), "kscore-versions"),
	}
}

// SetCacheDir sets the cache directory for downloaded versions.
func (p *HTTPVersionProvider) SetCacheDir(dir string) {
	p.cacheDir = dir
}

// GetCurrentVersion returns the current installed version.
func (p *HTTPVersionProvider) GetCurrentVersion(ctx context.Context, component ComponentType) (Version, error) {
	// This would typically read from the installed binary or a version file
	// For now, return a placeholder
	return Version{}, errors.New("GetCurrentVersion not implemented - requires local inspection")
}

// GetAvailableVersions returns available versions for a component.
func (p *HTTPVersionProvider) GetAvailableVersions(ctx context.Context, component ComponentType, channel string) ([]VersionInfo, error) {
	url := fmt.Sprintf("%s/versions/%s", p.baseURL, component)
	if channel != "" {
		url += "?channel=" + channel
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var versions []VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return versions, nil
}

// GetVersionInfo returns detailed information about a version.
func (p *HTTPVersionProvider) GetVersionInfo(ctx context.Context, component ComponentType, version string) (*VersionInfo, error) {
	url := fmt.Sprintf("%s/versions/%s/%s", p.baseURL, component, version)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching version info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrVersionNotFound
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var info VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &info, nil
}

// DownloadVersion downloads a specific version.
func (p *HTTPVersionProvider) DownloadVersion(ctx context.Context, component ComponentType, version string) (string, error) {
	// Ensure cache directory exists
	if err := os.MkdirAll(p.cacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache directory: %w", err)
	}

	// Check if already downloaded
	localPath := filepath.Join(p.cacheDir, fmt.Sprintf("%s-%s", component, version))
	if _, err := os.Stat(localPath); err == nil {
		p.logger.Debug("Version already cached", "component", component, "version", version, "path", localPath)
		return localPath, nil
	}

	url := fmt.Sprintf("%s/downloads/%s/%s", p.baseURL, component, version)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrVersionNotFound
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	// Create temp file and copy content
	tmpFile, err := os.CreateTemp(p.cacheDir, fmt.Sprintf("%s-%s-*", component, version))
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("writing file: %w", err)
	}

	// Rename to final location
	if err := os.Rename(tmpPath, localPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("moving file: %w", err)
	}

	p.logger.Info("Downloaded version", "component", component, "version", version, "path", localPath)
	return localPath, nil
}

// VerifyVersion verifies the integrity of a downloaded version.
func (p *HTTPVersionProvider) VerifyVersion(ctx context.Context, component ComponentType, version string, path string) error {
	// Get version info for checksum
	info, err := p.GetVersionInfo(ctx, component, version)
	if err != nil {
		return fmt.Errorf("getting version info: %w", err)
	}

	if info.Checksum == "" {
		p.logger.Warn("No checksum available for verification", "component", component, "version", version)
		return nil
	}

	// Calculate file checksum
	checksum, err := calculateChecksum(path)
	if err != nil {
		return fmt.Errorf("calculating checksum: %w", err)
	}

	if checksum != info.Checksum {
		return fmt.Errorf("%w: expected %s, got %s", ErrVerificationFailed, info.Checksum, checksum)
	}

	p.logger.Debug("Version checksum verified", "component", component, "version", version)
	return nil
}

// calculateChecksum calculates the SHA-256 checksum of a file.
func calculateChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// LocalVersionProvider reads version information from local files.
type LocalVersionProvider struct {
	versionsDir string
	binaryDir   string
	logger      Logger
}

// NewLocalVersionProvider creates a new local version provider.
func NewLocalVersionProvider(versionsDir, binaryDir string, logger Logger) *LocalVersionProvider {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &LocalVersionProvider{
		versionsDir: versionsDir,
		binaryDir:   binaryDir,
		logger:      logger,
	}
}

// GetCurrentVersion returns the current installed version by inspecting binaries.
func (p *LocalVersionProvider) GetCurrentVersion(ctx context.Context, component ComponentType) (Version, error) {
	// Try reading version file
	versionFile := filepath.Join(p.binaryDir, string(component)+".version")
	data, err := os.ReadFile(versionFile)
	if err == nil {
		return ParseVersion(strings.TrimSpace(string(data)))
	}

	// Could also try running binary with --version flag
	return Version{}, fmt.Errorf("could not determine current version for %s", component)
}

// GetAvailableVersions returns available versions from local storage.
func (p *LocalVersionProvider) GetAvailableVersions(ctx context.Context, component ComponentType, channel string) ([]VersionInfo, error) {
	pattern := filepath.Join(p.versionsDir, string(component), "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("listing versions: %w", err)
	}

	var versions []VersionInfo
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			p.logger.Warn("Failed to read version file", "file", file, "error", err)
			continue
		}

		var info VersionInfo
		if err := json.Unmarshal(data, &info); err != nil {
			p.logger.Warn("Failed to parse version file", "file", file, "error", err)
			continue
		}

		if channel == "" || info.Channel == channel {
			versions = append(versions, info)
		}
	}

	return versions, nil
}

// GetVersionInfo returns detailed information about a version.
func (p *LocalVersionProvider) GetVersionInfo(ctx context.Context, component ComponentType, version string) (*VersionInfo, error) {
	path := filepath.Join(p.versionsDir, string(component), version+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading version info: %w", err)
	}

	var info VersionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parsing version info: %w", err)
	}

	return &info, nil
}

// DownloadVersion returns the path to a local version (no download needed).
func (p *LocalVersionProvider) DownloadVersion(ctx context.Context, component ComponentType, version string) (string, error) {
	path := filepath.Join(p.versionsDir, string(component), version)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", ErrVersionNotFound
	}
	return path, nil
}

// VerifyVersion verifies the integrity of a local version.
func (p *LocalVersionProvider) VerifyVersion(ctx context.Context, component ComponentType, version string, path string) error {
	info, err := p.GetVersionInfo(ctx, component, version)
	if err != nil {
		return err
	}

	if info.Checksum == "" {
		return nil
	}

	checksum, err := calculateChecksum(path)
	if err != nil {
		return fmt.Errorf("calculating checksum: %w", err)
	}

	if checksum != info.Checksum {
		return fmt.Errorf("%w: expected %s, got %s", ErrVerificationFailed, info.Checksum, checksum)
	}

	return nil
}

// noopLogger is a no-op logger implementation.
type noopLogger struct{}

func (l *noopLogger) Debug(msg string, args ...interface{}) {}
func (l *noopLogger) Info(msg string, args ...interface{})  {}
func (l *noopLogger) Warn(msg string, args ...interface{})  {}
func (l *noopLogger) Error(msg string, args ...interface{}) {}
