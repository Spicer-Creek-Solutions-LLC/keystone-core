# Codeberg repository settings audit

Authoritative record of the Codeberg-side configuration for
`Spicer-Creek-Solutions-LLC/keystone-core`, captured as part of
closing out
[`PUBLIC-LAUNCH-CHECKLIST.md`](PUBLIC-LAUNCH-CHECKLIST.md) E1.

Updated: 2026-05-27.

## Why this file exists

E1 of the public-launch checklist asks for a range of Codeberg-side
settings to match the v0.1.x posture. Those settings live in the
Codeberg web UI and aren't otherwise visible in-tree, so a drift
between "what the checklist says" and "what Codeberg actually has"
is easy to introduce silently. This doc is the in-repo source of
truth for what was configured, what was deliberately deferred, and
how to re-verify each setting.

If you change something on Codeberg that's listed here, update this
file in the same PR.

## Audit scope

Every Codeberg-side setting called out in E1, plus the merge-method
defaults (not in E1 but tightened during the same audit).

## Current state

### Repository metadata

| Setting | Value | Source |
|---|---|---|
| Default branch | `main` | `GET /repos/{o}/{r}` `default_branch` |
| Description | `GitOps deploys it. We keep it running. A runtime operations control plane for Linux fleets — state, audit, events, secrets. v0.x pre-stable.` | `GET /repos/{o}/{r}` `description` |
| Website | `https://docs.keystone-core.io` | `GET /repos/{o}/{r}` `website` |
| Topics | `configuration-management`, `control-plane`, `day2-operations`, `fleet-management`, `gitops`, `golang`, `linux` | `GET /repos/{o}/{r}/topics` |

### Feature toggles

| Feature | State | Rationale |
|---|---|---|
| Issues | enabled | primary tracker per [`ISSUE-TRACKING.md`](ISSUE-TRACKING.md) |
| Pull requests | enabled | contribution path |
| Releases | enabled | required by [`../../RELEASE-PLAYBOOK.md`](../../RELEASE-PLAYBOOK.md) |
| Actions | enabled | CI lives at [`.forgejo/workflows/`](../../.forgejo/workflows/) |
| Wiki | **disabled** | docs live in-tree under `docs/project/` |
| Packages | **disabled** | distribution is via release artifacts + tag-signed downloads, not Codeberg's package registry |
| Projects (kanban) | **disabled** | issue tracker covers the planning need; project boards add UI surface without value at v0.1.x |

### Merge methods

| Setting | Value | Rationale |
|---|---|---|
| `default_merge_style` | `merge` | every PR landing on `main` produces a merge commit (visible PR-boundary marker for audit / governance / release notes) |
| `allow_merge_commits` | true | default style |
| `allow_squash_merge` | **false** | squash collapses per-step commits and breaks the incremental-commit / per-commit DCO sign-off / per-commit AI-attribution chain mandated by [`../../AGENTS.md`](../../AGENTS.md) §§ 2 + 4 |
| `allow_rebase` (fast-forward, no merge commit) | **false** | every PR must leave a merge-commit marker on `main` |
| `allow_rebase_explicit` (rebase + create merge commit) | true | lets contributors clean up the branch before the merge commit lands |
| `allow_fast_forward_only_merge` | false | redundant with `allow_rebase: false` |
| `default_delete_branch_after_merge` | true | keeps the branch list tidy without manual cleanup |

### Branch protection on `main`

Applied via the Codeberg UI. The `FORGEJO_TOKEN` (`keystone-bot`)
lacks admin scope on this org repo, so neither audit nor write of
branch-protection rules is possible via API with that token. The
maintainer applied each setting directly through the web UI.

