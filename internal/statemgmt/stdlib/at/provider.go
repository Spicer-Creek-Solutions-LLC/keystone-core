// SPDX-License-Identifier: Apache-2.0

package at

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (v1.0 manages
// `at` jobs via the Linux `at` toolchain).
var ErrUnsupportedOS = errors.New("at: unsupported OS for v0.1 (Linux only)")

// ErrNoAt is returned by mutating operations on a host where the
// `at` binary is not on PATH (the at daemon / package isn't
// installed).
var ErrNoAt = errors.New("at: the `at` binary was not found on PATH (install the `at` package)")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoAt(err error) bool          { return errors.Is(err, ErrNoAt) }

// Provider abstracts the `at` queue operations. Production shells out
// to the `at` binary (`at -l`, `at -c`, `at -r`, `at -q <q> <time>`);
// tests inject a fake.
type Provider interface {
	// ListJobs returns the queued job IDs for the current user. An
	// empty queue is ([], nil) — not an error.
	ListJobs(ctx context.Context) ([]string, error)
	// JobScript returns the full script of job id (`at -c id`),
	// which includes at's environment preamble followed by the
	// submitted commands.
	JobScript(ctx context.Context, id string) (string, error)
	// Submit queues a new job in `queue` for `timeSpec`, piping
	// `script` to it.
	Submit(ctx context.Context, queue, timeSpec, script string) error
	// Remove deletes job id from the queue.
	Remove(ctx context.Context, id string) error
}

// commandRunner runs the `at` binary. stdin carries the job script
// for Submit (empty for the other verbs). It returns combined
// stdout+stderr and, on a non-zero exit, an error wrapping the exit
// code and trimmed output.
type commandRunner func(ctx context.Context, bin string, args []string, stdin string) (string, error)
