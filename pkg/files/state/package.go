// Package state provides integration between the file distribution system and state management.
package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/shawnbutts/keystone-core/pkg/files"
)

// PackageRepository represents a package repository configuration.
type PackageRepository struct {
	// Name is the repository name.
	Name string

	// URL is the repository URL (can be kscore://).
	URL string

	// Type is the repository type (yum, apt, etc.).
	Type string

	// Enabled indicates if the repository is enabled.
	Enabled bool

	// GPGCheck indicates if GPG checking is enabled.
	GPGCheck bool

	// GPGKey is the GPG key URL.
	GPGKey string

	// Priority is the repository priority.
	Priority int
}

// PackageFile represents a package file from the file server.
type PackageFile struct {
	// Name is the package name.
	Name string

	// Source is the package source URL (kscore://).
	Source string

	// Version is the package version.
	Version string

	// Checksum is the expected checksum.
	Checksum string

	// Architecture is the package architecture.
	Architecture string
}

// PackageManager manages package files and repositories.
type PackageManager struct {
	// resolver resolves file sources.
	resolver *FileSourceResolver

	// repoDir is the directory for repository files.
	repoDir string

	// cacheDir is the directory for cached packages.
	cacheDir string

	// mu protects operations.
	mu sync.Mutex
}

// PackageManagerConfig configures the package manager.
type PackageManagerConfig struct {
	// Client is the file distribution client.
	Client *files.Client

	// CacheDir is the directory for cached files.
	CacheDir string

	// RepoDir is the directory for repository files.
	RepoDir string
}

// NewPackageManager creates a new package manager.
func NewPackageManager(config *PackageManagerConfig) (*PackageManager, error) {
	// Create cache directory.
	if err := os.MkdirAll(config.CacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Create file cache.
	cache, err := NewLocalFileCache(config.CacheDir)
	if err != nil {
		return nil, err
	}

	// Create resolver.
	resolver := NewFileSourceResolver(config.Client, cache)

	return &PackageManager{
		resolver: resolver,
		repoDir:  config.RepoDir,
		cacheDir: config.CacheDir,
	}, nil
}

// DownloadPackage downloads a package file to the cache.
func (m *PackageManager) DownloadPackage(ctx context.Context, pkg *PackageFile) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build source URL with version and checksum.
	sourceURL := pkg.Source
	if pkg.Version != "" && !strings.Contains(sourceURL, "version=") {
		if strings.Contains(sourceURL, "?") {
			sourceURL += "&version=" + pkg.Version
		} else {
			sourceURL += "?version=" + pkg.Version
		}
	}
	if pkg.Checksum != "" && !strings.Contains(sourceURL, "checksum=") {
		if strings.Contains(sourceURL, "?") {
			sourceURL += "&checksum=" + pkg.Checksum
		} else {
			sourceURL += "?checksum=" + pkg.Checksum
		}
	}

	// Resolve the package file.
	localPath, err := m.resolver.Resolve(ctx, sourceURL)
	if err != nil {
		return "", fmt.Errorf("failed to download package %s: %w", pkg.Name, err)
	}

	return localPath, nil
}

// GetPackagePath returns the local path for a package.
func (m *PackageManager) GetPackagePath(pkg *PackageFile) string {
	filename := pkg.Name
	if pkg.Version != "" {
		filename += "-" + pkg.Version
	}
	if pkg.Architecture != "" {
		filename += "." + pkg.Architecture
	}
	return filepath.Join(m.cacheDir, filename)
}

// YumRepoConfig generates a yum repository configuration.
type YumRepoConfig struct {
	// BaseConfig contains common settings.
	BaseConfig

	// MirrorList is the mirror list URL.
	MirrorList string

	// MetadataExpire is the metadata expiry time.
	MetadataExpire string
}

// BaseConfig contains common repository settings.
type BaseConfig struct {
	// Name is the repository name.
	Name string

	// BaseURL is the base URL.
	BaseURL string

	// Enabled indicates if enabled.
	Enabled bool

	// GPGCheck indicates if GPG checking is enabled.
	GPGCheck bool

	// GPGKey is the GPG key URL.
	GPGKey string
}

// GenerateYumRepo generates a yum repository file content.
func GenerateYumRepo(repo *PackageRepository) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("[%s]\n", repo.Name))
	sb.WriteString(fmt.Sprintf("name=%s\n", repo.Name))
	sb.WriteString(fmt.Sprintf("baseurl=%s\n", repo.URL))

	if repo.Enabled {
		sb.WriteString("enabled=1\n")
	} else {
		sb.WriteString("enabled=0\n")
	}

	if repo.GPGCheck {
		sb.WriteString("gpgcheck=1\n")
		if repo.GPGKey != "" {
			sb.WriteString(fmt.Sprintf("gpgkey=%s\n", repo.GPGKey))
		}
	} else {
		sb.WriteString("gpgcheck=0\n")
	}

	if repo.Priority > 0 {
		sb.WriteString(fmt.Sprintf("priority=%d\n", repo.Priority))
	}

	return sb.String()
}

