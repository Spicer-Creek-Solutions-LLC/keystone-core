# RFC 0003: A caller-selected execution user, with its login environment

- **Status:** Accepted
- **Decision date:** 2026-09-14
- **Decision owner:** Project maintainer
- **Amends:** [RFC 0001](0001-generation-2-reboot.md) § Implementation
  governance, and the execution boundary it froze
- **Consequently amends:** [`PRODUCT-CHARTER.md`](../project/PRODUCT-CHARTER.md)
  § 6, and the documents that restate the boundary
- **Owned by:** **P07**, which designs the execution this admits

## Summary

RFC 0001 froze the first release's execution surface as argv-only and
non-interactive, with nine excluded forms. This admits **one** of them and
**narrows** a second:

- A **caller-selected execution user** is admitted. A command names the user it
  runs as; when it names none, a deployment-configured default applies.
- **A shell** is admitted **only** to compute that user's login environment,
  under a fixed command the agent authors. It is never admitted to interpret the
  command's argv.

Seven forms remain excluded unchanged. The boundary stays frozen against
everything else, and widening it still requires an amendment.

## Motivation

### The problem is file ownership, not feature parity

A command that must run as a service account and instead runs as root leaves
the host subtly broken. A dump written to `/tmp` is owned by root; the service
that owns the directory cannot rotate or remove it; the next run fails for a
reason unrelated to the command that caused it. No argv discipline fixes this
afterwards — the ownership is already wrong and a human has to find it.

This is a repeated operational defect rather than a missing feature, which is
the class RFC 0001's own promotion criteria ask for. **Parity with
configuration-management tools is explicitly not the argument**: Generation 1
had `--user`, `--shell`, `--working-dir` and `--env`, and RFC 0001's stated
cause for the reboot was a surface that grew before the core journey was proven.
Admitting a form back because another product has it would be repeating that.

### It is partly a narrowing, and that is the stronger half

Without this amendment, P07 must choose between an agent that executes
everything as root and an agent that executes nothing useful. `PROB-1` — running
one command on one known host without standing interactive access — is not met
by an agent that cannot restart a unit or read a root-owned log, so the
realistic outcome is **every command running as root, permanently**.

With this amendment, root becomes a **per-command, named, audited choice**
against a non-root default. Measured against what the first release would
otherwise ship, the amendment reduces the blast radius of the common case. It is
a widening of the boundary and a narrowing of the privilege actually exercised,
and both are true at once.

### Why the login environment, and not just the uid

The environment is where deployments keep the settings that make a service
account work: `PGPASSFILE`, `ORACLE_HOME`, `MYSQL_HOME`, a `PATH` that reaches
vendor binaries. Those are set in `/etc/profile` and the account's own profile
scripts. A command that runs as the account but without its environment fails in
a way that looks like a Keystone defect and is not one.

### Who is affected

No operator and no deployment today: there is no Generation 2 product code, and
it begins at P11. The affected parties are **P07**, which designs the execution,
**C07**, which implements it, and **C13**, which must create whatever identity
the design requires.

### If we do nothing

P07 decides between the two poor options above, and the first release ships with
every command running as root or with an agent that cannot do the work the
charter describes.

## Design Overview

The execution sequence this admits, stated so the boundary's shape is visible:

1. The agent determines the target user — named by the command, or the
   deployment default.
2. **Environment harvest.** The agent spawns the target's login shell running a
   fixed, agent-authored command that prints the environment, with `su -l`
   semantics so `/etc/profile` and the account's profile scripts execute exactly
   as a login would. The agent captures the result.
3. **Identity change.** `setgid`, then `initgroups` for the target's
   supplementary groups, then `setuid`.
4. **Execution.** The command's **argv vector** is `exec`ed with the harvested
   environment.

**The operator's argv never reaches a shell.** The only thing any shell
executes is the agent's own fixed command and the host's own profile scripts.

## Detailed Design

### What amends the boundary

| Form | Before | After |
|---|---|---|
| A shell | Excluded outright | **Excluded for interpreting the command's argv.** Admitted solely to compute a login environment under a fixed agent-authored command |
| A caller-selected user | Excluded | **Admitted**, with a deployment-configured default |
| stdin streaming, scripts, pipelines, caller-provided environment, arbitrary working directory, batch fan-out, interactive session | Excluded | **Excluded, unchanged** |

