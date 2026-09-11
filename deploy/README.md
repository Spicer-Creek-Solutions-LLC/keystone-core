# Deploy

Deployment assets for the public `keystone-core.io` sites.

- [`docs/`](docs/) — the page served at `docs.keystone-core.io`. Since reboot
  task R09 it is the reboot announcement.
- [`vanity/`](vanity/) — the Go vanity-import site at `go.keystone-core.io`,
  which makes `go.keystone-core.io/keystone-core/...` resolve to this
  repository.

## What used to be here

`systemd/` and `grafana/` held operator-facing assets bundled with the
Generation 1 binaries: unit files for `kscore-server` and `kscore-agent`, and
Grafana dashboards for the metrics they emitted. Reboot task R08 removed them
along with the rest of the Generation 1 implementation. There are no binaries to
bundle them with, and shipping deployment assets for software nobody can install
would misrepresent what this repository contains.

They are preserved, unchanged, in the archive:

- [`deploy/systemd/`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/tag/archive-2026-09-pre-v0.6-reboot/deploy/systemd)
- [`deploy/grafana/`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/tag/archive-2026-09-pre-v0.6-reboot/deploy/grafana)
