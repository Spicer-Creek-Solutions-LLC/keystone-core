// SPDX-License-Identifier: Apache-2.0

// Command kscore-state-harness applies a state file against the local
// host using the real stdlib module registry, then re-applies it to
// assert idempotency. It is the in-container driver for the Epic 08
// cross-distro matrix (test/e2e/state): unlike the in-process
// integration test it runs the modules against the live system
// (package managers, init systems, /etc, …), which is the whole point
// of the matrix.
//
// It deliberately does NOT stand up a kscore-server (no NATS/Postgres):
// the Runner + Registry are sufficient to drive Check → Apply → Test
// locally, and a static (CGO-free) build runs on glibc and musl alike.
//
// Usage:
//
//	kscore-state-harness <state-file.yaml>
//
// Exit status is 0 only when both the apply pass and the idempotency
// re-apply succeed with zero failures and the second pass reports zero
// changes. Any failure prints a per-declaration diagnostic and exits 1.
package main

import (
	"context"
	"fmt"
	"os"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: kscore-state-harness <state-file.yaml>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "harness:", err)
		os.Exit(1)
	}
}

func run(path string) error {
	reg := statemgmt.NewRegistry()
	if err := stdlib.RegisterAll(reg); err != nil {
		return fmt.Errorf("register stdlib: %w", err)
	}

	decls, err := compile(path)
	if err != nil {
		return fmt.Errorf("compile %s: %w", path, err)
	}
	fmt.Printf("harness: %d declaration(s) from %s\n", len(decls), path)

	runner := statemgmt.NewRunner(reg, nil) // nil observer → no-op; nil Metrics → no emission
	ctx := context.Background()

	// Pass 1 — apply. Must converge with zero failures.
	apply, err := runner.Run(ctx, decls)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	report("apply", apply)
	if apply.Failed > 0 {
		return fmt.Errorf("apply: %d declaration(s) failed", apply.Failed)
	}

	// Pass 2 — re-apply. Idempotent modules must report zero changes
	// the second time (statemgmt.Module contract: Changed=false on the
	// second call). A non-zero Changed here is a real idempotency bug.
	reapply, err := runner.Run(ctx, decls)
	if err != nil {
		return fmt.Errorf("re-apply: %w", err)
	}
	report("re-apply", reapply)
	if reapply.Failed > 0 {
		return fmt.Errorf("re-apply: %d declaration(s) failed", reapply.Failed)
	}
	if reapply.Changed > 0 {
		return fmt.Errorf("re-apply: %d declaration(s) changed on the second pass — not idempotent", reapply.Changed)
	}

	fmt.Println("harness: OK (applied + idempotent)")
	return nil
}

// compile turns a state-file path into ordered declarations using the
// same Parse → Render → Resolve pipeline the CLI's local compile uses.
func compile(path string) ([]*statemgmt.Declaration, error) {
	data, err := os.ReadFile(path) // #nosec G304 G703 -- operator-supplied state-file path, by design
	if err != nil {
		return nil, err
	}
	sf, err := statemgmt.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	rendered, err := statemgmt.NewRenderer().RenderStateFile(sf, nil)
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	decls, err := statemgmt.NewResolver().Resolve(rendered)
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}
	return decls, nil
}

// report prints a one-line summary plus, for any failed declaration,
// its module + error so a red matrix run is diagnosable from the log.
func report(phase string, r *statemgmt.RunReport) {
	fmt.Printf("harness: %s — total=%d changed=%d unchanged=%d failed=%d skipped=%d\n",
		phase, r.Total, r.Changed, r.Unchanged, r.Failed, r.Skipped)
	for _, res := range r.Results {
		if res.Outcome == statemgmt.OutcomeFailed {
			fmt.Printf("  FAIL [%s] %s: %v\n", res.Module, res.DeclID, res.Error)
		}
	}
}
