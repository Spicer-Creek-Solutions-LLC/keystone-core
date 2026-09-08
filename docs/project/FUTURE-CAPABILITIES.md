# Future Capabilities — Unscheduled

This catalog preserves the direction and lessons of Generation 1 without making
a version or delivery promise. Every item below is `Future / Unscheduled` unless
an accepted RFC explicitly promotes it to the active roadmap.

After archive task R05, canonical historical links will resolve under these
locations:

- `archive/2026-09-pre-v0.6-reboot:FEATURES.md`
- `archive/2026-09-pre-v0.6-reboot:PROJECT-DETAILS.md`
- `archive/2026-09-pre-v0.6-reboot:docs/project/ROADMAP.md`
- `archive/2026-09-pre-v0.6-reboot:epics/`

They are code-form placeholders, not broken forward links. R03 expands this
catalog to checklist-level coverage and pins every source to the final
Generation 1 commit; R05 turns the convenience archive locations into links.

## Promotion policy

Catalog presence means “retain the idea,” not “planned.” Promotion requires the
gate in `REBOOT-EXECUTION-PLAN.md`. An archived implementation can inform a new
design but is not automatically copied; its production wiring, threat model,
NATS use, and black-box acceptance must be re-established.

## Scope subtraction rule

Some Generation 1 domains also contain the narrow foundations explicitly
required by RFC 0001—for example NATS accounts, a server database, an agent
runtime, exact-agent execution, and lifecycle audit. Those minimum slices are
active v0.6 work and are excluded from `Future / Unscheduled`. The catalog
entries below retain only capabilities beyond the accepted RFC/P/C workstreams,
even when a paragraph names the broader historical domain for context. R03 must
mark every checklist-level item `v0.6 baseline` or `Future / Unscheduled` so an
item cannot be in both.

## Catalog

### Foundations and build system

Configuration layering and validation; structured logging; typed errors and
exit codes; build metadata; Make targets; generated documentation; release
archives and native packages; SBOMs, provenance, signing, vanity imports,
package repositories, dependency automation, and multi-platform builds.

### NATS messaging

Embedded and external deployment modes; endpoint failover; typed subjects;
envelopes; JetStream; health; circuit breaking; IPv6; subject mappings; response
permissions; KV and Object Store; queue groups; leaf nodes; superclusters;
WebSocket; federation; and air-gap operation. Generation 2 re-evaluates each
against the NATS-first rule instead of inheriting Keystone-side substitutes.

### Storage

SQLite and Postgres backends; migrations; transaction wrappers; retention;
backup/restore; encryption at rest; multi-table consistency; migration journals;
replication considerations; and storage observability.

### Control plane and APIs

Server lifecycle; gRPC and REST APIs; authentication middleware; RBAC; health;
rate limiting; generated API references; gateway/OpenAPI generation; multiple
listeners; graceful shutdown; service registration; and compatibility policy.

### Agent runtime

Bootstrap UX; service installation; reconnect behavior; metadata collection;
heartbeat; credential rotation; offline buffering; upgrade and rollback;
self-management; dedicated OS users; systemd notification; Windows service and
macOS launchd support; and proxy agents.

### Remote execution and targeting

Label, hostname, OS, architecture, IP, fact, Boolean, glob, and compound
targeting; batch concurrency; streaming output; scripts; pipelines; shell
abstraction; user/environment/working-directory selection; rolling/percentage
batches; resumable output; output archival; command policy; and interactive
sessions.

### State management and standard library

Declarative compilation, dependency ordering, requisites, check/apply,
idempotency, drift, dry-run, rollback, and remote convergence. Candidate modules
include files, directories, templates, services, packages, users, groups, cron,
systemd timers, links, config files, archives, Git, SSH, certificates, security,
kernel/sysctl, system/reboot/locale, networking, routes, bonds, bridges, VLANs,
iptables, nftables, firewalld, mounts, swap, disks, LVM, language packages,
containers, databases, cloud resources, and cross-distribution renderers.

