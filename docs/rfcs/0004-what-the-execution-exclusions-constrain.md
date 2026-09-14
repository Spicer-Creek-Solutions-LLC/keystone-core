# RFC 0004: What the execution exclusions constrain

- **Status:** Accepted
- **Decision date:** 2026-09-15
- **Decision owner:** Project maintainer
- **Amends:** [RFC 0001](0001-generation-2-reboot.md) § Implementation
  governance, and the execution boundary it froze
- **Settles:** the question [`ADR-0007`](../adr/0007-safe-execution.md) § 2.1
  raised and declined to answer
- **Unblocks:** **P07**'s completion and **C07**

## Summary

The execution boundary lists forms the first release does not have. It has never
said whether those forms are **things Keystone builds** or **things that may
happen on the host**, and the two readings produce different products.

This settles it: **the exclusions constrain what Keystone constructs on the
operator's behalf.** They do not constrain which programs an operator may name.

An operator may name a shell as `argv[0]`, or an executable file whose kernel
handler is an interpreter. Keystone still builds no shell invocation, accepts no
script body, writes no file to execute, and chooses no interpreter. Items 2 and
4 to 8 are untouched, and the list stays at eight.

## Motivation

### The question, stated so it cannot be waved away

`keystone run --agent a -- /bin/sh -c 'systemctl restart nginx; tail -1 /var/log/x'`

The command's argv is the vector `["/bin/sh", "-c", "…"]`. Keystone passes it to
`execve` unmodified. `/bin/sh` then interprets its own argument, which the
operator wrote as a shell program.

Is that *a shell that interprets the command's argv*? On the literal words, yes —
a shell is interpreting element 2 of a command's argv. On the reading that the
exclusions describe what Keystone provides, no — Keystone provided nothing; the
operator named a program and it ran.

The same question arrives without anyone naming a shell:
`keystone run -- /usr/sbin/service nginx reload`, where `service` is a shell
script on most distributions and the kernel invokes an interpreter that nobody
selected.

### Why it had to be settled rather than left

`ADR-0007` could not decide it. `D-P07-1` of P07's dossier says the task inherits
the boundary and may narrow it, never widen it, and choosing the permissive
reading inside an ADR is widening by interpretation. **P07 drafted it both ways
and withdrew both**, which is in that task's acceptance evidence, and then
stopped: `ADR-0007` § 2.1 records the two readings and decides neither, P07's
epic entry stayed unticked, and C07 was recorded as blocked.

That is the process working. It is also not a state the project can stay in: the
two readings produce different executors, so no implementation task could start.

### Why this reading

**It is what RFC 0001 removed.** Generation 1 offered `--shell bash`, which made
*Keystone* wrap a command string in a shell. That flag is the form item 1 took
away. Nothing in the reboot's record describes a mechanism for refusing a program
an operator named, and no accepted document has ever specified one.

**The strict reading defeats the charter's own problem.** `PROB-1` is *running
one command on one known host*. Under the strict reading the agent must refuse
any resolved `argv[0]` carrying a `#!` line, which rejects `service`, `ldconfig`
and a large part of a distribution's administrative surface — the work the
product exists for. Refusing every binary that *could* interpret something is an
allowlist of permitted programs, which is a policy engine RFC 0001 discarded.

**And the strict reading buys nothing against the actor it would target.**
`ACT-9`'s stated capability is *execute on the fleet under legitimate authority*.
A caller who cannot name `/bin/sh` can name `/usr/bin/python3`, `/usr/bin/perl`,
`/usr/bin/env`, or a binary they wrote. The boundary would cost the product its
ordinary work and would not remove a capability from the attacker it is aimed at.

### What this does not claim

It does not claim the permissive reading is free. § Security Considerations
states what it costs, and `THR-54` records it.

## Design Overview

Two items are reworded. Neither list grows or shrinks.

| Item | Before | After |
|---|---|---|
| 1 | a shell that interprets the command's argv | **a shell that Keystone interposes** — Keystone builds no shell invocation, wraps no argv in a string, and adds no `-c` |
| 3 | scripts | **a script Keystone accepts or writes** — Keystone takes no script body, writes no file to execute, and chooses no interpreter |

Items 2, 4, 5, 6, 7 and 8 — stdin streaming, pipelines, a caller-provided
environment, an arbitrary working directory, batch fan-out, an interactive
session — are unchanged and unaffected.

## Detailed Design

### What an operator may do

- Name a shell as `argv[0]`, with its program as a following argument.
- Name an executable file whose first line is `#!`, so the kernel invokes the
  interpreter that file specifies.
- Name any other program the target account may execute.

In every case the operator writes the whole vector and Keystone passes it
through. Nothing is added, concatenated, quoted, split or re-interpreted.

### What Keystone still never does

- Accept a command **string** and split it.
- Concatenate argv elements into one argument.
- Add `-c`, `-e`, or any interpreter flag the operator did not write.
- Accept a script **body** and write it to a file to run.
- Choose an interpreter for a file the operator named.
- Select the user, environment or working directory beyond what
  [RFC 0003](0003-caller-selected-execution-user.md) and `ADR-0007` decide.

**The distinction is authorship.** Every element of the vector that reaches
`execve` was written by the operator. That is the property `ARCH-EXEC-001`'s
argv-only surface exists to hold, and it is untouched by which binary element
zero names.

### Why this is not a widening of what a caller can do

A caller who can run one command can run any program the target account may
execute, including an interpreter. This RFC does not grant that; it was already
true of any argv-only executor and is what `ACT-9`'s row already describes.

What the RFC changes is **what the documents say**, which had been ambiguous
enough that P07 could not proceed. That is a real change and it is worth an RFC —
but it is a change to the record, not to the reachable behaviour.

