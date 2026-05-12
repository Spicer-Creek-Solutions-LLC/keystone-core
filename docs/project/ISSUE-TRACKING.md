# Issue Tracking Conventions

How work is organised in the Forgejo issue tracker (`http://192.168.10.21:3000/sbutts/keystone-core`,
mirrored to Codeberg/GitHub). This document is the convention; the planning documents it references
remain the content source of truth.

> **Tooling note**: the `fj` CLI (forgejo-cli) can create/edit/search/close issues and manage labels,
> but has **no** milestone or project commands — milestones and the roadmap board are managed via the
> web UI or the REST API. `fj` against this instance needs `-H http://192.168.10.21:3000` and
> `-r sbutts/keystone-core` (a wrapper function in the maintainer's shell injects the host).

---

## 1. Principles

- **Documents own content; tickets own workflow state.** A ticket carries open/closed, assignee,
  order, and links — not the authoritative description. The authoritative description lives in the
  planning document the ticket was minted from (see §4). Tickets backlink to it; they do not
  duplicate prose that would then drift.
- **Just-in-time decomposition.** Only the active release and the next release are exploded into
  leaf tickets. Everything beyond that exists as epic tracker issues (§3) or as entries in a
  planning document — not as tickets. Leaf tickets for work two-plus releases out are noise: they
  get rewritten before anyone touches them and they pollute search.
- **Order is explicit.** Forgejo has no priority/rank field, and issue-number order is brittle.
  Execution order lives in per-release tracker issues (§5). "Do the next ticket for vX.Y" resolves
  to the first unchecked box in that release's tracker.
- **Close-out keeps three artifacts in sync.** Code (PR), the planning document, and the ticket are
  all updated in one ritual (§6).

---

## 2. The tiers

```
version milestone  (v1.1, v1.2, …)
  └── epic tracker issue        kind/epic — ordered task list of its children, links to epics/NN-*.md
        └── leaf ticket         one actionable unit of work
  └── standalone leaf ticket    small items with no epic (most v1.x backlog entries)
release tracker issue           kind/release-tracker — ordered list of the epics + standalone tickets for one version
```

Forgejo has no native epic type; the "epic" and "release tracker" tiers are ordinary issues whose
**body is a Markdown task list** referencing child issue numbers (`- [ ] #42 …`). Forgejo renders
those as live links with a progress bar. Sub-issue / dependency links in the web UI are a welcome
nicety but the task-list body is what the CLI can read, so it is authoritative for order.

---

## 3. When to create an epic tracker issue vs. leaf tickets

- A roadmap **theme** or **feature** that is more than one actionable unit of work → an epic tracker
  issue (`kind/epic`), body = ordered task list, linked to its `epics/NN-*.md` document once that
  exists. Explode it into leaf tickets only when its release becomes active or next.
- A small, self-contained unit (most `V1X-BACKLOG.md` entries) → a standalone leaf ticket, no epic.
- Far-future work (two-plus releases out) → leave it as a `kind/epic` placeholder under the version
  milestone with no leaf tickets, or just as a planning-document entry. Do not decompose it.

---

## 4. Labels

Labels carry the axes that `fj issue search` can filter on (`-l/--labels`, `-s/--state`). Create
them once with `fj repo labels create`.

### `source/*` — which planning document owns this work (drives the close-out ritual, §6)

| Label | Origin document | On completion |
| --- | --- | --- |
| `source/v1x-backlog` | `docs/project/V1X-BACKLOG.md` | move the entry to a `### Done` block under its version |
| `source/features` | `FEATURES.md` | update the feature's `(landed: …)` annotation |
| `source/roadmap` | `PROJECT-DETAILS.md §6.2` or a roadmap doc | close; no doc edit beyond roadmap status |
| `source/epic-task` | `epics/NN-*.md` | tick the task's acceptance criteria in the epic file (existing `AGENTS.md` workflow) |
| `source/triage` | none — a bug or idea filed directly | close; add a regression test if it was a bug |

### `kind/*` — the shape of the issue

`kind/epic`, `kind/release-tracker`, `kind/feature`, `kind/bug`, `kind/chore`, `kind/docs`.

### `area/*` — the part of the system

`area/statemgmt`, `area/agent`, `area/nats`, `area/server`, `area/schema`, `area/bootstrap`,
`area/cli`, `area/security`, `area/policy`, `area/observability`, … (add as needed; keep the set
small).

### `v1x-backlog`

Umbrella label on every issue minted from `V1X-BACKLOG.md`, so they are distinguishable from
regular tracker traffic. (Equivalent umbrella labels may be added for other bulk imports.)

### `v1.0-narrowing`

Marks an issue that came from the *Implementation-time narrowings* section of `V1X-BACKLOG.md` —
i.e. somewhere v1.0 shipped with deliberately reduced scope. It stays on the issue after the gap is
scheduled (it's a useful "what's new since v1.0" filter for release notes), so a `v1.0-narrowing`
issue normally also has a `kind/feature` and a milestone. The unscheduled ones (none at present)
carry `kind/chore` and no milestone instead; their target lives in the `### Targeted: vX.Y` headings
in `V1X-BACKLOG.md` and is pushed onto the issues by `trackerctl reconcile-issues`.

### Version

Version is **not** a label and **not** part of the title — it is the **milestone**. Re-slotting work
between releases is a milestone change, nothing else. (A version label may be added only if CLI
filtering by version becomes necessary; the milestone is canonical.)

---

## 5. Milestones, tracker issues, and the roadmap board

- **One milestone per target version** (`v1.1`, `v1.2`, … `v2.0`). Defined in
  `tools/trackerctl/config/milestones.yaml` and applied by `trackerctl sync-milestones`. The
  milestone gives the progress bar and an optional due date; every leaf ticket and epic for that
  release is assigned to it.
- **One release tracker issue per version** — title `vX.Y — release tracker`, label
  `kind/release-tracker`, assigned to that milestone. Its body is an ordered checklist of the
  release's leaf issues, **generated by `trackerctl gen-tracker --version vX.Y`** from
  `tools/trackerctl/config/release-order.yaml` (the canonical execution order — entries listed there
  first, anything unlisted appended in `V1X-BACKLOG.md` file order). Example:

  ```markdown
  Execution order for v1.1. "Next item" = first unchecked box.

  - [x] #9  Schema versioning via `golang-migrate`
  - [ ] #10 Reactor engine + event lifecycle tracking      ← next
  - [ ] #20 `state.apply.skip` event taxonomy + wiring
  - [ ] #19 `link` stdlib module — relative-target normalisation
  ```

  **Reprioritising = editing `release-order.yaml` and re-running `gen-tracker`** (it preserves ticked
  boxes; the rest of the body is regenerated, so don't hand-edit it or keep notes there). Order
  entries so every `Blocked by` target precedes its dependents. "Do the next vX.Y item" = first
  unchecked box.
- **One Forgejo Project ("Roadmap")** — board columns `Backlog / Next release / In progress / In
  review / Done` — as a human-facing dashboard across milestones. Maintained via the web UI; not
  driven by `fj`. The milestone + labels + tracker issues remain the machine-readable layer.

---

## 6. Ticket lifecycle

**Creating** a leaf ticket:

1. Title = the feature/work name, no version prefix. For bulk imports, a stable greppable prefix is
   fine (e.g. backlog entries keep their `####` heading text).
2. Body = the originating document section copied verbatim (What / Why / Acceptance / References),
   plus: a backlink to the document heading anchor, the epic/task it was deferred from (if any), and
   `Blocked by #N` lines for each dependency.
3. Apply labels: one `source/*`, one `kind/*`, one or more `area/*`, plus `v1x-backlog` (or the
   relevant umbrella). `fj issue create` cannot set labels at creation time — create, then
   `fj issue edit <n> labels`.
4. Assign the milestone (web UI / API).
5. Slot it into the execution order: add its title to `tools/trackerctl/config/release-order.yaml`
   under the right version and re-run `trackerctl gen-tracker --version vX.Y`; add it to the epic
   tracker's list too if it has a parent epic. (Most v1.x leaf issues, and the v1.1 tracker itself,
   are created in bulk by `trackerctl gen-issues` / `gen-tracker` rather than one at a time — this
   step is for issues added later.)

**Picking up "the next vX.Y item"**:

1. Open `vX.Y — release tracker`; take the first unchecked box, say `#15`.
2. Open `#15`; read Acceptance + References.
3. Check `Blocked by` — if any referenced issue is still open, **stop and report**; do not proceed
   out of order.
4. If the work maps to an `epics/NN-*.md` task, follow the `AGENTS.md` epic-task workflow (present a
   plan, get explicit approval) before writing code.
5. Implement.

**Closing out**:

1. PR with `Closes #15` in the description (auto-closes the ticket on merge).
2. Check the box in the release tracker (and the epic tracker, if any).
3. Update the originating document per the `source/*` row in §4 — e.g. move a `V1X-BACKLOG.md` entry
   to that version's `### Done` block with a "landed in PR #x" note.

Three artifacts — code, planning document, ticket — one ritual. If you find them out of sync, the
planning document wins on content and the ticket wins on state; reconcile toward those.

---

## 7. First adoption

Do not mint every backlog/feature/roadmap item at once. The full set of roadmap milestones
(v1.1…v2.0, per `PROJECT-DETAILS.md §6.2`) exists in `tools/trackerctl/config/milestones.yaml` so
they are visible and assignable — but creating *tickets* is gated separately: `trackerctl gen-issues
--versions v1.1` only decomposes v1.1. Bring up v1.1 first: run `trackerctl sync` (labels +
milestones), then `trackerctl gen-issues --versions v1.1` to mint the ~21 leaf tickets under
`## v1.1`, then `trackerctl gen-tracker --version v1.1` to build the `v1.1 — release tracker`.
Decompose v1.2+ only when each becomes the next release (`gen-issues --versions v1.2` then
`gen-tracker --version v1.2`, after adding a `v1.2:` block to `release-order.yaml`). The
*Implementation-time narrowings* section of `V1X-BACKLOG.md` has its own one-time bootstrap —
`gen-issues --versions v1.0-narrowing` to create those tickets — and is triaged by editing the
`### Targeted: vX.Y` headings in that section and running `reconcile-issues --apply`, which pushes
each one's milestone and `kind/*` onto the live issue (and a narrowing targeted at vX.Y then shows
up in that release's tracker, since `gen-tracker --version vX.Y` picks it up too).

---

## 8. Tooling and the test → production cutover

The label set, milestones, and the v1.x leaf issues are not hand-clicked — they are generated by
`tools/trackerctl` from configuration in the repo:

- `tools/trackerctl/config/labels.yaml` — the label set (`trackerctl sync-labels`).
- `tools/trackerctl/config/milestones.yaml` — every roadmap milestone v1.1…v2.0 (`trackerctl
  sync-milestones`).
- `docs/project/V1X-BACKLOG.md` — the leaf-issue source. `trackerctl gen-issues --versions vX.Y`
  creates them (one release at a time); `trackerctl reconcile-issues` pushes any later edits
  (re-slotting, label fixes) onto the already-created issues — it updates milestone + `source/*` /
  `kind/*` / umbrella labels and never touches `area/*`.
- `tools/trackerctl/config/release-order.yaml` — execution order per release (`trackerctl
  gen-tracker --version vX.Y`, which creates/updates the `vX.Y — release tracker` issue).

All operations are idempotent and host-parameterized (`--host`, `--repo`, `--apply`,
`FORGEJO_TOKEN`); `gen-issues` and `reconcile-issues` take `--versions` to scope to release(s),
`gen-tracker` takes `--version` for the tracker issue. See `tools/trackerctl/README.md`.

**Cutover model: (b) clean regeneration.** When the project is announced, the production tracker is
built by re-running `trackerctl` against the production Forgejo — not by migrating the internal test
repo. Consequences:

- The internal server is a rehearsal; it does not need to be kept pristine.
- Issue numbers differ between servers. Nothing that must survive the cutover may hard-code `#N`.
  Canonical execution order therefore lives in `config/release-order.yaml` (regenerable into the
  tracker issue by `gen-tracker`), not in scattered issue bodies.
- Anything created directly on the test server that should exist on prod must be reflected back into
  the config files / backlog doc, or it will not be regenerated.
- The Codeberg/GitHub mirrors carry git only — no labels, milestones, or issues — so the cutover
  targets exactly one canonical Forgejo instance.
