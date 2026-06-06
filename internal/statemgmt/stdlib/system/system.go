// SPDX-License-Identifier: Apache-2.0

// Package system implements the `system` stdlib state module — a
// multi-op surface for the system-level settings that don't fit the
// other stdlib modules, per PROJECT-DETAILS §4.8 (System & core
// category).
//
// Operations per declaration (exactly one):
//
//   - **Banner** — `banner: motd|issue|issue_net` + `content: <string>`
//     manages the contents of `/etc/motd` / `/etc/issue` /
//     `/etc/issue.net`. States: `present` (the file's content equals
//     `content`) and `absent` (the file is empty). Apply writes
//     atomically and preserves the file's existing mode (defaults to
//     0644 when creating).
//
//   - **Reboot** — `reboot: true` plus optional `when_file:` (default
//     `/var/run/reboot-required`) and `delay:` (default 1, range
//     0–60 minutes). State `present` only. Apply shells `shutdown -r
//     +<delay>` (or `shutdown -r now` for `delay: 0`). The
//     reboot-needed signal is cross-distro: the marker file is checked
//     first (Debian/Ubuntu's `/var/run/reboot-required`, plus any
//     operator `when_file:`), and when it is absent the host's
//     reboot-hint tool is consulted — `needs-restarting -r` (or `dnf
//     needs-restarting -r`), exit code 1 = reboot needed — on RHEL /
//     Rocky / Fedora hosts where dnf-utils is present. Hosts with
//     neither (e.g. Alpine, which has no reboot-required convention)
//     fall back to the marker file alone.
//
//   - **Locale** — `locale: <LANG>` (e.g. `en_US.UTF-8`). State
//     `present` only. Apply writes `/etc/locale.conf`
//     (`LANG=<value>`, atomic, preserves mode) and — when
//     `localectl` is on PATH — runs `localectl set-locale LANG=…`
//     so the change is live too. Debian's `/etc/default/locale`
//     dual-file is V1X.
//
// Declaration.Name is just a human label (the decl ID); the
// operation is identified by which params are set. ValidStates
// returns `["present","absent"]`; per-op validation rejects `absent`
// for reboot and locale.
//
// v0.1 out of scope (v0.x candidates):
//   - `absent` semantics for `reboot` (cancel a scheduled reboot via
//     `shutdown -c`) and for `locale` (revert to compile-time
//     default; ambiguous).
//   - Unconditional reboot (no marker gate, `force: true`) — v1.0
//     requires a marker so every Apply is gated and idempotent.
//   - Arch's `checkservices` / `needrestart` reboot-hint detection
//     (the marker file + `needs-restarting -r` cover Debian/Ubuntu and
//     RHEL/Rocky/Fedora; Arch's tooling is a separate pattern).
//   - Result delivery across the reboot disconnect — v1.0 leaves
//     `shutdown -r +<delay>` running asynchronously; for `delay: 0`
//     the kscore-agent's RPC reply may not reach the controller
//     before kernel kills the process. The Comment documents this.
//   - Debian's `/etc/default/locale` dual-file (v1.0 writes
//     `/etc/locale.conf` only); per-`LC_*` overrides; setxkbmap /
//     vconsole keyboard layout management.
package system

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

