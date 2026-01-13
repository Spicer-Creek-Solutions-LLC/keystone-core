// Package backend provides storage backend implementations for the file server.
package backend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// GitConfig is the configuration for the Git repository backend.
type GitConfig struct {
	Config `yaml:",inline"`

	// URL is the Git repository URL (HTTPS, SSH, or local path).
	URL string `yaml:"url"`

	// Branch is the branch to serve files from (default: main).
	Branch string `yaml:"branch,omitempty"`

	// Tag is a specific tag to serve files from (alternative to branch).
	Tag string `yaml:"tag,omitempty"`

	// Commit is a specific commit SHA to serve files from.
	Commit string `yaml:"commit,omitempty"`

	// LocalPath is the local directory to clone/pull the repository to.
	LocalPath string `yaml:"local_path"`

	// Prefix is an optional path prefix within the repository.
	Prefix string `yaml:"prefix,omitempty"`

	// SSHKeyFile is the path to the SSH private key for authentication.
	SSHKeyFile string `yaml:"ssh_key_file,omitempty"`

	// SSHKeyPassword is the password for the SSH key (if encrypted).
	SSHKeyPassword string `yaml:"ssh_key_password,omitempty"`

	// Username is the username for HTTPS authentication.
	Username string `yaml:"username,omitempty"`

	// Password is the password or token for HTTPS authentication.
	Password string `yaml:"password,omitempty"`

	// AutoPull enables automatic pulling on a schedule.
	AutoPull bool `yaml:"auto_pull,omitempty"`

	// PullInterval is how often to pull updates (default: 5m).
	PullInterval time.Duration `yaml:"pull_interval,omitempty"`

	// Depth is the clone depth (0 for full history).
	Depth int `yaml:"depth,omitempty"`

	// InsecureSkipTLS skips TLS verification for HTTPS repositories.
	InsecureSkipTLS bool `yaml:"insecure_skip_tls,omitempty"`
}

// GitBackend implements the Backend interface for Git repositories.
// This is a stub implementation that provides the interface without
// actual go-git dependencies. For production use, wire up the github.com/go-git/go-git/v5 client.
type GitBackend struct {
	config *GitConfig
	repo   GitRepository
	mu     sync.RWMutex
}

// GitRepository is an interface for Git repository operations.
// This allows for mocking in tests and alternative implementations.
type GitRepository interface {
	// Open opens an existing repository at the local path.
	Open(ctx context.Context) error

	// Clone clones the repository to the local path.
	Clone(ctx context.Context) error

	// Pull pulls the latest changes from the remote.
	Pull(ctx context.Context) error

	// Checkout checks out a specific branch, tag, or commit.
	Checkout(ctx context.Context, ref string) error

	// GetFile retrieves a file from the repository.
	GetFile(ctx context.Context, path string) (*GitFile, error)

	// ListFiles lists files in a directory.
	ListFiles(ctx context.Context, path string, recursive bool) ([]*GitFileInfo, error)

	// GetCommit returns the current commit information.
	GetCommit(ctx context.Context) (*GitCommitInfo, error)

	// FileExists checks if a file exists.
	FileExists(ctx context.Context, path string) (bool, error)
}

// GitFile represents a file from the repository.
type GitFile struct {
	// Content is the file content.
	Content []byte

	// Info is the file metadata.
	Info *GitFileInfo
}

// GitFileInfo contains file metadata from Git.
type GitFileInfo struct {
	// Path is the file path relative to the repository root.
	Path string

	// Name is the file name.
	Name string

	// Size is the file size in bytes.
	Size int64

	// Mode is the file mode (permissions).
	Mode uint32

	// IsDir indicates if this is a directory.
	IsDir bool

	// Hash is the Git blob hash.
	Hash string

	// ModTime is the last commit time affecting this file.
	ModTime time.Time
}

// GitCommitInfo contains commit information.
type GitCommitInfo struct {
	// Hash is the commit SHA.
	Hash string

	// Author is the commit author.
	Author string

	// AuthorEmail is the author's email.
	AuthorEmail string

	// Message is the commit message.
	Message string

	// Time is the commit timestamp.
	Time time.Time
}

