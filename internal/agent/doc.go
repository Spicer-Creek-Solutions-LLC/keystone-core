// Package agent is the kscore-agent daemon's runtime (Epic 06). The
// Agent struct owns lifecycle: subscribes to its command topic and
// spawns heartbeat + metadata loops. Tasks 2–11 fill in the
// Executor (Task 2), MetadataCollector (Task 3), SecurityEnforcer
// (Task 4), full command-response wire-up (Task 5), and so on.
//
// Layering: internal/agent stays free of internal/nats imports —
// the NATSClient and Subjects interfaces are satisfied structurally
// by internal/nats.Manager (via a thin adapter in cmd/kscore-agent).
// Same pattern internal/controlplane uses.
package agent
