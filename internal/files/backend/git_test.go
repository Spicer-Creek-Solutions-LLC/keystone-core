package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// MockGitRepository implements GitRepository for testing.
type MockGitRepository struct {
	files      map[string]*GitFile
	commitInfo *GitCommitInfo
	opened     bool
	cloned     bool
}

func NewMockGitRepository() *MockGitRepository {
	return &MockGitRepository{
		files: make(map[string]*GitFile),
		commitInfo: &GitCommitInfo{
			Hash:        "abc123def456",
			Author:      "Test Author",
			AuthorEmail: "test@example.com",
			Message:     "Test commit",
			Time:        time.Now(),
		},
	}
}

func (m *MockGitRepository) AddFile(path string, content []byte, modTime time.Time) {
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]

	m.files[path] = &GitFile{
		Content: content,
		Info: &GitFileInfo{
			Path:    path,
			Name:    name,
			Size:    int64(len(content)),
			Hash:    fmt.Sprintf("blob-%x", content[:min(8, len(content))]),
			ModTime: modTime,
		},
	}
}

func (m *MockGitRepository) Open(ctx context.Context) error {
	m.opened = true
	return nil
}

func (m *MockGitRepository) Clone(ctx context.Context) error {
	m.cloned = true
	return nil
}

func (m *MockGitRepository) Pull(ctx context.Context) error {
	return nil
}

func (m *MockGitRepository) Checkout(ctx context.Context, ref string) error {
	return nil
}

func (m *MockGitRepository) GetFile(ctx context.Context, path string) (*GitFile, error) {
	file, ok := m.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return file, nil
}

func (m *MockGitRepository) ListFiles(ctx context.Context, path string, recursive bool) ([]*GitFileInfo, error) {
	var result []*GitFileInfo
	for filePath, file := range m.files {
		if path == "" || strings.HasPrefix(filePath, path) {
			if !recursive {
				// For non-recursive, only include direct children
				remaining := strings.TrimPrefix(filePath, path)
				remaining = strings.TrimPrefix(remaining, "/")
				if strings.Contains(remaining, "/") {
					continue
				}
			}
			result = append(result, file.Info)
		}
	}
	return result, nil
}

func (m *MockGitRepository) GetCommit(ctx context.Context) (*GitCommitInfo, error) {
	return m.commitInfo, nil
}

func (m *MockGitRepository) FileExists(ctx context.Context, path string) (bool, error) {
	_, ok := m.files[path]
	return ok, nil
}

func TestNewGitBackend(t *testing.T) {
	tests := []struct {
		name    string
		config  *GitConfig
		wantErr bool
	}{
		{
			name: "valid config with branch",
			config: &GitConfig{
				URL:       "https://github.com/example/repo.git",
				LocalPath: "/tmp/repo",
				Branch:    "main",
			},
			wantErr: false,
		},
		{
			name: "valid config with tag",
			config: &GitConfig{
				URL:       "https://github.com/example/repo.git",
				LocalPath: "/tmp/repo",
				Tag:       "v1.0.0",
			},
			wantErr: false,
		},
		{
			name: "valid config defaults to main branch",
			config: &GitConfig{
				URL:       "https://github.com/example/repo.git",
				LocalPath: "/tmp/repo",
			},
			wantErr: false,
		},
		{
			name: "missing URL",
			config: &GitConfig{
				LocalPath: "/tmp/repo",
			},
			wantErr: true,
		},
		{
			name: "missing local path",
			config: &GitConfig{
				URL: "https://github.com/example/repo.git",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := NewGitBackend(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if backend == nil {
					t.Error("expected backend, got nil")
				}
			}
		})
	}
}

