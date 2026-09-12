# Defect Ledger

Recurring defects in how agents work on this repository, with root causes and
countermeasures. It exists because several of these have recurred *after* being
written down somewhere private, and a checked-in, cross-agent, reviewable
record has a better chance than a note only one agent can see.

**Read this before writing an acceptance check or calling a pull request
ready.** Those are the two places every entry below has bitten.

## What belongs here

An entry records a defect **class** that has occurred more than once, or once
with a cause likely to recur. Not a bug tracker, not a list of every mistake.

**An entry without a countermeasure is not an entry.** Recording that something
went wrong changes nothing; recording what would have caught it is the point.

Each entry carries a concrete instance with a commit, pull request or task
reference, so a reader can check the claim rather than take it. Where a
defective commit no longer exists — rebased away, for instance — the entry cites
the fix and says so, rather than a reference that cannot be resolved.

## Status values

Exactly one value per entry. Where a countermeasure has changed, the current
value is recorded and the history goes in the entry's prose — a compound status
is not a status.

| Status | Meaning |
|---|---|
| `Proposed` | Countermeasure defined, not yet tested by events |
| `Adopted` | In use; no recurrence since |
| `Held` | Recurred, the countermeasure caught it |
| `Failed` | Recurred *despite* the countermeasure — the countermeasure is wrong or unusable |

`Failed` is the most valuable value in the table. Two entries below carry it,
and both are cases where writing a lesson down did not prevent the repeat.

## Entries

| ID | Defect class | Status |
|---|---|---|
| `DL-1` | A check that cannot fail, mistaken for evidence | `Adopted` |
| `DL-2` | An acceptance property expressed as pattern-matching over prose | `Adopted` |
| `DL-3` | A factual claim never compared with its source | `Failed` |
| `DL-4` | The record and the branch go unchecked | `Proposed` |
| `DL-5` | A check verified only against fixtures | `Adopted` |
| `DL-6` | Shell-hostile command construction | `Held` |
| `DL-7` | A demonstration that did not demonstrate | `Proposed` |

### `DL-1` — A check that cannot fail, mistaken for evidence

**Instances.** Four in the repository transition, all in R10's audit:
`gofmt -l . | (! read)`, where under `dash` a `read` with no variable is itself
an error so the negation always succeeded
([raw report](../transition/r10/raw/assembler-verification-A05.md)); the
markdownlint glob that covered `docs/**` and `*.md` only, leaving `deploy/`,
`tools/`, `.forgejo/` and the active epic unlinted; the archive manifest's
"content-duplicate" claim, measured with `refs/replace` substitution active so
both sides of every pair resolved to the same object and no other answer was
possible ([audit report](../transition/r10/audit-report.md) `A-05`); and R09
writing "pinned" into two documents and a pull request while the `403` sat in
its own apply log — an intention recorded as an accomplishment
([decision 7](../transition/r10/maintainer-decisions.md)).

**Root cause.** The check was never run against a document or state that should
fail it, so "it passed" and "it cannot fail" were indistinguishable.

**Countermeasure.** Plant a defect that violates exactly the property, confirm
the check reports it, restore, confirm it passes. Record which checks fired: one
that fires on every defect is not discriminating. **Assert that the planted
defect actually landed, and that the reported failure names the thing you
mutated** — see `DL-7`, which is how that gap was found; without those two
assertions this countermeasure has a silent failure mode of its own. Normative
for dossiers in
[`REBOOT-EXECUTION-PLAN.md`](REBOOT-EXECUTION-PLAN.md) § "Required task
dossier".

**Status: `Adopted`.** First applied in P00, where it immediately found `AC-5`
passing the defect it existed to catch, and it has since surfaced nine
defective checks — see `DL-2`. The mutation-landed and targeted-failure
assertions were added after `DL-7` and have not yet been tested by events; the
rest of the countermeasure has.

### `DL-2` — An acceptance property expressed as pattern-matching over prose

**Instances.** Nine across P00, G01 and P01, in both directions. Too loose:
`AC-5` skipped a malformed table row instead of failing it; `AC-1` accepted the
presence of an `**Exit:**` heading as proof of an exact exit code, then accepted
a vague "non-zero" beside exact ones; `AC-3` accepted a `P\d\d` reference as
authority, so "is P03's to decide" — a pointer to a decision *not yet made* —
satisfied a rule demanding the decision that establishes the term; `AC-3`'s
search corpus included [`FUTURE-CAPABILITIES.md`](FUTURE-CAPABILITIES.md), so a
term's presence in the catalogue of what the project decided *not* to do
counted as authority for defining it as current; `AC-1` in P01 checked actor presence while claiming to check motivations too. Too tight:
`AC-1` rejected a correct charter because it matched the token `non-zero`
without distinguishing the remote command's status from Keystone's own exit;
`AC-3` rejected correctly-classified terms whose headings carried expansions the
list omitted. And a parser that over-captured past the end of a prose list into
the following paragraph, silently dropping its last entry.

