// SPDX-License-Identifier: Apache-2.0

// Package cron implements the `cron` stdlib state module — per-user
// crontab entries per PROJECT-DETAILS §4.8 (Scheduled tasks
// category).
//
// Declaration.Name is a stable identifier for the job. The managed
// entry is tagged with a "# keystone-cron: <name>" marker comment on
// the line above it, so the module owns exactly that entry and can
// update its schedule/command (or remove it) idempotently. Other
// lines in the user's crontab are never touched.
//
// State semantics:
//
//	present — the user's crontab has an entry tagged <name> with the
//	          declared `schedule` and `command`.
//	absent  — the user's crontab has no entry tagged <name>.
//
// `schedule` is either a five-field cron spec ("*/5 * * * *") or an
// @-shortcut ("@daily", "@reboot", …). The schedule is normalised to
// single-space separators when written. `user` selects whose crontab
// (default "root"). The module shells out to `crontab(1)`; when that
// binary is absent, mutating operations fail with ErrNoCrontab.
//
// v0.1 out of scope (v0.x candidates):
//   - Separate minute/hour/day-of-month/month/day-of-week params
//     (Salt-style) — v1.0 takes one `schedule` string.
//   - /etc/cron.d drop-in mode (use the `file` module for those).
//   - Environment-variable lines (KEY=value) in the crontab.
//   - Deep validation of cron field syntax (ranges, steps, names).
//   - Non-Linux cron implementations.
package cron

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

func (m *Module) Name() string { return "cron" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a missing or wrong scheduled job is a config-level
// mismatch (MEDIUM). A job declared absent but present is more
// suspicious — treat as HIGH, mirroring the file/link modules.
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
	content, err := m.provider.Read(ctx, p.User)
	if err != nil {
		return nil, err
	}
	_, changed, diff := reconcile(content, p)
	if changed {
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: diff}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: true}, nil
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
	content, err := m.provider.Read(ctx, p.User)
	if err != nil {
		return nil, err
	}
	newContent, changed, diff := reconcile(content, p)
	if !changed {
		return &statemgmt.StateResult{
			Success:  true,
			Changed:  false,
			Comment:  "already converged",
			Duration: time.Since(start),
		}, nil
	}
	if err := m.provider.Write(ctx, p.User, newContent); err != nil {
		return &statemgmt.StateResult{
			Success:  false,
			Changed:  false,
			Diff:     diff,
			Duration: time.Since(start),
		}, err
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

// reconcile computes the crontab content that satisfies p. changed
// reports whether content needed to change; diff is a human-readable
// description of the gap (empty when changed is false). It is the
// shared core of Check and Apply, so the two never disagree.
func reconcile(content string, p *params) (newContent string, changed bool, diff string) {
	switch p.State {
	case StatePresent:
		want := desiredLine(p)
		got, found := findJob(content, p.ID)
		if found && strings.TrimSpace(got) == want {
			return content, false, ""
		}
		nc := upsertJob(content, p.ID, want)
		if found {
			return nc, true, fmt.Sprintf("job %q: %q → %q", p.ID, strings.TrimSpace(got), want)
		}
		return nc, true, fmt.Sprintf("job %q: missing → %q", p.ID, want)
	case StateAbsent:
		got, found := findJob(content, p.ID)
		if !found {
			return content, false, ""
		}
		return removeJob(content, p.ID), true, fmt.Sprintf("job %q present (%q); want absent", p.ID, strings.TrimSpace(got))
	}
	return content, false, ""
}
