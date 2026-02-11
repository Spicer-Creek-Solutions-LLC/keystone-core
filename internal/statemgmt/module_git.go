package statemgmt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ============================================================================
// Git Module - Manage Git repositories
// ============================================================================

// GitModule manages Git repositories
type GitModule struct {
	*BaseModule
}

// NewGitModule creates a new Git module
func NewGitModule() *GitModule {
	return &GitModule{
		BaseModule: NewBaseModule("git", []string{"present", "absent", "latest"}),
	}
}

// Check examines the current state of a Git repository
func (m *GitModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	dest := getStringParameter(decl, "dest", "")
	if dest == "" {
		return nil, fmt.Errorf("dest parameter is required")
	}

	repo := getStringParameter(decl, "repo", "")
	if repo == "" && decl.State != "absent" {
		return nil, fmt.Errorf("repo parameter is required for state %s", decl.State)
	}

	version := getStringParameter(decl, "version", "HEAD")

	result := &ModuleCheckResult{
		Metadata: make(map[string]interface{}),
	}

	// Check if destination exists
	info, err := os.Stat(dest)
	if os.IsNotExist(err) {
		result.Present = false
		result.Matches = decl.State == "absent"
		return result, nil //nolint:nilerr // error captured in result.Error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat %s: %w", dest, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("dest %s exists but is not a directory", dest)
	}

	// Check if it's a git repository
	gitDir := filepath.Join(dest, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		result.Present = false
		result.Matches = decl.State == "absent"
		result.Metadata["exists"] = true
		result.Metadata["is_git_repo"] = false
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	result.Present = true
	result.Metadata["exists"] = true
	result.Metadata["is_git_repo"] = true

	// Get current remote URL
	remoteURL, err := m.getRemoteURL(ctx, dest)
	if err == nil {
		result.Metadata["remote_url"] = remoteURL
	}

	// Get current commit
	currentCommit, err := m.getCurrentCommit(ctx, dest)
	if err == nil {
		result.Metadata["current_commit"] = currentCommit
	}

	// Get current branch
	currentBranch, err := m.getCurrentBranch(ctx, dest)
	if err == nil {
		result.Metadata["current_branch"] = currentBranch
	}

	// Check if clean
	isClean, err := m.isWorkingTreeClean(ctx, dest)
	if err == nil {
		result.Metadata["is_clean"] = isClean
	}

	switch decl.State {
	case "absent":
		result.Matches = false
		result.Diff = map[string]interface{}{
			"current": "present",
			"desired": "absent",
		}
	case "present":
		// Just needs to exist as a git repo
		result.Matches = true
		result.CurrentState = "present"
	case "latest":
		// Check if we're at the desired version
		if version == "HEAD" || version == "" {
			// Fetch and compare with origin
			behindCount, err := m.getBehindCount(ctx, dest)
			if err == nil {
				result.Metadata["behind_count"] = behindCount
				result.Matches = behindCount == 0
				if behindCount > 0 {
					result.Diff = map[string]interface{}{
						"behind": behindCount,
					}
				}
			} else {
				// Can't determine, assume needs update
				result.Matches = false
			}
		} else {
			// Check if at specific version/tag/branch
			atVersion, err := m.isAtVersion(ctx, dest, version)
			if err == nil {
				result.Matches = atVersion
				if !atVersion {
					result.Diff = map[string]interface{}{
						"current_version": currentCommit,
						"desired_version": version,
					}
				}
			} else {
				result.Matches = false
			}
		}
		result.CurrentState = "present"
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply ensures the Git repository is in the desired state
func (m *GitModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	dest := getStringParameter(decl, "dest", "")
	repo := getStringParameter(decl, "repo", "")
	version := getStringParameter(decl, "version", "HEAD")
	force := getBoolParameter(decl, "force", false)
	depth := getIntParameter(decl, "depth", 0)
	recursive := getBoolParameter(decl, "recursive", true)
	sshKey := getStringParameter(decl, "ssh_key", "")

	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	check, err := m.Check(ctx, decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Check failed: %v", err)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	if check.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Repository already in desired state"
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	switch decl.State {
	case "absent":
		if check.Present {
			if err := os.RemoveAll(dest); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to remove repository: %v", err)
				return result, nil //nolint:nilerr // error captured in result.Error
			}
			result.Success = true
			result.Changed = true
			result.Comment = fmt.Sprintf("Removed repository at %s", dest)
		} else {
			result.Success = true
			result.Changed = false
			result.Comment = "Repository already absent"
		}

	case "present", "latest":
		switch {
		case !check.Present:
			// Clone the repository
			args := []string{"clone"}
			if depth > 0 {
				args = append(args, "--depth", fmt.Sprintf("%d", depth))
			}
			if recursive {
				args = append(args, "--recursive")
			}
			if version != "" && version != "HEAD" {
				args = append(args, "--branch", version)
			}
			args = append(args, repo, dest)

			if err := m.runGitCommand(ctx, "", args, sshKey); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to clone repository: %v", err)
				return result, nil //nolint:nilerr // error captured in result.Error
			}
			result.Success = true
			result.Changed = true
			result.Comment = fmt.Sprintf("Cloned %s to %s", repo, dest)
		case decl.State == "latest":
			// Repository exists, update it
			if force {
				// Reset hard to clean state
				if err := m.runGitCommand(ctx, dest, []string{"reset", "--hard"}, ""); err != nil {
					result.Success = false
					result.Comment = fmt.Sprintf("Failed to reset repository: %v", err)
					return result, nil //nolint:nilerr // error captured in result.Error
				}
				// Clean untracked files
				if err := m.runGitCommand(ctx, dest, []string{"clean", "-fd"}, ""); err != nil {
					result.Success = false
					result.Comment = fmt.Sprintf("Failed to clean repository: %v", err)
					return result, nil //nolint:nilerr // error captured in result.Error
				}
			}

			// Fetch updates
			if err := m.runGitCommand(ctx, dest, []string{"fetch", "--all"}, sshKey); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to fetch updates: %v", err)
				return result, nil //nolint:nilerr // error captured in result.Error
			}

			// Checkout/pull to desired version
			if version != "" && version != "HEAD" {
				if err := m.runGitCommand(ctx, dest, []string{"checkout", version}, ""); err != nil {
					result.Success = false
					result.Comment = fmt.Sprintf("Failed to checkout %s: %v", version, err)
					return result, nil //nolint:nilerr // error captured in result.Error
				}
			}

			// Pull if on a branch
			currentBranch, _ := m.getCurrentBranch(ctx, dest)
			if currentBranch != "" && !strings.HasPrefix(currentBranch, "(") {
				pullArgs := []string{"pull"}
				if force {
					pullArgs = append(pullArgs, "--force")
				}
				if err := m.runGitCommand(ctx, dest, pullArgs, sshKey); err != nil {
					// Pull might fail if detached HEAD, that's ok
					if !strings.Contains(err.Error(), "detached HEAD") {
						result.Success = false
						result.Comment = fmt.Sprintf("Failed to pull updates: %v", err)
						return result, nil //nolint:nilerr // error captured in result.Error
					}
				}
			}

			// Update submodules if recursive
			if recursive {
				if err := m.runGitCommand(ctx, dest, []string{"submodule", "update", "--init", "--recursive"}, sshKey); err != nil {
					// Submodule update might fail if no submodules, that's ok
					if !strings.Contains(err.Error(), "No submodule") {
						result.Success = false
						result.Comment = fmt.Sprintf("Failed to update submodules: %v", err)
						return result, nil //nolint:nilerr // error captured in result.Error
					}
				}
			}

			result.Success = true
			result.Changed = true
			result.Comment = fmt.Sprintf("Updated repository at %s", dest)
		default:
			result.Success = true
			result.Changed = false
			result.Comment = "Repository already present"
		}
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Test validates module parameters
func (m *GitModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	check, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return check.Matches, nil
}

// Helper methods

func (m *GitModule) runGitCommand(ctx context.Context, dir string, args []string, sshKey string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	// Set up SSH key if provided
	if sshKey != "" {
		sshCmd := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no", sshKey)
		cmd.Env = append(os.Environ(), "GIT_SSH_COMMAND="+sshCmd)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

func (m *GitModule) getRemoteURL(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil //nolint:nilerr // intentional
}

func (m *GitModule) getCurrentCommit(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil //nolint:nilerr // intentional
}

func (m *GitModule) getCurrentBranch(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil //nolint:nilerr // intentional
}

func (m *GitModule) isWorkingTreeClean(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(output)) == "", nil //nolint:nilerr // intentional
}

func (m *GitModule) getBehindCount(ctx context.Context, dir string) (int, error) {
	// Fetch first
	exec.CommandContext(ctx, "git", "-C", dir, "fetch").Run()

	// Get behind count
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-list", "--count", "HEAD..@{u}")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	var count int
	fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count)
	return count, nil //nolint:nilerr // intentional
}

func (m *GitModule) isAtVersion(ctx context.Context, dir, version string) (bool, error) {
	// Get commit hash of the version
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", version)
	versionCommit, err := cmd.Output()
	if err != nil {
		return false, err
	}

	// Get current commit
	currentCommit, err := m.getCurrentCommit(ctx, dir)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(string(versionCommit)) == currentCommit, nil //nolint:nilerr // intentional
}

// ============================================================================
// Git Config Module - Manage Git configuration
// ============================================================================

// GitConfigModule manages Git configuration settings
type GitConfigModule struct {
	*BaseModule
}

// NewGitConfigModule creates a new Git config module
func NewGitConfigModule() *GitConfigModule {
	return &GitConfigModule{
		BaseModule: NewBaseModule("git_config", []string{"present", "absent"}),
	}
}

// Check examines the current state of a Git configuration setting
func (m *GitConfigModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	scope := getStringParameter(decl, "scope", "global")
	file := getStringParameter(decl, "file", "")
	value := getStringParameter(decl, "value", "")

	result := &ModuleCheckResult{
		Metadata: make(map[string]interface{}),
	}

	// Get current value
	currentValue, err := m.getConfigValue(ctx, name, scope, file)
	if err != nil {
		result.Present = false
		result.Matches = decl.State == "absent"
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	result.Present = true
	result.Metadata["current_value"] = currentValue
	result.Metadata["scope"] = scope

	switch decl.State {
	case "absent":
		result.Matches = false
		result.Diff = map[string]interface{}{
			"current": currentValue,
			"desired": "absent",
		}
	case "present":
		if value == "" {
			// Just check existence
			result.Matches = true
		} else {
			result.Matches = currentValue == value
			if !result.Matches {
				result.Diff = map[string]interface{}{
					"current": currentValue,
					"desired": value,
				}
			}
		}
	}

	result.CurrentState = "present"
	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply ensures the Git configuration is in the desired state
func (m *GitConfigModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	name := getStringParameter(decl, "name", "")
	value := getStringParameter(decl, "value", "")
	scope := getStringParameter(decl, "scope", "global")
	file := getStringParameter(decl, "file", "")

	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	check, err := m.Check(ctx, decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Check failed: %v", err)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	if check.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Git config already in desired state"
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	switch decl.State {
	case "absent":
		if check.Present {
			if err := m.unsetConfigValue(ctx, name, scope, file); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to unset config: %v", err)
				return result, nil //nolint:nilerr // error captured in result.Error
			}
			result.Success = true
			result.Changed = true
			result.Comment = fmt.Sprintf("Removed git config %s", name)
		} else {
			result.Success = true
			result.Changed = false
			result.Comment = "Git config already absent"
		}

	case "present":
		if err := m.setConfigValue(ctx, name, value, scope, file); err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to set config: %v", err)
			return result, nil //nolint:nilerr // error captured in result.Error
		}
		result.Success = true
		result.Changed = true
		if check.Present {
			result.Comment = fmt.Sprintf("Updated git config %s to %s", name, value)
		} else {
			result.Comment = fmt.Sprintf("Set git config %s to %s", name, value)
		}
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Test validates module parameters
func (m *GitConfigModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	check, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return check.Matches, nil
}

// Helper methods

func (m *GitConfigModule) getConfigValue(ctx context.Context, name, scope, file string) (string, error) {
	args := []string{"config"}

	if file != "" {
		args = append(args, "--file", file)
	} else {
		switch scope {
		case "system":
			args = append(args, "--system")
		case "global":
			args = append(args, "--global")
		case "local":
			args = append(args, "--local")
		case "worktree":
			args = append(args, "--worktree")
		}
	}

	args = append(args, "--get", name)

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil //nolint:nilerr // intentional
}

func (m *GitConfigModule) setConfigValue(ctx context.Context, name, value, scope, file string) error {
	args := []string{"config"}

	if file != "" {
		args = append(args, "--file", file)
	} else {
		switch scope {
		case "system":
			args = append(args, "--system")
		case "global":
			args = append(args, "--global")
		case "local":
			args = append(args, "--local")
		case "worktree":
			args = append(args, "--worktree")
		}
	}

	args = append(args, name, value)

	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

func (m *GitConfigModule) unsetConfigValue(ctx context.Context, name, scope, file string) error {
	args := []string{"config"}

	if file != "" {
		args = append(args, "--file", file)
	} else {
		switch scope {
		case "system":
			args = append(args, "--system")
		case "global":
			args = append(args, "--global")
		case "local":
			args = append(args, "--local")
		case "worktree":
			args = append(args, "--worktree")
		}
	}

	args = append(args, "--unset", name)

	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

// getGitConfigPath returns the path to the git config file for the given scope
func getGitConfigPath(scope string) string {
	switch scope {
	case "system":
		if runtime.GOOS == "windows" {
			return filepath.Join(os.Getenv("ProgramFiles"), "Git", "etc", "gitconfig")
		}
		return "/etc/gitconfig"
	case "global":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".gitconfig")
	default:
		return ""
	}
}

func init() {
	_ = RegisterModule(NewGitModule())
	_ = RegisterModule(NewGitConfigModule())
}
