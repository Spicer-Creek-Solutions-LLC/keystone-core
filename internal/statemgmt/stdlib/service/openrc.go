// SPDX-License-Identifier: Apache-2.0

//go:build linux

package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// openrcProvider implements Provider against OpenRC (Alpine's default
// init; also Gentoo). rcService and rcUpdate are absolute paths
// resolved at detect time.
//
// OpenRC has no single-command equivalent of `systemctl show`, so
// Lookup runs three queries:
//   - rc-service --exists <name>   → Exists  (exit 0 = the init script is present)
//   - rc-service <name> status     → Active  (exit 0 = started)
//   - rc-update show default       → Enabled (name listed in the default runlevel)
//
// Enable/Disable manage the **default runlevel** (rc-update add/del
// <name> default), which is the symmetric counterpart of the
// default-runlevel Enabled check above — a service enabled in some
// other runlevel (e.g. boot) is reported Enabled=false here, matching
// what Disable would and wouldn't touch.
//
// Two injection points mirror the systemd backend:
//   - runner for the mutating verbs (start/stop/enable/disable)
//   - query for the read-only lookups (returns exit code so a stopped
//     service's expected non-zero status isn't mistaken for an error)
type openrcProvider struct {
	rcService string
	rcUpdate  string
	runner    commandRunner
	query     openrcQueryFn
}

// openrcQueryFn runs a read-only OpenRC command and returns stdout +
// exit code. A non-zero exit is a normal signal here (rc-service
// status returns 3 for a stopped service, --exists returns 1 for an
// absent one), so err is non-nil only for a genuine failure (binary
// missing, etc).
type openrcQueryFn func(ctx context.Context, bin string, args []string) (stdout string, exitCode int, err error)

func newOpenrcProvider(rcService, rcUpdate string) *openrcProvider {
	return &openrcProvider{
		rcService: rcService,
		rcUpdate:  rcUpdate,
		runner:    execRun,
		query:     realOpenrcQuery,
	}
}

func (p *openrcProvider) Lookup(name string) (*ServiceInfo, error) {
	ctx := context.Background()
	info := &ServiceInfo{Name: name}

	// Exists: rc-service --exists returns 0 when /etc/init.d/<name>
	// is present, non-zero otherwise (no stdout either way).
	_, code, err := p.query(ctx, p.rcService, []string{"--exists", name})
	if err != nil {
		return nil, fmt.Errorf("rc-service --exists %s: %w", name, err)
	}
	if code != 0 {
		return info, nil // not present; Active/Enabled meaningless
	}
	info.Exists = true

	// Active: rc-service <name> status returns 0 when started, 3 when
	// stopped (LSB exit codes); anything other than 0 is not-running.
	_, code, err = p.query(ctx, p.rcService, []string{name, "status"})
	if err != nil {
		return nil, fmt.Errorf("rc-service %s status: %w", name, err)
	}
	info.Active = code == 0

	// Enabled: present in the default runlevel.
	out, _, err := p.query(ctx, p.rcUpdate, []string{"show", "default"})
	if err != nil {
		return nil, fmt.Errorf("rc-update show default: %w", err)
	}
	info.Enabled = rcUpdateShowHasService(out, name)
	return info, nil
}

func (p *openrcProvider) Start(ctx context.Context, name string) error {
	return p.runner(ctx, p.rcService, []string{name, "start"})
}
func (p *openrcProvider) Stop(ctx context.Context, name string) error {
	return p.runner(ctx, p.rcService, []string{name, "stop"})
}
func (p *openrcProvider) Enable(ctx context.Context, name string) error {
	return p.runner(ctx, p.rcUpdate, []string{"add", name, "default"})
}
func (p *openrcProvider) Disable(ctx context.Context, name string) error {
	return p.runner(ctx, p.rcUpdate, []string{"del", name, "default"})
}

// rcUpdateShowHasService reports whether name appears in the output of
// `rc-update show <runlevel>`, whose lines look like:
//
//	nginx | default
//	 sshd |
//
// The service name is the field before the "|" (the right-hand side is
// the runlevel list, which is empty/redundant when a single runlevel
// was queried). A line with no "|" is taken verbatim as the name.
func rcUpdateShowHasService(out, name string) bool {
	for _, line := range strings.Split(out, "\n") {
		field := line
		if i := strings.IndexByte(field, '|'); i >= 0 {
			field = field[:i]
		}
		if strings.TrimSpace(field) == name {
			return true
		}
	}
	return false
}

// realOpenrcQuery runs a read-only OpenRC command. It returns the exit
// code without surfacing a non-zero exit as a Go error (the caller
// interprets the code); only a non-ExitError failure (binary missing,
// etc.) returns err. Mirrors the apt/dnf real* lookups.
func realOpenrcQuery(ctx context.Context, bin string, args []string) (string, int, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath at detect time; args are fixed verbs + a unit name validated by unitNameRE before Lookup is called
	out, err := cmd.Output()
	if err == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(out), exitErr.ExitCode(), nil
	}
	return "", -1, err
}
