// SPDX-License-Identifier: Apache-2.0

// kscore-bootstrap is the Keystone Core bootstrap CLI (Epic 18
// task 7b). It reads a SeedConfig YAML (Epic 18 task 1) and drives
// the BootstrapManager state machine (Epic 18 task 2) through every
// phase, reporting the final state.
//
// Real install / configure / verify work defers to the gate-v1.0
// ROADMAP entry "Bootstrap phase handlers + durable checkpointer";
// this binary ships the CLI shell + a logging NoOp PhaseHandler so
// the FSM + wiring are testable end-to-end today.
package main

import (
	"fmt"
	"os"

	"go.keystone-core.io/keystone-core/internal/cli/bootstrap"
)

func main() {
	if err := bootstrap.NewBootstrapCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
