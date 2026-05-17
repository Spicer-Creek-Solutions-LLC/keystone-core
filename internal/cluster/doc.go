// Package cluster implements Keystone Core's clustering & HA layer
// (Epic 13, PROJECT-DETAILS §4.15) — the v1.0 commercial-trial
// differentiator: a 3-node kscore-server cluster with leader
// election, automatic failover, consistent-hash agent assignment,
// NATS-fallback recovery, and split-brain prevention.
//
// The whole layer is built on etcd v3 primitives (linearizable KV,
// leases, watches, Raft quorum, concurrency.Election). This file
// documents the package; the components land task-by-task per
// epics/13-clustering-ha.md.
//
// Task 1 (this commit) ships only the foundation:
//
//   - [EtcdClient] — wraps etcd v3 in embedded (single-binary,
//     in-process server via the embed package) or external
//     (connect to an existing cluster) mode, with lifecycle,
//     lease management, and thin KV/Watch/Txn primitives the rest
//     of the epic builds on. [EtcdClient.Client] exposes the
//     underlying *clientv3.Client so Task 3's LeaderElector can
//     layer concurrency.Election on top without re-dialing.
//
// Deliberately NOT in Task 1 (each has its own task):
// MembershipManager (T2), LeaderElector (T3), the consistent-hash
// ShardManager/ShardStore (T4–T6), HealthMonitor/FailoverManager
// (T7–T8), SingletonTaskManager (T9 — also where the real leader
// check replaces the AlwaysLeader seam in internal/audit and
// internal/events RetentionEnforcers), RecoveryManager (T10),
// FencingManager (T11), Coordination server/client (T12–T13),
// graceful shutdown (T14), ClusterService gRPC+REST (T15), the
// kscore-cluster CLIs (T16), and the HA E2E suite (T17–T18).
//
// Clustering is opt-in: it is disabled by default
// (config.ClusterConfig.Enabled=false) so the single-node path is
// unaffected until an operator turns it on.
package cluster
