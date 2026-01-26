//go:build windows

package agent

import (
	"fmt"
	"os/user"
	"syscall"
)

// UserSwitchResult contains the result of looking up a user for switching
type UserSwitchResult struct {
	UID         uint32
	GID         uint32
	Username    string
	HomeDir     string
	Groups      []uint32
	SysProcAttr *syscall.SysProcAttr
}

// LookupUserForSwitch looks up a user by name on Windows
// Note: Windows user switching requires additional credentials (password or token)
// which are not supported in this basic implementation
func LookupUserForSwitch(username string) (*UserSwitchResult, error) {
	if username == "" {
		return nil, nil // No user switch requested
	}

	// Look up the user to verify they exist
	u, err := user.Lookup(username)
	if err != nil {
		return nil, fmt.Errorf("user %q not found: %w", username, err)
	}

	result := &UserSwitchResult{
		Username: u.Username,
		HomeDir:  u.HomeDir,
		// Windows doesn't have UID/GID in the same way
		UID: 0,
		GID: 0,
	}

	// On Windows, we can't easily create a process as another user without
	// either the user's password (for LogonUser) or an existing token.
	// The SysProcAttr.Token field requires a windows.Token which requires
	// either:
	// 1. LogonUser (requires password)
	// 2. An inherited token from a service
	// 3. Impersonation token from another source
	//
	// For now, we return the result without SysProcAttr - the executor
	// will check and return an appropriate error.
	result.SysProcAttr = nil

	return result, nil
}

// SetUserCredential sets the user credentials on a SysProcAttr
// On Windows, this is a no-op without proper token handling
func SetUserCredential(existing *syscall.SysProcAttr, userSwitch *UserSwitchResult) *syscall.SysProcAttr {
	if userSwitch == nil {
		return existing
	}
	// On Windows, we can't set user credentials without a token
	// Return existing unchanged
	return existing
}

// CanSwitchUser returns an error on Windows since user switching
// requires additional credentials not available in basic implementation
func CanSwitchUser(username string) error {
	if username == "" {
		return nil
	}

	// Verify the user exists
	_, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("user %q not found: %w", username, err)
	}

	// Windows user switching requires password or token
	return fmt.Errorf("user switching on Windows requires password-based authentication which is not supported; use Windows services or scheduled tasks to run as a different user")
}

// GetCurrentUser returns the current user's username
func GetCurrentUser() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}
