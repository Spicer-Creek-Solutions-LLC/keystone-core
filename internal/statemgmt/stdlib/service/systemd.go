//go:build linux

package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// systemdProvider implements Provider against systemctl. systemctl
// is the absolute path resolved at detect time.
//
// Two injection points:
//   - runner for the mutating verbs (start/stop/enable/disable)
//   - showLookup for the Lookup query
//
// Production wires them to execRun + realShowLookup; tests inject
// capturing / scripted versions so arg formation and parser logic
// can be tested without a running systemd.
type systemdProvider struct {
	systemctl  string
	runner     commandRunner
	showLookup showLookupFn
}

// showLookupFn runs `systemctl show <name> -p ...` and returns the
// raw stdout. exitCode is surfaced so the caller can distinguish
// "unit not found" (systemctl returns 0 with LoadState=not-found,
// confusingly) from a real failure.
type showLookupFn func(ctx context.Context, systemctl, name string) (stdout string, err error)

func newSystemdProvider(systemctl string) *systemdProvider {
	return &systemdProvider{
		systemctl:  systemctl,
		runner:     execRun,
		showLookup: realShowLookup,
	}
}

func (p *systemdProvider) Lookup(name string) (*ServiceInfo, error) {
	out, err := p.showLookup(context.Background(), p.systemctl, name)
	if err != nil {
		return nil, fmt.Errorf("systemctl show %s: %w", name, err)
	}
	return parseSystemctlShow(name, out)
}

func (p *systemdProvider) Start(ctx context.Context, name string) error {
	return p.runner(ctx, p.systemctl, []string{"start", name})
}
func (p *systemdProvider) Stop(ctx context.Context, name string) error {
	return p.runner(ctx, p.systemctl, []string{"stop", name})
}
func (p *systemdProvider) Enable(ctx context.Context, name string) error {
	return p.runner(ctx, p.systemctl, []string{"enable", name})
}
func (p *systemdProvider) Disable(ctx context.Context, name string) error {
	return p.runner(ctx, p.systemctl, []string{"disable", name})
}

// parseSystemctlShow reads the output of:
//
//	systemctl show <name> -p LoadState -p ActiveState -p UnitFileState
//
// which is a series of KEY=VALUE lines (no --value flag so each
// line is self-describing):
//
//	LoadState=loaded            → unit exists
//	LoadState=not-found         → unit doesn't exist (Exists=false)
//	LoadState=masked            → exists, but treated as disabled
//	ActiveState=active          → Active=true; anything else → false
//	UnitFileState=enabled       → Enabled=true
//	UnitFileState=static        → Enabled=true (started via its
//	                              dependent units; no enable/disable)
//	UnitFileState=disabled      → Enabled=false
//	UnitFileState=masked        → Enabled=false
//	UnitFileState=              → Enabled=false (no [Install] section)
//
// Unknown LoadState or UnitFileState values return an error so a
// future systemd format change is loud.
func parseSystemctlShow(name, out string) (*ServiceInfo, error) {
	fields := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("systemctl show: unexpected line %q", line)
		}
		fields[k] = v
	}

	loadState, ok := fields["LoadState"]
	if !ok {
		return nil, fmt.Errorf("systemctl show: missing LoadState for %s", name)
	}
	info := &ServiceInfo{Name: name}
	switch loadState {
	case "loaded":
		info.Exists = true
	case "not-found", "bad-setting", "error":
		info.Exists = false
		return info, nil // Active/Enabled meaningless; bail early
	case "masked":
		// Masked: the unit exists but is symlinked to /dev/null —
		// it can't be started. For our compare purposes treat it
		// as exists-but-not-enabled-not-active. The operator who
		// declared state=stopped will see a match; state=running
		// will see drift (and Start will fail with a clear
		// systemctl error, which we surface).
		info.Exists = true
		info.Active = false
		info.Enabled = false
		return info, nil
	default:
		return nil, fmt.Errorf("systemctl show: unknown LoadState %q for %s", loadState, name)
	}

	info.Active = fields["ActiveState"] == "active"

	switch ufs := fields["UnitFileState"]; ufs {
	case "enabled", "enabled-runtime", "static", "indirect", "alias":
		info.Enabled = true
	case "disabled", "masked", "masked-runtime", "linked", "linked-runtime", "generated", "transient", "":
		info.Enabled = false
	default:
		return nil, fmt.Errorf("systemctl show: unknown UnitFileState %q for %s", ufs, name)
	}
	return info, nil
}

// realShowLookup runs `systemctl show`. systemctl returns exit 0
// even for a nonexistent unit (with LoadState=not-found), so a
// non-zero exit here means a real failure (binary missing, no PID 1
// systemd, etc).
func realShowLookup(ctx context.Context, systemctl, name string) (string, error) {
	cmd := exec.CommandContext(ctx, systemctl, "show", name, //nolint:gosec // systemctl resolved via exec.LookPath at detect time; name validated by unitNameRE before Lookup is called
		"-p", "LoadState", "-p", "ActiveState", "-p", "UnitFileState")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("exit %d: %s", exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

// execRun is the production commandRunner. Captures stderr so the
// operator sees systemctl's actual complaint ("Unit nginx.service
// not found", "Interactive authentication required", etc).
func execRun(ctx context.Context, bin string, args []string) error {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath at detect time; args are a fixed verb + a validated unit name
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("%s %s: exit %d: %s", bin, strings.Join(args, " "), exitErr.ExitCode(), strings.TrimSpace(string(out)))
	}
	return fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
}
