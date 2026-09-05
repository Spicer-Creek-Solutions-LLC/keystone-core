// SPDX-License-Identifier: Apache-2.0

package main

// Coverage thresholds. Documented in docs/project/COVERAGE-GATES.md.
const (
	engineThreshold   = 85.0 // state engine — internal/statemgmt (v0.5 gate)
	moduleThreshold   = 80.0 // state stdlib modules — internal/statemgmt/stdlib* (v0.5 gate)
	criticalThreshold = 70.0 // critical packages — business logic
	cliThreshold      = 40.0 // CLI packages — cmd/* and internal/cli/*
)

// enginePackages hold the state engine to the v0.5 gate's engine bar
// (>=engineThreshold). See docs/project/VERSIONING.md § v0.5 gate §
// Engine + acceptance and docs/project/COVERAGE-GATES.md.
var enginePackages = []string{
	"internal/statemgmt",
}

// modulePrefixes hold the state stdlib modules (and the registry that
// composes them) to the v0.5 gate's module bar (>=moduleThreshold).
// A prefix because every child of internal/statemgmt/stdlib is a
// gated state-declaration module.
var modulePrefixes = []string{
	"internal/statemgmt/stdlib",
}

// excludedPrefixes opts packages out of the gate entirely. Use only
// for generated code, tooling, test scaffolding, and SDK / example
// scaffolds that have no productive code to gate.
var excludedPrefixes = []string{
	"pkg/api/v1",        // generated proto + gRPC stubs
	"tools/",            // build / release tooling
	"test/",             // E2E suites themselves
	"modules/examples/", // example modules — illustration, not product code
}

// criticalPrefixes catches packages by directory prefix when an
// explicit entry in criticalPackages would force one entry per
// subpackage. Use only for cohesive directories (the statemgmt
// stdlib modules; the rate-limit subpackages) where every child is
// product code held to the critical bar.
var criticalPrefixes = []string{
	"internal/ratelimit",   // rate-limit subpackages (extract, middleware)
	"modules/sdk/starlark", // Starlark SDK (product code under modules/sdk)
}

// criticalPackages are the v1.0 business-logic packages that hold
// the runtime contract — they must be ≥criticalThreshold. Adding a
// new package here is a deliberate decision: documented in
// docs/project/COVERAGE-GATES.md.
//
// Anything that isn't in this list and isn't excluded falls under
// the cli default (≥cliThreshold). That keeps surprising new
// packages from sneaking in below the floor.
var criticalPackages = []string{
	// Agent runtime
	"internal/agent",
	"internal/agent/bootstrap",
	"internal/agent/bootstrap/tui",
	"internal/agent/systemd",

	// Audit + policy
	"internal/audit",
	"internal/policy",

	// Backup
	"internal/backup",
	"internal/backup/age",
	"internal/backup/dest",

	// Blueprint + runbook (state mgmt is held to the engine/module
	// bars — see enginePackages / modulePrefixes)
	"internal/blueprint",
	"internal/runbook",
	"internal/runbook/observer",
	"internal/runbook/steps",

	// Cluster + clustering
	"internal/cluster",

	// Config + foundational
	"internal/config",
	"internal/health",
	"internal/logging",
	"internal/profiling",
	"internal/tracing",
	"internal/targeting",

	// Control plane
	"internal/controlplane",

	// Events
	"internal/events",
	"internal/events/audit",

	// Execution + files
	"internal/execution",
	"internal/files",
	"internal/files/acl",
	"internal/files/backend",
	"internal/files/proxy",
	"internal/files/transport",

	// GitOps
	"internal/gitops/rollback",
	"internal/gitops/rollback/argoexec",
	"internal/gitops/rollback/gitexec",
	"internal/gitops/verification",
	"internal/gitops/webhook",

	// Identity + secrets
	"internal/identity",
	"internal/sealed",
	"internal/secrets",
	"internal/secrets/file",
	"internal/secrets/vault",

	// Metrics
	"internal/metrics",
	"internal/metrics/cardinality",

	// NATS transport
	"internal/nats",

	// Registry + selfmgmt + s3
	"internal/registry/storage",
	"internal/s3client",
	"internal/selfmgmt",

	// State persistence
	"internal/state",

	// Outbound webhook
	"internal/webhook/outbound",

	// REST + gRPC server
	"pkg/api/agents",
	"pkg/api/apierror",
	"pkg/api/apikeys",
	"pkg/api/auth",
	"pkg/api/blueprint",
	"pkg/api/cluster",
	"pkg/api/events",
	"pkg/api/execution",
	"pkg/api/files",
	"pkg/api/gitops",
	"pkg/api/maintenance",
	"pkg/api/policy",
	"pkg/api/runbook",
	"pkg/api/schedule",
	"pkg/api/secrets",
	"pkg/api/server",
	"pkg/api/state",
	"pkg/api/versioning",
	"pkg/api/webhooks",

	// Module system + plugins
	"pkg/module/audit",
	"pkg/module/cache",
	"pkg/module/capability",
	"pkg/module/cas",
	"pkg/module/loader",
	"pkg/module/manifest",
	"pkg/module/registry",
	"pkg/module/resolver",
	"pkg/module/runtime/starlark",
	"pkg/module/testing",
	"pkg/module/verify",
	"pkg/plugin",

	// Foundational pkg
	"pkg/dbutil",
	"pkg/envelope",
	"pkg/saga",
	"pkg/semver",
	"pkg/statemachine",
	"pkg/version",
	"pkg/wait",
}