| Setting | Value | Notes |
|---|---|---|
| Branch protected | true | rule exists on `main` |
| Required status checks | enabled, 11 contexts | every `ci-fast.yml` job (see below) |
| `block_on_outdated_branch` | true | PRs must rebase against latest `main` before merge |
| Required approvals | 0 | solo-maintainer v0.1.x posture; bumps to ≥1 when a second maintainer onboards |
| Force-push to `main` | blocked | |
| Delete `main` | blocked | |
| Require signed commits | **false** | **deferred to v0.x** — see [`ROADMAP.md`](ROADMAP.md) "Phase E1: required signed commits on `main` branch protection". Contributor key-onboarding flow needs documenting first. |
| DCO check | enforced via CI status check (`ci / dco-check (pull_request)`) — see below | Forgejo has no native DCO branch-protection toggle |

Required status-check contexts (all from
[`.forgejo/workflows/ci-fast.yml`](../../.forgejo/workflows/ci-fast.yml)):

- `ci / lint (pull_request)`
- `ci / test (pull_request)`
- `ci / slo (pull_request)`
- `ci / integration (pull_request)`
- `ci / smoke (pull_request)`
- `ci / security (pull_request)`
- `ci / e2e (pull_request)`
- `ci / openapi (pull_request)`
- `ci / docs (pull_request)`
- `ci / proto (pull_request)`
- `ci / build (amd64, linux) (pull_request)`

Add `ci / dco-check (pull_request)` to this list after the first PR
that includes the new job lands on `main` (Codeberg autocompletes
required-check names from observed history; the context must exist
before it can be selected).

### DCO enforcement

DCO sign-off is required on every commit per
[`DCO.md`](DCO.md) and [`../../AGENTS.md`](../../AGENTS.md) § 4.
Forgejo branch protection has no native DCO toggle, so enforcement
runs as a CI job:
[`.forgejo/workflows/ci-fast.yml`](../../.forgejo/workflows/ci-fast.yml)
`dco-check`. The job walks every non-merge commit in the PR range
and fails if any commit's message lacks a `Signed-off-by:` trailer
in the standard `Name <email>` format.

Lenient v0.1.x policy: any well-formed trailer counts. Tightening
to "trailer email must match author email" is on the v0.x backlog
alongside the signed-commits enablement.

## How to re-verify

The repo metadata, feature toggles, and merge methods are
readable via the public Forgejo REST API and the
`keystone-bot` token:

```sh
OWNER=Spicer-Creek-Solutions-LLC
REPO=keystone-core
BASE=https://codeberg.org/api/v1/repos/$OWNER/$REPO

curl -s -H "Authorization: token $FORGEJO_TOKEN" "$BASE" \
  | jq '{default_branch, description, website,
         has_issues, has_pull_requests, has_releases, has_actions,
         has_wiki, has_packages, has_projects,
         default_merge_style, allow_merge_commits, allow_squash_merge,
         allow_rebase, allow_rebase_explicit, allow_fast_forward_only_merge,
         default_delete_branch_after_merge}'

curl -s -H "Authorization: token $FORGEJO_TOKEN" "$BASE/topics" | jq
```

Branch-protection settings require admin scope on the repo —
verify via the Codeberg UI at
`https://codeberg.org/$OWNER/$REPO/settings/branches`, or with an
admin-scoped token via `GET $BASE/branch_protections`.

## Deferrals

Recorded in [`ROADMAP.md`](ROADMAP.md):

- **Required signed commits** on `main` — deferred to v0.x.
  Contributor key-onboarding flow needs documenting before mandatory
  enforcement is reasonable. `RELEASE-PLAYBOOK.md` already covers
  signed release tags, which is the higher-value signing surface
  for v0.1.x.

## References

- [`PUBLIC-LAUNCH-CHECKLIST.md`](PUBLIC-LAUNCH-CHECKLIST.md) E1 (this audit closes that item)
- [`ROADMAP.md`](ROADMAP.md) "Phase E1: required signed commits on `main` branch protection"
- [`DCO.md`](DCO.md)
- [`../../AGENTS.md`](../../AGENTS.md) §§ 2, 4
- [`.forgejo/workflows/ci-fast.yml`](../../.forgejo/workflows/ci-fast.yml) `dco-check` job
