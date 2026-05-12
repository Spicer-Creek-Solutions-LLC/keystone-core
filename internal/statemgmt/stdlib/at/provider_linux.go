//go:build linux

package at

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func defaultProvider() Provider {
	bin, err := exec.LookPath("at")
	if err != nil {
		return &noAtProvider{}
	}
	return &linuxProvider{at: bin, run: execRun}
}

type linuxProvider struct {
	at  string
	run commandRunner
}

func (p *linuxProvider) ListJobs(ctx context.Context) ([]string, error) {
	out, err := p.run(ctx, p.at, []string{"-l"}, "")
	if err != nil {
		return nil, err
	}
	return parseAtq(out), nil
}

func (p *linuxProvider) JobScript(ctx context.Context, id string) (string, error) {
	out, err := p.run(ctx, p.at, []string{"-c", id}, "")
	if err != nil {
		return "", err
	}
	return out, nil
}

func (p *linuxProvider) Submit(ctx context.Context, queue, timeSpec, script string) error {
	_, err := p.run(ctx, p.at, []string{"-q", queue, timeSpec}, script)
	return err
}

func (p *linuxProvider) Remove(ctx context.Context, id string) error {
	_, err := p.run(ctx, p.at, []string{"-r", id}, "")
	return err
}

// parseAtq extracts the job IDs from `at -l` / `atq` output. Each
// job line begins with the numeric job number followed by the
// scheduled time, queue letter and user, e.g.
//
//	3	Wed Jun  1 09:00:00 2026 a sbutts
//
// Lines whose first whitespace-delimited token isn't all digits are
// ignored, so warnings or a stray header don't trip the parser.
func parseAtq(out string) []string {
	var ids []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if isDigits(fields[0]) {
			ids = append(ids, fields[0])
		}
	}
	return ids
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// execRun is the production commandRunner. Captures combined output
// so at's complaint (a garbled time spec, "must be privileged", …)
// reaches the operator.
func execRun(ctx context.Context, bin string, args []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed flags + a queue letter / numeric job id / operator-supplied time spec
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

// noAtProvider stands in when the `at` binary is absent. ListJobs
// reports an empty queue (so `absent` declarations match and
// `present` ones drift); mutating ops fail with ErrNoAt.
type noAtProvider struct{}

func (*noAtProvider) ListJobs(context.Context) ([]string, error)           { return nil, nil }
func (*noAtProvider) JobScript(context.Context, string) (string, error)    { return "", ErrNoAt }
func (*noAtProvider) Submit(context.Context, string, string, string) error { return ErrNoAt }
func (*noAtProvider) Remove(context.Context, string) error                 { return ErrNoAt }
