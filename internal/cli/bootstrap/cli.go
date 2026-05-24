// SPDX-License-Identifier: Apache-2.0

// Package bootstrap implements the kscore-bootstrap CLI (Epic 18
// task 7b). The binary reads a [selfmgmt.SeedConfig] from --seed,
// runs the [selfmgmt.BootstrapManager] state machine through every
// phase, and reports the final state.
//
// Real PhaseHandler implementations (host detect, binary install,
// systemd unit gen, blueprint engine invocation, /health verify) are
// deferred under the gate-v1.0 ROADMAP entry "Bootstrap phase
// handlers + durable checkpointer". Task 7b ships only the CLI
// shell + a [loggingPhaseHandler] whose methods log entry at info
// level and return nil — enough to drive the FSM end-to-end and
// prove the wiring works.
package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/internal/selfmgmt"
)

// NewBootstrapCommand returns the kscore-bootstrap cobra root. The
// binary entry point in cmd/kscore-bootstrap/main.go calls this and
// Execute.
func NewBootstrapCommand() *cobra.Command {
	var (
		seedPath string
		logLevel string
	)
	cmd := &cobra.Command{
		Use:   "kscore-bootstrap",
		Short: "Bootstrap a kscore-server cluster from a seed config",
		Long: `kscore-bootstrap reads a SeedConfig YAML and drives the
selfmgmt.BootstrapManager state machine through the six
phases (detect, configure, validate, install, blueprints,
verify). v1.0 ships only the FSM wiring with a logging
NoOp PhaseHandler; real install / configure / verify work
lands per the gate-v1.0 ROADMAP entry.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := newLogger(logLevel)
			return run(cmd.Context(), cmd.OutOrStdout(), logger, seedPath)
		},
	}
	cmd.Flags().StringVar(&seedPath, "seed", "", "Path to the SeedConfig YAML")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn, error")
	_ = cmd.MarkFlagRequired("seed")
	cli.AddVersion(cmd)
	return cmd
}

// run is the testable entry point — accepts io.Writer + *slog.Logger
// so tests can drive the binary with deterministic output capture.
func run(ctx context.Context, out io.Writer, logger *slog.Logger, seedPath string) error {
	seed, err := selfmgmt.LoadSeed(seedPath)
	if err != nil {
		return err
	}

	handler := &loggingPhaseHandler{logger: logger}
	mgr, err := selfmgmt.NewBootstrapManager(seed, handler, selfmgmt.WithLogger(logger))
	if err != nil {
		return err
	}

	if err := mgr.Run(ctx); err != nil {
		return err
	}

	fmt.Fprintln(out, "OK")
	fmt.Fprintf(out, "state:        %s\n", mgr.State())
	fmt.Fprintf(out, "cluster_name: %s\n", seed.ClusterName)
	fmt.Fprintf(out, "node_role:    %s\n", seed.NodeRole)
	fmt.Fprintf(out, "transitions:  %d\n", len(mgr.History()))
	return nil
}

// loggingPhaseHandler satisfies [selfmgmt.PhaseHandler] by logging
// each phase entry and returning nil. Real implementations land per
// the deferred ROADMAP entry — this handler proves the CLI + FSM
// wiring is correct without overreaching into host work.
type loggingPhaseHandler struct {
	logger *slog.Logger
}

func (h *loggingPhaseHandler) Detect(ctx context.Context, s *selfmgmt.SeedConfig) error {
	h.logger.InfoContext(ctx, "bootstrap: phase Detect", "node_role", s.NodeRole)
	return nil
}
func (h *loggingPhaseHandler) Configure(ctx context.Context, s *selfmgmt.SeedConfig) error {
	h.logger.InfoContext(ctx, "bootstrap: phase Configure", "storage_driver", s.Storage.Driver)
	return nil
}
func (h *loggingPhaseHandler) Validate(ctx context.Context, s *selfmgmt.SeedConfig) error {
	h.logger.InfoContext(ctx, "bootstrap: phase Validate", "tls_strategy", s.TLSStrategy)
	return nil
}
func (h *loggingPhaseHandler) Install(ctx context.Context, _ *selfmgmt.SeedConfig) error {
	h.logger.InfoContext(ctx, "bootstrap: phase Install")
	return nil
}
func (h *loggingPhaseHandler) ApplyBlueprints(ctx context.Context, s *selfmgmt.SeedConfig) error {
	h.logger.InfoContext(ctx, "bootstrap: phase ApplyBlueprints", "blueprints", len(s.Blueprints))
	return nil
}
func (h *loggingPhaseHandler) Verify(ctx context.Context, _ *selfmgmt.SeedConfig) error {
	h.logger.InfoContext(ctx, "bootstrap: phase Verify")
	return nil
}
func (h *loggingPhaseHandler) Rollback(ctx context.Context, _ *selfmgmt.SeedConfig, failedAt selfmgmt.BootstrapState) error {
	h.logger.WarnContext(ctx, "bootstrap: phase Rollback", "failed_at", failedAt)
	return nil
}

// newLogger builds a stderr-backed slog.Logger filtered to the
// requested level. Unknown levels degrade to info.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
