package gitsync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// Repository manages a Git repository
type Repository struct {
	config *RepositoryConfig
	repo   *git.Repository
	auth   transport.AuthMethod
	mu     sync.RWMutex
}

// NewRepository creates a new repository manager
func NewRepository(cfg *RepositoryConfig) (*Repository, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	r := &Repository{
		config: cfg,
	}

	// Set up authentication
	if err := r.setupAuth(); err != nil {
		return nil, fmt.Errorf("failed to setup auth: %w", err)
	}

	return r, nil
}

// validateConfig validates repository configuration
func validateConfig(cfg *RepositoryConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("name is required")
	}
	if cfg.URL == "" {
		return fmt.Errorf("url is required")
	}
	if cfg.Branch == "" {
		return fmt.Errorf("branch is required")
	}
	if cfg.LocalPath == "" {
		return fmt.Errorf("local_path is required")
	}
	return nil
}

// setupAuth configures authentication for the repository
func (r *Repository) setupAuth() error {
	switch r.config.Auth.Type {
	case AuthTypeNone:
		r.auth = nil
	case AuthTypeToken:
		if r.config.Auth.Token == "" {
			return fmt.Errorf("token is required for token auth")
		}
		r.auth = &http.BasicAuth{
			Username: r.config.Auth.Username,
			Password: r.config.Auth.Token,
		}
	case AuthTypeSSH:
		if r.config.Auth.SSHKeyPath == "" {
			return fmt.Errorf("ssh_key_path is required for ssh auth")
		}
		publicKeys, err := ssh.NewPublicKeysFromFile("git", r.config.Auth.SSHKeyPath, r.config.Auth.SSHKeyPassphrase)
		if err != nil {
			return fmt.Errorf("failed to load ssh key: %w", err)
		}
		r.auth = publicKeys
	default:
		return fmt.Errorf("unknown auth type: %s", r.config.Auth.Type)
	}
	return nil
}

// Clone clones the repository to the local path
func (r *Repository) Clone(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if directory already exists
	if _, err := os.Stat(r.config.LocalPath); err == nil {
		return fmt.Errorf("directory already exists: %s", r.config.LocalPath)
	}

	// Clone repository
	repo, err := git.PlainCloneContext(ctx, r.config.LocalPath, false, &git.CloneOptions{
		URL:           r.config.URL,
		Auth:          r.auth,
		ReferenceName: plumbing.NewBranchReferenceName(r.config.Branch),
		SingleBranch:  true,
		Progress:      nil,
	})
	if err != nil {
		return fmt.Errorf("failed to clone: %w", err)
	}

	r.repo = repo
	return nil
}

// Open opens an existing repository
func (r *Repository) Open() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	repo, err := git.PlainOpen(r.config.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	r.repo = repo
	return nil
}

// Sync syncs the repository with the remote
func (r *Repository) Sync(ctx context.Context) (*SyncResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	result := &SyncResult{
		Repository: r.config.Name,
		Timestamp:  time.Now(),
	}

	// Get current commit
	head, err := r.repo.Head()
	if err != nil {
		result.Success = false
		result.Error = err
		result.Message = fmt.Sprintf("Failed to get HEAD: %v", err)
		return result, err
	}
	result.PreviousCommit = head.Hash().String()

	// Fetch from remote
	worktree, err := r.repo.Worktree()
	if err != nil {
		result.Success = false
		result.Error = err
		result.Message = fmt.Sprintf("Failed to get worktree: %v", err)
		return result, err
	}

	err = worktree.PullContext(ctx, &git.PullOptions{
		RemoteName:    "origin",
		ReferenceName: plumbing.NewBranchReferenceName(r.config.Branch),
		Auth:          r.auth,
		Force:         false,
	})

	if err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			result.Success = true
			result.Updated = false
			result.CurrentCommit = result.PreviousCommit
			result.Message = "Already up to date"
			return result, nil
		}
		result.Success = false
		result.Error = err
		result.Message = fmt.Sprintf("Failed to pull: %v", err)
		return result, err
	}

	// Get new commit
	head, err = r.repo.Head()
	if err != nil {
		result.Success = false
		result.Error = err
		result.Message = fmt.Sprintf("Failed to get new HEAD: %v", err)
		return result, err
	}
	result.CurrentCommit = head.Hash().String()

	// Get changed files
	if result.PreviousCommit != result.CurrentCommit {
		result.Updated = true
		filesChanged, err := r.getChangedFiles(result.PreviousCommit, result.CurrentCommit)
		if err == nil {
			result.FilesChanged = filesChanged
		}
	}

	result.Success = true
	result.Message = fmt.Sprintf("Updated from %s to %s", result.PreviousCommit[:7], result.CurrentCommit[:7])
	return result, nil
}

