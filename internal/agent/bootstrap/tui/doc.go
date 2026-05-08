// Package tui implements the interactive bootstrap wizard for
// kscore-agent. It exposes a Configurer that satisfies
// internal/agent/bootstrap.Configurer; the wizard runs a charm
// huh.Form with grouped screens (Mode → Cluster identity →
// Node role → NATS join → Config path → Confirm), validates each
// field with the pure validators in validate.go, and returns a
// fully populated *bootstrap.Configuration.
//
// v1.0 supports demo mode end-to-end. Production / enterprise
// modes display a "v1.0 supports demo only" notice and abort —
// they gate on Epic 11 (Identity & Auth) for TLS cert collection
// and Epic 14 + 17 for blueprint selection. Tracked in
// docs/project/V1X-BACKLOG.md.
//
// The TUI never invokes the bootstrap engine itself; the caller
// (cmd/kscore-agent's bootstrap subcommand) wires this Configurer
// into bootstrap.NewEngine alongside the default Detector /
// Validator / Installer / Verifier. State persistence + checkpoint
// resume from Task 6 keeps working — interrupted bootstrap re-runs
// land the user back in the Configure phase with previously
// collected values prefilled.
package tui
