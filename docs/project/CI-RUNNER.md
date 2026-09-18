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

## What the probes established

**Three probe events, and they are not the same thing.** Review of #343 found
them conflated — the dates disagreed and "two dispatched runs" named two of them.
Times are the forge's, `+02:00`.

| Event | Runs | When | What it produced |
|---|---|---|---|
| **P1** the original probe | `828`, `829` | 2026-09-15 22:08, 22:11 | the table below — `docker version`, the socket, `CAP_SYS_ADMIN`, `unshare` |
| **P2** verification | `835`, `836` | 2026-09-16 12:57, 13:02 | § What verification has established — the `v`/`w` series, `R3`'s marker test, `R1`'s samples |
| **P3** the diagnostic probe | `876` **attempts 1 and 3** | 2026-09-18 00:42 and 01:07 | § What is deployed — the image, hostnames, and both socket paths either side of the mount |

**P3 is one run re-run, not two runs.** Attempt 2 (01:01) is not cited: attempt 1
is the before and attempt 3 the after.

### P1 — the original probe

Two dispatched probe runs, 2026-09-15. Job conclusions rather than logs, because
this forge's API does not expose job output.

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
| ~~**`/var/run/docker.sock` exists**~~ | **superseded.** Withdrawn at G31 because the probe asked about a path that need not exist; G32 measured both and found **neither present then, both present now** |
| a Docker client can be installed | succeeds |
| `CAP_SYS_ADMIN` is held | fails |
| `unshare --net` | fails |
| the Go toolchain works | succeeds |

**The socket row was withdrawn at G31 and is now superseded by measurement.**
This host's socket is `/run/docker.sock`; the old probe tested only
`/var/run/docker.sock`. `/var/run` is *usually* a symlink to `/run` — and in
`node:22-bookworm` it is — so the original negative happened to be sound. **The
test was still unsound and withdrawing it was still right**: it asked a question
whose answer depended on an image detail nobody had checked. G32's probe reports
both paths, and both now read present.

**The capability rows stand and no longer matter.** A job holds no
`CAP_SYS_ADMIN` and `unshare --net` fails, so it cannot start a daemon of its own
— which the chosen shape does not ask it to. See § How a job gets Docker.

**`w3` was right, and G31 was wrong to doubt it.** It concluded *jobs run inside
a container* from `/.dockerenv`, and G31 objected that a containerised runner
would produce the same reading. The objection was sound in general and false
here: **the runner's own log shows it `docker create`-ing a container per job**,
and P3's two cited attempts reported different hostnames. Recorded because doubting a
correct result is as much an error as trusting a wrong one, and this document has
now done both about the same probe.

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
| R6 | **Docker works inside a job** | `ADR-0010` § 2 requires the isolation to be *proved by a probe* rather than asserted — and proved in both directions. **How** a job reaches Docker is § How a job gets Docker's to decide, and it has changed once; this row states the property, not the mechanism. It read *through a daemon of the job's own* and **not the host's socket**, which is now the declined shape | § Verification, step 3 |
| R7 | **What was installed is recorded** | So a rebuild reproduces this host rather than becoming a first install again. This is the trust anchor the checksum is not | § What is deployed |

## How a job gets Docker

**The host's socket, mounted into the job's container.** Applied by the
maintainer between P3's attempts 1 and 3, **2026-09-18**, and measured either
side: see § What is deployed.

| Shape | Assessment |
|---|---|
| **Jobs run on the host**, where Docker already is | **Considered and declined.** It gives up `R3` — a marker written to `/tmp` survives into the next job — and `R3` is the only thing separating one job from the next |
| **A container with the host's socket mounted in** — **chosen** | Keeps `R3`'s filesystem isolation. Containers the job creates are **siblings on the host daemon** and outlive it unless the suite cleans up, so cleanup is the suite's job and has to be visible there. Keeps the host's **image cache** |
| A daemon per job | **Was chosen at G23, now declined.** It removes the sibling problem — and costs the image cache entirely, needs privileged containers, and needs a client in the image anyway |

**Why the choice moved, since G23 argued the other way.** The case for a per-job
daemon was that job code *"never touches the host's socket at all"*. `R5`, two
rows above in § What this project requires, says the quiet part: **the runner
needs the Docker socket, that is root on the host, and "sandboxing inside a job is
decoration; the host is the boundary."**

**Once a job can reach Docker by any route, the container around it is not a
security boundary** — dind included, since the daemon it talks to is started by a
runner that holds the host's socket. So the per-job daemon was buying hygiene,
not safety, and paying the whole image cache for it. G23 weighed it as though it
were buying safety. That was wrong, and the maintainer asked the question that
exposed it: *why run Docker commands in a container instead of on the host?*

**What the chosen shape costs, stated rather than discovered:** a failed run can
leave containers and networks behind on the host daemon, and the next run meets
them. **The suite must tear down what it creates, including when it fails** —
that is a requirement on the suite, and it is where this cost is paid.

**What it keeps:** the host's image cache, so a run does not re-pull what it
already has. G23 accepted *"every run pulls what it needs"* as the price of the
per-job daemon; that price is no longer paid, and the registry-rate-limit trigger
it recorded is moot.

### Rejected: sharing a per-job daemon's storage between jobs

