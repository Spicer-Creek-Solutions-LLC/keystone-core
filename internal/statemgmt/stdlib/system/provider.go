// SPDX-License-Identifier: Apache-2.0

package system

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (the v1.0
// system module is Linux-only).
var ErrUnsupportedOS = errors.New("system: unsupported OS for v0.1 (Linux only)")

// ErrNoShutdown is returned when the reboot op is invoked but
// `shutdown` is not on PATH.
var ErrNoShutdown = errors.New("system: the shutdown binary was not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoShutdown(err error) bool    { return errors.Is(err, ErrNoShutdown) }

// Provider abstracts the per-op shell-outs and file operations the
// system module performs. Production reads / writes the standard
// system files and shells `shutdown` / `localectl`; tests inject a
// fake.
type Provider interface {
	// --- banner ---
	// ReadBanner returns the file contents for the named banner.
	// A missing file returns ("", nil) — the absent baseline.
	ReadBanner(ctx context.Context, name string) (string, error)
	// WriteBanner writes content atomically to the named banner
	// file, preserving the file's existing mode (0644 for a new
	// file).
	WriteBanner(ctx context.Context, name, content string) error

	// --- reboot ---
	// IsRebootNeeded reports whether the marker file exists. An
	// unreachable marker path (a real I/O error, not a missing file)
	// surfaces as an error.
	IsRebootNeeded(ctx context.Context, markerFile string) (bool, error)
	// ScheduleReboot shells `shutdown -r +<delay>` (or `-r now` for
	// delay==0). Returns when the shutdown command itself returns;
	// the reboot happens asynchronously.
	ScheduleReboot(ctx context.Context, delayMinutes int) error

	// --- locale ---
	// ReadLocale returns the persistent LANG= value from
	// /etc/locale.conf. A missing file returns ("", nil); a file
	// without a LANG= line also returns ("", nil).
	ReadLocale(ctx context.Context) (string, error)
	// WriteLocale writes /etc/locale.conf (LANG=<value>, atomic,
	// preserves existing file mode — 0644 when creating) and — when
	// `localectl` is on PATH — also runs `localectl set-locale
	// LANG=<value>` so the change is live.
	WriteLocale(ctx context.Context, lang string) error
}

// commandRunner runs `shutdown` / `localectl`. It returns combined
// stdout+stderr and, on a non-zero exit, an error wrapping the exit
// code and trimmed output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