## Compatibility & Upgrade Impact

**Not a breaking change and not a change to any running system.** Generation 2
code begins at P11; there are no deployed agents, configurations or schemas.

No version bump.

**Compatibility with accepted decisions.** `ADR-0007` becomes completable: its
§ 2.1 resolves to this reading, its § 2 rows stand as written because they
already describe what Keystone constructs, and P07 can be ticked. No other ADR is
affected — `ADR-0002` through `ADR-0006` decide transport, identity, subjects,
envelopes and lifecycle, none of which mention execution forms.

**`docs/dossiers/P00-acceptance-evidence.md` remains superseded**, as
[RFC 0003](0003-caller-selected-execution-user.md) § Compatibility recorded. Its
`AC-3` asserts the charter's excluded-form list covers all nine forms with the
nine hardcoded; that was true at P00 and has not been true since RFC 0003. This
RFC rewords two entries and does not change the count, so it adds nothing to that
supersession. The gap RFC 0003 raised — that nothing in the process says what
happens when a later decision invalidates accepted evidence — is still open and
still unowned.

## Migration Plan

None required. The one task blocked on this question, P07, completes by resolving
`ADR-0007` § 2.1 and ticking its epic entry; that is P07's PR, not this one.

## Alternatives Considered

**The strict reading, with a refusal mechanism.** Rejected on the evidence above:
it rejects `service` and `ldconfig`, it escalates to an allowlist, and it removes
no capability from `ACT-9`. It is stated here rather than dismissed because it is
the reading the charter's literal words support, and a reader who reaches for it
deserves to find why it was not taken.

**Leaving it ambiguous and letting each task decide.** Rejected. It is what
produced three withdrawn drafts in P07 and would produce a different answer in
C07, C14 and P10.

**Deciding it inside `ADR-0007`.** Rejected, and P07 was right to refuse. An ADR
that resolves a boundary question in its own favour has widened the boundary
without the amendment the boundary requires — which is exactly what `D-P07-1`
exists to prevent, and what the first two drafts did.

**Narrowing item 1 to name specific interpreters.** Rejected. Any list of
forbidden binaries is a list to be worked around, and maintaining it is the
policy engine again.

## Security Considerations

### `THR-15` is settled, not weakened

Its subject is *supplied argv **escapes** the bounded surface — shell
metacharacters, a script, a pipeline*. The word is **escapes**, and escape is
what argv-as-a-vector prevents: no element becomes a second command, no
metacharacter acquires meaning, no quoting error changes what runs.

A caller who names `/bin/sh` has escaped nothing. They ran what they wrote.

### What this reading costs, recorded as `THR-54`

An operator can be handed an opaque payload. `keystone run -- /bin/sh -c '<200
characters>'` is harder to review before running than
`keystone run -- systemctl restart nginx`, and `ACT-9`'s route is explicitly *an
operator misled into running it*.

Its controls are disclosure rather than prevention:

- **The full argv reaches the audit record** (`ARCH-OBS-001`, `ADR-0007` § 11),
  so what ran is attributable and reviewable afterwards.
- **It exceeds no authority the caller held.** An operator who can be misled into
  pasting a shell program can be misled into pasting a binary's arguments.
- **The execution user is named and recorded** (RFC 0003), so a misled operator's
  blast radius is the account they chose.

**It is not a residual risk.** A residual risk is an accepted gap with an owner
and an expiry; this is a property of granting execution at all, and no control in
or out of scope would remove it. Recording it as a risk would imply a future in
which it is closed.

### What is unchanged

`RSK-9` is untouched: it concerns a compromised agent host, and nothing here
changes what such a host can do. `ADR-0007` § 7's privilege split, § 4's fixed
`PATH` resolution and § 5's harvest bounds all stand unaltered.

## Observability & Operations

No new logging or metric. `ADR-0007` § 11 already records argv and the execution
user; this RFC changes nothing about what is recorded, and § 11's stated limit —
**a secret placed in argv is a secret placed in the audit record** — becomes more
likely to bite, because a longer argv has more room for one.

## Rollout & Adoption

No flag, no phased enablement. The amendment takes effect on merge. P07
completes against it, and C07 is the first code measured by it.

## Open Questions

**None blocking.** One is recorded: whether the charter should say plainly that
an operator may name an interpreter, or whether stating what Keystone constructs
is enough. This RFC does the second, on the grounds that a boundary is best
written as a constraint on the product rather than as a list of things a user may
do. If a reader arrives at the strict reading anyway, that judgement was wrong
and the charter should say so outright.

## Prior Art

Ansible's `command` module runs a vector and its `shell` module runs a string;
the distinction is which one *Ansible* builds, and `command: /bin/sh -c "…"` has
always been permitted. systemd's `ExecStart=` is argv unless `sh -c` is written
explicitly. Both draw the line this RFC draws, in the same place, for the same
reason: the danger is a tool constructing a command line, not a user naming a
program.

## Decision

- **Outcome:** Accepted.
- **Notes:** Accepted on merge by the project maintainer. The scope is two
  rewordings and no change to the number of excluded forms. The strict reading
  was considered and rejected for the reasons in § Alternatives; `THR-54` records
  what the accepted reading costs.

## References

- [RFC 0001 — Generation 2 reboot](0001-generation-2-reboot.md)
- [RFC 0003 — A caller-selected execution user](0003-caller-selected-execution-user.md)
- [ADR-0007 — Safe execution](../adr/0007-safe-execution.md)
- [PRODUCT-CHARTER.md](../project/PRODUCT-CHARTER.md)
- [THREAT-MODEL.md](../project/THREAT-MODEL.md)