Every module requires real remote-agent Docker acceptance; host-integrated
modules additionally require the relevant VM matrix.

### Events and automation

Durable event storage; taxonomy; filtering; subscriptions; lifecycle tracking;
maintenance windows; schedules; reactors; routing; retention; analysis;
correlation; replay; external sinks; and event-driven automation.

### Identity and authorization

Embedded CA; API keys; mTLS; JWT; join tokens; certificate rotation and
revocation; RBAC CRUD; SPIFFE/SPIRE; cloud workload identities; trust federation;
OIDC/SSO; multi-tenancy; hardware-backed keys; and disaster recovery.

### Secrets

File and Vault backends; dynamic secrets; leases; caching; templates; grants;
transit encryption; data keys; AWS IAM auth; cloud KMS providers; master-key
rotation; per-secret TTL; auditing; and agent delivery.

### Audit and policy

Cryptographic tamper evidence, advanced search, export, redaction, timeline,
analysis, watch, external sinks, and extended retention controls; policy
authoring and CRUD; audit, warn, and enforce modes; violation handlers;
remediation; monitoring; exceptions; and compliance reporting. The narrow
append-oriented correlated lifecycle audit in RFC 0001 is active baseline work.

### GitOps

Repository polling; webhook receivers; commit verification; desired-state
reconciliation; deployment verification; drift feedback; pull-request status;
progressive delivery; rollback; multi-repository policy; and provider adapters.

### Outbound webhooks and integrations

Signed delivery; retries; dead-letter handling; filters; secrets; delivery
history; Slack/Teams/PagerDuty-style integrations; incident tooling; ticketing;
and generic HTTP sinks.

### Clustering and high availability

Leader election; etcd; server membership; sharding; failover; fencing;
split-brain prevention; quorum behavior; agent reassignment; replicated job
state; rolling upgrades; backup coordination; multi-region; and federation.

### Observability

Structured logs; Prometheus metrics; OpenTelemetry traces; health and readiness;
NATS system advisories; dashboards; profiling; capacity planning; adaptive
sampling; telemetry gateway; TUI monitoring; and diagnostic bundles.

### Blueprints and runbooks

Versioned catalogs; publish/install/apply; targeting; parameterization;
dependencies; durable execution; cancellation; rollback; saga/checkpoint
semantics; scheduling; approvals; reusable steps; secrets; testing; and example
catalogs. Remote execution must be proven in separate agent processes before a
blueprint is described as applied.

### Plugin and module system

Starlark and WASM runtimes; capability hosts; sandboxing; resource limits;
signing and transparency; filesystem and remote registries; publish/install;
dependency resolution; test framework; record/replay; multi-file modules;
marketplace; and compatibility lifecycle.

### Multi-environment support

Linux distribution matrix; architectures; IPv6; containers; virtual machines;
bare metal; Kubernetes nodes and operator; Windows; macOS; air-gapped sites;
cloud workload identity; proxying; and heterogeneous-fleet targeting.

### File distribution

NATS Object Store and Git backends; chunking; integrity; resume; mirroring;
conflict handling; caching; authorization; encryption; retention; and large-file
capacity controls.

### Backup, restore, and self-management

Server, SQLite/Postgres, JetStream, etcd, configuration, secrets, and identity
backup adapters; encrypted destinations; restore verification; scheduling;
retention; bootstrap from seed; disaster recovery; and upgrade orchestration.

### Specialized and extension domains

DNS, DHCP, advanced network management, database orchestration, container and
Kubernetes management, cloud providers, rotation orchestration, web UI, MCP
server, marketplace, fleet analytics, compliance reports, and managed/BYOC
control-plane services.

## Not planned by default

The catalog does not imply that Keystone should become a general-purpose IaC
engine, Kubernetes replacement, remote desktop, endpoint-detection product, or
unbounded shell gateway. Those directions require a new product-level RFC, not
ordinary promotion from Future.
