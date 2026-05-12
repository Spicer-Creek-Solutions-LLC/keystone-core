package timer

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (v1.0 ships a
// systemd backend only).
var ErrUnsupportedOS = errors.New("systemd_timer: unsupported OS for v1.0 (Linux only)")

// ErrNoBackend is returned by the systemctl-backed operations on a
// Linux host where `systemctl` is not on PATH (no systemd).
var ErrNoBackend = errors.New("systemd_timer: systemd not found on this host (systemctl not on PATH)")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoBackend(err error) bool     { return errors.Is(err, ErrNoBackend) }

// TimerStatus is the live enable/active state of a .timer unit.
// Exists==false → systemd has no record of the unit (not loaded).
type TimerStatus struct {
	Exists  bool
	Enabled bool // starts at boot (UnitFileState enabled/static/…)
	Active  bool // currently armed (ActiveState active)
}

// Provider abstracts the unit-file and systemctl operations the
// module needs. All methods take the full unit name ("<name>.timer").
// Production uses the systemd impl; tests inject a fake.
type Provider interface {
	// ReadUnit returns the content of the named .timer unit file
	// under the unit directory, or ("", false, nil) if it doesn't
	// exist.
	ReadUnit(name string) (content string, exists bool, err error)
	// WriteUnit atomically writes the named .timer unit file.
	WriteUnit(name, content string) error
	// RemoveUnit deletes the named .timer unit file (no-op if
	// already gone).
	RemoveUnit(name string) error
	// DaemonReload runs `systemctl daemon-reload`.
	DaemonReload(ctx context.Context) error
	// Status reports whether the unit is loaded, enabled and active.
	Status(ctx context.Context, name string) (*TimerStatus, error)
	// EnableNow runs `systemctl enable --now <name>`.
	EnableNow(ctx context.Context, name string) error
	// DisableStop runs `systemctl disable --now <name>`.
	DisableStop(ctx context.Context, name string) error
}

// commandRunner is the injection point for the systemctl verbs.
// Production wires execRun; tests inject a capturing / scripted
// version so arg formation and parsing can be tested without a
// running systemd.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
