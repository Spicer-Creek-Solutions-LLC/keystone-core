# trackerctl

Provisions the keystone-core Forgejo issue tracker from configuration checked
into this directory. It is idempotent and host-parameterized, so the same
invocations that set up the internal test server are what set up the production
server at announcement time.

Cutover model: **(b) clean regeneration** — production is rebuilt from this
config and from `docs/project/V1X-BACKLOG.md`, not migrated from the test
repo. Issue numbers therefore differ between servers; nothing should hard-code
`#N` cross-references that need to survive the cutover. The canonical execution
order lives in `config/release-order.yaml` (mirrored into each release's tracker
issue by `gen-tracker`) — see `docs/project/ISSUE-TRACKING.md`.

## What it manages

| Command | Source of truth | Action |
| --- | --- | --- |
| `sync-labels` | `config/labels.yaml` | create/update the label set (never deletes; reports extras) |
| `sync-milestones` | `config/milestones.yaml` | create/update milestones |
| `sync` | both of the above | `sync-labels` then `sync-milestones` |
| `gen-issues` | `docs/project/V1X-BACKLOG.md` | create one leaf issue per `####` entry not already present, labelled + assigned to its version milestone |
| `gen-tracker` | `config/release-order.yaml` (+ existing issues) | create/update the `vX.Y — release tracker` issue: an ordered checklist of that release's leaf issues; `--version` required |

Issue creation (`gen-issues`) is intentionally separate from `sync`: labels and
milestones are cheap to converge, issues are not. `config/milestones.yaml`
carries every roadmap version (v1.1…v2.0) as a milestone, but you almost always
want to create *tickets* one release at a time — use `gen-issues --versions
v1.1`. Without `--versions`, `gen-issues` would create every backlog entry whose
milestone exists, which is now all of them.

`gen-tracker` orders the release's leaf issues by `config/release-order.yaml`
(falling back to `V1X-BACKLOG.md` file order for any version or entry not listed
there), and on re-run **preserves ticked checkboxes** — the rest of the tracker
body is regenerated, so reorder via `release-order.yaml`, not by hand-editing
the issue, and don't keep notes in the tracker body. Run it after `gen-issues`
for the same release.

## Usage

```sh
export FORGEJO_TOKEN=<application token with repo scope>

# dry-run (default): prints a plan, changes nothing
go run ./tools/trackerctl --host http://192.168.10.21:3000 sync
go run ./tools/trackerctl --host http://192.168.10.21:3000 gen-issues --versions v1.1
go run ./tools/trackerctl --host http://192.168.10.21:3000 gen-tracker --version v1.1

# apply (one release at a time)
go run ./tools/trackerctl --host http://192.168.10.21:3000 --apply sync
go run ./tools/trackerctl --host http://192.168.10.21:3000 --apply gen-issues --versions v1.1
go run ./tools/trackerctl --host http://192.168.10.21:3000 --apply gen-issues --versions v1.1,v1.0-narrowing
go run ./tools/trackerctl --host http://192.168.10.21:3000 --apply gen-tracker --version v1.1
```

Flags: `--host` (required), `--repo` (default `sbutts/keystone-core`),
`--apply` (default off), `--backlog` (default `docs/project/V1X-BACKLOG.md`),
`--versions` (gen-issues: comma-separated version tags to limit creation to,
e.g. `v1.1` or `v1.1,v1.0-narrowing`; empty = every entry whose milestone
exists), `--version` (gen-tracker: the single release whose tracker issue to
create/update, e.g. `v1.1` — required). `FORGEJO_TOKEN` must be set. Run from
the repo root so the default `--backlog` path resolves.

> The local test instance is plain HTTP on port 3000; pass the `http://` URL
> explicitly. The `fj` CLI has the same requirement (see the maintainer's shell
> wrapper) — `trackerctl` does not use `fj`, it calls the Forgejo REST API
> directly.

## Notes / limitations

- **Labels are never deleted** — a label present on the server but absent from
  `config/labels.yaml` is reported and left alone. Remove it by hand if you mean
  to.
- **`area/*` inference is heuristic** — `gen-issues` attaches an `area/*` label
  only on a confident keyword match (see `areaKeywords` in `issues.go`);
  otherwise it leaves area off for a human to add. `kind/*` defaults to
  `kind/feature` for versioned entries and `kind/chore` for the v1.0-narrowings
  section. Refine after creation via the web UI or `fj issue edit`.
- **Milestone assignment** — versioned entries are assigned to the milestone
  matching their `## vX.Y` heading; if it doesn't exist yet the issue is skipped
  with a warning (run `sync-milestones` first). Narrowings get no milestone (they
  carry `v1.0-narrowing` instead, pending triage into a real version).
- **`gen-tracker` body is machine-managed** — it regenerates the whole tracker
  body except the checkbox states (which it carries over by matching `#N`). Keep
  discussion in issue comments, not the body. It omits, with a warning, any
  release-order entry that has no corresponding issue yet — run `gen-issues` for
  that release first.
- **No project/board management** — the "Roadmap" Forgejo Project is maintained
  in the web UI; `trackerctl` only touches labels, milestones, and issues.
- **Not a release artifact** — this lives under `tools/`, outside `cmd/`, so it
  is not built by `make build` or shipped by goreleaser.
