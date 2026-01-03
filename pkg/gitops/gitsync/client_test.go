package gitsync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func createTestRepo(t *testing.T, path string) *git.Repository {
	t.Helper()

	repo, err := git.PlainInit(path, false)
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	// Create initial commit
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	// Create a file
	testFile := filepath.Join(path, "README.md")
	err = os.WriteFile(testFile, []byte("# Test Repository"), 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	_, err = worktree.Add("README.md")
	if err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	return repo
}

func TestNewRepository(t *testing.T) {
	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "https://github.com/test/repo.git",
		Branch:    "main",
		LocalPath: "/tmp/test-repo",
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	if repo.config.Name != "test-repo" {
		t.Errorf("Name = %s, want test-repo", repo.config.Name)
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *RepositoryConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &RepositoryConfig{
				Name:      "test",
				URL:       "https://github.com/test/repo.git",
				Branch:    "main",
				LocalPath: "/tmp/test",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			cfg: &RepositoryConfig{
				URL:       "https://github.com/test/repo.git",
				Branch:    "main",
				LocalPath: "/tmp/test",
			},
			wantErr: true,
		},
		{
			name: "missing url",
			cfg: &RepositoryConfig{
				Name:      "test",
				Branch:    "main",
				LocalPath: "/tmp/test",
			},
			wantErr: true,
		},
		{
			name: "missing branch",
			cfg: &RepositoryConfig{
				Name:      "test",
				URL:       "https://github.com/test/repo.git",
				LocalPath: "/tmp/test",
			},
			wantErr: true,
		},
		{
			name: "missing local path",
			cfg: &RepositoryConfig{
				Name:   "test",
				URL:    "https://github.com/test/repo.git",
				Branch: "main",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetupAuth(t *testing.T) {
	tests := []struct {
		name    string
		auth    AuthConfig
		wantErr bool
	}{
		{
			name: "none auth",
			auth: AuthConfig{
				Type: AuthTypeNone,
			},
			wantErr: false,
		},
		{
			name: "token auth",
			auth: AuthConfig{
				Type:  AuthTypeToken,
				Token: "test-token",
			},
			wantErr: false,
		},
		{
			name: "token auth without token",
			auth: AuthConfig{
				Type: AuthTypeToken,
			},
			wantErr: true,
		},
		{
			name: "ssh auth without key",
			auth: AuthConfig{
				Type: AuthTypeSSH,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Repository{
				config: &RepositoryConfig{
					Auth: tt.auth,
				},
			}
			err := r.setupAuth()
			if (err != nil) != tt.wantErr {
				t.Errorf("setupAuth() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRepositoryOpen(t *testing.T) {
	tmpDir := t.TempDir()
	createTestRepo(t, tmpDir)

	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "file://" + tmpDir,
		Branch:    "master",
		LocalPath: tmpDir,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	err = repo.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if repo.repo == nil {
		t.Error("Repository should be initialized")
	}
}

func TestRepositoryCommit(t *testing.T) {
	tmpDir := t.TempDir()
	createTestRepo(t, tmpDir)

	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "file://" + tmpDir,
		Branch:    "master",
		LocalPath: tmpDir,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	err = repo.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Create a new file
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Commit the file
	ctx := context.Background()
	err = repo.Commit(ctx, &CommitRequest{
		Repository:  "test-repo",
		Message:     "Add test file",
		Files:       []string{"test.txt"},
		AuthorName:  "Test User",
		AuthorEmail: "test@example.com",
	})
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify commit exists
	head, err := repo.repo.Head()
	if err != nil {
		t.Fatalf("Failed to get HEAD: %v", err)
	}

	commit, err := repo.repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("Failed to get commit: %v", err)
	}

	if commit.Message != "Add test file" {
		t.Errorf("Commit message = %s, want 'Add test file'", commit.Message)
	}
}

func TestRepositoryCreateBranch(t *testing.T) {
	tmpDir := t.TempDir()
	createTestRepo(t, tmpDir)

	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "file://" + tmpDir,
		Branch:    "master",
		LocalPath: tmpDir,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	err = repo.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Create new branch
	err = repo.CreateBranch(&BranchRequest{
		Repository: "test-repo",
		BranchName: "feature-branch",
	})
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Verify branch exists
	_, err = repo.repo.Reference("refs/heads/feature-branch", false)
	if err != nil {
		t.Errorf("Branch should exist: %v", err)
	}
}

func TestRepositoryGetPathFiles(t *testing.T) {
	tmpDir := t.TempDir()
	createTestRepo(t, tmpDir)

	// Create subdirectory with files
	subdir := filepath.Join(tmpDir, "states")
	err := os.MkdirAll(subdir, 0755)
	if err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	err = os.WriteFile(filepath.Join(subdir, "state1.yaml"), []byte("content"), 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "file://" + tmpDir,
		Branch:    "master",
		LocalPath: tmpDir,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	err = repo.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	files, err := repo.GetPathFiles("states")
	if err != nil {
		t.Fatalf("GetPathFiles failed: %v", err)
	}

	if len(files) == 0 {
		t.Error("Should have found files")
	}
}

func TestRepositoryGetCurrentCommit(t *testing.T) {
	tmpDir := t.TempDir()
	createTestRepo(t, tmpDir)

	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "file://" + tmpDir,
		Branch:    "master",
		LocalPath: tmpDir,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	err = repo.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	commit, err := repo.GetCurrentCommit()
	if err != nil {
		t.Fatalf("GetCurrentCommit failed: %v", err)
	}

	if commit == "" {
		t.Error("Commit hash should not be empty")
	}

	if len(commit) != 40 {
		t.Errorf("Commit hash length = %d, want 40", len(commit))
	}
}

func TestManager(t *testing.T) {
	manager := NewManager()

	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "https://github.com/test/repo.git",
		Branch:    "main",
		LocalPath: "/tmp/test-repo",
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	err := manager.AddRepository(cfg)
	if err != nil {
		t.Fatalf("AddRepository failed: %v", err)
	}

	repo, ok := manager.GetRepository("test-repo")
	if !ok {
		t.Error("Repository should exist")
	}

	if repo.config.Name != "test-repo" {
		t.Errorf("Name = %s, want test-repo", repo.config.Name)
	}
}

func TestManagerSyncAll(t *testing.T) {
	manager := NewManager()

	tmpDir1 := t.TempDir()
	createTestRepo(t, tmpDir1)

	tmpDir2 := t.TempDir()
	createTestRepo(t, tmpDir2)

	cfg1 := &RepositoryConfig{
		Name:      "repo1",
		URL:       "file://" + tmpDir1,
		Branch:    "master",
		LocalPath: tmpDir1,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	cfg2 := &RepositoryConfig{
		Name:      "repo2",
		URL:       "file://" + tmpDir2,
		Branch:    "master",
		LocalPath: tmpDir2,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	err := manager.AddRepository(cfg1)
	if err != nil {
		t.Fatalf("AddRepository failed: %v", err)
	}

	err = manager.AddRepository(cfg2)
	if err != nil {
		t.Fatalf("AddRepository failed: %v", err)
	}

	// Open repos
	repo1, _ := manager.GetRepository("repo1")
	err = repo1.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	repo2, _ := manager.GetRepository("repo2")
	err = repo2.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	ctx := context.Background()
	results := manager.SyncAll(ctx)

	if len(results) != 2 {
		t.Errorf("Results count = %d, want 2", len(results))
	}
}

// createTestRepoWithMultipleCommits creates a test repo with multiple commits
func createTestRepoWithMultipleCommits(t *testing.T, path string, numCommits int) *git.Repository {
	t.Helper()

	gitRepo, err := git.PlainInit(path, false)
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	worktree, err := gitRepo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	for i := 0; i < numCommits; i++ {
		filename := filepath.Join(path, "file"+filepath.Base(time.Now().String())+".txt")
		err = os.WriteFile(filename, []byte("content "+time.Now().String()), 0644)
		if err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}

		_, err = worktree.Add(filepath.Base(filename))
		if err != nil {
			t.Fatalf("Failed to add file: %v", err)
		}

		_, err = worktree.Commit("Commit "+time.Now().String(), &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)
	}

	return gitRepo
}

func TestRepositoryGetPreviousCommit(t *testing.T) {
	tmpDir := t.TempDir()
	createTestRepoWithMultipleCommits(t, tmpDir, 3)

	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "file://" + tmpDir,
		Branch:    "master",
		LocalPath: tmpDir,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	err = repo.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	currentCommit, err := repo.GetCurrentCommit()
	if err != nil {
		t.Fatalf("GetCurrentCommit failed: %v", err)
	}

	previousCommit, err := repo.GetPreviousCommit()
	if err != nil {
		t.Fatalf("GetPreviousCommit failed: %v", err)
	}

	if previousCommit == "" {
		t.Error("Previous commit hash should not be empty")
	}

	if len(previousCommit) != 40 {
		t.Errorf("Previous commit hash length = %d, want 40", len(previousCommit))
	}

	if previousCommit == currentCommit {
		t.Error("Previous commit should be different from current commit")
	}
}

func TestRepositoryGetPreviousCommit_RootCommit(t *testing.T) {
	tmpDir := t.TempDir()
	// Create repo with only one commit
	createTestRepo(t, tmpDir)

	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "file://" + tmpDir,
		Branch:    "master",
		LocalPath: tmpDir,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	err = repo.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	_, err = repo.GetPreviousCommit()
	if err == nil {
		t.Error("GetPreviousCommit should fail for root commit")
	}
}

func TestRepositoryGetCommitHistory(t *testing.T) {
	tmpDir := t.TempDir()
	createTestRepoWithMultipleCommits(t, tmpDir, 5)

	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "file://" + tmpDir,
		Branch:    "master",
		LocalPath: tmpDir,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	err = repo.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Get all 5 commits
	history, err := repo.GetCommitHistory(5)
	if err != nil {
		t.Fatalf("GetCommitHistory failed: %v", err)
	}

	if len(history) != 5 {
		t.Errorf("History length = %d, want 5", len(history))
	}

	// Verify commits are in order (newest first)
	for i := 0; i < len(history)-1; i++ {
		if history[i].Timestamp.Before(history[i+1].Timestamp) {
			t.Errorf("Commits should be ordered newest first")
		}
	}

	// Verify each commit has required fields
	for i, commit := range history {
		if commit.Hash == "" {
			t.Errorf("Commit %d: Hash should not be empty", i)
		}
		if len(commit.Hash) != 40 {
			t.Errorf("Commit %d: Hash length = %d, want 40", i, len(commit.Hash))
		}
		if commit.Message == "" {
			t.Errorf("Commit %d: Message should not be empty", i)
		}
		if commit.Author == "" {
			t.Errorf("Commit %d: Author should not be empty", i)
		}
	}
}

func TestRepositoryGetCommitHistory_Limit(t *testing.T) {
	tmpDir := t.TempDir()
	createTestRepoWithMultipleCommits(t, tmpDir, 5)

	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "file://" + tmpDir,
		Branch:    "master",
		LocalPath: tmpDir,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	err = repo.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Get only 2 commits
	history, err := repo.GetCommitHistory(2)
	if err != nil {
		t.Fatalf("GetCommitHistory failed: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("History length = %d, want 2", len(history))
	}
}

func TestRepositoryGetCommit(t *testing.T) {
	tmpDir := t.TempDir()
	createTestRepoWithMultipleCommits(t, tmpDir, 3)

	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "file://" + tmpDir,
		Branch:    "master",
		LocalPath: tmpDir,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	err = repo.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Get current commit hash
	currentHash, err := repo.GetCurrentCommit()
	if err != nil {
		t.Fatalf("GetCurrentCommit failed: %v", err)
	}

	// Get commit info
	commit, err := repo.GetCommit(currentHash)
	if err != nil {
		t.Fatalf("GetCommit failed: %v", err)
	}

	if commit.Hash != currentHash {
		t.Errorf("Hash = %s, want %s", commit.Hash, currentHash)
	}

	if commit.Author == "" {
		t.Error("Author should not be empty")
	}

	if commit.AuthorEmail == "" {
		t.Error("AuthorEmail should not be empty")
	}

	if commit.Message == "" {
		t.Error("Message should not be empty")
	}

	// Current commit should have a parent
	if commit.ParentHash == "" {
		t.Error("ParentHash should not be empty for non-root commit")
	}
}

func TestRepositoryGetCommit_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	createTestRepo(t, tmpDir)

	cfg := &RepositoryConfig{
		Name:      "test-repo",
		URL:       "file://" + tmpDir,
		Branch:    "master",
		LocalPath: tmpDir,
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	err = repo.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Try to get a non-existent commit
	_, err = repo.GetCommit("0000000000000000000000000000000000000000")
	if err == nil {
		t.Error("GetCommit should fail for non-existent commit")
	}
}
