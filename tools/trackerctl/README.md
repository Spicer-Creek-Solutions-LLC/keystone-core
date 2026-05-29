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
| `reconcile-issues` | `docs/project/ROADMAP.md` | update *existing* issues' milestone and managed labels (`source/*`, `kind/*`, `roadmap-backlog`) to match the backlog; never creates, never touches `area/*` |
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

> **Legacy label note.** The umbrella label was renamed `v1x-backlog` →
> `roadmap-backlog` (the version-pinned name predated the v0.x rename). The
> paired source label `source/v1x-backlog` still carries the legacy name;
> renaming it is tracked as a v0.x ROADMAP entry.

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

> **Flag ordering matters.** Every flag must come *before* the subcommand
> (`--versions gate-v0.5 gen-issues`, never `gen-issues --versions gate-v0.5`).
> The tool uses Go's `flag` package, which stops parsing at the first
> positional argument, so a flag placed after the subcommand is silently
> ignored — a misplaced `--versions` makes `gen-issues` fall back to *every*
> backlog entry across all buckets.

```sh
export FORGEJO_TOKEN=<application token with repo scope>
export FORGE_URL=https://codeberg.org      # or your self-hosted Forgejo URL

# dry-run (default): prints a plan, changes nothing
go run ./tools/trackerctl --host "$FORGE_URL" sync
go run ./tools/trackerctl --host "$FORGE_URL" --versions gate-v0.5 gen-issues
go run ./tools/trackerctl --host "$FORGE_URL" reconcile-issues
go run ./tools/trackerctl --host "$FORGE_URL" --version gate-v0.5 gen-tracker

# apply (one bucket at a time)
go run ./tools/trackerctl --host "$FORGE_URL" --apply sync
go run ./tools/trackerctl --host "$FORGE_URL" --apply --versions gate-v0.5 gen-issues
go run ./tools/trackerctl --host "$FORGE_URL" --apply --versions gate-v0.5,gate-v1.0 gen-issues
go run ./tools/trackerctl --host "$FORGE_URL" --apply reconcile-issues   # after editing ROADMAP.md
go run ./tools/trackerctl --host "$FORGE_URL" --apply --version gate-v0.5 gen-tracker

# bulk create against a windowed-rate-limited host (e.g. Codeberg): self-pace
# through the tiered limits in one invocation (see "rate limiting" below)
go run ./tools/trackerctl --host "$FORGE_URL" --apply --max-wait 45m --versions gate-v0.5 gen-issues
```

Flags: `--host` (required), `--repo` (default `Spicer-Creek-Solutions-LLC/keystone-core`),
`--apply` (default off), `--backlog` (default `docs/project/ROADMAP.md`),
`--versions` (gen-issues / reconcile-issues: comma-separated priority buckets
to limit to, e.g. `gate-v0.5` or `gate-v0.5,gate-v1.0`; empty = all entries),
`--version` (gen-tracker: the single priority bucket whose tracker issue to
create/update, e.g. `gate-v0.5` — required), `--throttle` (duration; pause
before each create/update request — see "rate limiting" below; default 0),
`--max-wait` (duration; per-request budget for waiting out a server-stated
rate-limit window, see "rate limiting" below; default 0 = fail fast).

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
- **Rate limiting.** Transient failures (`502/503/504`, network drops) and a
  `429 Too Many Requests` that carries a `Retry-After` header are retried
  automatically, up to 5 attempts with exponential backoff + jitter (capped at
  60s); each prints a one-line notice to stderr. **Windowed** rate limits are
  different: Codeberg caps issue/PR creation in overlapping tiers (5 per 5 min,
  7 per 10 min, 11 per 30 min) and returns a `429` whose *body* states the
  window (`posted N issues in under M minutes`) with **no** `Retry-After`. For
  those, pass `--max-wait <duration>` (e.g. `--max-wait 45m`): the tool parses
  the window from the body, sleeps it out, and retries, so one `gen-issues
  --apply --max-wait 45m` self-paces through every tier instead of giving up
  after ~5 creates. `--max-wait` is a per-request budget (default 0 = fail
  fast); set it larger than the biggest tier you expect (30 min on Codeberg).
  `--throttle` still paces individual writes but does **not** help a windowed
  limit (it is a per-request pause, not a per-window rate). GET/list calls are
  never throttled or counted. Everything is idempotent, so an interrupted bulk
  run can just be re-run.
- **No project/board management** — the "Roadmap" Forgejo Project is maintained
  in the web UI; `trackerctl` only touches labels, milestones, and issues.
- **Not a release artifact** — this lives under `tools/`, outside `cmd/`, so it
  is not built by `make build` or shipped by goreleaser.
