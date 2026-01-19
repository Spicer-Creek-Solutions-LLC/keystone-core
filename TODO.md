# TODO.md

This is a TODO list of work that still needs to be done outside any current epic.

## Immediate Priority (0.1.0 Release)

### Security (Epic 31 Gaps)
- [x] Complete comprehensive threat model documentation
- [x] Implement formal ADR (Architectural Decision Records) framework with templates
- [x] Document security review process with checklists for PRs
- [x] Add automated security checks to CI/CD pipeline
- [x] Create security training material for contributors
- [x] Establish formal security governance structure
- [x] Create security incident response plan
- [x] Document security release process

### CLI & Documentation (Epic 30 Gaps)
- [x] Create comprehensive migration guide for all command splits
- [x] Update shell completion scripts for all platforms (bash, zsh, fish, PowerShell)
- [x] Consolidate command documentation (currently scattered)
- [x] Enhance help system to mention new command locations for deprecated commands
- [x] Create script migration tools for common patterns

### Test Coverage
- [x] Increase CLI command test coverage from 6-22% to >40%
- [x] Add tests for TUI components (currently 0% coverage)
- [x] Add tests for telemetry gateway (currently 0% coverage)

---

## Short-Term Priority (1-2 Releases)

### Test Coverage Improvements
- [ ] Epic 3 (State Management): Increase coverage from 44% to >80%
- [ ] Add edge case tests for complex requisite chains and circular dependencies
- [ ] Add comprehensive cross-platform testing (Windows, macOS)
- [ ] Add integration tests between all major epics
- [ ] Implement load test scenarios with configurable agent counts

### Performance & Benchmarking
- [x] Document performance baselines for agent registration throughput
- [x] Document performance baselines for command execution latency percentiles
- [x] Document performance baselines for state application throughput
- [x] Document performance baselines for event processing latency
- [x] Document performance baselines for policy evaluation latency
- [x] Add automated performance benchmarking to CI/CD with regression alerting
- [x] Benchmark pure Go SQLite vs CGO implementation (Epic 13)
- [x] Benchmark IPv6 vs IPv4 performance (Epic 18)
- [ ] Benchmark Windows vs Linux agent performance (Epic 20)
- [x] Benchmark large file distribution performance (Epic 22)
- [ ] Benchmark high-volume SVID issuance scenarios (Epic 17)
- [ ] Benchmark gateway performance at scale (Epic 19)

### Documentation Improvements
- [x] Expand REST API reference with OpenAPI/Swagger documentation
- [x] Add 10+ real-world scenario examples for each major feature
- [x] Review all documentation to verify diagrams are in Mermaid format (convert ASCII art diagrams)
- [x] Create video tutorial series for getting started
- [x] Expand migration guides from Salt/Ansible/Puppet with step-by-step procedures
- [x] Document non-Go SDK development guidelines
- [x] Publish API versioning and compatibility policy
- [x] Add link validation to CI/CD pipeline
- [x] Create troubleshooting guides for common issues across all epics
- [x] Document sizing guidelines for embedded NATS/SQLite deployments

### Epic 1 (Core Infrastructure) Gaps
- [x] Add transaction logging for long-running SQLite→PostgreSQL migrations
- [x] Document edge cases for embedded NATS memory sizing
- [x] Create smoke tests for both SQLite and PostgreSQL backends in pre-commit hooks
- [x] Implement progress reporting for long-running migrations

### Epic 2 (Remote Execution) Gaps
- [x] Implement exponential backoff for timed-out command retries
- [x] Add aggregation metrics showing success/failure ratios in batch execution
- [x] Document command output retention policies and archival options
- [x] Add support for command pipelining (output as input to another)
- [x] Implement dry-run mode for all execution commands
- [x] Document Windows PowerShell encoding edge cases

### Epic 3 (State Management) Gaps
- [x] Implement `--preview` mode to show rendered state before apply
- [x] Auto-generate module reference documentation from code annotations
- [x] Add conditional state blocks (`when: facts.os == 'Ubuntu'`)
- [x] Implement atomic transactions with automatic rollback on partial failures
- [x] Create state dependency graph visualization tool
- [x] Add state validation before apply with detailed error messages
- [x] Document performance characteristics for large state files (1000+ resources)

### Epic 4 (Event System) Gaps
- [x] Implement event replay capability with time range filtering
- [x] Add dead letter queue for failed reactor executions with alerting
- [x] Enforce event schema validation at ingress with detailed error messages
- [x] Add event-to-audit-log integration for security-relevant events
- [x] Implement event lifecycle tracking (created, routed, processed, archived)
- [x] Document event ordering semantics across NATS cluster
- [x] Document retention sizing recommendations based on deployment scale