func TestGitBackend_BasicOperations(t *testing.T) {
	config := &GitConfig{
		Config: Config{
			Name: "test-git",
			Type: TypeGit,
		},
		URL:       "https://github.com/example/repo.git",
		LocalPath: "/tmp/repo",
	}

	backend, err := NewGitBackend(config)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}

	repo := NewMockGitRepository()
	repo.AddFile("README.md", []byte("# Test Repository"), time.Now())
	repo.AddFile("configs/app.yaml", []byte("key: value"), time.Now())
	backend.SetRepository(repo)

	ctx := context.Background()

	// Test Name and Type
	if backend.Name() != "test-git" {
		t.Errorf("expected name 'test-git', got '%s'", backend.Name())
	}
	if backend.Type() != TypeGit {
		t.Errorf("expected type %s, got %s", TypeGit, backend.Type())
	}

	// Test Get
	result, err := backend.Get(ctx, "/README.md", nil)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer result.Reader.Close()

	data, _ := io.ReadAll(result.Reader)
	if !bytes.Equal(data, []byte("# Test Repository")) {
		t.Errorf("expected content '# Test Repository', got '%s'", string(data))
	}

	// Test Exists
	exists, err := backend.Exists(ctx, "/README.md")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("expected file to exist")
	}

	// Test Stat
	info, err := backend.Stat(ctx, "/README.md")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Name != "README.md" {
		t.Errorf("expected name 'README.md', got '%s'", info.Name)
	}

	// Test List
	listResult, err := backend.List(ctx, "", &ListOptions{Recursive: true})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(listResult.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(listResult.Files))
	}
}

func TestGitBackend_ReadOnly(t *testing.T) {
	config := &GitConfig{
		URL:       "https://github.com/example/repo.git",
		LocalPath: "/tmp/repo",
	}

	backend, _ := NewGitBackend(config)
	repo := NewMockGitRepository()
	backend.SetRepository(repo)

	ctx := context.Background()

	// Test Put is not allowed
	_, err := backend.Put(ctx, "/test.txt", bytes.NewReader([]byte("test")), nil)
	if err == nil {
		t.Error("expected error for Put on git backend")
	}

	// Test Delete is not allowed
	err = backend.Delete(ctx, "/test.txt")
	if err == nil {
		t.Error("expected error for Delete on git backend")
	}
}

func TestGitBackend_NotFound(t *testing.T) {
	config := &GitConfig{
		URL:       "https://github.com/example/repo.git",
		LocalPath: "/tmp/repo",
	}

	backend, _ := NewGitBackend(config)
	repo := NewMockGitRepository()
	backend.SetRepository(repo)

	ctx := context.Background()

	// Test Get not found
	_, err := backend.Get(ctx, "/nonexistent.txt", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Test Stat not found
	_, err = backend.Stat(ctx, "/nonexistent.txt")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Test Exists returns false for nonexistent
	exists, err := backend.Exists(ctx, "/nonexistent.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("expected file to not exist")
	}
}

func TestGitBackend_ConditionalGet(t *testing.T) {
	config := &GitConfig{
		URL:       "https://github.com/example/repo.git",
		LocalPath: "/tmp/repo",
	}

	backend, _ := NewGitBackend(config)
	repo := NewMockGitRepository()
	repo.AddFile("test.txt", []byte("test content"), time.Now())
	backend.SetRepository(repo)

	ctx := context.Background()

	// Get info to get the checksum
	info, err := backend.Stat(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	// Conditional get with matching checksum should return NotModified
	result, err := backend.Get(ctx, "/test.txt", &GetOptions{
		IfNoneMatch: info.Checksum,
	})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !result.NotModified {
		t.Error("expected NotModified to be true")
	}

	// Conditional get with matching blob hash should also return NotModified
	result, err = backend.Get(ctx, "/test.txt", &GetOptions{
		IfNoneMatch: info.ETag, // Git blob hash
	})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !result.NotModified {
		t.Error("expected NotModified to be true for blob hash")
	}

	// Conditional get with non-matching should return content
	result, err = backend.Get(ctx, "/test.txt", &GetOptions{
		IfNoneMatch: "different-hash",
	})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if result.NotModified {
		t.Error("expected NotModified to be false")
	}
	result.Reader.Close()
}

func TestGitBackend_Prefix(t *testing.T) {
	config := &GitConfig{
		URL:       "https://github.com/example/repo.git",
		LocalPath: "/tmp/repo",
		Prefix:    "configs",
	}

	backend, _ := NewGitBackend(config)
	repo := NewMockGitRepository()
	repo.AddFile("configs/app.yaml", []byte("key: value"), time.Now())
	backend.SetRepository(repo)

	ctx := context.Background()

	// Get with prefix - should look for configs/app.yaml
	result, err := backend.Get(ctx, "/app.yaml", nil)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer result.Reader.Close()

	data, _ := io.ReadAll(result.Reader)
	if string(data) != "key: value" {
		t.Errorf("expected 'key: value', got '%s'", string(data))
	}
}

func TestGitBackend_RangeRequest(t *testing.T) {
	config := &GitConfig{
		URL:       "https://github.com/example/repo.git",
		LocalPath: "/tmp/repo",
	}

	backend, _ := NewGitBackend(config)
	repo := NewMockGitRepository()
	repo.AddFile("test.txt", []byte("Hello, World!"), time.Now())
	backend.SetRepository(repo)

	ctx := context.Background()

	// Test range request
	result, err := backend.Get(ctx, "/test.txt", &GetOptions{
		Range: &ByteRange{Start: 7, End: 12},
	})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer result.Reader.Close()

	data, _ := io.ReadAll(result.Reader)
	if string(data) != "World" {
		t.Errorf("expected 'World', got '%s'", string(data))
	}
}

func TestGitBackend_NoRepository(t *testing.T) {
	config := &GitConfig{
		URL:       "https://github.com/example/repo.git",
		LocalPath: "/tmp/repo",
	}

	backend, _ := NewGitBackend(config)
	// Don't set repository

	ctx := context.Background()

	// All operations should fail with "not configured" error
	_, err := backend.Get(ctx, "/test.txt", nil)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got: %v", err)
	}

	_, err = backend.Exists(ctx, "/test.txt")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got: %v", err)
	}

	_, err = backend.Stat(ctx, "/test.txt")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got: %v", err)
	}

	_, err = backend.List(ctx, "", nil)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got: %v", err)
	}

	err = backend.Health(ctx)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got: %v", err)
	}
}

