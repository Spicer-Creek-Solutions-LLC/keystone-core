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

**Nothing here is installed by piping a download into a shell.** This host's
Docker socket is root on it, so an unverified root-level download *is* host
compromise — and `curl … | sh` cannot be reviewed before it runs, cannot be
pinned, and leaves no record of what executed. That convenience is exactly what
this machine cannot afford.

### 1. Docker, from the distribution's signed repository

The package manager verifies signatures against a key installed out of band.
That is the property `curl | sh` lacks, and it is why the packaged path is used
even where it lags a release or two.

**Debian and Ubuntu only.** Docker publishes a separate repository per
distribution with its own key and its own codenames, so the path and the
codename are both read from `/etc/os-release` rather than written for one and
used for the other. A RHEL-family host needs a `dnf` repository instead and is
not covered here; pick the distribution before building the machine.

```sh
. /etc/os-release                      # $ID is debian or ubuntu; $VERSION_CODENAME matches it
case "$ID" in debian|ubuntu) ;; *) echo "unsupported: $ID"; exit 1 ;; esac

# The key is fetched once and pinned by the repository entry; apt refuses the
# repository if a later package is not signed by it.
install -m 0755 -d /etc/apt/keyrings
curl -fsSL "https://download.docker.com/linux/$ID/gpg" -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
https://download.docker.com/linux/$ID $VERSION_CODENAME stable" \
  > /etc/apt/sources.list.d/docker.list

apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

systemctl enable --now docker
```

**Record the installed version in this file at apply time.** An unrecorded
version is a host nobody can reproduce, and reproducing it is the whole point of
a runbook.

### 2. The runner binary, pinned — and the trust anchor it still lacks

```sh
VERSION=<pinned at apply>        # never "latest"
BASE=https://code.forgejo.org/forgejo/runner/releases/download/v${VERSION}

curl -fsSLo /tmp/forgejo-runner "${BASE}/forgejo-runner-${VERSION}-linux-amd64"
curl -fsSLo /tmp/forgejo-runner.sha256 \
  "${BASE}/forgejo-runner-${VERSION}-linux-amd64.sha256"

# Verify BEFORE the binary is anywhere it could be executed from. The expected
# digest is recorded below at apply time, so a later rebuild compares against
# what this host actually ran rather than against whatever the URL serves then.
echo "$(cat /tmp/forgejo-runner.sha256)  /tmp/forgejo-runner" | sha256sum -c -

install -m 0755 /tmp/forgejo-runner /usr/local/bin/forgejo-runner
```

| Recorded at apply | Value |
|---|---|
| Docker version | *(unset — recorded when the host is built)* |
| Runner version | *(unset)* |
| Runner binary sha256 | *(unset)* |

**This is not yet a verified install, and calling it one would be the defect
this repository keeps finding.** The binary and its checksum come from the same
origin, so anything able to change one can change the other and `sha256sum -c`
still passes. What that step gives is **detection of corruption in transit** — a
truncated download, a bad mirror — and nothing about authenticity.

**The trust anchor is the digest recorded in the table above**, and it does not
exist until the host is built. Once recorded, it is under version control, in a
signed commit, and reviewed — so a *rebuild* verifies against a value this
project attested to rather than against whatever the origin serves that day.
**Until that row is filled there is no anchor at all**, and this section says so
rather than implying the checksum is one.

**At apply time, in this order:**

1. **Look for a signature on the release.** If Forgejo publishes one, verify it
   and record the **key fingerprint** here — that is a real anchor and it
   replaces the argument above rather than supplementing it.
2. **If there is none**, record the digest and the date, and note that the first
   install was trust-on-first-use. That is a weaker control, honestly labelled.
3. **Either way, fill the table.** An unrecorded digest makes every later rebuild
   a first install again.

### 3. A dedicated account

```sh
# In the docker group, which is root-equivalent on this host — the separation
# buys process hygiene, not privilege separation. Said plainly so nobody later
# mistakes it for a sandbox.
useradd --system --create-home --home-dir /var/lib/forgejo-runner forgejo-runner
usermod -aG docker forgejo-runner
```

## Registration

**The registration token is obtained by the repository owner**, in the web
interface under the repository's Actions settings. It cannot be fetched with the
bot credential: `actions/runners/registration-token` returns `403 — user should
be the owner of the repo` at both repository and organisation scope.

**No user switch.** Two attempts to do this under one failed review: `sudo`
resets the environment by default and strips the token, and `runuser` resets it
too unless `-m` is given. Both defaults are easy to get wrong and neither failure
is loud — the register receives an empty token and reports something unhelpful.

Registering **as root in the runner's working directory and then fixing
ownership** removes the question rather than answering it:

```sh
cd /var/lib/forgejo-runner

# Repository scope, never organisation or instance: no other repository may
# claim this runner.
#
# The label is distinct. Hosted jobs keep `runs-on: docker` and never reach this
# machine; only jobs asking for `keystone-docker` do.
forgejo-runner register \
  --no-interactive \
  --instance https://codeberg.org \
  --token "$REGISTRATION_TOKEN" \
  --name keystone-container-runner \
  --labels keystone-docker:docker://docker:cli

# The register wrote its configuration as root; the daemon runs as the runner.
chown -R forgejo-runner:forgejo-runner /var/lib/forgejo-runner
```

**This exact invocation is unverified against the target.** No host has been
reachable, so the flag names and the config file's location are taken from the
tool's documented interface and not from a run. **Verification step 1 below is
what confirms it**, and if the flags differ, the runbook is corrected from the
apply rather than the apply improvised around the runbook.

**The token is a credential, and this command puts it in `argv`.**

That is visible to any local user through `/proc/<pid>/cmdline` for as long as
the process runs. This project already knows the shape: `ADR-0007` § 11 records
that **a secret placed in argv is a secret placed in the audit record**, and the
same reasoning applies to a process table.

It is accepted here rather than hidden, for reasons that have to hold at apply
time:

- **The host is single-purpose and single-user.** If it is not — if anyone else
  has a shell on it — this command is the wrong one and the registration should
  be done interactively instead, where the token is never an argument.
- **The exposure lasts seconds**, and the registration runs once per host build.
- **The token is regenerated afterwards** in the web interface, so the value that
  was briefly exposed stops being usable.

**It must not reach a shell history, a log, or this file.** Export it in the
current shell only, and prefix the export with a space where the shell's
`HISTCONTROL` honours `ignorespace`.

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
