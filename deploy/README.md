# Deploy

Operator-facing deployment assets bundled with the Keystone Core
binaries.

## Subdirectories

- [`systemd/`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/tag/archive-2026-09-pre-v0.6-reboot/deploy/systemd) — unit files for `kscore-server.service`
  and `kscore-agent.service`. Installed automatically by the
  `kscore-server` / `kscore-agent` `.deb` / `.rpm` packages to
  `/lib/systemd/system/`.
  - The **server** unit is hardened
    (`NoNewPrivileges=yes`, `ProtectSystem=strict`,
    `RestrictNamespaces=yes`, `MemoryDenyWriteExecute=yes`).
  - The **agent** unit is intentionally less restricted because
    it executes operator-issued commands by design.
- [`grafana/`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/tag/archive-2026-09-pre-v0.6-reboot/deploy/grafana) — Grafana dashboards for the Prometheus
  metrics emitted by `kscore-server` + `kscore-agent`. See
  [`grafana/README.md`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/tag/archive-2026-09-pre-v0.6-reboot/deploy/grafana/README.md) for the dashboard
  inventory and `expected_metrics.txt` for the metric contract.
