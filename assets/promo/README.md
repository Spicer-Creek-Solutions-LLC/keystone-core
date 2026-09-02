# Promo video — script, shot list, and pipeline

The 30-second Keystone Core promo. Terminal-first and text-only: every
terminal shot is real `kscorectl` output recorded against a live
topology, and the on-screen captions carry the whole narration (there
is no voiceover).

This file is the script. [`manifest.yaml`](manifest.yaml) is the same
shot list as data — the edit decision list the renderer consumes. Keep
the two in step; `make promo-check` enforces the parts a machine can
see.

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
| 4 | 0:11 | 5.5s | Terminal | *The host drifts anyway.* | `drift.sh` mutates the files out of band; `kscorectl state drift` reports it, non-zero exit shown |
| 5 | 0:16.5 | 6.0s | Terminal | *Keystone converges it.* | `kscorectl state drift --fix`, then a bare `drift` proving it is back in sync |
| 6 | 0:22.5 | 4.0s | Terminal | *Every change, audited.* | `kscorectl audit list --limit 5` |
| 7 | 0:26.5 | 3.5s | Card | Logo · **GitOps deploys it. We keep it running.** · repo URL · release status | — |

### Why the script is shaped this way

- **Shots 3–5 are the entire argument.** Declare → drift → converge is
  the loop that separates this from a deploy tool, so it holds 16.5 of
  the 30 seconds. Everything else is framing.
- **Terminal dwell time is the binding constraint.** Below about four
  seconds a viewer cannot read a table. That is why the budget fits
  four terminal shots and not five — and why adding a shot means taking
  the time from an existing one, which `promogen validate` enforces.
- **Shot 4 shows the non-zero exit on purpose.** Drift detection that
  scripts cleanly is the difference between a dashboard and a control
  plane.
- **Shot 5's second `drift` call is the point of the shot.** Anyone can
  print a diff; the claim being made is convergence.
- **Shot 7 states the release status out loud.** While the line is
  v0.x the end card reads `vX.Y.Z — pre-release`, generated from the
  git tag. Implying GA is the one genuinely damaging thing a promo for
  a pre-1.0 project can do, so it is not left to whoever last edited a
  card.

## Layout

```
assets/promo/
  README.md                     this file — the script
  manifest.yaml                 the shot list as data (validated)
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
clips) and `dist/promo/` (finished `keystone-30s.mp4` and a 1080×1080
square cut). The `.mp4` files are deliberately not committed — publish
them as a release asset or on the docs site and keep only the sources
here.

## Targets

| Target | Needs | What it does |
|---|---|---|
| `make update-promo` | Go | Regenerates the cards from the branch, validates the shot list, reconciles demo tags |
| `make promo-check` | Go | Non-mutating version of the above; the CI gate |
| `make promo` | vhs, ttyd, ffmpeg, docker | Renders the video to `dist/promo/` |
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

## Adding a shot

1. Tag the changelog fragment: `make changelog-new` now prompts for an
   optional **Demo** value. Set it to a short feature tag when the
   change is legible in a few seconds of terminal output; leave it
   blank otherwise (most entries).
2. `make promo-check` will report the tag as `UNSHOT`.
3. Write a tape under `tapes/`, add the shot to `manifest.yaml` with
   the matching `feature`, and **take the duration back from another
   shot** — the total is asserted at 30.0s ±0.25s.
4. Re-run `make promo-check`, then `make promo` and watch the 30
   seconds before publishing. The pipeline's automated guard only
   catches a blank clip; a stale tape whose command now errors renders
   perfectly happily with the error in frame.
