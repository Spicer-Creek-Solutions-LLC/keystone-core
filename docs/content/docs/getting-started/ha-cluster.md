---
title: HA Cluster Topology
weight: 6
---

A single `kscore-server` is fine for a lab or a small fleet. For
high availability you run several control-plane nodes that elect a
leader, share ownership of the agent fleet, monitor each other's health,
and fence a partitioned node from writing. The membership and
coordination layer is backed by etcd.

{{< callout type="warning" >}}
**v0.1 scope.** The cluster **configuration surface**, **single-node**
operation, leader election, health/quorum reporting
(`GET /cluster/status`), and the `kscore-cluster` operator CLI are in
place. **Automatic failover reassignment** (moving a failed node's agents
and jobs to survivors) and the hardened **multi-process** topology are
still in progress for v1.0/v1.1 — so treat multi-node as a preview: it
forms, elects, and reports health, but don't yet rely on automatic
work-reassignment on node loss. The HA failure modes are exercised by an
in-process 3-node test suite today.
{{< /callout >}}

This guide brings up a single-node cluster, inspects it, and walks the
path to multiple nodes.

## 1. Enable clustering (single node)

Clustering is opt-in. The simplest topology is one node with an embedded
etcd. Add a `cluster` block and restart `kscore-server`:

```yaml
cluster:
  enabled: true
  node:
    name: kscore-1
    advertise_addr: 127.0.0.1:7400   # host:port peers dial
  etcd:
    mode: embedded                   # embedded | external
    data_dir: ./data/etcd
    client_urls: ["http://127.0.0.1:2379"]
    peer_urls: ["http://127.0.0.1:2380"]
```

With `enabled: false` (the default) the single-node, non-clustered path
runs and no etcd is started.

## 2. Inspect the cluster

```sh
kscore-cluster status     # members, leader, health, quorum
kscore-cluster leader     # the current leader
kscore-cluster members    # member list (or one by ID)
```

On a single node, `status` shows one healthy member that is also the
leader, with quorum satisfied. The CLI talks to the server via
`--server host:port` (default `localhost:5397`) + `--api-key`.

## 3. The topology, briefly

The `cluster` block configures each layer (see the
[configuration reference](/docs/reference/configuration-reference/) for
the full schema):

- **`etcd`** — membership + coordination store, `embedded` (in-process)
  or `external` (a managed etcd cluster, via `endpoints`).
- **`election`** — leadership via an etcd session (`session_ttl_seconds`,
  `recampaign_delay`).
- **`shard`** — consistent-hash ownership of the agent fleet across
  nodes (`virtual_nodes`, `rebalance_cooldown`).
- **`health`** — peer health checks (`check_interval`, `failure_threshold`).
- **`fencing`** — what a node that loses quorum may do: `strict`,
  `read_only` (the default — refuse writes), or `graceful`.
- **`coordination`** — the mTLS-only server↔server channel
  (`listen_addr`) that carries health/leader/recovery even when NATS is
  down.

## 4. Add nodes

Members **self-register**: boot another `kscore-server` with its own
`node.name`/`advertise_addr`, pointed at the same etcd (for `external`
etcd) or joined to the embedded etcd's peers. It appears in
`kscore-cluster members` once it has registered. (`kscore-cluster add` is
a passthrough — there is no manual add; nodes join by starting up.)

Operate the cluster:

```sh
kscore-cluster transfer-leader            # hand off leadership
kscore-cluster transfer-leader --to <id>  # to a specific member
kscore-cluster rebalance                  # redistribute shard ownership
kscore-cluster remove <id>                # evict a member
```

Snapshot and restore the cluster's coordination state:

```sh
kscore-cluster backup                     # leader-initiated snapshot
kscore-cluster restore <snapshot-file>
```

## Next steps

- [GitOps Integration](../gitops/) — drive deployments into the cluster.
- The [configuration reference](/docs/reference/configuration-reference/)
  for every `cluster.*` field.