// GenerateAptSource generates an apt sources.list entry.
func GenerateAptSource(repo *PackageRepository) string {
	// Format: deb [options] uri suite [component1] [component2] [...]
	var sb strings.Builder

	if !repo.Enabled {
		sb.WriteString("# ")
	}

	sb.WriteString("deb ")

	// Add signed-by option if GPG key is provided.
	if repo.GPGKey != "" {
		sb.WriteString(fmt.Sprintf("[signed-by=%s] ", repo.GPGKey))
	}

	sb.WriteString(repo.URL)

	return sb.String()
}

// WriteYumRepo writes a yum repository file.
func (m *PackageManager) WriteYumRepo(repo *PackageRepository) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.repoDir == "" {
		return fmt.Errorf("repository directory not configured")
	}

	// Generate content.
	content := GenerateYumRepo(repo)

	// Write file.
	repoFile := filepath.Join(m.repoDir, repo.Name+".repo")
	if err := os.WriteFile(repoFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write repo file: %w", err)
	}

	return nil
}

// WriteAptSource writes an apt sources.list.d entry.
func (m *PackageManager) WriteAptSource(repo *PackageRepository) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.repoDir == "" {
		return fmt.Errorf("repository directory not configured")
	}

	// Generate content.
	content := GenerateAptSource(repo)

	// Write file.
	sourceFile := filepath.Join(m.repoDir, repo.Name+".list")
	if err := os.WriteFile(sourceFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write source file: %w", err)
	}

	return nil
}

// RemoveRepo removes a repository file.
func (m *PackageManager) RemoveRepo(name string, repoType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.repoDir == "" {
		return fmt.Errorf("repository directory not configured")
	}

	var filename string
	switch repoType {
	case "yum", "dnf":
		filename = name + ".repo"
	case "apt":
		filename = name + ".list"
	default:
		return fmt.Errorf("unsupported repository type: %s", repoType)
	}

	repoFile := filepath.Join(m.repoDir, filename)
	if err := os.Remove(repoFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove repo file: %w", err)
	}

	return nil
}

// RepositoryMirror represents a mirror of a package repository.
type RepositoryMirror struct {
	// Name is the mirror name.
	Name string

	// SourceURL is the source repository URL.
	SourceURL string

	// LocalPath is the local mirror path.
	LocalPath string

	// Includes is a list of package patterns to include.
	Includes []string

	// Excludes is a list of package patterns to exclude.
	Excludes []string

	// SyncInterval is how often to sync.
	SyncInterval string
}

// MirrorManager manages repository mirrors.
type MirrorManager struct {
	// resolver resolves file sources.
	resolver *FileSourceResolver

	// mirrors contains configured mirrors.
	mirrors map[string]*RepositoryMirror

	// mu protects mirrors.
	mu sync.RWMutex
}

// NewMirrorManager creates a new mirror manager.
func NewMirrorManager(resolver *FileSourceResolver) *MirrorManager {
	return &MirrorManager{
		resolver: resolver,
		mirrors:  make(map[string]*RepositoryMirror),
	}
}

// AddMirror adds a repository mirror configuration.
func (m *MirrorManager) AddMirror(mirror *RepositoryMirror) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if mirror.Name == "" {
		return fmt.Errorf("mirror name is required")
	}

	if mirror.SourceURL == "" {
		return fmt.Errorf("source URL is required")
	}

	if mirror.LocalPath == "" {
		return fmt.Errorf("local path is required")
	}

	m.mirrors[mirror.Name] = mirror
	return nil
}

// RemoveMirror removes a repository mirror.
func (m *MirrorManager) RemoveMirror(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.mirrors, name)
}

// GetMirror returns a mirror by name.
func (m *MirrorManager) GetMirror(name string) (*RepositoryMirror, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mirror, ok := m.mirrors[name]
	return mirror, ok
}

// ListMirrors returns all configured mirrors.
func (m *MirrorManager) ListMirrors() []*RepositoryMirror {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mirrors := make([]*RepositoryMirror, 0, len(m.mirrors))
	for _, mirror := range m.mirrors {
		mirrors = append(mirrors, mirror)
	}
	return mirrors
}

// ShouldInclude checks if a package should be included based on patterns.
func (mirror *RepositoryMirror) ShouldInclude(packageName string) bool {
	// If excludes match, exclude.
	for _, pattern := range mirror.Excludes {
		if matchPattern(pattern, packageName) {
			return false
		}
	}

	// If no includes, include everything.
	if len(mirror.Includes) == 0 {
		return true
	}

	// Check if any include matches.
	for _, pattern := range mirror.Includes {
		if matchPattern(pattern, packageName) {
			return true
		}
	}

	return false
}

// matchPattern matches a simple glob pattern.
func matchPattern(pattern, name string) bool {
	// Simple glob matching.
	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		return strings.Contains(name, pattern[1:len(pattern)-1])
	}

	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(name, pattern[1:])
	}

	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(name, pattern[:len(pattern)-1])
	}

	return pattern == name
}
