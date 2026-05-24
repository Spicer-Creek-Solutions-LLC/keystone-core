// SPDX-License-Identifier: Apache-2.0

//go:build linux

package iptables

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func defaultProvider() Provider {
	p := &linuxProvider{bin: map[string]string{}, saveBin: map[string]string{}, run: execRun}
	p.bin[FamilyIPv4], _ = exec.LookPath("iptables")
	p.bin[FamilyIPv6], _ = exec.LookPath("ip6tables")
	p.saveBin[FamilyIPv4], _ = exec.LookPath("iptables-save")
	p.saveBin[FamilyIPv6], _ = exec.LookPath("ip6tables-save")
	return p
}

type linuxProvider struct {
	bin     map[string]string // family → iptables/ip6tables path ("" if absent)
	saveBin map[string]string // family → iptables-save/ip6tables-save path
	run     commandRunner
}

func (p *linuxProvider) binFor(family string) (string, error) {
	if b := p.bin[family]; b != "" {
		return b, nil
	}
	return "", fmt.Errorf("%w (family %s)", ErrNoIptables, family)
}

func (p *linuxProvider) HasRule(ctx context.Context, family, table, chain string, rule []string) (bool, error) {
	bin, err := p.binFor(family)
	if err != nil {
		return false, err
	}
	args := append([]string{"-t", table, "-C", chain}, rule...)
	if _, runErr := p.run(ctx, bin, args); runErr != nil {
		// Any non-zero exit from -C: "rule absent". A real
		// structural error (a missing chain) surfaces when AddRule
		// is attempted.
		return false, nil
	}
	return true, nil
}

func (p *linuxProvider) AddRule(ctx context.Context, family, table, chain string, position int, rule []string) error {
	bin, err := p.binFor(family)
	if err != nil {
		return err
	}
	var args []string
	if position <= 0 {
		args = append([]string{"-t", table, "-A", chain}, rule...)
	} else {
		args = append([]string{"-t", table, "-I", chain, strconv.Itoa(position)}, rule...)
	}
	_, err = p.run(ctx, bin, args)
	return err
}

func (p *linuxProvider) DeleteRule(ctx context.Context, family, table, chain string, rule []string) error {
	bin, err := p.binFor(family)
	if err != nil {
		return err
	}
	_, err = p.run(ctx, bin, append([]string{"-t", table, "-D", chain}, rule...))
	return err
}

func (p *linuxProvider) Save(ctx context.Context, family, path string) error {
	bin := p.saveBin[family]
	if bin == "" {
		return fmt.Errorf("%w (no %s-save tool)", ErrNoIptables, ipFamilyToolPrefix(family))
	}
	out, err := p.run(ctx, bin, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", bin, err)
	}
	mode := os.FileMode(0o600)
	if fi, statErr := os.Stat(path); statErr == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.WriteFile(path, []byte(out), mode); err != nil { //nolint:gosec // operator-supplied save path; 0600 (firewall rules) or the existing file's mode
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func ipFamilyToolPrefix(family string) string {
	if family == FamilyIPv6 {
		return "ip6tables"
	}
	return "iptables"
}

// execRun is the production commandRunner. Captures combined output
// so iptables' complaint reaches the operator.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed flags + a validated table/chain + the operator-supplied rule spec from a validated state declaration
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "", fmt.Errorf("%s %s: exit %d: %s", bin, strings.Join(args, " "), exitErr.ExitCode(), strings.TrimSpace(string(out)))
	}
	return "", fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
}