### Epic 5 (GitOps Integration) Gaps
- [x] Implement automated webhook secret rotation with versioning
- [x] Add comprehensive rollback audit trail with reasons and timestamps
- [x] Create approval workflow integration guide for Slack/PagerDuty/Teams
- [x] Implement conflict resolution strategy documentation for Git sync
- [x] Add support for multi-repo deployment orchestration
- [x] Make canary/blue-green thresholds configurable per deployment
- [x] Implement automatic remediation on verification failure

### Epic 6 (Policy Enforcement) Gaps
- [x] Add built-in policy testing framework with unit test support
- [x] Implement policy migration tooling for versioning transitions
- [x] Add custom policy type registry with plugin architecture
- [x] Generate compliance reports with resource-level audit trails
- [x] Implement policy conflict resolution with clear precedence rules
- [x] Integrate RBAC context into policy evaluation engine
- [x] Add policy performance profiling and benchmarking
- [x] Create policy library with common use cases (CIS benchmarks, SOC2)

### Epic 7 (Observability) Gaps
- [x] Implement custom metrics registration framework for users
- [x] Add cardinality limiting middleware with configuration
- [x] Implement configurable trace sampling with adaptive sampling based on error rates
- [x] Provide default alert rules for common scenarios (high CPU, memory, error rates)
- [x] Document metrics retention sizing based on scrape interval and cardinality
- [x] Add metrics dashboards for each major epic
- [x] Implement distributed tracing correlation IDs across all request types

### Epic 8 (Multi-Environment) Gaps
- [ ] Expand GCP/Azure integrations to match AWS coverage
- [ ] Implement automated bare metal discovery with profile matching
- [x] Document edge cache invalidation policies and TTL configuration
- [ ] Complete service mesh integration with mTLS policy verification
- [ ] Add CI/CD jobs for real K8s cluster testing (EKS, GKE, AKS)
- [ ] Implement K8s NetworkPolicy integration for network enforcement
- [ ] Add comprehensive container registry authentication support

### Epic 9 (Plugin System) Gaps
- [x] Expand stdlib with yaml, xml, database, and http2 modules
- [ ] Implement Starlark debugger with breakpoints and step execution
- [x] Auto-generate module documentation from code annotations
- [x] Version lock file format with migration tooling
- [x] Create standard module testing framework
- [x] Optimize WASM module size with build time flags
- [x] Implement configurable resource limits (CPU time, memory) for modules
- [x] Add module dependency audit tooling to detect breaking changes

---

## Medium-Term Priority (2-3 Releases)

### Epic 11 (HA Clustering) Gaps
- [x] Document zero-downtime etcd upgrade procedure
- [x] Implement disaster recovery playbook for split-brain scenarios
- [ ] Integrate etcd backup/restore with cluster backup system
- [x] Publish latency benchmarks for leader election and agent handoff
- [x] Document scaling guidelines for clusters >5 nodes
- [ ] Characterize rebalance performance impact with metrics
- [x] Provide default alert rules for cluster health monitoring
- [x] Add network partition simulation tests

### Epic 12 (E2E Testing) Gaps
- [x] Replace time.Sleep with condition-based waits and timeout helpers
- [ ] Implement performance baseline tracking with alerting for regressions
- [ ] Add multi-cloud CI/CD pipelines for real cluster testing
- [ ] Implement chaos engineering tests (partition, latency injection, failures)
- [ ] Add comprehensive security testing covering auth/authz/audit scenarios

### Epic 14 (NATS Mesh) Gaps
- [x] Document subject namespace with examples
- [ ] Characterize supercluster failover and recovery times
- [x] Implement and document message ordering guarantees
- [x] Implement publisher backpressure with flow control
- [x] Curate NATS metrics dashboards for common scenarios
- [x] Implement configuration validation with clear error messages
- [x] Document WebSocket security considerations and deployment patterns
- [x] Create NATS performance tuning guide with benchmarks
- [x] Implement NATS circuit breakers for cascading failure prevention

### Epic 15 (Observability Enhancements) Gaps
- [x] Complete audit logging coverage across all CLI commands
- [x] Standardize syslog field mapping with common SIEM formats
- [x] Implement correlation ID propagation across all systems
- [x] Create comprehensive Windows Event Log documentation
- [x] Add built-in Loki pusher with batching and backoff
- [x] Link traces to logs via correlation IDs
- [x] Version all observability schemas with migration tooling

### Epic 16 (Stdlib Modules) Gaps
- [ ] Increase unit test coverage to >80% for all modules
- [ ] Add comprehensive cross-platform testing (Windows, macOS)
- [x] Add real-world examples to module documentation
- [x] Improve error messages with actionable suggestions
- [x] Implement dry-run/preview mode for all write operations
- [x] Create state application performance tuning guide
- [x] Document custom module development with templates
- [x] Implement module versioning with deprecation support

### Epic 17 (SPIFFE Identity) Gaps
- [x] Implement automatic CA rotation scheduling with alerting
- [ ] Simplify trust federation setup with interactive wizard
- [ ] Add automatic fallback between attestation methods
- [ ] Expand cloud provider attestation testing to real environments
- [x] Implement certificate pinning for service-to-service communication
- [x] Add identity audit query API with compliance reporting

