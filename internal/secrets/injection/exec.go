package injection

import (
	"context"
	"os/exec"
)

// execCommand creates an exec.Cmd with context support.
// This is a wrapper to enable testing.
//
//nolint:gocritic // unlambda: wrapper function is intentional for testing
var execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
