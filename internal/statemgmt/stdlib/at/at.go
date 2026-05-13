// Package at implements the `at` stdlib state module — one-shot
// scheduled jobs via the Linux `at` toolchain, per PROJECT-DETAILS
// §4.8 (Scheduled tasks category).
//
// `at` jobs have no native name — the daemon assigns a numeric ID —
// so each submitted job's script is tagged with a "# keystone-at:
// <name>" marker comment, and the module finds its own jobs by
// scanning the queue (`at -l`, then `at -c <id>` for each).
//
// Declaration.Name is a stable job identifier.
//
// State semantics — note that `at` is fundamentally fire-once:
//
//	present — a job tagged <name> is in the queue. If none is, one is
//	          submitted running `command` at `time`. The exact
//	          command/time of an *existing* tagged job is not
//	          re-checked (an at job is immutable; change the
//	          declaration name to queue a different job, or remove it
//	          first). A job that already ran is gone from the queue,
//	          so a later state run re-queues it — at jobs are
//	          one-shot, not recurring; use cron / systemd_timer for
//	          recurring schedules.
//	absent  — no job tagged <name> is in the queue (all matching
//	          jobs are removed).
//
// `time` is the at time spec, passed verbatim ("now + 1 hour",
// "10:30 PM", "midnight tomorrow", "2026-06-01 09:00"). `queue` is
// the at queue letter (default "a"). The module manages the agent's
// own at queue; on a host without the `at` binary, mutating
// operations fail with ErrNoAt.
//
// v0.1 out of scope (v0.x candidates):
//   - Replace-on-change (detect a queued job whose command/time
//     differs from the declaration and re-queue) — v1.0 matches by
//     name only.
//   - Per-user at queues (run `at` as another user via su).
//   - The `batch` low-load variant.
//   - Queue-letter scoping of the queue scan; richer multi-line
//     script handling.
package at

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// New selects the platform's real Provider via auto-detection.
func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

type Module struct {
	provider Provider
}

func (m *Module) Name() string { return "at" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a missing scheduled job is a config-level mismatch
// (MEDIUM). A job declared absent but still queued is more
// suspicious — HIGH, mirroring the cron module.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl != nil && decl.State == StateAbsent {
		return statemgmt.DriftSeverityHigh
	}
	return statemgmt.DriftSeverityMedium
}

func (m *Module) Check(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	jobs, err := m.findJobs(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if p.State == StatePresent {
		if len(jobs) > 0 {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{
			Matches: false,
			Diff:    fmt.Sprintf("no at-job tagged %q queued → submit %q at %q", p.ID, oneline(p.Command), p.Time),
		}, nil
	}
	if len(jobs) == 0 {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{
		Matches: false,
		Diff:    fmt.Sprintf("at-job(s) %s tagged %q queued; want absent", strings.Join(jobs, ", "), p.ID),
	}, nil
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	changed, diff, err := m.apply(ctx, p)
	if err != nil {
		return &statemgmt.StateResult{
			Success:  false,
			Changed:  false,
			Duration: time.Since(start),
		}, err
	}
	if !changed {
		return &statemgmt.StateResult{
			Success:  true,
			Changed:  false,
			Comment:  "already converged",
			Duration: time.Since(start),
		}, nil
	}
	return &statemgmt.StateResult{
		Success:  true,
		Changed:  true,
		Diff:     diff,
		Comment:  "applied",
		Duration: time.Since(start),
	}, nil
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

func (m *Module) apply(ctx context.Context, p *params) (changed bool, diff string, err error) {
	jobs, err := m.findJobs(ctx, p.ID)
	if err != nil {
		return false, "", err
	}
	if p.State == StatePresent {
		if len(jobs) > 0 {
			return false, "", nil
		}
		script := markerLine(p.ID) + "\n" + strings.TrimRight(p.Command, "\n") + "\n"
		if err := m.provider.Submit(ctx, p.Queue, p.Time, script); err != nil {
			return false, "", fmt.Errorf("submit at-job: %w", err)
		}
		return true, fmt.Sprintf("queued at-job tagged %q for %q", p.ID, p.Time), nil
	}
	if len(jobs) == 0 {
		return false, "", nil
	}
	for _, id := range jobs {
		if err := m.provider.Remove(ctx, id); err != nil {
			return false, "", fmt.Errorf("remove at-job %s: %w", id, err)
		}
	}
	return true, fmt.Sprintf("removed at-job(s) %s tagged %q", strings.Join(jobs, ", "), p.ID), nil
}

// findJobs returns the queued job IDs whose script carries the
// "# keystone-at: <id>" marker. Shared by Check and Apply so the two
// never disagree.
func (m *Module) findJobs(ctx context.Context, id string) ([]string, error) {
	ids, err := m.provider.ListJobs(ctx)
	if err != nil {
		return nil, err
	}
	want := markerLine(id)
	var matching []string
	for _, jobID := range ids {
		script, err := m.provider.JobScript(ctx, jobID)
		if err != nil {
			return nil, fmt.Errorf("read at-job %s: %w", jobID, err)
		}
		for _, line := range strings.Split(script, "\n") {
			if strings.TrimSpace(line) == want {
				matching = append(matching, jobID)
				break
			}
		}
	}
	return matching, nil
}
