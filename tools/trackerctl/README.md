# trackerctl

Provisions the keystone-core Forgejo issue tracker from configuration checked
into this directory. It is idempotent and host-parameterized (`--host` /
`--repo`), so the same invocations that bootstrap one instance bootstrap any
other — the public tracker on Codeberg, or a self-hosted Forgejo used during
reconstruction.

Cutover model: **(b) clean regeneration** — a new instance is rebuilt from this
config and from `docs/project/ROADMAP.md`, not migrated from another repo.
Issue numbers therefore differ between instances; nothing should hard-code `#N`
cross-references that need to survive a cutover. The canonical execution order
lives in `config/release-order.yaml` (mirrored into each release's tracker issue
by `gen-tracker`) — see `docs/project/ISSUE-TRACKING.md`.

## What it manages

| Command | Source of truth | Action |
| --- | --- | --- |
| `sync-labels` | `config/labels.yaml` | create/update the label set (never deletes; reports extras) |
| `sync-milestones` | `config/milestones.yaml` | create/update milestones |
| `sync` | both of the above | `sync-labels` then `sync-milestones` |
| `gen-issues` | `docs/project/ROADMAP.md` | create one leaf issue per `####` entry not already present, labelled + assigned to its priority-bucket milestone |
| `reconcile-issues` | `docs/project/ROADMAP.md` | update *existing* issues' milestone and managed labels (`source/*`, `kind/*`, `v1x-backlog`, `v1.0-narrowing`) to match the backlog; never creates, never touches `area/*` |
| `gen-tracker` | `config/release-order.yaml` (+ existing issues) | create/update the `<bucket> — tracker` issue: an ordered checklist of that bucket's leaf issues; `--version <bucket>` required |

Issue creation (`gen-issues`) is intentionally separate from `sync`: labels and
milestones are cheap to converge, issues are not. `config/milestones.yaml`
carries every priority bucket (`gate-v0.5`, `gate-v1.0`, `v0.x`, `v1.x`,
`v2.x+`) as a milestone, but you almost always want to create *tickets* one
bucket at a time — use `gen-issues --versions gate-v0.5`. Without `--versions`,
`gen-issues` would create every backlog entry whose milestone exists.

`gen-issues` and `reconcile-issues` are the two halves of issue management:
`gen-issues` only ever **creates** (skips anything already present),
`reconcile-issues` only ever **updates** (milestone + the deterministic labels;
it leaves `area/*` and any other hand-added label alone). Workflow: after
editing `ROADMAP.md` — e.g. moving an entry between priority buckets — run
`reconcile-issues --apply` to push the change to the live issues. Both take
`--versions` to scope to one or more buckets.

> **Legacy label note.** The `v1x-backlog` umbrella label and the
> `v1.0-narrowing` marker label predate the v0.x rename. They're kept in
> `isManagedLabel` so old Forgejo issues that carry them reconcile cleanly,
> but new issues no longer emit `v1.0-narrowing`. Renaming `v1x-backlog` →
> `v0x-backlog` on the Forgejo side is a separate operator task tracked in
> ROADMAP.md.

`gen-tracker` orders the bucket's leaf issues by `config/release-order.yaml`
(falling back to `ROADMAP.md` file order for any bucket or entry not listed
there), and on re-run **preserves ticked checkboxes** — the rest of the tracker
body is regenerated, so reorder via `release-order.yaml`, not by hand-editing
the issue, and don't keep notes in the tracker body. Run it after `gen-issues`
for the same bucket.

## Usage

`FORGEJO_TOKEN` must hold an application token with repo scope. `FORGE_URL`
below is the instance base URL — the public tracker is `https://codeberg.org`;
a self-hosted Forgejo uses its own URL (and if it serves plain HTTP, give the
`http://` URL with the port, e.g. `http://forge.internal:3000`). `--repo`
defaults to the public Codeberg canonical (`Spicer-Creek-Solutions-LLC/keystone-core`);
pass `--repo sbutts/keystone-core` when targeting the self-hosted test server,
or any other `owner/name` when targeting somewhere else. Run from the repo
root so the default `--backlog` path resolves.