**A caller-provided environment stays excluded, and that is consistent.** The
caller supplies no variables. The environment comes from the target account, and
the caller's only influence over it is the choice of account.

### The default user

When a command names no user, a **deployment-configured default** applies.

**The product does not ship root as an implicit default.** A deployment states
its default, and an unstated default is a configuration error rather than a
silent escalation. This is the requirement that makes the narrowing argument
above true rather than aspirational; without it the amendment is a pure
widening. The mechanism — configuration key, installer prompt, or refusal to
start — is **P07**'s and **C13**'s.

### Accounts with no usable shell

A target whose shell is `nologin` or absent has no login environment to harvest.
The agent **falls back to the derived environment** — `HOME`, `USER`, `LOGNAME`,
`SHELL` and `PATH` as the system's user database and `login.defs` define them —
and executes.

It does not refuse. Service accounts are routinely `nologin`, and refusing would
deny the amendment's own motivating case.

### What the harvest must bound

Profile scripts are host-controlled code that the agent causes to run. Each of
these is a requirement on P07, stated here because each is a way the harvest
could damage the execution it exists to serve:

| Concern | Requirement |
|---|---|
| A profile that hangs | The harvest carries its **own timeout**, distinct from the command's deadline. Exceeding it falls back to the derived environment |
| A profile that prints | Harvest output is **never** merged into the command's captured stdout or stderr |
| A profile that fails | A non-zero exit from the harvest falls back to the derived environment; it does not fail the command |
| A profile that returns an enormous environment | The harvested environment is bounded, and exceeding the bound falls back |
| **A profile that carries secrets** | The harvested environment **must never reach an audit record** (`ARCH-OBS-001`). This is the point of harvesting, so it is the case redaction must actually cover |

### What this does not change

`ARCH-EXEC-001`'s deny-by-default for unsupported execution forms is unchanged
and now governs a shorter list. `ARCH-EXEC-002`'s complete process-group
termination is unchanged and applies to the process however it was started.

## Compatibility & Upgrade Impact

**Not a breaking change and not a change to any running system.** There are no
existing configurations, no deployed agents, no schemas and no rolling upgrades:
Generation 2 code begins at P11.

No version bump. `VERSIONING.md` governs released artefacts.

**Compatibility with accepted decisions.** No accepted ADR becomes wrong.
`ADR-0005` carries argv as the command payload and is indifferent to the user it
runs as; `ADR-0006`'s lifecycle states are unchanged; `ADR-0003`'s identity and
key material are untouched. The documents this amendment changes state the
boundary; none of them decide execution, because execution is P07's and P07 has
not been written.

**One piece of accepted evidence is superseded**, and it is named rather than
quietly left: `docs/dossiers/P00-acceptance-evidence.md`'s `AC-3` asserts that
the charter's excluded-form list covers **all nine** forms, with the nine
hardcoded in its checker. After this amendment that check no longer passes.

The evidence is not wrong. It records what was demonstrably true at P00, and
`REBOOT-EXECUTION-PLAN.md` § "Required task dossier" describes evidence as a
record of a demonstration rather than a standing gate. It is therefore **left
unedited**, and this section is the supersession notice.

**That there was nowhere else to record it is a gap**, raised and not fixed
here: nothing in the process says what happens when a later decision invalidates
accepted acceptance evidence. `G16` avoided the question by making its amendment
purely additive; that was not available here.

## Migration Plan

None required, and this is not vacuous: the accepted ADRs were checked against
this amendment rather than assumed compatible, and the one artifact it
invalidates is named above.

## Alternatives Considered

**A deployment-configured user with no per-command selection.** Rejected, and it
was the position this RFC started from. It solves the security half — root stops
being universal — and not the operational half: a host runs commands for more
than one service account, and a deployment that must choose one is back to
choosing root.

**Emulating the login environment without running profile scripts.** Rejected.
Deriving `HOME`, `SHELL` and `PATH` from the user database is cheap and safe, and
it misses exactly what the motivating case needs — the settings a deployment put
in `/etc/profile.d` and the account's profile. It remains the **fallback** for
accounts that cannot be harvested, which is the right role for it.

