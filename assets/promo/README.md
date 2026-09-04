# Promo + docs clips — scripts, shot lists, and pipeline

Terminal-first and text-only: every terminal shot is real `kscorectl`
output recorded against a live topology, and the on-screen captions
carry the whole narration (there is no voiceover).

The manifest describes **reels**. A reel renders to one video. Today
that is the 30-second promo; per-feature clips for the docs pages are
the planned additions (ROADMAP, `v0.x`: *Per-feature docs clips*).

This file is the script. [`manifest.yaml`](manifest.yaml) is the same
shot lists as data — the edit decision list the renderer consumes. Keep
the two in step; `make promo-check` enforces the parts a machine can
see.

## Reels

| Reel | Output | Budget | Purpose |
| --- | --- | --- | --- |
| `promo` | `keystone-30s.mp4` (+ square cut) | 30.0s | The hook. Makes a stranger curious enough to click. |
| `operate-a-fleet` | `docs-operate-a-fleet.mp4` | 11.5s | List agents, resolve a target, query a package version fleet-wide. |
| `manage-secrets` | `docs-manage-secrets.mp4` | 17.5s | Store a credential, then render it into config without printing it. |
| `gitops-rollback` | `docs-gitops-rollback.mp4` | 13.5s | Propose a rollback, see it waiting, decide. |
| `outbound-webhooks` | `docs-outbound-webhooks.mp4` | 13.5s | Subscribe, deliver, read the delivery record. |

Docs clips carry no square cut and no end card: the page around them
supplies the context a promo has to build for itself.

**One reel per task, one tape per reel.** The first cut of these was one
clip per *command* — 4 to 11 seconds, two or three shots each — and it
was wrong twice over. A 4-second clip of a single command shows nothing
an operator can act on. And every shot is its own tape, so it is its own
terminal with its own `clear`: a multi-shot clip wiped the screen
mid-task and read as a rendering fault rather than a scene change. A
docs clip is one continuous session that accumulates the way a real
terminal does.

Durations are **measured from the rendered tape**, not guessed. The
assembler pads a short clip with its final frame but truncates a long
one, so each budget sits just above its raw length.

Show the feature being *used*, not merely invoked. The secrets clip
originally ended on `secrets get --show-cleartext`, which no operator
does for its own sake — printing a password to a terminal is the thing
the store exists to avoid. It now fetches the credential into a shell
variable at deploy time and passes it to `state apply` as a
`--variable`, so it lands in the application's env file without ever
appearing on screen or in the committed state file.

One VHS quoting note: `Type "..."` does not support `\"` escaping. Use
the backtick delimiter when the command contains double quotes.

Two clips set a smaller `FontSize` than `_common.tape`, and the tapes
say why: their output carries full UUIDs, which wrap at the default and
read as a rendering fault rather than as output. Shrinking the type
beats trimming real output or swapping to a command that says less.

Each reel carries its own duration budget, output name, and shot list,
and inherits `tolerance` / `resolution` from `defaults` unless it
overrides them. **Budgets are asserted per reel**: one reel busting its
budget is not masked by another being under.

`go run ./tools/promogen reels` lists them.

## Why it is generated rather than hand-edited

A promo video rots faster than any other artifact in a repository:
version numbers move, CLI output changes shape, and the feature it
leads with stops being the interesting one. So the pipeline splits
along what a machine can actually be trusted with.

**Hand-authored, reviewed like source**: which shots exist, in what
order, saying what. That is editorial judgement and stays in
`manifest.yaml` and the tapes.

**Generated from the branch**: the version and pre-release status on
the end card, the module/binary/distro counts, the runtime budget, and
whether a changelog entry flagged as demo-worthy actually has a shot.
No card hard-codes a number that the repository already knows.

## Script

