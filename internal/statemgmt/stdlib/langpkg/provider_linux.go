//go:build linux

package langpkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

func defaultProvider() Provider {
	p := &linuxProvider{run: execRun}
	p.pipBin, _ = exec.LookPath("pip")
	if p.pipBin == "" {
		p.pipBin, _ = exec.LookPath("pip3")
	}
	p.npmBin, _ = exec.LookPath("npm")
	p.gemBin, _ = exec.LookPath("gem")
	return p
}

type linuxProvider struct {
	pipBin string
	npmBin string
	gemBin string
	run    commandRunner
}

// --- pip --------------------------------------------------------------

// pipVersionRE matches the "Version: 1.2.3" line in `pip show` output.
var pipVersionRE = regexp.MustCompile(`(?m)^Version:\s*(\S+)\s*$`)

func (p *linuxProvider) HasPipPackage(ctx context.Context, name string) (bool, string, error) {
	if p.pipBin == "" {
		return false, "", ErrNoPip
	}
	out, runErr := p.run(ctx, p.pipBin, []string{"show", name})
	if runErr != nil {
		// `pip show` exits non-zero when the package isn't installed.
		// Any non-zero exit is taken as "absent" here; a real
		// structural problem (broken pip / EACCES on metadata) will
		// resurface from Install.
		return false, "", nil
	}
	if m := pipVersionRE.FindStringSubmatch(out); m != nil {
		return true, strings.TrimSpace(m[1]), nil
	}
	// Output without a Version line shouldn't happen for a real
	// `pip show` hit, but be defensive: report installed with no
	// known version.
	return true, "", nil
}

func (p *linuxProvider) InstallPipPackage(ctx context.Context, name, version string) error {
	if p.pipBin == "" {
		return ErrNoPip
	}
	spec := name
	if version != "" {
		spec = name + "==" + version
	}
	_, err := p.run(ctx, p.pipBin, []string{"install", spec})
	return err
}

func (p *linuxProvider) UninstallPipPackage(ctx context.Context, name string) error {
	if p.pipBin == "" {
		return ErrNoPip
	}
	_, err := p.run(ctx, p.pipBin, []string{"uninstall", "-y", name})
	return err
}

// --- npm --------------------------------------------------------------

// npmListResult is the minimal shape we need from
// `npm list -g <name> --depth=0 --json`.
type npmListResult struct {
	Dependencies map[string]struct {
		Version string `json:"version"`
	} `json:"dependencies"`
}

func (p *linuxProvider) HasNpmPackage(ctx context.Context, name string) (bool, string, error) {
	if p.npmBin == "" {
		return false, "", ErrNoNpm
	}
	// `npm list -g <name> --depth=0 --json` exits 0 with dependency
	// info when installed; exits non-zero with an empty
	// `dependencies` map when not. We parse the JSON regardless and
	// rely on presence in the map.
	out, _ := p.run(ctx, p.npmBin, []string{"list", "-g", name, "--depth=0", "--json"})
	if out == "" {
		return false, "", nil
	}
	var r npmListResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		// Output isn't JSON — treat as absent rather than failing the
		// Check (a real install failure will surface from Install).
		return false, "", nil
	}
	if dep, ok := r.Dependencies[name]; ok && dep.Version != "" {
		return true, dep.Version, nil
	}
	return false, "", nil
}

func (p *linuxProvider) InstallNpmPackage(ctx context.Context, name, version string) error {
	if p.npmBin == "" {
		return ErrNoNpm
	}
	spec := name
	if version != "" {
		spec = name + "@" + version
	}
	_, err := p.run(ctx, p.npmBin, []string{"install", "-g", spec})
	return err
}

func (p *linuxProvider) UninstallNpmPackage(ctx context.Context, name string) error {
	if p.npmBin == "" {
		return ErrNoNpm
	}
	_, err := p.run(ctx, p.npmBin, []string{"uninstall", "-g", name})
	return err
}

// --- gem --------------------------------------------------------------

// gemListRE matches `gem list --exact NAME` output: "NAME (1.0.0,
// 0.9.1)". The version-list is captured for parsing.
var gemListRE = regexp.MustCompile(`^\S+\s*\(([^)]+)\)\s*$`)

func (p *linuxProvider) HasGemPackage(ctx context.Context, name string) (bool, string, error) {
	if p.gemBin == "" {
		return false, "", ErrNoGem
	}
	out, runErr := p.run(ctx, p.gemBin, []string{"list", "--exact", name})
	if runErr != nil {
		return false, "", nil
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if m := gemListRE.FindStringSubmatch(line); m != nil {
			// Return the first listed (highest) version. gem prints
			// versions newest-first.
			versions := strings.Split(m[1], ",")
			if len(versions) > 0 {
				return true, strings.TrimSpace(versions[0]), nil
			}
		}
	}
	return false, "", nil
}

func (p *linuxProvider) InstallGemPackage(ctx context.Context, name, version string) error {
	if p.gemBin == "" {
		return ErrNoGem
	}
	args := []string{"install", name}
	if version != "" {
		args = append(args, "-v", version)
	}
	_, err := p.run(ctx, p.gemBin, args)
	return err
}

func (p *linuxProvider) UninstallGemPackage(ctx context.Context, name string) error {
	if p.gemBin == "" {
		return ErrNoGem
	}
	// -a removes all versions; -I doesn't fail on dependency issues;
	// -x removes installed executables. These are the standard flags
	// for "ensure this gem is gone".
	_, err := p.run(ctx, p.gemBin, []string{"uninstall", "-aIx", name})
	return err
}

// --- exec --------------------------------------------------------------

// execRun is the production commandRunner. Captures combined output
// so the underlying manager's complaint reaches the operator.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed pip/npm/gem flags + a validated package name + an operator-supplied charset-checked version from a validated declaration
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(out), fmt.Errorf("%s %s: exit %d: %s", bin, strings.Join(args, " "), exitErr.ExitCode(), strings.TrimSpace(string(out)))
	}
	return string(out), fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
}
