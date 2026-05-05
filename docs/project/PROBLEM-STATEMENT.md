# Problem Statement

> Derived from a review of `new-epics/` (Epics 00–19). Companion to `FEATURES.md`,
> `PROJECT-DETAILS.md`, and `DESIGN.md`. Captures *why* Keystone Core exists and what
> v1.0 specifically commits to solving.

## The gap

Modern infrastructure has two well-served extremes and a hollow middle:

- **Deployment tooling** (Argo CD, Flux, Terraform, Pulumi, Crossplane) is excellent at
  *getting* infrastructure into a desired state from a Git repository.
- **Cluster orchestration** (Kubernetes, service mesh, cloud control planes) is excellent
  at *running* containerized workloads inside a tightly-scoped substrate.

What sits between them — **the runtime operations control plane that keeps real,
heterogeneous fleets healthy after deployment** — is fragmented across legacy
config-management tools, hand-rolled glue, ticketing workflows, and ad-hoc SSH.
Sysadmins are forced to choose between:

1. **Tools that do too little.** GitOps controllers reconcile manifests but cannot run a
   one-shot command across 500 hosts, broker secrets to an agent, verify a deployment
   with an HTTP probe and roll it back, detect configuration drift on a bare-metal box,
   or coordinate a saga across mixed VMs and containers.
2. **Tools that do too much, or assume the wrong substrate.** Kubernetes-native
   operators presume Kubernetes. Heavy enterprise suites bring vendor lock-in, pricing
   friction, and operational weight unjustified for trial or mid-scale environments.
3. **Legacy config-management tools** (Salt, Puppet, Chef, Ansible) whose UX sysadmins
   still respect, but whose architectures predate cloud-native expectations: clustering
   bolted on, identity weak or absent by default, GitOps integration retrofitted,
   observability secondary, packaging painful, and modern security primitives (mTLS,
   SPIFFE, signed modules, capability-based plugins) not native.

The result: **GitOps deploys it, and then nothing reliably keeps it running.** Day-2
operations — remote execution with targeting, declarative state with drift remediation,
deployment verification, rollback orchestration, secret distribution, audit, automation
triggered by events — get stitched together from incompatible tools, with HA, identity,
and policy as afterthoughts.

## What Keystone Core sets out to solve

Be the **runtime operations control plane** between deployment tooling and day-2
operations — clusterable from day 1, security-real from day 1, and shaped like the
Salt-Project UX sysadmins are already productive in, but built on a modern Go stack
with cloud-native primitives.

Concretely, v1.0 must demonstrate, in a single trial-friendly install, that one system can:

- **Manage heterogeneous fleets** (Linux VMs, bare metal, mixed distros) via a single
  agent talking NATS, with bootstrap UX measured in minutes (Epic 06).
- **Run remote commands** with rich targeting (glob, label, compound expressions), batch
  concurrency, streaming output, and command policy (Epic 07).
- **Apply declarative state** through ~40 universal Linux modules covering ~90% of daily
  sysadmin tasks, with drift detection and remediation (Epic 08).
- **Run highly available out of the box** — 3-node cluster, embedded etcd, leader
  election <3s, failover <10s, split-brain prevention, NATS-fallback recovery — *not*
  deferred to "enterprise edition" (Epic 13).
- **Issue real identity from day 1** — embedded SPIFFE-shaped CA, mTLS, join tokens, API
  keys, JWT — with a clean swap-in path to SPIRE later (Epic 09).
- **Broker secrets** via an encrypted-file backend (zero-deps) or HashiCorp Vault, with
  leases, transit, and audit (Epic 10).
- **Integrate with GitOps tooling** by accepting Argo CD / Flux / GitHub / GitLab
  webhooks, running verification workflows, and orchestrating manual rollback —
  extending GitOps rather than replacing it (Epic 16).
- **Be extensible safely** through Starlark modules with capability-based sandboxing and
  Cosign-verified distribution, without forcing operators into WASM toolchains or cloud
  registries on day 1 (Epic 14).
- **Compose higher-level operations** via blueprints (Salt-formula-shaped), runbooks,
  and a saga coordinator for multi-step rollback (Epic 15).
- **Audit and observe** every sensitive action with structured events, a full audit log,
  audit-mode policy evaluation, Prometheus metrics, OpenTelemetry traces, and
  ready-to-import Grafana dashboards (Epics 11, 12, 17).
- **Self-manage** — bootstrap from seed, back up, restore, rate-limit, distribute files —
  so the control plane's own lifecycle is a first-class capability, not a runbook
  (Epic 18).

## Audience

Sysadmins, platform/SRE teams, and infrastructure engineers running mixed environments
who:

- already use GitOps for deployment but lack a credible day-2 control plane,
- want Salt-Project-shaped ergonomics without inheriting Salt's architectural debt,
- need HA, mTLS identity, and audit as defaults — not as a paid tier — to clear basic
  security review for a commercial trial,
- need to be productive in a lab in under an hour and in a real environment in under a
  day.

## Success criteria for v1.0

A trial-ready release in which:

- a 3-node cluster forms in under 10 seconds and elects its first leader in under 3,
- a single binary boots with embedded NATS + SQLite for zero-deps demos,
- ~40 state modules pass a cross-distro matrix (Debian, Ubuntu, RHEL, Rocky, Alpine),
- a signed Starlark module installs and executes,
- a blueprint catalog of six applies end-to-end,
- GitOps webhooks trigger verification + rollback,
- HA resilience tests (NATS failure, etcd failure, partition, split-brain) pass on
  every PR,
- backup → restore round-trips losslessly,
- every sensitive operation is audited.

Full acceptance criteria are tracked in `new-epics/00-meta-reconstruction-plan.md`.

## What is explicitly *not* the problem v1.0 solves

To avoid scope creep that has historically sunk comparable projects, v1.0 deliberately
defers: WASM modules, Windows/macOS agents, K8s operator embedding, full SPIRE
integration, policy *enforcement* (audit-only in v1.0), federation, supercluster/leaf
NATS, web UI, and blueprint marketplace. Each has a target version in
`PROJECT-DETAILS.md §6.2`. The discipline is intentional: a credible v1.0 that wins
commercial trials beats a sprawling v1.0 that ships late.
