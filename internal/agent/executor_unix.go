//go:build unix

package agent

import (
	"fmt"
	"os/user"
	"strconv"
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

// LookupUserForSwitch looks up a user by name or UID and returns credentials for switching
func LookupUserForSwitch(username string) (*UserSwitchResult, error) {
	if username == "" {
		return nil, nil // No user switch requested
	}

	var u *user.User
	var err error

	// Try to look up by name first
	u, err = user.Lookup(username)
	if err != nil {
		// Try to look up by UID if name lookup failed
		u, err = user.LookupId(username)
		if err != nil {
			return nil, fmt.Errorf("user %q not found: %w", username, err)
		}
	}

	// Parse UID
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid UID %q: %w", u.Uid, err)
	}

	// Parse GID
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid GID %q: %w", u.Gid, err)
	}

	result := &UserSwitchResult{
		UID:      uint32(uid),
		GID:      uint32(gid),
		Username: u.Username,
		HomeDir:  u.HomeDir,
	}

	// Get supplementary groups
	groupIDs, err := u.GroupIds()
	if err == nil {
		for _, gidStr := range groupIDs {
			g, err := strconv.ParseUint(gidStr, 10, 32)
			if err == nil {
				result.Groups = append(result.Groups, uint32(g))
			}
		}
	}

	// Create SysProcAttr with credential
	result.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid:    result.UID,
			Gid:    result.GID,
			Groups: result.Groups,
		},
	}

	return result, nil
}

// SetUserCredential sets the user credentials on a SysProcAttr
// If existing is nil, a new SysProcAttr is created
func SetUserCredential(existing *syscall.SysProcAttr, userSwitch *UserSwitchResult) *syscall.SysProcAttr {
	if userSwitch == nil {
		return existing
	}

	if existing == nil {
		return userSwitch.SysProcAttr
	}

	// Merge into existing
	existing.Credential = userSwitch.SysProcAttr.Credential
	return existing
}

// CanSwitchUser returns true if the current process can switch to the specified user
// On Unix, this typically requires root privileges
func CanSwitchUser(username string) error {
	if username == "" {
		return nil
	}

	// Check if we're root
	if syscall.Getuid() != 0 {
		return fmt.Errorf("switching to user %q requires root privileges", username)
	}

	// Verify the user exists
	_, err := LookupUserForSwitch(username)
	if err != nil {
		return err
	}

	return nil
}

// GetCurrentUser returns the current user's username
func GetCurrentUser() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}
