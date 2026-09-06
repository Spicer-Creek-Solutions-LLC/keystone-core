// SPDX-License-Identifier: Apache-2.0

// Package blueprint implements the kscore-blueprint CLI (Epic 15
// task 10): init, validate, lint, info, install, update, remove,
// applied, apply, rollback, bundle. The package is dependency-light;
// the apply/rollback engine is injected via Deps so cmd/kscore-
// blueprint wires the real (local-host) State Runner at boot — the
// established Epic 14 production-wiring pattern.
package blueprint

import (
	"context"
	"errors"
	"io"

	"github.com/spf13/cobra"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
	"go.keystone-core.io/keystone-core/internal/cli"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// ErrEngineNotConfigured is returned by apply/rollback/applied when no
// Executor was injected (e.g. a unit test exercising only the local
// verbs, or a build that has not wired the State Runner).
var ErrEngineNotConfigured = errors.New("kscore-blueprint: apply/rollback engine not configured")

// Deps wires the CLI's engine seam. A zero Deps supports every local
// verb (init/validate/lint/info/bundle/install/update/remove); apply/
// rollback/applied additionally need Executor.
type Deps struct {
	// Executor backs apply/rollback/applied. nil → those verbs return
	// ErrEngineNotConfigured.
	Executor *bp.Executor

	// Dial reaches the control plane for a TARGETED apply. A remote
	// apply cannot run in this process -- the control plane holds the
	// converge dispatcher and the agent registry -- so the CLI asks it
	// to do the work. nil falls back to the production dialer.
	Dial func(ctx context.Context, server, apiKey string) (v1.BlueprintServiceClient, io.Closer, error)
	// Server and APIKey address that control plane. Both are also
	// settable per-invocation with --server / --api-key.
	Server string
	APIKey string
}

// dial returns the configured dialer or the production one.
func (d Deps) dial() func(ctx context.Context, server, apiKey string) (v1.BlueprintServiceClient, io.Closer, error) {
	if d.Dial != nil {
		return d.Dial
	}
	return dialBlueprintService
}

func (d Deps) engine() (*bp.Executor, error) {
	if d.Executor == nil {
		return nil, ErrEngineNotConfigured
	}
	return d.Executor, nil
}

// NewCommand returns the kscore-blueprint root command.
func NewCommand(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "kscore-blueprint",
		Short:         "Keystone Core blueprint authoring + apply CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		initCmd(), validateCmd(), lintCmd(), infoCmd(),
		installCmd(), updateCmd(), removeCmd(),
		applyCmd(d), rollbackCmd(d), appliedCmd(d), bundleCmd(),
	)
	cli.AddVersion(root)
	return root
}

// loadManifest is the shared "load + structurally validate" step
// (blueprint.Load already runs Validate).
func loadManifest(dir string) (*bp.Manifest, error) {
	return bp.Load(dir)
}

func argDir(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return "."
}

// withContext returns the command's context (cobra wires one).
func withContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