**Admitting a shell outright.** Rejected. The property that protects `THR-15` is
not that no shell exists on the host; it is that **operator argv is a vector and
not a string a parser interprets**. Admitting a shell for argv would give up
that property to gain nothing the harvest does not already provide.

**Running the command through the login shell** — `su -l user -c '<argv>'`.
Rejected for the same reason, more sharply: it is the shape that turns argv into
a string, and it is how injection defects are written.

**Leaving the boundary frozen and catalogueing the need.** Rejected on the
evidence: the operational defect is concrete, the alternative forces a permanent
root default, and `CAP-AGENT-037` has already held this capability as a Future
candidate since R03 without the underlying problem going away.

## Security Considerations

### `THR-15` is restated, not weakened

Its mitigation read *"No shell exists on the path; argv is a vector, not a
string."* The first clause becomes false and the second is what protected
anything. It is restated as **no shell interprets argv**, and the threat model's
row changes accordingly.

### Running profiles adds no privilege

The profile executes **as the target user**, exactly as that user's own login
would, and the command then executes as the same user. A hostile `~/.profile`
can set `PATH` or `LD_PRELOAD` and influence the command — and can influence
nothing the account could not already influence. **Whoever can write that
profile can already act as that account**, which is the authority the command was
granted anyway.

The one case that deserves naming: if the target is root, root's profile runs as
root. That is not an escalation either, for the same reason.

### What is genuinely new

A caller who holds the operator credential can now name a privileged user for a
command that does not need one. Its controls are the non-root default, the fact
that the choice is **per command and therefore recorded**, and `ARCH-OBS-001`'s
requirement that every lifecycle transition be auditable. The threat model gains
a row for it.

### What is unchanged

`RSK-9` — a compromised agent host is total within that host — is unchanged in
kind. The agent must be able to become other users, so the component doing so
holds the authority to become root. **How that authority is confined is P07's**,
and the privilege separation this makes worth designing is named there rather
than decided here.

## Observability & Operations

The execution user is part of what an operator asked for, so it belongs in the
audit record for the job (`ARCH-OBS-001`). **The harvested environment does
not**, and must not: it is the place the motivating case puts credentials.

A harvest that fell back — timeout, failure, `nologin` — is an operationally
useful fact and should be visible to an operator diagnosing a command that ran
without the environment it expected. Where it is surfaced is **P09**'s.

## Rollout & Adoption

No flag and no phased enablement. The amendment takes effect on merge, and the
first code measured against it is C07's.

## Open Questions

**None blocking.** Two are recorded rather than answered, both P07's:

- Whether the agent holds root permanently or acquires the identity change
  through a smaller privileged component. The amendment requires the capability
  and does not choose the shape.
- What the shipped configuration does when no default user is stated. This RFC
  requires that it not be root; whether the agent refuses to start or the
  installer insists is a design question.

## Prior Art

`su -l` and `runuser -l` define the login-environment semantics this adopts, and
this design deliberately takes their *environment* behaviour without their
*command* behaviour. sudo's `-i` has the same split. Both make the same
distinction this RFC rests on: computing an environment and interpreting a
command line are separate operations that happen to be available from the same
binary.

## Decision

- **Outcome:** Accepted.
- **Notes:** Accepted on merge by the project maintainer, together with the
  amendments it authorises. The scope is deliberately two forms: item 5 admitted
  outright, item 1 admitted only for the harvest. An unrestricted shell
  amendment was considered and rejected in the same discussion that produced
  this one.

## References

- [RFC 0001 — Generation 2 reboot](0001-generation-2-reboot.md)
- [RFC 0002 — Every envelope class is signed](0002-signed-envelope-classes.md)
- [PRODUCT-CHARTER.md](../project/PRODUCT-CHARTER.md)
- [THREAT-MODEL.md](../project/THREAT-MODEL.md)
- [FUTURE-CAPABILITIES.md](../project/FUTURE-CAPABILITIES.md)
- [P07 dossier](../dossiers/P07.md)