**Recorded because it was the tempting wrong answer to a question that is now
moot**, and an unrecorded rejection gets rediscovered as a good idea.

While a per-job daemon was the design, persisting `/var/lib/docker` across jobs
would have removed the pull cost — and it was **state surviving between jobs,
which is `R3` undone by the back door.** The whole reason for a per-job daemon
was that nothing it holds outlives the job; a shared image store is something it
holds.

The chosen shape uses the host's daemon and therefore its store, deliberately.
**That is not this rejection reappearing**: the sharing here is of images, which
no job writes as part of its work, and `R3` is measured on the job's filesystem —
see § What verification has established.

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
| Installed how (package, binary, container) | *(unset — not observable from a job)* |
| Runner version | *(unset)* — the probe log's `forgejo(version:v12.10.1)` is the **instance** it reports to, not the runner |
| Runner binary digest, if installed as a binary | *(unset — not observable from a job)* |
| **Job image** | **`node:22-bookworm`**, Debian 12. The label carries `docker://`, so each job gets a container from this image |
| **Job user** | **`uid=0(root)`** — which is why the socket's `srw-rw---- root:983` is readable without the job being in group 983 |
| Docker version | *(unset)* — the host's, and no job could ask until a client exists |
| Working directory, user, config location | *(unset)* |
| **Isolation between jobs (R3)** | **holds** — a fresh container per job, observed directly: the runner `docker create`s it, and P3's attempts 1 and 3 reported hostnames `75edc82fe619` and `65c47ef08f31` |
| **Host socket, in the job** | **`/run/docker.sock`, `srw-rw---- 1 root 983`** — absent in P3 attempt 1, present in attempt 3, mounted between them on **2026-09-18**. `/var/run` is a symlink to `/run`, so both paths resolve to it |
| Scope (R2) | *(unset)* |
| Labels (R1) | `keystone-docker`, which the runner list shows on this runner alone — **read from the list by the maintainer, 2026-09-16** |

### What verification has established so far

**P2**, two dispatched runs against the live runner, 2026-09-16. Job conclusions,
since job logs are not readable through this forge's API.

| | Result |
|---|---|
| Jobs run on the runner's own label | **yes** |
| `actions/checkout` works there | **yes** |
| **R3** — a marker written outside the workspace does not survive into the next job | **holds**, and **P3** corroborated it independently: a fresh container per job, two attempts, two hostnames |
| **R6** — Docker usable inside a job | **not met.** P2 established that and no more; **its cause was measured by P3, not here** — `node:22-bookworm` carries no `docker` binary, and the socket became present between P3's attempts on 2026-09-18, which is after P2 and could not have been observed by it |
| **R1** — the label is neither `docker` nor `ubuntu-latest` | **holds**, on the runner list — not on the dispatched samples, which cannot establish it |

**`R6`'s diagnosis, which is why the shape above was chosen.** Jobs run *inside a
container* — confirmed, not inferred — and that container has **neither the
Docker client nor a socket**, so a job cannot reach Docker at all.

**`R6`'s cause is established, and it is one thing.** `node:22-bookworm` carries
**no `docker` binary** — `command -v docker` returns nothing inside a job, while
it returns a path on the ci-agent, because those are two different filesystems.
Everything else a job needs is now present: the host's socket is mounted, and the
job runs as root, so it can open it.

The label carries `docker://`, which is what puts the job in a container at all;
the image it names is what decides whether a client is there. **G23 asked for
`docker:cli`; the deployed label names a Node image**, and that is not a mistake
— `docs-lint` runs `npx markdownlint-cli2`, and `actions/checkout` and
`actions/setup-go` are JavaScript actions needing Node in the container. Neither
stock image carries both: `docker:cli` has the client, compose and git but no
Node; `node:22-bookworm` the reverse. Measured, not assumed.

**So the client is the repository's to supply, not the deployment's**, and it
lands the way `reboot-baseline` already installs lychee: a pinned tarball,
verified, in the job. `R6` goes green when a job runs `docker compose version`
against the mounted socket, which is § Verification step 1.

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

**The maintainer cancelled them on 2026-09-17.** Counted two ways, because the
first draft of this paragraph counted them twice: **four runs** — `835`, `839`,
`840` and `842` — carrying **seven queued jobs**, the four `docker`-labelled
samples in `835` plus one `verify` job in each of the three `reboot-baseline`
runs. Three further runs, `837`, `838` and `841`, were already cancelled by the
workflow's own `cancel-in-progress` when later commits landed on the same
branches; they were superseded rather than stalled and are not part of either
count. Nothing is queued against `docker` any more.

**Nothing above asks for anything.** Until G29 this paragraph ended with a
request for that cancellation, and the request stayed in place after it was
carried out — so the document went on asking for work that was already done.
That is the defect G29 corrected, and the sentence you are reading is a record
of it rather than a further request.

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

   **Diagnose before changing anything.** `.forgejo/workflows/runner-probe.yml`
   is a manual `workflow_dispatch` job that reports what a job can see — the
   client, **both** socket paths, `docker version` split into client and server,
   the capability, and enough of `/proc/1` to tell a per-job container from the
   runner's own. It **prints**; nothing in it can pass or fail, because a
   diagnostic that stops at the first missing thing hides everything after it.
   Read the log in the web interface and record the values in § What is deployed.

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