Evidence in [`../dossiers/P00-acceptance-evidence.md`](../dossiers/P00-acceptance-evidence.md);
fixes in `fd0d20b81`, `b1ff567d0`, `f55a04515`, `dd48d8e57`, `cb2472203`,
`a2dfd383a`.

**Root cause.** The check tested a proxy — a string is present, a pattern
matches, an identifier resolves — rather than the property the case claims.
Prose has no stable structure to match against, so the regex is always either
wider or narrower than the claim.

**Countermeasure.** Make the document carry the structure the check needs — a
markdown list, a table, a stable identifier — then parse that structure and
treat unparseable input as **failure**, never as absence of evidence. Worked
example: [`GLOSSARY.md`](GLOSSARY.md) § "Reader aids" is a list rather than a
prose sentence precisely so its check reads list items; the entry table in this
document exists for the same reason. When a case false-positives, preserve the
property and fix the matching — do not substitute a narrower property and call
it a restatement.

**Status: `Adopted`.** Previously `Failed`: "parse structurally, not by regex
over prose" was recorded after P00 and a prose-regex check was written in G01
two tasks later. It now holds because G01's reader-aids list was converted to a
markdown list that the check parses as list items — the document changed, not
only the intention.

### `DL-3` — A factual claim never compared with its source

**Instances.** `GLOSSARY.md` described JetStream as providing "exactly-once
message delivery", the claim `ARCH-JOB-001` forbids; the same claim had already
reached the product charter twice and been corrected there both times before
anyone looked at its likely source (fixed in `a15b51d37`). An `Attestation`
entry asserted that the only accepted evidence is the one-use token and cited
`ARCH-NATS-004`, an invariant that specifies the staged enrollment protocol and
says nothing about attestation evidence (fixed in `dd48d8e57`).
`docs/dossiers/README.md` told future agents that a control-document amendment
"is itself the authorization", which program rule 1 and
[`AGENTS.md`](../../AGENTS.md) § 3 contradict; it merged, and was then
reproduced verbatim in a later pull request description (fixed in `557b62a29`).

**Root cause.** Not that nothing contradicted the claim — `ARCH-JOB-001`
contradicted the exactly-once claim and [`AGENTS.md`](../../AGENTS.md) § 3
contradicted the self-authorization claim, and semantic review caught both. The
defect is that **the claim was never compared with the text it cites or the rule
that governs it**. Nothing automated performs that comparison, and a diff review
does not either: the diff shows the claim, not the source it is wrong about. A
citation that *resolves* is not a citation that *supports the claim*.

**Countermeasure.** Before recording a factual claim, check it. Read the cited
text and confirm it says what you are about to say it says — `git show <ref>` or
open the file; `grep` the term, the count, the file name; `git merge-base
--is-ancestor <sha> main` before citing a commit, since a rebased-away SHA is
unresolvable for every reader but you. For anything in a control document listed
in [`AGENTS.md`](../../AGENTS.md) § 7, treat the claim as load-bearing: it
licenses the next agent to act, and that agent has no reason to doubt it.

**Status: `Failed`.** Adopted mid-session, applied to a document body, and four
stale claims shipped in the same pull request's description hours later. The
practice is correct; its scope was too narrow. See `DL-4`.

### `DL-4` — The record and the branch go unchecked

**Instances.** A pull request description asserting a superseded acceptance
criterion, which mattered because the same task had just made that description
normative; acceptance evidence labelled with an earlier commit than the merge
candidate, after a later commit changed the corpus the checks read; "both new
dossier files" when the diff added one, present from the first push and
surviving three review rounds; and a locally-created merge commit carrying no
signature and no DCO or attribution trailers, on the branch whose own dossier
cites the signing rule. That commit was removed by rebasing rather than signed,
so it is unreachable; the fix is `a2dfd383a` in pull request #287.

**Root cause.** `make check` inspects files. Nothing inspects the pull request
description, the acceptance evidence's provenance, or the commit history — so
the artifact converges while the record around it drifts.

**Countermeasure.** Before calling a pull request ready:

1. Re-read the description against the current diff — counts, file names,
   criteria, dependencies.
