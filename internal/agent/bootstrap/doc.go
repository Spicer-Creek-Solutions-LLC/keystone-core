// SPDX-License-Identifier: Apache-2.0

// Package bootstrap is the kscore-agent bootstrap engine (Epic 06
// task 6). The Engine runs five phases as a state machine —
// Detect, Configure, Validate, Install, Verify — and persists
// progress to disk between phases so a re-run resumes from the
// last checkpoint.
//
// Layering: Engine wires pluggable Detector / Configurer /
// Validator / Installer / Verifier interfaces. Task 6 (this code)
// ships the FSM + demo-mode concretes; Task 7 (TUI) and Task 8
// (CLI flags) ship Configurer drivers; Task 9 lands the systemd
// production-mode Installer.
//
// Idempotency is the load-bearing contract — operators can re-run
// kscore-agent bootstrap without breaking existing state.
// PROJECT-DETAILS §4.6 risks: "Bootstrap idempotency — every phase
// has a checkpoint. Re-running must not break existing state.
// Hammer with tests."
//
// Rollback (transactional revert of Install side-effects) is a v1.x
// addition; v1.0 ships LastError recording + operator-driven
// recovery.
package bootstrap