func (m *Module) Name() string { return "system" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: per-op — reboot HIGH (pending reboot often means
// kernel patches aren't active yet); banner / locale LOW
// (informational). nil → MEDIUM. Operators override via `severity:`.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
	p, err := parseParams(decl)
	if err != nil {
		return statemgmt.DriftSeverityMedium
	}
	switch p.Op {
	case OpReboot:
		return statemgmt.DriftSeverityHigh
	case OpBanner, OpLocale:
		return statemgmt.DriftSeverityLow
	}
	return statemgmt.DriftSeverityMedium
}

func (m *Module) Check(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	switch p.Op {
	case OpBanner:
		return m.checkBanner(ctx, p)
	case OpReboot:
		return m.checkReboot(ctx, p)
	case OpLocale:
		return m.checkLocale(ctx, p)
	}
	return nil, fmt.Errorf("unknown op %v", p.Op)
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	switch p.Op {
	case OpBanner:
		return m.applyBanner(ctx, start, p)
	case OpReboot:
		return m.applyReboot(ctx, start, p)
	case OpLocale:
		return m.applyLocale(ctx, start, p)
	}
	return nil, fmt.Errorf("unknown op %v", p.Op)
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

func (m *Module) parsed(decl *statemgmt.Declaration) (*params, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// --- banner op ---------------------------------------------------------

func (m *Module) checkBanner(ctx context.Context, p *params) (*statemgmt.ModuleCheckResult, error) {
	current, err := m.provider.ReadBanner(ctx, p.BannerName)
	if err != nil {
		return nil, err
	}
	want := p.BannerContent
	if p.State == StateAbsent {
		want = ""
	}
	if current == want {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s: %d → %d bytes", p.BannerName, len(current), len(want))}, nil
}

func (m *Module) applyBanner(ctx context.Context, start time.Time, p *params) (*statemgmt.StateResult, error) {
	current, err := m.provider.ReadBanner(ctx, p.BannerName)
	if err != nil {
		return failure(start), err
	}
	want := p.BannerContent
	if p.State == StateAbsent {
		want = ""
	}
	if current == want {
		return ok(start, false, "", "already converged"), nil
	}
	if err := m.provider.WriteBanner(ctx, p.BannerName, want); err != nil {
		return failure(start), fmt.Errorf("write %s: %w", p.BannerName, err)
	}
	return ok(start, true, fmt.Sprintf("%s: %d → %d bytes", p.BannerName, len(current), len(want)), "applied"), nil
}

// --- reboot op ---------------------------------------------------------

func (m *Module) checkReboot(ctx context.Context, p *params) (*statemgmt.ModuleCheckResult, error) {
	needed, err := m.provider.IsRebootNeeded(ctx, p.WhenFile)
	if err != nil {
		return nil, err
	}
	if !needed {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("reboot required (marker %s present)", p.WhenFile)}, nil
}

func (m *Module) applyReboot(ctx context.Context, start time.Time, p *params) (*statemgmt.StateResult, error) {
	needed, err := m.provider.IsRebootNeeded(ctx, p.WhenFile)
	if err != nil {
		return failure(start), err
	}
	if !needed {
		return ok(start, false, "", "already converged"), nil
	}
	if err := m.provider.ScheduleReboot(ctx, p.Delay); err != nil {
		return failure(start), fmt.Errorf("schedule reboot: %w", err)
	}
	var comment string
	switch p.Delay {
	case 0:
		comment = "reboot scheduled now — the agent may not deliver this result before kernel kill"
	case 1:
		comment = "reboot scheduled in 1 minute"
	default:
		comment = fmt.Sprintf("reboot scheduled in %d minutes", p.Delay)
	}
	return ok(start, true, fmt.Sprintf("scheduled reboot (marker %s)", p.WhenFile), comment), nil
}

// --- locale op ---------------------------------------------------------

func (m *Module) checkLocale(ctx context.Context, p *params) (*statemgmt.ModuleCheckResult, error) {
	current, err := m.provider.ReadLocale(ctx)
	if err != nil {
		return nil, err
	}
	if current == p.Locale {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("locale: %s → %s", displayLocale(current), p.Locale)}, nil
}

func (m *Module) applyLocale(ctx context.Context, start time.Time, p *params) (*statemgmt.StateResult, error) {
	current, err := m.provider.ReadLocale(ctx)
	if err != nil {
		return failure(start), err
	}
	if current == p.Locale {
		return ok(start, false, "", "already converged"), nil
	}
	if err := m.provider.WriteLocale(ctx, p.Locale); err != nil {
		return failure(start), fmt.Errorf("write locale: %w", err)
	}
	return ok(start, true, fmt.Sprintf("locale: %s → %s", displayLocale(current), p.Locale), "applied"), nil
}

func displayLocale(s string) string {
	if strings.TrimSpace(s) == "" {
		return "<unset>"
	}
	return s
}

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}