### Epic 18 (IPv6) Gaps
- [x] Add IPv6 deployment documentation
- [ ] Enable IPv6 testing in all CI/CD jobs
- [ ] Implement cloud provider IPv6 metadata detection
- [x] Document IPv6 firewall configuration for common platforms
- [x] Implement automatic IPv4/IPv6 failover
- [x] Create IPv6 troubleshooting guide

### Epic 19 (Observability Gateway) Gaps
- [x] Add cardinality limiting with automatic label reduction
- [x] Implement intelligent sampling (adaptive, based on error rates)
- [x] Add backend failover with queuing
- [x] Simplify Docker Compose deployment
- [x] Create Kubernetes Helm chart for gateway
- [x] Document retention policy sizing
- [x] Implement gateway configuration validation

### Epic 20 (Windows) Gaps
- [ ] Add comprehensive Windows CI/CD testing matrix
- [x] Expand Windows deployment documentation
- [x] Document PowerShell version requirements and compatibility
- [x] Create UAC handling guide for common scenarios
- [x] Document antivirus exclusions and integration patterns
- [x] Create Group Policy deployment guide
- [x] Add registry state module examples
- [x] Create Windows troubleshooting guide

### Epic 21 (Proxy Agents) Gaps
- [ ] Add support for additional protocols (Telnet, NETCONF, RESTCONF)
- [ ] Expand vendor support (HP, Dell, Checkpoint, etc.)
- [ ] Add real network device testing to CI/CD
- [ ] Benchmark large numbers of proxy agents
- [ ] Implement automatic credential rotation for all backends
- [x] Expand vendor-specific documentation
- [x] Add detailed protocol debugging/logging
- [x] Document vendor API compatibility matrix

### Epic 22 (File Distribution) Gaps
- [x] Implement automatic compression with configurable algorithms
- [x] Add bandwidth throttling for downloads
- [ ] Comprehensive mirror consistency testing
- [ ] Tighter integration with policy engine for access control
- [x] Implement conflict resolution strategies (last-write-wins, version-based)
- [x] Add storage backend failover with queuing
- [x] Implement encryption at rest for all backends

### Epic 23 (Self-Management) Gaps
- [ ] Expand configuration validation with comprehensive test suite
- [ ] Test automatic rollback on upgrade failure extensively
- [x] Document backup storage options with encryption recommendations
- [x] Implement automatic DR drill scheduling and reporting
- [x] Expand runbook templates with additional operational scenarios
- [x] Create comprehensive version compatibility matrix
- [ ] Characterize upgrade downtime by deployment scale

### Epic 25 (Blueprints) Gaps
- [ ] Complete runtime implementation with full feature support
- [ ] Build comprehensive blueprint testing framework
- [x] Create 10+ real-world blueprint examples
- [x] Improve parameter validation error messages
- [ ] Expand rollback mechanism with comprehensive testing
- [x] Complete blueprint authoring documentation
- [ ] Finalize registry design and implementation
- [ ] Create blueprint marketplace and discovery mechanism

### Epic 27 (Bootstrap) Gaps
- [ ] Implement comprehensive error recovery with detailed diagnostics
- [x] Complete bootstrap documentation for all scenarios
- [ ] Add Windows and macOS bootstrap support
- [ ] Implement atomic bootstrap with automatic rollback
- [x] Optimize bootstrap performance and add progress reporting

### Epic 28 (Standard Blueprints) Gaps
- [ ] Expand real-world testing on production-like environments
- [x] Review and refine parameter defaults based on feedback
- [x] Complete blueprint documentation with examples
- [ ] Create Windows and macOS deployment blueprints
- [ ] Implement automatic blueprint update mechanism

### Epic 29 (Bootstrap Testing) Gaps
- [ ] Implement VM-based testing infrastructure (Epic 100)
- [ ] Comprehensive failure scenario testing
- [ ] Establish and track bootstrap performance baselines
- [ ] Add Windows and macOS bootstrap testing
- [ ] Comprehensive recovery and rollback testing
- [ ] Test bootstrap at scale (many agents simultaneously)
- [ ] Add chaos testing for bootstrap scenarios

---

## Long-Term Priority (Future Versions)

### Major Features (from Roadmap)
- [ ] Web UI dashboard
- [ ] Mobile monitoring app
- [ ] Natural language interface
- [ ] Scheduled operations & maintenance windows
- [ ] Multi-tenancy and namespace isolation
- [ ] Agent self-update with staged rollouts

### Infrastructure
- [ ] Create automated translation pipeline for additional languages
- [x] Implement full-text search across documentation
- [x] Add API version history and deprecation notices

---

## Notes

- Test coverage targets: >70% for critical packages, >40% for CLI
- Performance benchmarks should be tracked in CI/CD with regression alerting
- All new features should include comprehensive documentation and tests
- Security considerations should be reviewed for all changes
