# The self-hosted container runner

`TESTING.md` makes Docker the integration floor for every feature, and
[`ADR-0010`](../adr/0010-acceptance-harness.md) § 2 designs the topology that
floor runs on. **The forge's hosted runners cannot run containers**, so that
floor needs a machine of ours.

This is how that machine is built, registered, locked down, and rebuilt — and
what accepting it costs.

## What the probe established

Two dispatched probe runs, on 2026-09-15. Job conclusions rather than logs,
because this forge's API does not expose job output.

| Question | Result |
|---|---|
| `docker version` | fails |
| `docker compose version` | fails |
| a nested container runs | fails |
| two user-defined networks, isolated | fails |
| **`/var/run/docker.sock` exists** | **fails** |
| a Docker client can be installed | succeeds |
| `CAP_SYS_ADMIN` is held | fails |
| `unshare --net` | fails |
| the Go toolchain works | succeeds |

**The decisive two are the socket and the capability.** There is no daemon to
talk to and no privilege to start one, so this is structural rather than a
missing package — and the installable client is a trap, since it would install
cleanly and have nothing to reach. `unshare --net` failing removes the
container-free fallback as well: there is no way to build an isolated network on
that runner at all.

## The trust boundary, stated before the mechanics

**After this runner exists, write access to this repository is code execution on
that machine.**

That is not a new *population* — it is the same principals who can already merge
— but it is now a host rather than a branch, and the difference matters when
something goes wrong. Two consequences follow, and both are design constraints
rather than advice:

- **The runner needs the Docker socket, and access to the Docker socket is root
  on the host.** Any sandboxing inside a job is decoration; the host is the
  boundary.
- **The host therefore holds nothing worth stealing.** No SSH keys, no registry
  or cloud credentials, no route to anything on the network it does not need. It
  is rebuilt rather than repaired.

## Why the workflow has no `pull_request` trigger

A self-hosted runner on a **public** repository must never execute code from an
arbitrary contributor. This repository is public, and this forge has **no
first-time-contributor approval gate** — its only Actions gate fires on
workflow-file edits, which does not help here.

**The container workflow therefore triggers on `push` and nothing else.** Only a
principal with write access can create a branch, so only they can cause the
runner to run.

**This is defence by absence, not by condition.** A `pull_request` trigger with
an `if:` guard would still start a run from an untrusted event and rely on the
guard being right, forever, through every later edit. No trigger means no event
to guard.

**`pull_request_target` and `issue_comment` are forbidden in this repository.**
Both run the *base* repository's workflow, with its secrets, in the context of
code the contributor controls. They are the standard route by which self-hosted
runners are taken, and the fact that they would be convenient is exactly why
this sentence exists.

### The reporting is unaffected, which is why this costs nothing

A workflow run attaches its result as a **commit status**, and a pull request's
checks are the statuses of its **head commit**. A branch push and the pull
request that carries it are the same commit — so the container result appears on
the pull request without the pull-request event ever being involved.

Verified before adopting it: commit `8bed67df1` carries
`reboot-baseline / verify (push)` as a status, reported from a push-triggered
run.

**Two cases the trigger does not cover**, and what to do instead:

| Case | What to do |
|---|---|
| A pull request from a **fork** | Read the diff, fetch its head to a branch in this repository, push it. That is a deliberate approval by a known account, performed rather than automated — which is the only safe form |
| A **re-run** without a new commit | `workflow_dispatch` with a `ref` input, which the container workflow carries for this purpose |

## Provisioning

**A dedicated host**, not one of the OS-test VMs. Those are snapshot-reset for
distribution testing, and a reset silently unregisters the runner — which does
not fail loudly. It leaves jobs queued against a label nobody advertises, which
is the worst shape of failure to diagnose.

```sh
# 1. A current Docker engine. The runner needs the engine, not just a client.
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker

# 2. The runner binary. Pin a release; :latest turns a green pipeline into an
#    unannounced upgrade, which is the lesson the lychee pin in the Makefile
#    already records.
VERSION=<pinned>
curl -fsSLo /usr/local/bin/forgejo-runner \
  "https://code.forgejo.org/forgejo/runner/releases/download/v${VERSION}/forgejo-runner-${VERSION}-linux-amd64"
chmod +x /usr/local/bin/forgejo-runner

# 3. A dedicated account. It is in the docker group, which is root-equivalent —
#    the separation buys process hygiene, not privilege separation.
useradd --system --create-home --home-dir /var/lib/forgejo-runner forgejo-runner
usermod -aG docker forgejo-runner
```

## Registration

**The registration token is obtained by the repository owner**, in the web
interface under the repository's Actions settings. It cannot be fetched with the
bot credential: `actions/runners/registration-token` returns `403 — user should
be the owner of the repo` at both repository and organisation scope.

```sh
# Repository scope, never organisation or instance: no other repository may
# claim this runner.
#
# The label is distinct. Hosted jobs keep `runs-on: docker` and never reach
# this machine; only jobs asking for `keystone-docker` do.
sudo -u forgejo-runner forgejo-runner register \
  --no-interactive \
  --instance https://codeberg.org \
  --token "$REGISTRATION_TOKEN" \
  --name keystone-container-runner \
  --labels keystone-docker:docker://docker:cli
```

**The token is a credential.** Pass it through the environment; it must not
reach a shell history, a log, or this file.

## Running it, ephemerally

```ini
# /etc/systemd/system/forgejo-runner.service
[Unit]
Description=Forgejo runner (keystone container suite)
After=docker.service
Requires=docker.service

[Service]
User=forgejo-runner
WorkingDirectory=/var/lib/forgejo-runner
# One job, then exit. systemd restarts it, so every job begins on a machine
# that has run nothing else. Persistence between jobs is the thing worth
# denying: it is how one job's residue reaches the next.
ExecStart=/usr/local/bin/forgejo-runner daemon --once
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
```

## Verifying it, before anything depends on it

The apply is not finished when the runner registers. It is finished when a job
**that would fail on a misconfigured host** has passed:

1. The runner appears in the repository's Actions settings with the
   `keystone-docker` label and no other.
2. A dispatched job on that label runs `docker compose version` and succeeds —
   the question the hosted runner failed.
3. The same job brings up two networks with no route between them and proves a
   container on one **cannot** reach a container on the other. **A probe that
   passes when isolation is absent is worse than no probe** (`ADR-0010` § 2), so
   the verification runs it in both directions: isolated networks must pass, and
   a deliberately-joined pair must fail.
4. A push to a scratch branch produces a **commit status**, and that status is
   visible on a pull request opened from it.

Only after 4 does anything become a required check.

## Rebuilding it

A rebuild, a snapshot restore, or a host migration **invalidates the
registration**. The symptom is not an error: jobs queue against a label nobody
advertises, and a pull request waits on a check that will never report.

Re-run Provisioning and Registration with a fresh token. The old runner entry
should be deleted in the web interface first, so a stale entry cannot be
confused for the live one.

## What this does not decide

- **Which suites run here.** `ADR-0010` § 11 fixes the gate schedule; this
  document provides the machine it needs.
- **The VM harness.** `ADR-0010` § 8 places install, upgrade, reboot and the
  privilege model on a VM at the release-candidate gate. That is C13's, and it is
  a different machine from this one.
- **Whether the container suite ever becomes a required check.** That is a
  branch-protection decision, and R06's record of what happens when a required
  context stops being reported is the reason it waits for the verification above.
