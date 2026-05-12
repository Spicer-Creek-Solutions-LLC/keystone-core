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

### Version

Version is **not** a label and **not** part of the title — it is the **milestone**. Re-slotting work
between releases is a milestone change, nothing else. (A version label may be added only if CLI
filtering by version becomes necessary; the milestone is canonical.)

---

## 5. Milestones, tracker issues, and the roadmap board

- **One milestone per target version** (`v1.1`, `v1.2`, … `v2.0`). Created via the web UI / REST API
  (`POST /api/v1/repos/sbutts/keystone-core/milestones`). The milestone gives the progress bar and
  an optional due date; every leaf ticket and epic for that release is assigned to it.
- **One release tracker issue per version** — title `vX.Y — release tracker`, label
  `kind/release-tracker`, assigned to that milestone. Body is an ordered task list of the epics and
  standalone tickets for the release, sorted so every `Blocked by` target appears above its
  dependents:

  ```markdown
  Execution order for v1.1. "Next item" = first unchecked box.

  - [x] #12 Schema versioning via golang-migrate
  - [ ] #15 Replay protection on agent commands        ← next
  - [ ] #21 Reactor engine + event lifecycle tracking
  - [ ] #9  link stdlib module — relative-target normalisation
  ```

  Reprioritising = moving a line. Adding work = inserting a line.
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
5. Add the issue to the right place in its release tracker's task list, and to the epic tracker's
   list if it has a parent epic.

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

Do not mint every backlog/feature/roadmap item at once. Bring up v1.1 first: create the `source/*`,
`kind/*`, and `area/*` label sets and the `v1.1` milestone; mint the v1.1 leaf tickets from
`V1X-BACKLOG.md` (the ~21 entries under `## v1.1`); build the `v1.1 — release tracker`. Add v1.2+
milestones with epic placeholders only, and decompose each into tickets when it becomes the next
release. The "v1.0 narrowings" section of `V1X-BACKLOG.md` is not version-tagged — park those under
a `v1.0-narrowing` label and triage them into real versions later.
