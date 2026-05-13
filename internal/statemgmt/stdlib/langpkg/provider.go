package langpkg

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms.
var ErrUnsupportedOS = errors.New("langpkg: unsupported OS for v1.0 (Linux only)")

// Per-backend sentinels for missing tools.
var (
	ErrNoPip = errors.New("langpkg: the pip binary was not found on PATH")
	ErrNoNpm = errors.New("langpkg: the npm binary was not found on PATH")
	ErrNoGem = errors.New("langpkg: the gem binary was not found on PATH")
)

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoPip(err error) bool         { return errors.Is(err, ErrNoPip) }
func IsNoNpm(err error) bool         { return errors.Is(err, ErrNoNpm) }
func IsNoGem(err error) bool         { return errors.Is(err, ErrNoGem) }

// Provider abstracts the per-manager operations. Production shells
// out to `pip` / `npm` / `gem`; tests inject a fake.
//
// Each Has-method returns (installed, version, error). `version` is
// "" when the package isn't installed; for managers whose query
// command doesn't carry version information cleanly the
// implementation may also return "" with installed==true (and the
// module's version-pin check will then trigger a re-install).
type Provider interface {
	HasPipPackage(ctx context.Context, name string) (bool, string, error)
	InstallPipPackage(ctx context.Context, name, version string) error // version "" = latest
	UninstallPipPackage(ctx context.Context, name string) error

	HasNpmPackage(ctx context.Context, name string) (bool, string, error)
	InstallNpmPackage(ctx context.Context, name, version string) error
	UninstallNpmPackage(ctx context.Context, name string) error

	HasGemPackage(ctx context.Context, name string) (bool, string, error)
	InstallGemPackage(ctx context.Context, name, version string) error
	UninstallGemPackage(ctx context.Context, name string) error
}

// commandRunner runs a language-package-manager binary. It returns
// combined stdout+stderr and, on a non-zero exit, an error wrapping
// the exit code and trimmed output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
