# The self-hosted container runner

`TESTING.md` makes Docker the integration floor for every feature, and
[`ADR-0010`](../adr/0010-acceptance-harness.md) § 2 designs the topology that
floor runs on. **No runner available to this repository can run containers**, so
that floor needs a machine built to.

This said *the forge's hosted runners* cannot run containers until G25. That
claim was never established — see § What the probe established — and the weaker
one above is what the evidence supports. It reaches the same conclusion.

This is how that machine is built, registered, locked down, and rebuilt — and
what accepting it costs.

## What the probe established

Two dispatched probe runs, on 2026-09-15. Job conclusions rather than logs,
because this forge's API does not expose job output.

**Which machine they measured is not established, and this document used to say
it was the forge's hosted pool.** The probe asked for `runs-on: docker`, and on
2026-09-16 the only runner known to serve that label was this self-hosted host —
four `docker` samples queued indefinitely once its label changed, which is
[R1](#verification)'s evidence and is equally evidence that nothing else answers
to `docker`. The same questions, re-asked on `keystone-docker`, returned the same
answers. **The probe most likely measured this host.**

What that changes: the table below stands as a measurement, and the conclusion —
this repository needs a machine built for containers — is unaffected either way.
What it removes is the claim that a *hosted pool* was measured, and with it the
premise `R1` and § Verification step 2 were written on. Whether this repository
has a hosted pool at all is unknown; the runner list is owner-only.

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

**Both workflows therefore trigger on `push` and nothing else.** Only a
principal with write access can create a branch, so only they can cause the
runner to run.

This was written for the container workflow, which does not exist yet, while
`reboot-baseline` — which does — carried a `pull_request` trigger and asked for
the label this host advertised. **Every pull request had been running here**, and
the rule was stated in the one place it was not needed. G25 moved that workflow
to `push` and the rule now describes the repository rather than a plan.

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

## What this project requires of the runner

**These are the properties, not the procedure.** Forgejo documents how to
install and register a runner, and that documentation changes with its releases;
restating it here produces a worse copy that nothing can keep in step. What
follows is what this repository needs to be true of whatever host you build, and
how each is checked.

| # | Requirement | Why it is ours rather than upstream's | How it is checked |
|---|---|---|---|
| R1 | **The label is not `docker`, and not `ubuntu-latest`** | A label this host shares with anything else makes the two interchangeable, and no workflow condition can separate runners that advertise the same label. `docker` and `ubuntu-latest` are the conventional labels a general-purpose runner claims, so a workflow written against either may be scheduled here by a later author who never read this file. The label is the only thing that routes a job, so it is the only thing that can hold the boundary | § Verification, step 2 |
| R2 | **Repository scope** | An instance- or organisation-scoped runner can be claimed by another repository, which is a different trust decision than the one made here | Its entry in the repository's Actions settings |
| R3 | **No state carries from one job to the next** | Residue from one job reaching the next is how a compromised or merely broken job spreads. **This is an isolation property, not a process lifecycle** — a persistent runner giving each job a fresh container satisfies it, and `--once` is one mechanism among others. The requirement is the property; the mechanism is the deployment's | § Verification, step 4 |
| R4 | **A dedicated host** | The OS-test VMs are snapshot-reset for distribution testing, and a reset unregisters the runner **without failing loudly** — jobs queue against a label nobody advertises | Operational; stated here so a later reuse is a decision rather than an accident |
| R5 | **The host holds nothing worth stealing** | The runner needs the Docker socket, and that is root on the host. Sandboxing inside a job is decoration; the host is the boundary | Review of what is on it |
| R6 | **Docker works inside a job, through a daemon of the job's own** | `ADR-0010` § 2 requires the isolation to be *proved by a probe* rather than asserted — and proved in both directions. **Not the host's socket**: see § How a job gets Docker | § Verification, step 3 |
| R7 | **What was installed is recorded** | So a rebuild reproduces this host rather than becoming a first install again. This is the trust anchor the checksum is not | § What is deployed |

## How a job gets Docker

**A daemon of the job's own, not the host's socket.**

The alternatives and why this one:

| Shape | Why not |
|---|---|
| **Jobs run on the host**, where Docker already is | Simplest, and it **gives up `R3`** — a marker written to `/tmp` survives into the next job. `R3` is the only isolation between one job and the next, since `R5` concedes the host is the security boundary |
| **A container with the host's socket mounted in** | Keeps `R3` for the job, and **containers the job creates are siblings on the host daemon** — they outlive the job unless it cleans up, and a suite that fails partway leaves them behind. That is the state that makes the *next* run fail confusingly |
| **A daemon per job** — chosen | Containers and networks the harness creates belong to a daemon that **dies with the job**, so the sibling problem does not exist rather than being managed. Job code never touches the host's socket at all |

**What it costs, stated rather than discovered:** a daemon per job starts empty,
so **there is no image cache between jobs.** Every run pulls what it needs.

**That cost is accepted for now.** Nobody has run this suite, so how slow "slow"
is, is a guess, and optimising against a guess has a poor record here. The
trigger to revisit is specific: **the first run that fails on a registry rate
limit rather than on a defect.** The answer then is a pull-through cache on the
host — upstream of the daemon, so a job still starts from an empty store.

### Rejected: sharing the daemon's storage between jobs

**Recorded because it is the tempting wrong answer**, and an unrecorded rejection
gets rediscovered as a good idea.

Persisting `/var/lib/docker` across jobs would remove the pull cost — and it is
**state surviving between jobs, which is `R3` undone by the back door.** The
whole reason for a per-job daemon is that nothing it holds outlives the job. A
shared image store is something it holds.

## Illustrative configuration

**Examples, not instructions.** They exist to show what R1 and R2 look like in
practice; **Forgejo's own documentation is authoritative** for flags, file
locations and installation, and it is the thing to check when something here does
not match the tool in front of you.

Registration, with the two properties this project cares about marked:

```sh
forgejo-runner register \
  --no-interactive \
  --instance https://codeberg.org \
  --token "$REGISTRATION_TOKEN" \
  --name keystone-container-runner \
  --labels keystone-docker:docker://docker:cli   # R1: not `docker`, not `ubuntu-latest`
```

The same thing as runner configuration, where labels can be changed without
re-registering — the daemon re-declares them on start:

```yaml
# config.yml (illustrative)
runner:
  labels:
    - "keystone-docker:docker://docker:cli"      # R1
  capacity: 1                                    # concurrency, NOT R3 --
                                                 # R3 is isolation between
                                                 # jobs and is established
                                                 # by verification, not by
                                                 # a config line
```

**The token is a credential and `--token` puts it in `argv`**, readable by any
local user through `/proc/<pid>/cmdline`. This project already knows the shape —
`ADR-0007` § 11 records that a secret placed in argv is a secret placed in the
audit record. Prefer whatever non-argv mechanism the current tool offers; where
there is none, register on a host where nobody else has a shell, and **regenerate
the token afterwards** so the exposed value stops being usable.

## What is deployed

**Filled from the host, not from this document.** Empty rows are owed, and a
rebuild cannot reproduce a host whose values were never written down.

| | Value |
|---|---|
| Installed how (package, binary, container) | *(unset)* |
| Runner version | *(unset)* |
| Runner binary digest, if installed as a binary | *(unset)* |
| Docker version | *(unset)* |
| Working directory, user, config location | *(unset)* |
| Isolation between jobs (R3) | *(unset)* |
| Scope (R2) | *(unset)* |
| Labels (R1) | `keystone-docker` — **exclusive**, see below |

### What verification has established so far

Two dispatched runs against the live runner, 2026-09-16. Job conclusions, since
job logs are not readable through this forge's API.

| | Result |
|---|---|
| Jobs run on the runner's own label | **yes** |
| `actions/checkout` works there | **yes** |
| **R3** — a marker written outside the workspace does not survive into the next job | **holds** |
| **R6** — Docker usable inside a job | **not met** |
| **R1** — the label is exclusive | **holds** |

**`R6`'s diagnosis, which is why the shape above was chosen.** Jobs run *inside a
container* — confirmed, not inferred — and that container has **neither the
Docker client nor a socket**, so a job cannot reach Docker at all.

This read *a configuration detail rather than the structural impossibility the
hosted pool has*. That contrast does not survive § What the probe established:
the probe's findings are most likely this host's, so the two are one finding
measured twice rather than two machines failing differently. **Whether it is
configuration or structural is therefore open** — § "How a job gets Docker" is
the change that would settle it, and until a job runs `docker compose version`
here, `R6` is unmet for a reason this document does not know.

**`R1` holds, and the evidence is stronger than step 2 asked for.** The four
`runs-on: docker` samples in the verification run have now been queued for hours
alongside three `reboot-baseline` runs that ask for the same label, while the
runner sat idle. **A queued job is not a job that ran somewhere unexpected — it
is a job no runner claimed at all**, which establishes more than exclusivity: on
the evidence available to this repository, *nothing* serves `docker`. The
maintainer separately confirms that no other runner carries `keystone-docker`.

Those queued samples should be cancelled. They have answered, and they will not
run.

**An earlier deviation, closed by G25 rather than by the rename.** The runner
advertised `docker`, which was the label `reboot-baseline` asked for on
`pull_request`. This document said a pull request *could* be scheduled onto this
host. It is now clear that **every pull request was** — runs 809 to 834 ran here
uninterrupted, at a steady 22s before P11a and 38s after, and stopped dead at the
rename with nothing else picking them up.

**The rename did not close the exposure; it only broke the gate.** Pointing
`reboot-baseline` at `keystone-docker` would have reopened it unchanged, which is
why G25 removed the `pull_request` trigger in the same change that moved the
label. What closes this is the trigger, not the name.

Changing the labels in the runner configuration was sufficient to rename it;
**re-registration was not required**, and an earlier version of this document
wrongly said otherwise.

## Verification

**The apply is not finished when the runner registers.** It is finished when a
job **that would fail on a misconfigured host** has passed.

1. **Docker works there.** A job on the runner's label runs `docker compose
   version` and succeeds. This is the question `R6` currently fails, so it is the
   step that decides whether the apply is finished.

2. **The label is exclusive**, which is R1 and cannot be read from the API: the
   runner list is owner-only. It is established empirically, by **whether a
   `runs-on: docker` job is ever claimed at all**: several such jobs are
   dispatched, and none may start. A job that stays queued was claimed by no
   runner, which is the property R1 asserts.

   Several, not one: scheduling between matching runners is not deterministic, so
   one sample that happens not to be picked up proves nothing.

   **Docker was the discriminator here until G25, and it never could be.** The
   test read: Docker succeeds on `keystone-docker`, fails on `docker`. But `R6`
   records Docker failing on `keystone-docker` too — both sides of the comparison
   return the same answer, so it separates nothing. It was written on the
   assumption that a hosted pool holds the `docker` label and lacks Docker; § What
   the probe established is why that assumption does not hold.

3. **The isolation probe fails when isolation is absent.** Two networks with no
   route between them, a container on each, and the one **cannot** reach the
   other — **and a deliberately joined pair must fail the same probe.** A probe
   demonstrated only in the passing direction is a probe that would pass on a
   topology with a route, which is what `ADR-0010` § 2 forbids.

4. **No state carries between jobs** — `R3`. A job writes a marker outside its
   workspace; the next job does not find it.

   **What this establishes and what it does not.** It shows the *isolation*,
   which is the requirement. It does **not** show the runner process exited
   between jobs, and it need not: a persistent runner handing each job a fresh
   container passes this and satisfies `R3`. Cleanup could also make it pass —
   so the check is on a path a job would not think to clean, outside the
   workspace, rather than inside it.

5. **A push produces a commit status visible on a pull request** opened from that
   branch — the claim the whole trigger design rests on.

Only after 5 does anything become a required check.

## Rebuilding it

A rebuild, a snapshot restore, or a host migration **invalidates the
registration**. The symptom is not an error: jobs queue against a label nobody
advertises, and a pull request waits on a check that will never report.

Re-register with a fresh token, delete the stale entry in the web interface
first so it cannot be confused for the live one, and **fill § What is deployed
again** — an unrecorded rebuild is a first install with extra steps.

## What this document does not cover, deliberately

**Installing anything.** The runner binary, its service unit, and Docker itself
are Forgejo's and the distribution's documentation respectively. An earlier
version of this file restated all three, and **every finding raised against it in
review was a defect in that restatement** — the environment behaviour of `sudo`,
then of `runuser`, then a Docker repository path correct for one of the two
distributions it named. None of those were facts about this project.

The rule this repository already applies elsewhere: **do not restate a source you
do not control.** `docs-links` enumerates from git rather than duplicating a
glob; `ADR-0010` § 13 refused to copy the invariant map into the ADR. A
restatement has no gate that can catch it drifting.

## What this does not decide

- **Which suites run here.** `ADR-0010` § 11 fixes the gate schedule; this
  document provides the machine it needs.
- **The VM harness.** `ADR-0010` § 8 places install, upgrade, reboot and the
  privilege model on a VM at the release-candidate gate. That is C13's, and it is
  a different machine from this one.
- **Whether the container suite ever becomes a required check.** That is a
  branch-protection decision, and R06's record of what happens when a required
  context stops being reported is the reason it waits for the verification above.