// getChangedFiles returns the list of files changed between two commits
func (r *Repository) getChangedFiles(from, to string) ([]string, error) {
	fromHash := plumbing.NewHash(from)
	toHash := plumbing.NewHash(to)

	fromCommit, err := r.repo.CommitObject(fromHash)
	if err != nil {
		return nil, err
	}

	toCommit, err := r.repo.CommitObject(toHash)
	if err != nil {
		return nil, err
	}

	fromTree, err := fromCommit.Tree()
	if err != nil {
		return nil, err
	}

	toTree, err := toCommit.Tree()
	if err != nil {
		return nil, err
	}

	changes, err := fromTree.Diff(toTree)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, change := range changes {
		files = append(files, change.To.Name)
	}

	return files, nil
}

// Commit creates a commit with the specified files
func (r *Repository) Commit(ctx context.Context, req *CommitRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.repo == nil {
		return fmt.Errorf("repository not initialized")
	}

	worktree, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add files
	for _, file := range req.Files {
		_, err := worktree.Add(file)
		if err != nil {
			return fmt.Errorf("failed to add file %s: %w", file, err)
		}
	}

	// Create commit
	_, err = worktree.Commit(req.Message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  req.AuthorName,
			Email: req.AuthorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// Push pushes commits to the remote repository
func (r *Repository) Push(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.repo == nil {
		return fmt.Errorf("repository not initialized")
	}

	err := r.repo.PushContext(ctx, &git.PushOptions{
		RemoteName: "origin",
		Auth:       r.auth,
	})
	if err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil
		}
		return fmt.Errorf("failed to push: %w", err)
	}

	return nil
}

// CreateBranch creates a new branch
func (r *Repository) CreateBranch(req *BranchRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.repo == nil {
		return fmt.Errorf("repository not initialized")
	}

	worktree, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Create branch reference
	branchRef := plumbing.NewBranchReferenceName(req.BranchName)

	// Get the reference to branch from
	var fromRef *plumbing.Reference
	if req.FromBranch != "" {
		fromRef, err = r.repo.Reference(plumbing.NewBranchReferenceName(req.FromBranch), true)
		if err != nil {
			return fmt.Errorf("failed to get from branch: %w", err)
		}
	} else {
		fromRef, err = r.repo.Head()
		if err != nil {
			return fmt.Errorf("failed to get HEAD: %w", err)
		}
	}

	// Create the branch
	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: branchRef,
		Hash:   fromRef.Hash(),
		Create: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	return nil
}

// GetPathFiles returns files in a specific path within the repository
func (r *Repository) GetPathFiles(path string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fullPath := filepath.Join(r.config.LocalPath, path)
	var files []string

	err := filepath.Walk(fullPath, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, err := filepath.Rel(r.config.LocalPath, p)
			if err != nil {
				return err
			}
			files = append(files, relPath)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk path: %w", err)
	}

	return files, nil
}

// GetCurrentCommit returns the current commit hash
func (r *Repository) GetCurrentCommit() (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.repo == nil {
		return "", fmt.Errorf("repository not initialized")
	}

	head, err := r.repo.Head()
	if err != nil {
		return "", err
	}

	return head.Hash().String(), nil
}

// GetPreviousCommit returns the parent commit of HEAD
func (r *Repository) GetPreviousCommit() (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.repo == nil {
		return "", fmt.Errorf("repository not initialized")
	}

	head, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	commit, err := r.repo.CommitObject(head.Hash())
	if err != nil {
		return "", fmt.Errorf("failed to get commit: %w", err)
	}

	// Get parent commit
	if commit.NumParents() == 0 {
		return "", fmt.Errorf("no parent commit (HEAD is root commit)")
	}

	parent, err := commit.Parent(0)
	if err != nil {
		return "", fmt.Errorf("failed to get parent commit: %w", err)
	}

	return parent.Hash.String(), nil
}

