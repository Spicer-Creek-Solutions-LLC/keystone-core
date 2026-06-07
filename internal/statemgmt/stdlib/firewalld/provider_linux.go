// SPDX-License-Identifier: Apache-2.0

//go:build linux

package firewalld

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func defaultProvider() Provider {
	bin, _ := exec.LookPath("firewall-cmd")
	return &linuxProvider{bin: bin, run: execRun}
}

type linuxProvider struct {
	bin string // resolved `firewall-cmd` path ("" if absent)
	run commandRunner
}

func (p *linuxProvider) fwcmd() (string, error) {
	if p.bin == "" {
		return "", ErrNoFirewallCmd
	}
	return p.bin, nil
}

func (p *linuxProvider) Has(ctx context.Context, zone string, it Item) (bool, error) {
	bin, err := p.fwcmd()
	if err != nil {
		return false, err
	}
	flag := fmt.Sprintf("--query-%s=%s", it.Kind, it.Value)
	if _, runErr := p.run(ctx, bin, []string{"--permanent", "--zone=" + zone, flag}); runErr != nil {
		// Any non-zero exit: "not present". A real structural error
		// (missing zone) surfaces from Add at Apply time.
		return false, nil
	}
	return true, nil
}

func (p *linuxProvider) Add(ctx context.Context, zone string, it Item) error {
	return p.mutate(ctx, zone, "add", it)
}

func (p *linuxProvider) Remove(ctx context.Context, zone string, it Item) error {
	return p.mutate(ctx, zone, "remove", it)
}

func (p *linuxProvider) mutate(ctx context.Context, zone, verb string, it Item) error {
	bin, err := p.fwcmd()
	if err != nil {
		return err
	}
	flag := fmt.Sprintf("--%s-%s=%s", verb, it.Kind, it.Value)
	_, err = p.run(ctx, bin, []string{"--permanent", "--zone=" + zone, flag})
	return err
}

func (p *linuxProvider) Reload(ctx context.Context) error {
	bin, err := p.fwcmd()
	if err != nil {
		return err
	}
	_, err = p.run(ctx, bin, []string{"--reload"})
	return err
}

func (p *linuxProvider) ListRichRules(ctx context.Context, zone string) ([]string, error) {
	bin, err := p.fwcmd()
	if err != nil {
		return nil, err
	}
	out, err := p.run(ctx, bin, []string{"--permanent", "--zone=" + zone, "--list-rich-rules"})
	if err != nil {
		return nil, err
	}
	return parseRichRuleList(out), nil
}

// parseRichRuleList splits `--list-rich-rules` output into one rule per
// line, dropping blank lines (an empty zone prints nothing).
func parseRichRuleList(out string) []string {
	var rules []string
	for _, line := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			rules = append(rules, s)
		}
	}
	return rules
}

// execRun is the production commandRunner. Captures combined output
// so firewall-cmd's complaint reaches the operator.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed firewall-cmd flags + a validated zone and item value from a validated state declaration
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
