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

	"github.com/spf13/cobra"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
	"go.keystone-core.io/keystone-core/internal/cli"
)

// ErrEngineNotConfigured is returned by apply/rollback/applied when no
// Executor was injected (e.g. a unit test exercising only the local
// verbs, or a build that has not wired the State Runner).
var ErrEngineNotConfigured = errors.New("kscore-blueprint: apply/rollback engine not configured")

// ErrRemoteNotConfigured is returned when --target selects a remote
// agent fleet; distributed apply wiring is a gate-v1.0 ROADMAP item.
var ErrRemoteNotConfigured = errors.New("kscore-blueprint: remote/distributed apply not configured (see ROADMAP: Remote / distributed blueprint apply wiring)")

// Deps wires the CLI's engine seam. A zero Deps supports every local
// verb (init/validate/lint/info/bundle/install/update/remove); apply/
// rollback/applied additionally need Executor.
type Deps struct {
	// Executor backs apply/rollback/applied. nil → those verbs return
	// ErrEngineNotConfigured.
	Executor *bp.Executor
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