// CommitInfo represents information about a commit
type CommitInfo struct {
	Hash        string
	Message     string
	Author      string
	AuthorEmail string
	Timestamp   time.Time
	ParentHash  string
}

// GetCommitHistory returns the last n commits from HEAD
func (r *Repository) GetCommitHistory(limit int) ([]*CommitInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	if limit <= 0 {
		limit = 10 // default
	}

	head, err := r.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Get commit log iterator
	logIter, err := r.repo.Log(&git.LogOptions{
		From:  head.Hash(),
		Order: git.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get log: %w", err)
	}
	defer logIter.Close()

	var commits []*CommitInfo
	count := 0

	err = logIter.ForEach(func(c *object.Commit) error {
		if count >= limit {
			return fmt.Errorf("limit reached") // Stop iteration
		}

		info := &CommitInfo{
			Hash:        c.Hash.String(),
			Message:     c.Message,
			Author:      c.Author.Name,
			AuthorEmail: c.Author.Email,
			Timestamp:   c.Author.When,
		}

		// Get parent hash if exists
		if c.NumParents() > 0 {
			parent, err := c.Parent(0)
			if err == nil {
				info.ParentHash = parent.Hash.String()
			}
		}

		commits = append(commits, info)
		count++
		return nil
	})

	// Ignore "limit reached" error
	if err != nil && err.Error() != "limit reached" {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return commits, nil
}

// GetCommit returns information about a specific commit
func (r *Repository) GetCommit(hash string) (*CommitInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	commitHash := plumbing.NewHash(hash)
	commit, err := r.repo.CommitObject(commitHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	info := &CommitInfo{
		Hash:        commit.Hash.String(),
		Message:     commit.Message,
		Author:      commit.Author.Name,
		AuthorEmail: commit.Author.Email,
		Timestamp:   commit.Author.When,
	}

	if commit.NumParents() > 0 {
		parent, err := commit.Parent(0)
		if err == nil {
			info.ParentHash = parent.Hash.String()
		}
	}

	return info, nil
}

// Manager manages multiple repositories
type Manager struct {
	repos map[string]*Repository
	mu    sync.RWMutex
}

// NewManager creates a new repository manager
func NewManager() *Manager {
	return &Manager{
		repos: make(map[string]*Repository),
	}
}

// AddRepository adds a repository to the manager
func (m *Manager) AddRepository(cfg *RepositoryConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	repo, err := NewRepository(cfg)
	if err != nil {
		return err
	}

	m.repos[cfg.Name] = repo
	return nil
}

// GetRepository returns a repository by name
func (m *Manager) GetRepository(name string) (*Repository, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	repo, ok := m.repos[name]
	return repo, ok
}

// SyncAll syncs all repositories
func (m *Manager) SyncAll(ctx context.Context) []*SyncResult {
	m.mu.RLock()
	repos := make([]*Repository, 0, len(m.repos))
	for _, repo := range m.repos {
		repos = append(repos, repo)
	}
	m.mu.RUnlock()

	results := make([]*SyncResult, 0, len(repos))
	for _, repo := range repos {
		result, err := repo.Sync(ctx)
		if err != nil && result == nil {
			result = &SyncResult{
				Repository: repo.config.Name,
				Success:    false,
				Error:      err,
				Message:    err.Error(),
				Timestamp:  time.Now(),
			}
		}
		results = append(results, result)
	}

	return results
}

// Watch starts watching repositories for changes
func (m *Manager) Watch(ctx context.Context) {
	m.mu.RLock()
	repos := make([]*Repository, 0, len(m.repos))
	for _, repo := range m.repos {
		repos = append(repos, repo)
	}
	m.mu.RUnlock()

	for _, repo := range repos {
		go m.watchRepository(ctx, repo)
	}
}

// watchRepository watches a single repository
func (m *Manager) watchRepository(ctx context.Context, repo *Repository) {
	ticker := time.NewTicker(repo.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = repo.Sync(ctx)
		}
	}
}