// NewGitBackend creates a new Git repository backend.
func NewGitBackend(config *GitConfig) (*GitBackend, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("git backend: url is required")
	}
	if config.LocalPath == "" {
		return nil, fmt.Errorf("git backend: local_path is required")
	}

	// Set defaults
	if config.Branch == "" && config.Tag == "" && config.Commit == "" {
		config.Branch = "main"
	}
	if config.PullInterval == 0 {
		config.PullInterval = 5 * time.Minute
	}

	return &GitBackend{
		config: config,
	}, nil
}

// SetRepository sets the Git repository for the backend.
// This should be called after creating the backend to wire up the actual go-git client.
func (b *GitBackend) SetRepository(repo GitRepository) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.repo = repo
}

// Name returns the backend name.
func (b *GitBackend) Name() string {
	return b.config.Name
}

// Type returns the backend type.
func (b *GitBackend) Type() BackendType {
	return BackendTypeGit
}

// BaseConfig returns the base configuration for path matching and priority.
func (b *GitBackend) BaseConfig() *Config {
	return &b.config.Config
}

// Get retrieves a file from the Git repository.
func (b *GitBackend) Get(ctx context.Context, filePath string, opts *GetOptions) (*GetResult, error) {
	b.mu.RLock()
	repo := b.repo
	b.mu.RUnlock()

	if repo == nil {
		return nil, &BackendError{Op: "get", Path: filePath, Err: fmt.Errorf("git repository not configured")}
	}

	fullPath := b.fullPath(filePath)

	file, err := repo.GetFile(ctx, fullPath)
	if err != nil {
		if isGitNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, &BackendError{Op: "get", Path: filePath, Err: err}
	}

	// Calculate SHA256 checksum
	hash := sha256.Sum256(file.Content)
	checksum := hex.EncodeToString(hash[:])

	// Handle conditional get
	if opts != nil && opts.IfNoneMatch != "" {
		if opts.IfNoneMatch == checksum || opts.IfNoneMatch == file.Info.Hash {
			return &GetResult{NotModified: true}, nil
		}
	}

	// Detect content type
	contentType := http.DetectContentType(file.Content)

	// Handle range requests
	content := file.Content
	if opts != nil && opts.Range != nil {
		start := opts.Range.Start
		end := opts.Range.End
		if end == 0 || end > int64(len(content)) {
			end = int64(len(content))
		}
		if start < int64(len(content)) {
			content = content[start:end]
		}
	}

	return &GetResult{
		Reader: io.NopCloser(bytes.NewReader(content)),
		Info: &FileInfo{
			Name:         path.Base(filePath),
			Path:         filePath,
			Size:         int64(len(file.Content)),
			ContentType:  contentType,
			Checksum:     checksum,
			ModifiedTime: file.Info.ModTime,
			ETag:         file.Info.Hash,
			Version:      file.Info.Hash,
		},
	}, nil
}

// Put is not supported for Git backend (read-only).
func (b *GitBackend) Put(ctx context.Context, filePath string, reader io.Reader, opts *PutOptions) (*PutResult, error) {
	return nil, &BackendError{Op: "put", Path: filePath, Err: ErrReadOnly}
}

// Delete is not supported for Git backend (read-only).
func (b *GitBackend) Delete(ctx context.Context, filePath string) error {
	return &BackendError{Op: "delete", Path: filePath, Err: ErrReadOnly}
}

// Exists checks if a file exists in the Git repository.
func (b *GitBackend) Exists(ctx context.Context, filePath string) (bool, error) {
	b.mu.RLock()
	repo := b.repo
	b.mu.RUnlock()

	if repo == nil {
		return false, &BackendError{Op: "exists", Path: filePath, Err: fmt.Errorf("git repository not configured")}
	}

	fullPath := b.fullPath(filePath)

	exists, err := repo.FileExists(ctx, fullPath)
	if err != nil {
		if isGitNotFound(err) {
			return false, nil
		}
		return false, &BackendError{Op: "exists", Path: filePath, Err: err}
	}

	return exists, nil
}

