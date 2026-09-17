# The self-hosted container runner

`TESTING.md` makes Docker the integration floor for every feature, and
[`ADR-0010`](../adr/0010-acceptance-harness.md) § 2 designs the topology that
floor runs on. **No job on this self-hosted runner can reach Docker**, so that
runner needs configuring to give one.

**Three earlier versions of this sentence claimed more than was measured, each
time about a different thing.** It said *the forge's hosted runners* cannot run
containers — a pool never established to exist. G25 replaced that with *no runner
available to this repository can run containers* — false, since the probe
measured a **job's container** and the host plainly starts containers. G27's
first draft said *no job on a runner available to this repository can reach
Docker* — still quantified over runners nobody measured.

**One runner was probed and that is the claim.** Whether a hosted runner is
available here is unknown (§ What the probe established), and if one exists its
job image could differ. The conclusion is unaffected: this is the runner claiming
this repository's jobs, so it is the one that has to change.

This is how that machine is built, registered, locked down, and rebuilt — and
what accepting it costs.

## What the probe established

Two dispatched probe runs, on 2026-09-15. Job conclusions rather than logs,
because this forge's API does not expose job output.

**What the probe measured is a job, not a machine**, and every row below is a
statement about the container a job runs inside.

The probe asked for `runs-on: docker`, which this host advertised at the time,
and the same questions re-asked on `keystone-docker` — a label the runner list
shows on this host alone — returned the same answers. So the findings are
reproduced on this host. **Attributing them to a hosted pool was unsupported, and
attributing them to the host's capability was wrong**: one of the probe's own
rows says the job *ran inside a container*, so the host starts containers. It is
how the job existed.

An earlier version of this paragraph also offered the queued `docker` samples as
evidence that *nothing else answers to `docker`*. **They are not**, for the
reason § Verification step 2 now gives: an unclaimed job does not report what
labels exist. The reproduction on `keystone-docker` is the evidence here, and the
samples say only that no runner is currently claiming `docker` jobs for this
repository.

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

**The decisive two are the socket and the capability.** A job has no daemon to
talk to and no privilege to start one — and the installable client is a trap,
since it would install cleanly and have nothing to reach.

**This said the shortage was *structural rather than a missing package*. It is
not, and nothing here established that it was.** A job reaches Docker when the
image its label maps to carries a client and the job is given a daemon; both are
configuration. The host's own ability to run containers was never in question and
is not what these rows measure. `unshare --net` failing removes the
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

**Both workflows therefore trigger on `push` and `workflow_dispatch`, and on no
event a contributor can cause.** Pushing a branch and dispatching a run both
require write access.

**The property is who can start a run, not how few triggers there are.** An
earlier wording here said *`push` and nothing else*, which `reboot-baseline`
contradicted in the same change that adopted it — it also declares
`workflow_dispatch`. Manual dispatch is a trigger; it is simply not one an
outside contributor can reach.

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

### The required status context changes with the trigger, and only an owner can move it

Moving `reboot-baseline` off `pull_request` changes the name of the status it
reports. Branch protection matches that name literally, so the rule must be moved
too — **and this is not a change this repository can make**. The API rejects a
non-owner with `403`, so the record of it lives here.

| | Value |
|---|---|
| Context reported before G25, on a pull request | `reboot-baseline / verify (pull_request)` |
| Context reported after G25 | `reboot-baseline / verify (push)` |
| Observed on | commit `5c08e3a4c`, run 843, combined state `success` |
| Required change | `main`'s protection rule must require the **`(push)`** context |
| Applied | **yes — 2026-09-16, by the maintainer**, who reports the rule now holds `reboot-baseline / verify (push)` and that status checks were required before the change and remain required |

**Status checks were enabled throughout**, which settles what the stalled gate
cost. `R06`'s note at the top of the workflow file records what a required
context that is never reported does: a branch requiring it can never be merged
again. The `(pull_request)` context stopped being reported when the label broke
and will not be reported again — so between G24's rename and this rule change,
**`main` was unmergeable**, and pull request #334 was blocked rather than merely
missing a result. G25 did not create that state; it ended it.