Seven shots, 30.0s. Terminal shots run against the single-topology E2E
stack plus a bind-mount overlay (see [Scenario](#scenario)).

| # | In | Dur | Type | On-screen copy | Terminal action |
|---|------|------|----------|----------------|-----------------|
| 1 | 0:00 | 3.0s | Card | **Argo deployed it.** / **Terraform provisioned it.** / *Then what?* | — |
| 2 | 0:03 | 3.0s | Card | **Keystone Core** — the runtime operations control plane, over a condensed topology | — |
| 3 | 0:06 | 5.0s | Terminal | *Declare the state.* | `kscorectl state apply` → per-resource Check → Apply → Test outcomes |
| 4 | 0:11 | 5.5s | Terminal | *The host drifts anyway.* | `drift.sh` edits `nginx.conf` out of band; `kscorectl state drift` reports per-declaration severity + the content-hash transition |
| 5 | 0:16.5 | 6.0s | Terminal | *Keystone converges it.* | `kscorectl state drift --fix`, then a bare `drift` proving it is back in sync |
| 6 | 0:22.5 | 4.0s | Terminal | *Every change, audited.* | `kscorectl audit stats --since 30m` — the evaluation count is produced by shots 3-5 |
| 7 | 0:26.5 | 3.5s | Card | Logo · **GitOps deploys it. We keep it running.** · repo URL · release status | — |

### Why the script is shaped this way

- **Shots 3–5 are the entire argument.** Declare → drift → converge is
  the loop that separates this from a deploy tool, so it holds 16.5 of
  the 30 seconds. Everything else is framing.
- **Terminal dwell time is the binding constraint.** Below about four
  seconds a viewer cannot read a table. That is why the budget fits
  four terminal shots and not five — and why adding a shot means taking
  the time from an existing one, which `promogen validate` enforces.
- **Shot 4 drifts one file of two on purpose.** The report lands a
  `drifted` row beside an `in_sync` row, which shows the check
  discriminating rather than flagging everything. (It is also the only
  option: both files are owned by the container's nonroot UID, and
  `app.env` at 0640 is not even readable from the host side of the
  bind mount.)
- **Shot 4 shows the drift report, not an exit code.** `state drift`
  returns 0 whether or not it finds drift, so an `echo exit=$?` beat
  would put a claim on screen that the CLI does not support. An
  `--exit-code` flag would make drift detection script cleanly and is
  worth having, but it is product scope rather than promo scope — see
  the ROADMAP entry.
- **Shot 5's second `drift` call is the point of the shot.** Anyone can
  print a diff; the claim being made is convergence.
- **Shot 6 uses `audit stats`, not `audit log`.** The log table is 139
  columns wide; fitting it needs FontSize 20 and it still is not
  readable in four seconds. `stats` is 57 columns and reads at the same
  font as every other shot, and its count comes from the operations the
  viewer just watched.
- **Shot 7 states the release status out loud.** While the line is
  v0.x the end card reads `vX.Y.Z — pre-release`, generated from the
  git tag. Implying GA is the one genuinely damaging thing a promo for
  a pre-1.0 project can do, so it is not left to whoever last edited a
  card.

## Layout

```
assets/promo/
  README.md                     this file — the script
  manifest.yaml                 every reel's shot list as data (validated)
  tapes/
    _common.tape                shared look: size, font, theme, speed
    *.tape.tmpl                 card sources; rendered by promogen
    *.tape                      terminal shots (hand-written) +
                                generated cards (DO NOT EDIT)
  scenario/
    state/web.yaml              the declaration shots 3-5 apply
    drift.sh                    induces the drift shot 4 detects
    docker-compose.promo.yml    bind-mount overlay on the E2E topology
  pipeline/
    build.sh                    deps -> up -> render -> assemble -> down
```

Render output goes to the gitignored `build/promo/` (intermediate
clips) and `dist/promo/` (the finished videos). Those stay uncommitted:
`dist/` is working output and a re-render is never byte-identical, since
the clips carry real UUIDs and timestamps.

`make promo-publish` is the separate, deliberate step that stages the
bytes the docs site serves, copying the promo and the docs clips into
**`assets/promo/video/`** (~1 MB, committed). `docs/hugo.toml` already
mounts the repo-root `assets/` tree at `static/keystone` — the same
mount the navbar logo uses — so the clips are served at
`/keystone/promo/video/<output>.mp4` with nothing copied into `docs/`.

Keeping render and publish apart matters: an experimental render should
not dirty tracked files, and publishing a visually identical re-render
costs ~1 MB of history for nothing. Publish when a clip's *content*
actually changed.

## Embedding a clip in a docs page

`docs/layouts/shortcodes/clip.html`:

```
{{< clip name="docs-secrets" caption="Write a secret, read it back masked." >}}
```

Clips are muted, looping and `playsinline`, so they read as an animated
figure rather than something the reader must decide to play;
`preload="none"` keeps a page with several of them from pulling
megabytes before anyone scrolls.

Set the reel's `docs_page` to the page you embedded it in.
`make promo-check` then asserts that page exists **and still contains
the shortcode** — otherwise the field is a comment, and a clip silently
stops being shown the first time someone restructures a page.

## Targets

| Target | Needs | What it does |
|---|---|---|
| `make update-promo` | Go | Regenerates the cards, validates the shot list, checks tape commands, reconciles demo tags |
| `make promo-check` | Go | Non-mutating version of the above; the CI gate |
| `make promo` | vhs, ttyd, ffmpeg, docker | Renders every reel to `dist/promo/` |
| `make promo-publish` | — | Copies rendered clips into `assets/promo/video/` for the docs site |
| `make promo REEL=<id>` | as above | Renders one reel — the loop to use while iterating on a clip |
| `make install-promo-tools` | Go | Installs `vhs`; reports what must come from a package manager |

Only the render half needs the media toolchain. `vhs` is go-installable;
`ttyd` (which `vhs` drives to get a real terminal) and `ffmpeg` are C
binaries and must come from your package manager.

Run `make build` before `make promo` — the tapes declare
`Require kscorectl`, and `pipeline/build.sh` puts `build/bin/$GOOS/$GOARCH`
on `PATH`.

## Scenario

Terminal shots run against `test/e2e/single/docker-compose.yml` with
`scenario/docker-compose.promo.yml` layered on top. The overlay adds
exactly one thing: a bind mount at `/srv/promo` backed by
`build/promo/workdir/` on the host.

That mount is how drift gets induced. The kscore images are distroless
and have no shell to `docker exec` into, so `drift.sh` edits the host
side of the mount instead — which is also a more honest demo, since it
is the "someone SSH'd in and changed it" case rather than something
staged inside the container.

Two constraints the scenario works around, both of which relax later:

- **State applies server-side.** Blueprints and state runs execute
  against `kscore-server`'s own stdlib `StateRunner` until the
  `gate-v1.0` ROADMAP entry *Remote / distributed blueprint apply
  wiring* lands. The shots are real runs; the managed files just live
  on the server rather than on an agent host.
- **The server container is distroless**, so `package` and `service`
  convergence has nothing to converge against. `scenario/state/web.yaml`
  is file-module-only for that reason. It exercises the same
  Check → Apply → Test loop.

When remote apply lands, the scenario should move to an agent host and
the state file should grow a `package` + `service` declaration — that
is a strictly better demo and the shot durations already accommodate it.

## Adding a reel

1. Add a `reels:` entry with an `id`, `output` (unique — two reels
   writing one file would silently clobber each other), and
   `target_duration`. Set `square_cut: true` only if it needs a 1:1
   social crop; that is noise for a docs clip. Record `docs_page` when
   the clip is embedded in one, so the two cannot drift apart silently.
2. Write its tapes under `tapes/`, and pick the scenario from what
   actually works against the E2E topology — the `TestE2E_*` set in
   `test/e2e/single/scenarios_test.go` is the honest list.
3. `make promo-check`, then `make promo REEL=<id>` and watch it.

Shot ids are unique *within* a reel, not globally: two reels can both
open on a shot called `hook`.

## Adding a shot

1. Tag the changelog fragment: `make changelog-new` now prompts for an
   optional **Demo** value. Set it to a short feature tag when the
   change is legible in a few seconds of terminal output; leave it
   blank otherwise (most entries).
2. `make promo-check` will report the tag as `UNSHOT`.
3. Write a tape under `tapes/`, add the shot to the right reel in
   `manifest.yaml` with the matching `feature`, and **take the duration
   back from another shot in that same reel** — each reel's total is
   asserted against its own budget.
4. Re-run `make promo-check`, then `make promo` and watch the 30
   seconds before publishing.

## The stale-tape guard

A tape whose command has been renamed does not fail loudly — it records
perfectly happily with the error text sitting in frame. That happened
during the first dry run: shot 6 called `kscorectl audit list`, which
does not exist.

`promogen tapes` is the guard. It extracts every project-binary
invocation from every tape, builds the referenced `cmd/` binaries into a
temp directory, and probes `<bin> <path...> --help` — which cobra
resolves without executing anything, so it needs no server, no topology,
and no media toolchain. It runs in `make promo-check` (so CI catches it)
and again at the top of `make promo`, before the three minutes of
rendering.

Two things it has to model, both learned the hard way:

- **Plugin dispatch.** `kscorectl audit` is not a registered subcommand;
  `kscorectl` exec's a `kscore-audit` binary from `$PATH`, git/kubectl
  style (`cmd/kscorectl/main.go`). The check builds the sibling
  alongside and puts the temp directory on `PATH`, or every delegated
  subcommand would look broken.
- **Cobra's fallback.** Exit status alone is not enough: given an
  unrecognised trailing token, cobra prints the *parent's* help and
  exits 0, so `kscorectl state aply --help` succeeds. The check also
  confirms the rendered `Usage:` line actually names the command that
  was asked for.

What it still does not catch: a command that resolves but whose *output*
has changed shape — a new column that pushes a table past the frame, or
a row that no longer appears. Watch the 30 seconds before publishing.
