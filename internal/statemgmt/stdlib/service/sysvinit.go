// SPDX-License-Identifier: Apache-2.0

//go:build linux

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// sysvinitProvider implements Provider against classic SysV init. Runtime
// control is universal (`service <name> start|stop|status`), but the
// boot-enable mechanism diverges by distro, so the provider carries a
// mode:
//
//   - sysvChkconfig  (RHEL / CentOS): `chkconfig <name> on|off`, and the
//     per-runlevel table from `chkconfig --list <name>` for the enabled
//     check.
//   - sysvUpdateRcd  (Debian / Devuan): `update-rc.d <name> enable|disable`,
//     and the presence of an `S??<name>` start-symlink in the multi-user
//     runlevel dirs (`/etc/rc[2-5].d`) for the enabled check.
//
// Existence is filesystem-based in both modes (`/etc/init.d/<name>`),
// since there is no universal "does this service exist" command.
//
// Seams mirror the OpenRC backend: runner for the mutating verbs, query
// for the read-only chkconfig lookup (returns the exit code so a
// not-registered service's non-zero exit isn't mistaken for a failure),
// and a filesystem seam (fs) so the init-script + symlink checks are
// testable without touching /etc.
type sysvinitProvider struct {
	serviceBin string // `service` — runtime control (universal)
	enableBin  string // `chkconfig` or `update-rc.d` per mode
	mode       sysvEnableMode
	runner     commandRunner
	query      sysvQueryFn
	fs         sysvFS
}

type sysvEnableMode int

const (
	sysvChkconfig sysvEnableMode = iota
	sysvUpdateRcd
)

// sysvQueryFn runs a read-only command and returns stdout + exit code; a
// non-zero exit is a signal (chkconfig --list exits non-zero for a
// service it doesn't manage), so err is non-nil only for a genuine
// failure (binary missing, etc.).
type sysvQueryFn func(ctx context.Context, bin string, args []string) (stdout string, exitCode int, err error)

// sysvFS abstracts the filesystem checks: whether an init script exists
// and whether a start-symlink is present in the multi-user runlevels.
type sysvFS interface {
	initScriptExists(name string) bool
	enabledViaSymlinks(name string) bool
}

func newSysvinitProvider(serviceBin, enableBin string, mode sysvEnableMode) *sysvinitProvider {
	return &sysvinitProvider{
		serviceBin: serviceBin,
		enableBin:  enableBin,
		mode:       mode,
		runner:     execRun,
		query:      realSysvQuery,
		fs:         realSysvFS{initdDir: "/etc/init.d", etcDir: "/etc"},
	}
}

func (p *sysvinitProvider) Lookup(name string) (*ServiceInfo, error) {
	ctx := context.Background()
	info := &ServiceInfo{Name: name}

	// Exists: the init script is present. Active/Enabled are meaningless
	// otherwise.
	if !p.fs.initScriptExists(name) {
		return info, nil
	}
	info.Exists = true

	// Active: `service <name> status` exits 0 when running (LSB: 3 when
	// stopped). Any non-zero is not-running.
	_, code, err := p.query(ctx, p.serviceBin, []string{name, "status"})
	if err != nil {
		return nil, fmt.Errorf("service %s status: %w", name, err)
	}
	info.Active = code == 0

	enabled, err := p.enabled(ctx, name)
	if err != nil {
		return nil, err
	}
	info.Enabled = enabled
	return info, nil
}

// enabled reports whether the service starts at boot, per the provider's
// mode.
func (p *sysvinitProvider) enabled(ctx context.Context, name string) (bool, error) {
	switch p.mode {
	case sysvChkconfig:
		// `chkconfig --list <name>` prints the per-runlevel table; a
		// non-zero exit means it isn't a chkconfig-managed service →
		// treat as not-enabled rather than an error.
		out, code, err := p.query(ctx, p.enableBin, []string{"--list", name})
		if err != nil {
			return false, fmt.Errorf("chkconfig --list %s: %w", name, err)
		}
		if code != 0 {
			return false, nil
		}
		return chkconfigEnabled(out, name), nil
	case sysvUpdateRcd:
		return p.fs.enabledViaSymlinks(name), nil
	}
	return false, fmt.Errorf("service: unknown sysvinit enable mode %d", p.mode)
}

func (p *sysvinitProvider) Start(ctx context.Context, name string) error {
	return p.runner(ctx, p.serviceBin, []string{name, "start"})
}
func (p *sysvinitProvider) Stop(ctx context.Context, name string) error {
	return p.runner(ctx, p.serviceBin, []string{name, "stop"})
}
func (p *sysvinitProvider) Enable(ctx context.Context, name string) error {
	switch p.mode {
	case sysvChkconfig:
		return p.runner(ctx, p.enableBin, []string{name, "on"})
	case sysvUpdateRcd:
		return p.runner(ctx, p.enableBin, []string{name, "enable"})
	}
	return fmt.Errorf("service: unknown sysvinit enable mode %d", p.mode)
}
func (p *sysvinitProvider) Disable(ctx context.Context, name string) error {
	switch p.mode {
	case sysvChkconfig:
		return p.runner(ctx, p.enableBin, []string{name, "off"})
	case sysvUpdateRcd:
		return p.runner(ctx, p.enableBin, []string{name, "disable"})
	}
	return fmt.Errorf("service: unknown sysvinit enable mode %d", p.mode)
}

// chkconfigEnabled reports whether name is on in any multi-user runlevel
// (2-5) per `chkconfig --list` output, whose line for a service looks
// like:
//
//	httpd          	0:off	1:off	2:on	3:on	4:on	5:on	6:off
//
// `chkconfig <name> on` turns runlevels 2-5 on, so "on in any of 2-5" is
// the faithful inverse of what Enable/Disable toggle.
func chkconfigEnabled(out, name string) bool {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != name {
			continue
		}
		for _, rl := range []string{"2:on", "3:on", "4:on", "5:on"} {
			for _, f := range fields[1:] {
				if f == rl {
					return true
				}
			}
		}
		return false
	}
	return false
}

// realSysvFS is the production sysvFS over /etc/init.d and /etc/rc?.d.
type realSysvFS struct {
	initdDir string
	etcDir   string
}

func (f realSysvFS) initScriptExists(name string) bool {
	fi, err := os.Stat(filepath.Join(f.initdDir, name))
	return err == nil && !fi.IsDir()
}

// enabledViaSymlinks reports whether a start-symlink (`S??<name>`) exists
// in any multi-user runlevel directory (rc2.d..rc5.d). `update-rc.d
// disable` swaps the S-link for a K-link, so the presence of an S-link is
// the enabled signal.
func (f realSysvFS) enabledViaSymlinks(name string) bool {
	for _, rl := range []string{"2", "3", "4", "5"} {
		pattern := filepath.Join(f.etcDir, "rc"+rl+".d", "S[0-9][0-9]"+name)
		if m, _ := filepath.Glob(pattern); len(m) > 0 {
			return true
		}
	}
	return false
}

// realSysvQuery runs a read-only command and returns its exit code
// without surfacing a non-zero exit as a Go error (the caller interprets
// the code). Mirrors realOpenrcQuery.
func realSysvQuery(ctx context.Context, bin string, args []string) (string, int, error) {
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