**The evidence is owner inspection, as it is for `R1`.** The protection rule is
not readable by a non-owner — the API returns `403` — so this row records what
the maintainer read, dated, rather than something a job demonstrated.

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
| R4 | **A dedicated host** | The OS-test VMs are snapshot-reset for distribution testing, and a reset unregisters the runner **without failing loudly** — its jobs then queue indefinitely | Operational; stated here so a later reuse is a decision rather than an accident |
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
| Labels (R1) | `keystone-docker`, which the runner list shows on this runner alone — **read from the list by the maintainer, 2026-09-16** |

### What verification has established so far

Two dispatched runs against the live runner, 2026-09-16. Job conclusions, since
job logs are not readable through this forge's API.

| | Result |
|---|---|
| Jobs run on the runner's own label | **yes** |
| `actions/checkout` works there | **yes** |
| **R3** — a marker written outside the workspace does not survive into the next job | **holds** |
| **R6** — Docker usable inside a job | **not met** |
| **R1** — the label is neither `docker` nor `ubuntu-latest` | **holds**, on the runner list — not on the dispatched samples, which cannot establish it |

**`R6`'s diagnosis, which is why the shape above was chosen.** Jobs run *inside a
container* — confirmed, not inferred — and that container has **neither the
Docker client nor a socket**, so a job cannot reach Docker at all.

**It is configuration, and the host was never the constraint.** The maintainer
states this runner has run Docker containers since before the reboot, and the
probe agrees with them: the row *a job runs inside a container* passed, so the
host starts containers. `R6` is about what a **job** is handed — the image its
label maps to, and whether a daemon comes with it — and § "How a job gets Docker"
is the change that closes it.

Two earlier readings are superseded. This first said *a configuration detail
rather than the structural impossibility the hosted pool has*, which assumed a
hosted pool that was never established; G25 replaced it with **whether it is
configuration or structural is open**, which treated a job's container as if it
were evidence about a machine. Neither survives contact with the row above.

**`R1` holds, on the runner list rather than on the queue.** R1 constrains *this
host's* configuration — that its label is neither `docker` nor `ubuntu-latest` —
and the runner list states that directly. The maintainer read it and confirms
`keystone-docker`, and the list shows that label on this runner alone. **Owner
inspection is the authoritative evidence for R1**, and it is better than any
probe, because it reads the configuration rather than inferring it.

**What the queued samples showed is less than this document first claimed.**
Four `runs-on: docker` samples and three `reboot-baseline` runs sat unclaimed
for hours across 2026-09-16 and 17 while the runner ran other jobs. An earlier
version of this section read that as proof that *nothing* serves `docker`. **It
is not.** A job stays queued when no *available* runner claims it, and a runner
can advertise a label while offline, disabled for this repository, or at
capacity — queue state does not report what labels exist. What the samples
established is the narrower and still useful fact that **no runner claimed
`docker` jobs for this repository over that window**, which is why the gate
stalled and why it could not have been diagnosed as a busy queue.

**The maintainer cancelled them on 2026-09-17**, along with the three stalled
`reboot-baseline` runs — nothing is queued against `docker` any more. This read
*will never run and should be cancelled*, which outlived the action it asked for.

**An earlier deviation, closed by G25 rather than by the rename.** The runner
advertised `docker`, which was the label `reboot-baseline` asked for on
`pull_request`. This document said a pull request *could* be scheduled onto this
host. **The evidence says they were, and the argument is worth stating rather
than asserting**: runs 809 to 834 were claimed uninterrupted at a steady 22s
before P11a and 38s after, and every run after the rename went unclaimed. For
those pull requests to have run somewhere other than this host, every other
runner advertising `docker` would have had to stop at the same instant this one
stopped advertising it. That is not proof, and it is the reading the timing
supports.

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

2. **The label is what R1 requires**, read from the runner list by someone who
   can see it. The list is owner-only, so this step is the maintainer's and its
   result is recorded in § What is deployed rather than produced by a job.

   **This step used to try to establish the label empirically, and no empirical
   test can.** Dispatching `runs-on: docker` jobs and watching them queue shows
   that nothing *claimed* them; a runner that advertises `docker` while offline
   or unavailable to this repository produces the identical observation. The
   configuration is readable directly, so reading it is the evidence and a probe
   is at best corroboration.

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
registration**. The symptom is not an error: the runner stops claiming jobs, so
they queue indefinitely and a pull request waits on a check that will never
report. G24's rename produced exactly this shape without a rebuild, and
§ Verification is why it is stated as the runner's behaviour rather than as a
conclusion about which labels exist.

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
