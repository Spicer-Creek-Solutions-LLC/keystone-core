// SPDX-License-Identifier: Apache-2.0

package cron

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (v1.0 manages
// crontabs via the Linux `crontab(1)` binary).
var ErrUnsupportedOS = errors.New("cron: unsupported OS for v0.1 (Linux only)")

// ErrNoCrontab is returned by mutating operations on a host where the
// `crontab` binary is not on PATH.
var ErrNoCrontab = errors.New("cron: the `crontab` binary was not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoCrontab(err error) bool     { return errors.Is(err, ErrNoCrontab) }

// Provider abstracts the per-user crontab read/write operations.
// Production shells out to `crontab(1)`; tests inject a fake.
type Provider interface {
	// Read returns the user's crontab text. A user with no crontab
	// yields ("", nil) — that is the empty crontab, not an error.
	Read(ctx context.Context, user string) (string, error)
	// Write installs content as the user's crontab (replacing it).
	Write(ctx context.Context, user, content string) error
}

// commandRunner runs the crontab binary. stdin carries the new
// crontab content for writes (empty for reads). It returns combined
// stdout+stderr and, on a non-zero exit, an error wrapping the exit
// code and trimmed output.
type commandRunner func(ctx context.Context, bin string, args []string, stdin string) (string, error)
