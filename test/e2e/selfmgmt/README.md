# Self-management end-to-end suite (Epic 18 task 8)

Build-tagged (`//go:build integration`) in-process integration suite
that wires the real Epic-18 self-management binaries' cobra commands
(`kscore-bootstrap`, `kscore-backup`) and drives them against a
temp-dir-staged "cluster" — a directory of config files standing in
for the kscore-server's persistent footprint.

## Run

```sh
make test-integration            # whole integration suite
go test -tags=integration ./test/e2e/selfmgmt/
```

Excluded from the default `go test ./...` by the build tag.

## What it proves

- `kscore-bootstrap --seed dev-seed.yaml` runs the
  `selfmgmt.BootstrapManager` state machine through every phase
  (detect → configure → validate → install → blueprints → verify),
  reaches `StateVerified`, and reports 12 transitions in history
  (acceptance: produces a working single-node cluster end-to-end).
- `kscore-backup create --dest /path/foo.tar` writes a tar with one
  manifest entry per `--config` file plus an integrity-verifiable
  manifest (acceptance: writes encrypted artifact when
  `--age-recipients` is wired in).
- `kscore-backup list` enumerates `.tar` artifacts at a destination
  prefix, sorts by name, filters out non-`.tar` files and subdirs.
- `kscore-backup verify` runs the full SHA-256 + schema + manifest
  integrity check; flips a byte mid-artifact and verifies the
  failure is reported (acceptance: confirms integrity).
- `kscore-backup restore --src ... --config-out-dir ...` restores
  config files byte-equal to the original cluster (acceptance:
  restores onto a fresh cluster successfully) — asserted via
  SHA-256 comparison of every restored file vs the staged original.
- Age round-trip: the artifact bytes do NOT contain plaintext
  `manifest.json` (envelope worked); the artifact starts with the
  `age-encryption.org/v1\n` header.
- Populated-cluster guard: a stub `ClusterDetector` returning
  `IsPopulated=true` rejects the restore with
  `backup.ErrClusterPopulated` and does NOT invoke the config
  handler; passing `Force=true` proceeds and invokes the handler
  once with the expected file (acceptance: restore over populated
  cluster requires `--force`).

## Scope

The integration test simulates a "cluster" via a config-file
directory because the v1.0 line ships only the `config` component
adapter. Real per-component adapters for storage / JetStream / etcd
/ secrets / cluster defer per the `gate-v1.0` ROADMAP entry
"Backup + restore component adapters"; the integration suite will
extend to exercise them as they land. The "real-cluster docker-
compose convergence" form is tracked under the same ROADMAP entry's
acceptance line.

The populated-cluster scenario instantiates `bkp.RestoreManager`
directly with a stub `ClusterDetector` because the CLI does not yet
wire a real detector — production wiring lands per the same
ROADMAP entry. The CLI's `--force` flag is plumbed and threaded
through to `RestoreOptions.Force` for the same reason: the contract
is testable now; the detector wiring follows.