2. Confirm every commit is signed **and** carries both trailers. `%G?` alone
   shows only the signature, so check the trailers explicitly:

   ```
   git log --format='%h %G? sob=%(trailers:key=Signed-off-by,valueonly) cab=%(trailers:key=Co-Authored-By,valueonly)' main..HEAD
   ```
3. **Never `git merge` into a task branch — rebase.**
   [`AGENTS.md`](../../AGENTS.md) § 4 exempts *forge-created* merge commits
   only; a merge an agent makes locally is an agent commit.
4. If the description states acceptance evidence, confirm it was produced at the
   current head, and name what the checks read. Evidence has a dependency graph,
   and a later commit can invalidate it silently.

**Status: `Proposed`.**

### `DL-5` — A check verified only against fixtures

**Instance.** R09's negative test for issue automation passed dry runs and unit
tests, then produced two false positives on first contact with the live tracker:
it flagged issues sharing a closed issue's title as "replacements" when they
were predecessors, because a nightly job had recreated the issue weekly. A
replacement is one created *after* the target closed (`6e755808e`).

**Root cause.** Fixtures encode the author's model of the data. Production data
encodes what is actually there.

**Countermeasure.** Run the check against production data before believing it —
for forge operations that means a dry run against the live tracker, recorded as
an artifact, as [`REBOOT-EXECUTION-PLAN.md`](REBOOT-EXECUTION-PLAN.md) requires
before any apply. A same-value match is not evidence of a relationship: state
the relationship the requirement actually asks about, and test that.

**Status: `Adopted`.**

### `DL-6` — Shell-hostile command construction

**Instances.** Backticks in `git commit -m` bodies, substituted by the shell —
recurred four or more times across Generation 1 sessions. A lint piped through
`tail` while gating on its exit status, which yields the exit status of `tail`.

*No repository reference:* both are defects in commands an agent runs, caught
before anything was committed, so there is no artifact to cite. They are recorded because they recur. Note that the first countermeasure is a
**prescribed practice, not an observable fact**: a commit object does not record
how its message was supplied, so "written with `git commit -F`" cannot be
verified from history. The second is observable — [`Makefile`](../../Makefile)
captures `PIPESTATUS` rather than piping a gated command.

**Root cause.** The command looks right and the failure is silent.

**Countermeasure.** Write commit messages to a file and use `git commit -F`.
Never pipe a command whose exit status is the gate; capture it, or check
`PIPESTATUS`.

**Status: `Held`.** Both recurred and were caught by the countermeasure rather
than by review.

### `DL-7` — A demonstration that did not demonstrate

**Instance.** While authoring this ledger, two planted defects produced no
failures and were nearly recorded as "the check does not fire". The checks were
correct; the **planting script** was wrong. `DL-5` is labelled `**Instance.**`
and the mutation regex matched `**Instances.**`, so it silently landed on a
different entry, and a second mutation replaced only the first sentence of a
justification it was meant to remove. Both `re.sub` calls returned a changed
document, so nothing looked wrong.

*No repository reference:* this occurred while authoring this ledger, in a
throwaway planting script that is not committed — the acceptance demonstrations
for a supporting task live in its pull request, not in the tree. The entry is
recorded because the failure mode is invisible by construction, and because the
check in this pull request caught it happening again to `DL-7` itself.

**Root cause.** `DL-1`'s countermeasure assumes the planted defect is actually
present. A mutation that misses its target produces a clean run that is
indistinguishable from a passing document — the demonstration becomes the very
thing it exists to detect, one level up.

**Countermeasure.** Three assertions, and the third is the one that matters:

1. The mutation landed — `re.subn` requiring `n == 1`, or compare before and
   after and fail if unchanged.
2. The reported failure **names the thing you mutated**. A defect planted in
   `DL-5` that reports `DL-6` is a planting bug, not a finding.
3. The mutation created the **intended** defect, not merely some change.
   Replacing a countermeasure's first sentence leaves the rest in place, so the
   document still passes and the run looks like a check that does not fire.
   Assertion 1 passes in that case; only the check firing on the right target
   distinguishes it.

**Status: `Proposed`.**

## On whether this document works

Its own efficacy is unproven, and `DL-2` and `DL-3` are evidence against the
premise: in both cases the lesson was written down and the defect recurred
anyway. What is different here is that this record is in the repository, visible
to every agent and to review, rather than in one agent's private notes.

That is a reason to expect better, not a demonstration of it. **Review this
document when Stage P completes:** entries that have not moved from `Proposed`,
or that sit at `Failed` with an unchanged countermeasure, are evidence the
countermeasure is wrong rather than that the defect is unavoidable.