// allowList tracks packages currently below their category threshold
// with a per-entry recorded coverage. The gate fails if a listed
// package's measured coverage rises above this number — that's the
// signal to remove the entry and graduate the package into the
// regular gate.
//
// Every entry MUST carry a comment naming the follow-up that will
// raise the coverage (PR, ROADMAP entry, or commitment to land
// alongside a domain epic). Untriaged exceptions are not allowed.
//
// The exact numbers are the present-day measurements rounded down
// to one decimal place — they're a snapshot, not a budget. The gate
// uses 1% headroom (e.g., an entry at 58.3% fails when actual
// hits ~58.9%+) to avoid false positives from CI scheduling jitter
// while still catching meaningful regressions.
var allowList = map[string]float64{
	// internal/state — 44.4% statement-weighted. The Postgres
	// driver path is partially exercised by KSCORE_TEST_POSTGRES_DSN-
	// gated integration tests; the unit-test path covers SQLite +
	// the shared helpers. Graduate via a Postgres-test job in CI
	// (v1.x ROADMAP: "internal/state Postgres path coverage").
	"internal/state": 44.4,

	// pkg/api/cluster — 68.7%. The handler covers Status / Leader /
	// Members happy paths; the rebalance/backup/evict paths return
	// 503 today because the providers aren't wired (gate-v1.0
	// "Cluster gRPC services boot registration"). Graduate when
	// the providers land.
	"pkg/api/cluster": 68.7,

	// pkg/api/secrets — 65.8%. Handler covers Get/Write happy paths
	// but not the lease/transit fallback branches that need the
	// Vault backend. Graduate when the Vault E2E lands (v1.x
	// ROADMAP: "Secrets transit + lease E2E against Vault").
	//
	// The number moved 63.0 -> 65.8 with the Go 1.27.1 bump and NOT
	// with any test change: the same commit on main reports 63.0%
	// under go1.26.4 and 65.8% under go1.27.1. Go 1.27 counts
	// statements differently, so this is a re-baselined snapshot, not
	// a coverage improvement — the untested branches are unchanged.
	"pkg/api/secrets": 65.8,

	// cmd/kscore-server — 38.9% statement-weighted. Boot-path
	// branches (TLS source, identity provider, bootstrap PSK vs
	// join-token, post-Start router wiring) need fixture-driven
	// tests. Graduate alongside the v1.0 release-prep hardening
	// pass (epic 19 task 8).
	"cmd/kscore-server": 38.9,

	// cmd/* binaries below the 40% CLI floor. Each is the v1.0
	// thin-wrapper around an internal/cli/* package, which IS
	// gated. Tests for the main() wrappers add boilerplate without
	// catching real bugs — graduating these is tracked as a v1.x
	// ROADMAP entry "Per-binary smoke tests for kscore-* wrappers".
	"cmd/kscore-audit":          0.0,
	"cmd/kscore-backup":         0.0,
	"cmd/kscore-blueprint":      0.0,
	"cmd/kscore-bootstrap":      0.0,
	"cmd/kscore-cluster":        0.0,
	"cmd/kscore-cluster-backup": 0.0,
	"cmd/kscore-events":         0.0,
	"cmd/kscore-files":          0.0,
	"cmd/kscore-gitops":         0.0,
	"cmd/kscore-identity":       0.0,
	"cmd/kscore-policy":         0.0,
	"cmd/kscore-runbook":        0.0,
	"cmd/kscore-secrets":        0.0,
	"cmd/kscore-webhook":        0.0,
}
