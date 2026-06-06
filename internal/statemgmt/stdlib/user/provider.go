// SPDX-License-Identifier: Apache-2.0

package user

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
)

// ErrUnsupportedOS is returned from mutating Provider methods on
// platforms where v1.0 doesn't ship a real implementation. Lookup
// works cross-platform via os/user.
var ErrUnsupportedOS = errors.New("user: unsupported OS for mutating operations in v1.0 (Linux only)")

// ErrNoBackend is returned on a Linux host where no supported
// user-management toolchain was detected — neither shadow-utils
// (useradd/usermod/userdel) nor BusyBox (adduser/deluser).
var ErrNoBackend = errors.New("user: no supported user toolchain detected on this host (neither shadow-utils useradd nor BusyBox adduser found)")

// ErrModUnsupported is returned by the BusyBox backend's Mod path.
// BusyBox ships no usermod equivalent, so changing an existing
// account's scalar fields (UID/GID/shell/comment/home) is not
// possible without shadow-utils. The operator should install the
// shadow package or recreate the account.
var ErrModUnsupported = errors.New("user: modifying an existing account is unavailable on BusyBox (no usermod); install the shadow package or recreate the account")

// UserInfo is the on-system shape the module compares against the
// declaration. Groups is sorted so set-equality comparisons are
// deterministic.
type UserInfo struct {
	Name    string
	UID     int
	GID     int
	Home    string
	Shell   string
	Comment string
	Groups  []string // supplementary group names, sorted
}

// AddOptions carries everything useradd needs.
type AddOptions struct {
	Name       string
	UID        *int
	GID        *int
	Group      string
	Home       string
	Shell      string
	Comment    string
	Groups     []string
	System     bool
	CreateHome bool
}

// ModOptions carries the scalar fields usermod can change. Empty
// strings mean "don't change"; nil ints mean "don't change."
// Supplementary groups travel through SetGroups so callers can
// pin which provider method fires on a partial change.
type ModOptions struct {
	Name    string
	UID     *int
	GID     *int
	Group   string
	Home    string
	Shell   string
	Comment string
}

// commandRunner is the injection point that lets the BusyBox
// backend's tests pin arg formation without invoking adduser. The
// production wiring is runManaged.
type commandRunner func(ctx context.Context, bin string, args []string) error

// IsNoBackend / IsModUnsupported expose the new sentinel matchers so
// the gRPC server + CLI can render friendlier messages on the
// operator-facing surface. (IsUnsupportedOS lives in user.go.)
func IsNoBackend(err error) bool      { return errors.Is(err, ErrNoBackend) }
func IsModUnsupported(err error) bool { return errors.Is(err, ErrModUnsupported) }

// Provider abstracts the OS-level user operations. Production code
// uses the platform-specific real impl returned by defaultProvider();
// tests inject a fake.
type Provider interface {
	Lookup(name string) (*UserInfo, error)
	Add(ctx context.Context, opts AddOptions) error
	Mod(ctx context.Context, opts ModOptions) error
	Del(ctx context.Context, name string, removeHome bool) error
	SetGroups(ctx context.Context, name string, groups []string) error
}

// osLookup implements the cross-platform Lookup half of Provider
// using os/user. Real platform providers embed this so they don't
// reimplement the read path.
type osLookup struct{}

func (osLookup) Lookup(name string) (*UserInfo, error) {
	u, err := user.Lookup(name)
	if err != nil {
		var unknown user.UnknownUserError
		if errors.As(err, &unknown) {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup user %q: %w", name, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return nil, fmt.Errorf("parse uid for %q: %w", name, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return nil, fmt.Errorf("parse gid for %q: %w", name, err)
	}

	// Supplementary groups: GroupIds returns GID strings; resolve
	// each to a name via LookupGroupId. Names are deterministic; an
	// unresolvable GID falls through as the numeric string so the
	// diff stays surfacable.
	gids, err := u.GroupIds()
	if err != nil {
		return nil, fmt.Errorf("group IDs for %q: %w", name, err)
	}
	groups := make([]string, 0, len(gids))
	for _, g := range gids {
		if name, err := groupNameForGID(g); err == nil {
			groups = append(groups, name)
		} else {
			groups = append(groups, g)
		}
	}
	sort.Strings(groups)

	// os/user doesn't expose the login shell — parse /etc/passwd
	// for it. Failure is non-fatal: an empty Shell just means a
	// declared shell:/bin/bash will report drift even when it
	// matches. Linux always has /etc/passwd readable to everyone.
	shell, _ := shellFromPasswd(passwdPath, u.Username)

	return &UserInfo{
		Name:    u.Username,
		UID:     uid,
		GID:     gid,
		Home:    u.HomeDir,
		Shell:   shell,
		Comment: u.Name,
		Groups:  groups,
	}, nil
}

// passwdPath is overridable for tests; production is /etc/passwd.
var passwdPath = "/etc/passwd"

// shellFromPasswd reads /etc/passwd looking for username; returns
// the shell field (column 7). Returns ("", error) on read failure
// or unknown user; callers tolerate the empty string.
func shellFromPasswd(path, username string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // passwd path is fixed
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}
		if fields[0] == username {
			return fields[6], nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("user %q not found in %s", username, path)
}

// groupNameForGID resolves a GID string to a group name. Pulled out
// for testability; the LookupGroupId failure path returns the raw
// GID so the diff still reads usefully on an unresolvable group.
func groupNameForGID(gid string) (string, error) {
	g, err := user.LookupGroupId(gid)
	if err != nil {
		return "", err
	}
	return g.Name, nil
}
