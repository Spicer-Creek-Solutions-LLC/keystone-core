---
title: "Best Practices"
weight: 15
description: >
  Recommended operational practices for reliable, secure Keystone Core deployments
---

## Overview

These practices help keep Keystone Core deployments reliable, secure, and easy to operate as you scale.

## Deployment Defaults

- **Start embedded, grow external**: Use embedded NATS + SQLite for dev and small clusters; move to external NATS + PostgreSQL for production.
- **Separate control plane and data**: Run `kscore-server` on stable hosts; keep databases and NATS on dedicated nodes in production.
- **Pin versions**: Use explicit versions for server, agents, and modules to avoid drift.

## Reliability

- **Back up state**: Snapshot SQLite or back up PostgreSQL regularly; store backups off-host.
- **Test restores**: Validate recovery procedures in a staging environment.
- **Prefer rolling updates**: Update agents in batches to reduce blast radius.

## Security

- **Use SPIFFE identities**: Prefer embedded SPIFFE or SPIRE over static credentials.
- **Enable audit logging**: Forward audit logs to a central store for review.
- **Treat parameters as secrets**: Mark sensitive parameters, use secret backends, and avoid logging.
- **Minimize capabilities**: Grant modules only required capabilities.

## Operations

- **Use dry-run first**: Validate state changes and module behavior before apply.
- **Keep blueprints small**: Compose larger systems from reusable blueprints.
- **Document runbooks**: Capture operational steps for migrations, rollback, and incident response.

## Performance

- **Monitor NATS and DB**: Track message latency, JetStream health, and DB connection saturation.
- **Right-size agents**: Use lightweight agents on edge devices; tune concurrency for bulk actions.