```sh
export FORGEJO_TOKEN=<application token with repo scope>
export FORGE_URL=https://codeberg.org      # or your self-hosted Forgejo URL

# dry-run (default): prints a plan, changes nothing
go run ./tools/trackerctl --host "$FORGE_URL" sync
go run ./tools/trackerctl --host "$FORGE_URL" gen-issues --versions gate-v0.5
go run ./tools/trackerctl --host "$FORGE_URL" reconcile-issues
go run ./tools/trackerctl --host "$FORGE_URL" gen-tracker --version gate-v0.5

# apply (one bucket at a time)
go run ./tools/trackerctl --host "$FORGE_URL" --apply sync
go run ./tools/trackerctl --host "$FORGE_URL" --apply gen-issues --versions gate-v0.5
go run ./tools/trackerctl --host "$FORGE_URL" --apply gen-issues --versions gate-v0.5,gate-v1.0
go run ./tools/trackerctl --host "$FORGE_URL" --apply reconcile-issues   # after editing ROADMAP.md
go run ./tools/trackerctl --host "$FORGE_URL" --apply gen-tracker --version gate-v0.5

# bulk create against a rate-limited host (e.g. Codeberg): pace the writes
go run ./tools/trackerctl --host "$FORGE_URL" --apply --throttle 300ms gen-issues --versions gate-v0.5
```

Flags: `--host` (required), `--repo` (default `Spicer-Creek-Solutions-LLC/keystone-core`),
`--apply` (default off), `--backlog` (default `docs/project/ROADMAP.md`),
`--versions` (gen-issues / reconcile-issues: comma-separated priority buckets
to limit to, e.g. `gate-v0.5` or `gate-v0.5,gate-v1.0`; empty = all entries),
`--version` (gen-tracker: the single priority bucket whose tracker issue to
create/update, e.g. `gate-v0.5` — required), `--throttle` (duration; pause
before each create/update request — see "rate limiting" below; default 0).

> `trackerctl` calls the Forgejo REST API directly — it does not shell out to
> `fj`, so it isn't affected by `fj`'s plain-HTTP-vs-HTTPS quirk; just give
> `--host` the right scheme and port for the instance.

## Notes / limitations

- **Labels are never deleted** — a label present on the server but absent from
  `config/labels.yaml` is reported and left alone. Remove it by hand if you mean
  to.
- **`area/*` inference is heuristic** — `gen-issues` attaches an `area/*` label
  only on a confident keyword match (see `areaKeywords` in `issues.go`);
  otherwise it leaves area off for a human to add. `reconcile-issues` never
  touches `area/*`, so curating those by hand (web UI or `fj issue edit`) is
  safe.
- **`kind/*` and milestones follow the backlog.** An entry under a priority
  section (`## gate-v0.5`, `## gate-v1.0`, `## v0.x`, `## v1.x`, `## v2.x+`)
  gets `kind/feature` and that bucket's milestone; an entry outside any
  priority section gets `kind/chore` and no milestone. `gen-issues` skips (with
  a warning) anything whose milestone doesn't exist yet — run
  `sync-milestones` first. To re-slot an already-created issue, move its entry
  between sections in `ROADMAP.md` and run `reconcile-issues --apply`.
- **`gen-tracker` body is machine-managed** — it regenerates the whole tracker
  body except the checkbox states (which it carries over by matching `#N`). Keep
  discussion in issue comments, not the body. It omits, with a warning, any
  release-order entry that has no corresponding issue yet — run `gen-issues` for
  that release first.
- **Rate limiting.** A `429 Too Many Requests` (or a transient `502/503/504`) is
  retried automatically — up to 5 attempts total, honouring the server's
  `Retry-After` header when present, otherwise exponential backoff with jitter
  (capped at 60s); each backoff prints a one-line notice to stderr. A request
  that never network-connects is retried the same way. On a host that throttles
  bursts of writes (Codeberg does), `--throttle 200ms`–`500ms` paces the
  create/update calls during a bulk `gen-issues` so the retries don't have to do
  all the work; GET/list calls are never throttled. Everything is idempotent, so
  a run that ultimately errors out mid-bulk can just be re-run.
- **No project/board management** — the "Roadmap" Forgejo Project is maintained
  in the web UI; `trackerctl` only touches labels, milestones, and issues.
- **Not a release artifact** — this lives under `tools/`, outside `cmd/`, so it
  is not built by `make build` or shipped by goreleaser.
