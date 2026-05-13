// Package langpkg implements the `langpkg` stdlib state module —
// manages one language-ecosystem package per declaration via pip /
// npm / gem, per PROJECT-DETAILS §4.8 (Files & VCS category).
//
// Distinct from the `package` module: `langpkg` manages packages
// installed by a language toolchain (Python's pip, Node's npm,
// Ruby's gem), not by the OS package manager (apt / dnf / …). The
// operator picks the manager explicitly — there is no auto-detect
// across ecosystems.
//
// Operations per declaration:
//
//   - `name: <pkg>` + `manager: pip|npm|gem` + optional `version:
//     <ver>` (strict-equality pin) → install / uninstall.
//
// Installs are **system-wide / global** in v1.0 (`pip install`,
// `npm install -g`, `gem install`). Per-user (`pip install --user`,
// `npm config prefix`) and per-project (a `working_dir:` + venv /
// `npm install` non-global / `bundle install --deployment`) modes
// are V1X.
//
// Version semantics:
//   - **Unset** → `present` matches if any version is installed.
//   - **Set** → `present` matches only if exactly that version is
//     installed; Apply runs the manager's install command pinned to
//     that version (pip `==`, npm `@`, gem `-v`). Strict equality
//     only; semver ranges and constraints are V1X.
//
// Declaration.Name is just a human label (the decl ID); the package
// identity is the `name` + `manager` pair. DriftSeverity HIGH —
// language-ecosystem packages typically back operational tooling
// (pm2, gunicorn, bundler, …); a missing or wrong-version package
// breaks the service that depends on it.
//
// v1.0 out of scope (V1X candidates):
//   - **Per-user / per-project installs**: `working_dir:`, `user:`,
//     pip `--user` and venv, pipx, npm `--prefix` / non-global, gem
//     `--user-install` / Bundler. v1.0 is global-only.
//   - **PEP-668 `--break-system-packages`** opt-in for pip on modern
//     Debian / Ubuntu / etc. (where system-wide pip is blocked by
//     default). v1.0 lets pip's own error surface; the operator
//     either uses an `apt` package, `pipx`, or sets the flag
//     manually outside the module.
//   - **Lockfile-driven installs**: `npm ci`, `pip install -r
//     requirements.txt`, `bundle install --deployment`.
//   - **Semver / version-range** matching: `>= 1.2`, `~> 2.5`,
//     `^3.0.0`. v1.0 is strict-equality only.
//   - **Manager-options pass-through**: arbitrary install flags
//     (`--index-url`, `--registry`, `--no-cache`, mirror configs).
//   - **Additional managers**: cargo (Rust), composer (PHP), mvn /
//     gradle (Java), `go install`.
package langpkg

import (
	"context"
	"fmt"
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

func (m *Module) Name() string { return "langpkg" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: language-ecosystem packages typically back
// operational tooling (process managers, app servers, build tools);
// a missing or wrong-version package breaks the dependent service.
// HIGH always; MEDIUM nil.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
	return statemgmt.DriftSeverityHigh
}

func (m *Module) Check(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	installed, currentVer, err := m.has(ctx, p)
	if err != nil {
		return nil, err
	}
	loc := fmt.Sprintf("%s package %q", p.Manager, p.Name)
	switch p.State {
	case StatePresent:
		if !installed {
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s not installed → install", loc)}, nil
		}
		if p.Version != "" && currentVer != p.Version {
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s: %s → %s", loc, displayVer(currentVer), p.Version)}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	case StateAbsent:
		if !installed {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s installed (%s); want absent", loc, displayVer(currentVer))}, nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	installed, currentVer, err := m.has(ctx, p)
	if err != nil {
		return failure(start), err
	}
	loc := fmt.Sprintf("%s package %q", p.Manager, p.Name)

	switch p.State {
	case StatePresent:
		needInstall := !installed
		if installed && p.Version != "" && currentVer != p.Version {
			needInstall = true // pin mismatch — re-install at the target version
		}
		if !needInstall {
			return ok(start, false, "", "already converged"), nil
		}
		if err := m.install(ctx, p); err != nil {
			return failure(start), fmt.Errorf("install %s: %w", loc, err)
		}
		from := displayVer(currentVer)
		to := p.Version
		if to == "" {
			to = "<latest>"
		}
		return ok(start, true, fmt.Sprintf("%s: %s → %s", loc, from, to), "applied"), nil
	case StateAbsent:
		if !installed {
			return ok(start, false, "", "already converged"), nil
		}
		if err := m.uninstall(ctx, p); err != nil {
			return failure(start), fmt.Errorf("uninstall %s: %w", loc, err)
		}
		return ok(start, true, fmt.Sprintf("removed %s (%s)", loc, displayVer(currentVer)), "applied"), nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
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

// has dispatches to the per-manager Provider methods. Returns
// (installed, current-version, error). current-version is "" when
// not installed or when the manager can't report it.
func (m *Module) has(ctx context.Context, p *params) (bool, string, error) {
	switch p.Manager {
	case ManagerPip:
		return m.provider.HasPipPackage(ctx, p.Name)
	case ManagerNpm:
		return m.provider.HasNpmPackage(ctx, p.Name)
	case ManagerGem:
		return m.provider.HasGemPackage(ctx, p.Name)
	}
	return false, "", fmt.Errorf("unknown manager %q", p.Manager)
}

func (m *Module) install(ctx context.Context, p *params) error {
	switch p.Manager {
	case ManagerPip:
		return m.provider.InstallPipPackage(ctx, p.Name, p.Version)
	case ManagerNpm:
		return m.provider.InstallNpmPackage(ctx, p.Name, p.Version)
	case ManagerGem:
		return m.provider.InstallGemPackage(ctx, p.Name, p.Version)
	}
	return fmt.Errorf("unknown manager %q", p.Manager)
}

func (m *Module) uninstall(ctx context.Context, p *params) error {
	switch p.Manager {
	case ManagerPip:
		return m.provider.UninstallPipPackage(ctx, p.Name)
	case ManagerNpm:
		return m.provider.UninstallNpmPackage(ctx, p.Name)
	case ManagerGem:
		return m.provider.UninstallGemPackage(ctx, p.Name)
	}
	return fmt.Errorf("unknown manager %q", p.Manager)
}

func displayVer(v string) string {
	if v == "" {
		return "<none>"
	}
	return v
}

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}