func TestGitBackend_Health(t *testing.T) {
	config := &GitConfig{
		URL:       "https://github.com/example/repo.git",
		LocalPath: "/tmp/repo",
	}

	backend, _ := NewGitBackend(config)
	repo := NewMockGitRepository()
	backend.SetRepository(repo)

	ctx := context.Background()

	err := backend.Health(ctx)
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

func TestGitBackend_Pull(t *testing.T) {
	config := &GitConfig{
		URL:       "https://github.com/example/repo.git",
		LocalPath: "/tmp/repo",
	}

	backend, _ := NewGitBackend(config)
	repo := NewMockGitRepository()
	backend.SetRepository(repo)

	ctx := context.Background()

	err := backend.Pull(ctx)
	if err != nil {
		t.Errorf("Pull failed: %v", err)
	}
}

func TestGitBackend_GetCurrentCommit(t *testing.T) {
	config := &GitConfig{
		URL:       "https://github.com/example/repo.git",
		LocalPath: "/tmp/repo",
	}

	backend, _ := NewGitBackend(config)
	repo := NewMockGitRepository()
	backend.SetRepository(repo)

	ctx := context.Background()

	commit, err := backend.GetCurrentCommit(ctx)
	if err != nil {
		t.Fatalf("GetCurrentCommit failed: %v", err)
	}
	if commit.Hash != "abc123def456" {
		t.Errorf("expected hash 'abc123def456', got '%s'", commit.Hash)
	}
	if commit.Author != "Test Author" {
		t.Errorf("expected author 'Test Author', got '%s'", commit.Author)
	}
}

func TestIsGitNotFound(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{fmt.Errorf("file not found"), true},
		{fmt.Errorf("object not found"), true},
		{fmt.Errorf("path not found"), true},
		{fmt.Errorf("does not exist"), true},
		{fmt.Errorf("some other error"), false},
	}

	for _, tt := range tests {
		result := isGitNotFound(tt.err)
		if result != tt.expected {
			t.Errorf("isGitNotFound(%v) = %v, expected %v", tt.err, result, tt.expected)
		}
	}
}
