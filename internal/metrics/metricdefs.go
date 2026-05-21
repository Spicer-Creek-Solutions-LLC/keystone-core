package metrics

// v1.0 metric directory.
//
// Every metric defined by epics/17-observability.md §Tasks v1.0 metric
// set lives here. Owning packages reference these var-defs from their
// own metrics.go when registering against a Registry, so:
//
//   - There is exactly one grep target for "what metrics does kscore
//     expose?"
//   - The future CI gate that diffs the running CP's exposed names
//     against deploy/grafana/expected_metrics.txt has a stable source
//     (epic risk line 121).
//   - Histogram buckets / label names cannot drift between docs and
//     code — the var defines both at once.
//
// Add new metrics here and reference them from the owning package.

// DefAgentsTotal — gauge of currently-tracked agents, partitioned by
// cluster and last-reported status. Emitted by ConnectionManager.
var DefAgentsTotal = MetricDef{
	Name:   "kscore_agents_total",
	Help:   "Number of agents currently tracked by the control plane, by cluster and status.",
	Labels: []string{"cluster", "status"},
}

// DefCommandsExecutedTotal — counter of remote-command executions
// observed by the control plane, partitioned by terminal status and
// the executing agent. Emitted by the command dispatcher when the
// response stream completes (or errors out).
var DefCommandsExecutedTotal = MetricDef{
	Name:   "kscore_commands_executed_total",
	Help:   "Remote-command executions observed by the control plane, by terminal status and agent.",
	Labels: []string{"status", "agent"},
}

// DefCommandDurationSeconds — histogram of command-execution wall-time
// as observed by the control plane (publish → final response).
var DefCommandDurationSeconds = MetricDef{
	Name:    "kscore_command_duration_seconds",
	Help:    "Wall-time of a command execution from dispatch to final response, by command type.",
	Labels:  []string{"type"},
	Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
}

// DefStateApplyTotal — counter of state-apply operations by result.
var DefStateApplyTotal = MetricDef{
	Name:   "kscore_state_apply_total",
	Help:   "State-apply operations completed, by result (success, failed, no_change).",
	Labels: []string{"result"},
}

// DefStateDriftDetectedTotal — counter of drift detections by
// aggregate severity (max across declarations in the run).
var DefStateDriftDetectedTotal = MetricDef{
	Name:   "kscore_state_drift_detected_total",
	Help:   "Drift-detection runs that found drift, by aggregate severity.",
	Labels: []string{"severity"},
}

// DefEventsEmittedTotal — counter of events emitted onto the bus.
var DefEventsEmittedTotal = MetricDef{
	Name:   "kscore_events_emitted_total",
	Help:   "Events emitted onto the bus, by type and severity.",
	Labels: []string{"type", "severity"},
}

// DefSecretsAccessTotal — counter of secrets-broker operations, by
// backend (file/vault/kms), op (read/write/delete/encrypt/decrypt),
// and result (success/error).
var DefSecretsAccessTotal = MetricDef{
	Name:   "kscore_secrets_access_total",
	Help:   "Secrets-broker access operations, by backend, op, and result.",
	Labels: []string{"backend", "op", "result"},
}

// DefAuditEntriesTotal — counter of audit-log entries written, by
// originating policy and whether the audited action was allowed.
var DefAuditEntriesTotal = MetricDef{
	Name:   "kscore_audit_entries_total",
	Help:   "Audit-log entries written, by policy name and decision (allowed=true|false).",
	Labels: []string{"policy", "allowed"},
}

// DefClusterMembersTotal — gauge of cluster members by state. Members
// transition through HEALTHY / DEGRADED / UNHEALTHY / LEAVING; the
// gauge always reflects the most recent observation.
var DefClusterMembersTotal = MetricDef{
	Name:   "kscore_cluster_members_total",
	Help:   "Cluster members by state (healthy, degraded, unhealthy, leaving).",
	Labels: []string{"state"},
}

// DefClusterQuorum — gauge: 1 when the local node believes the cluster
// has quorum, 0 when quorum is lost.
var DefClusterQuorum = MetricDef{
	Name: "kscore_cluster_quorum",
	Help: "Cluster quorum status (1 = healthy, 0 = lost).",
}

// DefClusterFailoverTotal — counter of leader-failover attempts by
// outcome (won, lost, aborted).
var DefClusterFailoverTotal = MetricDef{
	Name:   "kscore_cluster_failover_total",
	Help:   "Cluster leader-failover attempts, by outcome.",
	Labels: []string{"outcome"},
}

// DefGRPCRequestDurationSeconds — histogram of inbound gRPC request
// duration, labelled by full method (/svc/method) and response code
// (canonical gRPC status code name, e.g. OK, NotFound).
var DefGRPCRequestDurationSeconds = MetricDef{
	Name:    "kscore_grpc_request_duration_seconds",
	Help:    "Inbound gRPC request duration in seconds, by method and gRPC status code.",
	Labels:  []string{"method", "code"},
	Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
}

// DefHTTPRequestDurationSeconds — histogram of inbound HTTP request
// duration, labelled by method (GET, POST, ...), HTTP status code, and
// route (the registered pattern, NOT the raw URL — prevents cardinality
// explosion from request parameters).
var DefHTTPRequestDurationSeconds = MetricDef{
	Name:    "kscore_http_request_duration_seconds",
	Help:    "Inbound HTTP request duration in seconds, by method, status code, and route pattern.",
	Labels:  []string{"method", "code", "route"},
	Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
}

// DefFilesCacheHitsTotal — counter of proxy-cache hits for file
// distribution Get calls. Emitted by internal/files/proxy.Cache.
// Cardinality-bounded: no per-path label (would explode); single
// counter so operators see hit ratio across the whole cache.
var DefFilesCacheHitsTotal = MetricDef{
	Name: "kscore_files_cache_hits_total",
	Help: "File-distribution proxy cache hits (a Get satisfied from the cache).",
}

// DefFilesCacheMissesTotal — counter of proxy-cache misses for file
// distribution Get calls. Combined with hits gives the hit ratio.
// Bypassed reads (FromChunk > 0) are counted under reason="bypass"
// so operators can distinguish "cold cache" from "cache deliberately
// skipped for a partial-resume read".
var DefFilesCacheMissesTotal = MetricDef{
	Name:   "kscore_files_cache_misses_total",
	Help:   "File-distribution proxy cache misses, by reason (miss, expired, bypass).",
	Labels: []string{"reason"},
}

// DefRatelimitRejectedTotal — counter of requests rejected by the
// rate-limit middleware. v1.0 emits a single reason label value
// "limit_exceeded"; the label is reserved so v1.x can add
// quota_exceeded / circuit_open / etc. without a wire break.
var DefRatelimitRejectedTotal = MetricDef{
	Name:   "kscore_ratelimit_rejected_total",
	Help:   "Requests rejected by the rate-limit middleware, by reason.",
	Labels: []string{"reason"},
}
