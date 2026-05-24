// SPDX-License-Identifier: Apache-2.0

//go:build linux

package cron

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func defaultProvider() Provider {
	bin, err := exec.LookPath("crontab")
	if err != nil {
		return &noCrontabProvider{}
	}
	return &linuxProvider{crontab: bin, run: execRun}
}

type linuxProvider struct {
	crontab string
	run     commandRunner
}

func (p *linuxProvider) Read(ctx context.Context, user string) (string, error) {
	out, err := p.run(ctx, p.crontab, []string{"-l", "-u", user}, "")
	if err != nil {
		// `crontab -l` exits non-zero with "no crontab for <user>"
		// when the user simply has no crontab — that is the empty
		// crontab, not a failure.
		if strings.Contains(err.Error(), "no crontab") {
			return "", nil
		}
		return "", err
	}
	return out, nil
}

func (p *linuxProvider) Write(ctx context.Context, user, content string) error {
	_, err := p.run(ctx, p.crontab, []string{"-u", user, "-"}, content)
	return err
}

// execRun is the production commandRunner. Captures combined output
// so crontab's complaint (syntax errors it rejects, "must be
// privileged", …) reaches the operator.
func execRun(ctx context.Context, bin string, args []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed flags + a regex-validated user name
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
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

// noCrontabProvider stands in when the `crontab` binary is absent.
// Reads report an empty crontab (so `absent` declarations match and
// `present` ones drift); writes fail with ErrNoCrontab — the honest
// answer.
type noCrontabProvider struct{}

func (*noCrontabProvider) Read(context.Context, string) (string, error) { return "", nil }
func (*noCrontabProvider) Write(context.Context, string, string) error  { return ErrNoCrontab }