// Stat returns file metadata from the Git repository.
func (b *GitBackend) Stat(ctx context.Context, filePath string) (*FileInfo, error) {
	b.mu.RLock()
	repo := b.repo
	b.mu.RUnlock()

	if repo == nil {
		return nil, &BackendError{Op: "stat", Path: filePath, Err: fmt.Errorf("git repository not configured")}
	}

	fullPath := b.fullPath(filePath)

	file, err := repo.GetFile(ctx, fullPath)
	if err != nil {
		if isGitNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, &BackendError{Op: "stat", Path: filePath, Err: err}
	}

	// Calculate SHA256 checksum
	hash := sha256.Sum256(file.Content)
	checksum := hex.EncodeToString(hash[:])

	// Detect content type
	contentType := http.DetectContentType(file.Content)

	return &FileInfo{
		Name:         path.Base(filePath),
		Path:         filePath,
		Size:         int64(len(file.Content)),
		ContentType:  contentType,
		Checksum:     checksum,
		ModifiedTime: file.Info.ModTime,
		ETag:         file.Info.Hash,
		Version:      file.Info.Hash,
		IsDirectory:  file.Info.IsDir,
	}, nil
}

// List lists files in the Git repository with a prefix.
func (b *GitBackend) List(ctx context.Context, prefix string, opts *ListOptions) (*ListResult, error) {
	b.mu.RLock()
	repo := b.repo
	b.mu.RUnlock()

	if repo == nil {
		return nil, &BackendError{Op: "list", Path: prefix, Err: fmt.Errorf("git repository not configured")}
	}

	fullPath := b.fullPath(prefix)

	recursive := opts != nil && opts.Recursive
	files, err := repo.ListFiles(ctx, fullPath, recursive)
	if err != nil {
		return nil, &BackendError{Op: "list", Path: prefix, Err: err}
	}

	var result []FileInfo
	for _, f := range files {
		// Remove the base prefix to get the relative path
		relPath := "/" + strings.TrimPrefix(f.Path, b.config.Prefix)
		if b.config.Prefix != "" {
			relPath = "/" + strings.TrimPrefix(f.Path, b.config.Prefix+"/")
		}

		result = append(result, FileInfo{
			Name:         f.Name,
			Path:         relPath,
			Size:         f.Size,
			ModifiedTime: f.ModTime,
			ETag:         f.Hash,
			Version:      f.Hash,
			IsDirectory:  f.IsDir,
		})
	}

	return &ListResult{
		Files:     result,
		Truncated: false,
	}, nil
}

// Health checks Git repository connectivity.
func (b *GitBackend) Health(ctx context.Context) error {
	b.mu.RLock()
	repo := b.repo
	b.mu.RUnlock()

	if repo == nil {
		return fmt.Errorf("git repository not configured")
	}

	_, err := repo.GetCommit(ctx)
	if err != nil {
		return fmt.Errorf("git health check failed: %w", err)
	}

	return nil
}

// Close closes the Git backend.
func (b *GitBackend) Close() error {
	return nil
}

// Pull pulls the latest changes from the remote repository.
func (b *GitBackend) Pull(ctx context.Context) error {
	b.mu.RLock()
	repo := b.repo
	b.mu.RUnlock()

	if repo == nil {
		return fmt.Errorf("git repository not configured")
	}

	return repo.Pull(ctx)
}

// GetCurrentCommit returns the current commit information.
func (b *GitBackend) GetCurrentCommit(ctx context.Context) (*GitCommitInfo, error) {
	b.mu.RLock()
	repo := b.repo
	b.mu.RUnlock()

	if repo == nil {
		return nil, fmt.Errorf("git repository not configured")
	}

	return repo.GetCommit(ctx)
}

// fullPath returns the full path within the repository including the prefix.
func (b *GitBackend) fullPath(filePath string) string {
	// Remove leading slash
	p := strings.TrimPrefix(filePath, "/")

	if b.config.Prefix != "" {
		p = b.config.Prefix + "/" + p
	}

	return p
}

// isGitNotFound checks if an error indicates the file was not found.
func isGitNotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "file not found") ||
		strings.Contains(errStr, "object not found") ||
		strings.Contains(errStr, "path not found") ||
		strings.Contains(errStr, "does not exist")
}
