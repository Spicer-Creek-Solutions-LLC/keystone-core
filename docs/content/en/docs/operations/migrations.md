---
title: "Migrations"
weight: 16
description: >
  Practical guides for migrating to Keystone Core and scaling deployments
---

## Overview

This guide covers the most common migration paths.

## Salt -> Keystone Core

1. **Inventory existing states**: Identify the Salt states and pillars in use.
2. **Map to Keystone modules**: Translate Salt states to Keystone Core modules (`pkg/statemgmt` and module registry).
3. **Start with read-only checks**: Use `check` and dry-run workflows to validate parity.
4. **Phase rollout**: Migrate a subset of nodes first, then expand.

Tips:
- Keep naming consistent between Salt and Keystone for easier comparison.
- Move secrets into the Keystone secret system early to reduce leakage risk.

## SQLite -> PostgreSQL

Use `kscore-migrate` to move state data safely.

```bash
# Dry-run migration
kscore-migrate run \
  --source sqlite:///var/lib/keystone/keystone.db \
  --target postgres://kscore:pass@postgres.example.com/kscore \
  --dry-run

# Execute migration
kscore-migrate run \
  --source sqlite:///var/lib/keystone/keystone.db \
  --target postgres://kscore:pass@postgres.example.com/kscore

# Verify migration
kscore-migrate validate \
  --source sqlite:///var/lib/keystone/keystone.db \
  --target postgres://kscore:pass@postgres.example.com/kscore
```

After verification, update the control plane config to use PostgreSQL and restart the service.

## Embedded NATS -> External NATS

1. **Provision external NATS**: Deploy a cluster with JetStream enabled.
2. **Configure auth/TLS**: Align credentials and TLS trust roots.
3. **Update control plane config**: Switch `nats.mode` to `external` and set URLs.
4. **Roll agents gradually**: Update agent configs in batches to reduce downtime.

If you run leaf nodes for edge agents, keep leaf connections pointed at the external cluster.
