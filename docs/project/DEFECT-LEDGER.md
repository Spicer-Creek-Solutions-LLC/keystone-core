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
| `Adopted` | Exercised on a later task where the defect could have occurred, and it did not |
| `Held` | Recurred, the countermeasure caught it |
| `Failed` | Recurred *despite* the countermeasure — the countermeasure is wrong or unusable |

**The status describes the current countermeasure, which means it can be
laundered.** Revise a countermeasure after it fails and the entry becomes
`Proposed`, and the record of the failure disappears. Left unguarded, that would
let a ledger of eight defects show no failures at all.

`Proposed` becomes `Adopted` only when the countermeasure has been *exercised on
a later task* — that task presented the chance for the defect and it did not
occur. Time passing is not evidence, the author intending to follow it is not
evidence, and **continued use inside the task that recorded the defect is not
evidence either**: a countermeasure cannot be credited by the same task that
produced the thing it is meant to prevent.

So every entry also carries **`Last recurrence`**: the latest occasion the
defect is known to have occurred. A recurrence is a fact about the world; a
status is a claim about the countermeasure in force today, and editing the
second must not erase the first.

The field is **never cleared**. It is replaced only by a *later* occurrence,
which is why it says "last" rather than "most recent since the countermeasure
changed".

**There is no `None`.** An entry only belongs here if the defect has occurred —
that is the first rule in § "What belongs here" — so every entry has an
occurrence to record, and an empty field means the entry has not been kept up to
date rather than that nothing happened. Whether the occurrence left a commit to
cite is a separate question, answered by the instance's own reference or its
justified exemption.

## Entries

| ID | Defect class | Status |
|---|---|---|
| `DL-1` | A check that cannot fail, mistaken for evidence | `Proposed` |
| `DL-2` | An acceptance property tested through a proxy | `Proposed` |
| `DL-3` | A factual claim never compared with its source | `Failed` |
| `DL-4` | The record and the branch go unchecked | `Failed` |
| `DL-5` | A check verified only against fixtures | `Adopted` |
| `DL-6` | Shell-hostile command construction | `Proposed` |
| `DL-7` | A substitution that did not do what it claimed | `Proposed` |
| `DL-8` | A finding repaired only where it was pointed out | `Failed` |

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
that fires on every defect is not discriminating. **And apply `DL-7`'s three
assertions to the planting step itself** — without them this procedure cannot
tell a check that does not fire from a plant that missed. Normative for
dossiers in [`REBOOT-EXECUTION-PLAN.md`](REBOOT-EXECUTION-PLAN.md)
§ "Required task dossier".

**Last recurrence.** This pull request — the planting step missed its target and
the procedure as written could not distinguish that from a check that does not
fire. Recorded as `DL-7`.

**Status: `Proposed`.** The core measure found `AC-5` in P00 and has surfaced
nine defective checks since, but it recurred as above, so the countermeasure a
reader should follow is now the core *plus* `DL-7`'s assertions.

Those assertions have been exercised twice and caught both times, but they were
exercised catching `DL-7`'s class rather than preventing `DL-1`'s — and `DL-7`
itself then recurred uncaught in a step they did not cover. The strengthened
procedure has not been applied to a later task's acceptance cases, which is where
a check that cannot fail would actually slip through. P01 is the first occasion.
Judging it `Adopted` on this task's own evidence would be the laundering this
schema exists to prevent, applied to the entry that starts it.

### `DL-2` — An acceptance property tested through a proxy

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

**Root cause.** The check tested a proxy rather than the property the case
claims — a string is present, a pattern matches, an identifier resolves, one of
two duplicated fields agrees. Prose is the worst case, because it has no stable
structure and the regex is always wider or narrower than the claim. But
structure alone does not fix it: a table can be parsed correctly and still be
checked in only one of the places a fact appears.

**Countermeasure.** Make the document carry the structure the check needs — a
markdown list, a table, a stable identifier — then parse that structure and
treat unparseable input as **failure**, never as absence of evidence. Worked example: [`GLOSSARY.md`](GLOSSARY.md) § "Reader aids" is a list rather
than a prose sentence precisely so its check reads list items; the entry table
in this document exists for the same reason.

**And where a fact appears twice, check that the copies agree.** Structure makes
a fact parseable; it does not make a check look at every copy of it. `AC-1` now
compares the summary status against each entry's own `Status` field, which is
the property it had been asserting through a proxy. When a case false-positives, preserve the
property and fix the matching — do not substitute a narrower property and call
it a restatement.

**Last recurrence.** This pull request. `AC-1` compared the summary table's
*class* against each heading and treated that as agreement between table and
entry, while never comparing the *status* — so a substitution that swapped two
entries' status blocks passed every check. The document carried structure and
the check parsed it; it simply read one of the two duplicated fields.

Before that, G01, where a prose-regex check was written two tasks after the
lesson was recorded.

**Status: `Proposed`.** The countermeasure changed once already, from "remember
to parse structurally" to "make the document carry the structure", and held
across G01 into G02. It then failed on the duplicated-field case above, so it has
been extended a second time. The first extension worked because
[`GLOSSARY.md`](GLOSSARY.md)'s reader-aids list was converted to a markdown list
the check parses as list items — the document changed, not only the intention.
The second extension, comparing every duplicated field, is untested by a later
task.

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

**Last recurrence.** `ac7974c88` — two stale cross-references written into this
document: `DL-1` asserting that `DL-7` is `Held`, which had stopped being true in
the same commit, and `DL-7` claiming to be the only occasion a countermeasure
caught its own class, which `DL-6`'s `&&` falsifies. Both are claims about the
document's own contents that were never compared against them. Fixed in
`4674ab86c`, found in review.

Before that, `22fbcab82` — `DL-6` written asserting that every commit since is
made with `git commit -F`, which a commit object does not record.

Before that, P01's pull request, where four stale claims shipped in a
description hours after the practice was adopted for document bodies.

**Status: `Failed`.** An earlier draft claimed the countermeasure was widened
after the P01 failure and that the recurrence predated the widening. `git blame`
does not support that: the concrete-command countermeasure originates in
`22fbcab82`, the ledger's first commit, and has not changed since — the same
commit that carried the unverifiable `git commit -F` claim. The class then
recurred twice more in `ac7974c88`, with that countermeasure in force.

The claim that made this entry `Proposed` was itself a `DL-3` instance: a
statement about the repository's history, never compared against the history.

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

**Last recurrence.** Pull request #289, repeatedly. Most recently, a description
that still counted four `DL-7` occurrences after a fifth had been reported in the
thread. Before that, two stale acceptance statements found in review: `AC-1`
claiming four required fields where there were five, and `AC-3`'s limitation
still describing a recurrence as a `Failed` status one commit after that model
was replaced.

Before that, #287: a locally-created merge commit with no signature and no
trailers, alongside four stale claims in the description, fixed at `a2dfd383a`.
**This entry has now recurred repeatedly on the pull request that documents
it**, which is the clearest evidence available that its countermeasure does not
work — hence `Failed` below rather than a revision.

**Status: `Failed`.** The countermeasure is unchanged and has now been tested
three times on this pull request alone — each review round that found a stale
statement here was an occasion for step 1 to work, and it did not.

It is left unrevised deliberately. Step 1 is "re-read the description against
the current diff", an unaided-attention instruction of exactly the kind `DL-3`
already failed at; replacing it with a differently-worded unaided instruction
would move this entry to `Proposed` while changing nothing. A working version
would have to be mechanical — comparing the description's claims against the
changed files — and that is not written. Recording the failure is the honest
state until it is.

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

**Last recurrence.** R09. Exercised since on a later task: R10's audit
re-measured the archive bundle against the live repository with
`GIT_NO_REPLACE_OBJECTS=1` rather than against a recorded figure, and queried the
live forge for branch, tag and release state. That was an occasion for a
fixtures-only check and it did not occur.

**Status: `Adopted`.**

### `DL-6` — Shell-hostile command construction

**Instances.** Backticks in `git commit -m` bodies, substituted by the shell —
recurred four or more times across Generation 1 sessions. A lint piped through
`tail` while gating on its exit status, which yields the exit status of `tail`.

And commit `5449b3f90`, pushed while `docs-lint` was red. `make check` ran,
reported `MD031`, and the commit and push that followed it in the same command
sequence proceeded anyway, because nothing made the sequence stop. The
countermeasure as written covered pipelines and did not cover sequences, so it
did not prevent this. Fixed in `09b565a97`.

*No repository reference (first two instances only):* both are defects in
commands an agent runs, caught before anything was committed, so there is no
artifact to cite. The third instance is committed and cited above. They are recorded because they recur. Note that the first countermeasure is a
**prescribed practice, not an observable fact**: a commit object does not record
how its message was supplied, so "written with `git commit -F`" cannot be
verified from history. The second is observable — [`Makefile`](../../Makefile)
captures `PIPESTATUS` rather than piping a gated command.

**Root cause.** Two distinct causes under one class. In the first two the
failure is **silent** — the shell substitutes or the exit status is swallowed,
and nothing is printed. In the third the failure was **loud and not
propagated**: `MD031` was printed, and the sequence continued to the commit and
push anyway. A gate whose result nothing acts on is the same defect as a gate
that reports nothing.

**Countermeasure.** Write commit messages to a file and use `git commit -F`.
Never pipe a command whose exit status is the gate: capture it, or check
`PIPESTATUS`. **And never let a gate be followed by the action it gates in a
sequence that can continue past it** — join them with `&&`, run under `set -e`,
or capture the status and branch on it explicitly. A gate that runs and is then
ignored is worse than no gate, because its output looks like evidence.

**Last recurrence.** `5449b3f90` on this pull request, pushed while `docs-lint`
was red.

**Status: `Proposed`.** The first two instances were caught by the countermeasure
in force at the time. The third was not — it recurred because that countermeasure
covered pipelines and not sequences.

The widened version has been used since, and on one occasion `make check`
reported `MD012` and the `&&` chain stopped before the commit. That is continued
use *inside the task that recorded the defect*, which the schema explicitly does
not count. A later task has to exercise it.

### `DL-7` — A substitution that did not do what it claimed

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

**Root cause.** A scripted substitution is assumed to have done what its author
intended, and nothing checks. In a document edit the result is a change landing
somewhere it was not meant to. In a planted demonstration it is worse: a mutation
that misses its target produces a clean run indistinguishable from a passing
document, so the demonstration becomes the very thing it exists to detect, one
level up.

**Countermeasure.** Two tiers, because the scopes differ.

*Every scripted substitution*, edit or demonstration:

1. **Scope the replacement to the region it names** — a helper that searches only
   inside the named entry's block cannot land in a neighbour, which a
   whole-document `str.replace` can and did.
2. **Assert the anchor was unique and the document changed** — `re.subn`
   requiring `n == 1`, or compare before and after.
3. **Assert the edit's shape**, by parsing the result: the new text is present
   where it was meant to go and the replaced text is gone. This is a claim about
   *where and what* the substitution wrote, not about what the result now means.
   "Something changed" is not "the change I meant".

*Negative demonstrations additionally*:

1. The mutated document **actually has the defect** — a *semantic* property, not
   a shape one, verified by parsing the result and asserting the broken claim
   directly. The common rules cannot cover this: replacing a countermeasure's
   first sentence lands exactly where intended and writes exactly the specified
   text, so all three pass, while the surviving remainder means no defect
   exists. **The checker under demonstration cannot establish that its own input
   is defective** — treating "it fired" as proof is circular, and was how this
   entry was first written.
2. The reported failure **names the thing you mutated**. A defect planted in
   `DL-5` that reports `DL-6` is a planting bug, not a finding.

**Last recurrence.** This pull request, six times. The most recent was
**caught**: a demonstration case still anchored on `DL-3` being `Proposed` after
it became `Failed`, rejected by the anchor assertion before it could report a
passing run.

Most recently, and **uncaught**: an *editing* substitution whose anchor text
appeared in a different entry than assumed, which swapped `DL-3`'s and `DL-4`'s
status blocks. The three assertions did not catch it because they were written
for the demonstration harness and this was a document edit — the same defect in
a step the countermeasure did not cover.

Earlier and uncaught: a mutation that landed on the wrong entry (the instance
above), and one that replaced only a countermeasure's first sentence — it changed
the document, so the assertion then in force passed, but created no defect; that
is why the third assertion exists and does not rely on the checker.

Caught: two demonstration cases whose anchors referred to text since rewritten,
both rejected by the anchor assertion before they could produce false evidence.

**Status: `Proposed`.** The three assertions were exercised on the two
stale-anchor cases and rejected both before they could report a passing run —
one of only two countermeasures in this task to catch its own class without a
reader, the other being `DL-6`'s `&&`, which has stopped a commit after `MD012`,
`MD038` and `MD029`. But they did not cover document editing, where the class recurred uncaught, so
the countermeasure has been split into two tiers. The demonstration tier is
proven; the scoped-substitution tier that covers ordinary edits is new and
untested, and the status describes the whole.

### `DL-8` — A finding repaired only where it was pointed out

**Instances.** Three. Two in `P01`, in consecutive rounds of the same review on
pull request #290; a third in `G04`, on pull request #293, **after this entry
existed and while its countermeasure was being applied**.

*Siblings left standing.* Round 1 found the blast radius enumerated for `TD-SRV`
and not for the other domain that can author a record, and found two threats
naming an actor without the capability they required. Both were fixed **at the
cited sites** in `95a8b4df1`, which also wrote the actor/control rule into § 5 of
the threat model. Round 2 then found the same two classes elsewhere in the same
document: `TD-AGT` unenumerated, and the new rule violated in § 5.1 and § 5.3 by
the very commit that stated it.

*Dependents left unchecked.* Round 2's fix tightened `ACT-8` from "whatever that
credential authorizes" to exactly one credential holding no Keystone signing or
decryption key. Six threats cited `ACT-8`. Five were re-pointed; `THR-24` was
not, and it went on naming `ACT-8` for a cancellation no modelled credential
authorized — a contradiction the tightening created. Round 3 found it in
`f7c2a0ea5`; fixed in `bcc1003ac`.

*The countermeasure applied, and the class recurred anyway.* `G04` corrected a
rule in `AGENTS.md` that had drifted from the execution plan. Its pull request
claimed this entry's sweep and performed one — deriving the shape as "`AGENTS.md`
restating a rule that drifted from its source" and enumerating within
`AGENTS.md`. Review then found the same rule stated broadly in the epic, and
re-deriving the shape as "any governing document stating the workstream-split
rule" found a third site, `docs/dossiers/P02.md`, which contradicted `P00.md`
and `P01.md` in its own directory. One repository-wide search finds all three.
`69d06b289`; fixed in `d1d909bd7`.

**Root cause.** A review finding is treated as a fact about the instance the
reviewer cited, rather than as a statement about the document, so the fix's
scope is set by the citation instead of by the defect's shape. The second form is
the sharper one: there the fix *itself* changes a definition, and nothing
re-reads what depended on it. A narrow repair also looks complete — the cited
site is genuinely fixed, the reviewer's example genuinely no longer reproduces,
and the checks stay green, because every instance of this class so far has been
semantic.

The third instance moved the root cause one step back. Deriving the shape is
itself a judgement, and **nothing in the first version of this countermeasure
told you whether the shape you derived was wide enough**. The shape was drawn at
the boundary of the file being edited, which is the most available boundary and
has no authority at all.

**Review cannot substitute for the sweep.** The reviewer reads a diff.
`docs/dossiers/P02.md` was outside `G04`'s diff and therefore invisible to a
reviewer doing everything right, which is why this class survives review
repeatedly. The obligation is the author's and cannot be delegated.

**Countermeasure.** Two tiers, because the two forms fail at different moments.

*When acting on a finding*:

1. **Derive the shape, not the instance.** Restate the finding as the property
   that was violated — "a domain named as a total-compromise actor must have its
   blast radius enumerated", not "`TD-SRV`'s blast radius is missing".
2. **Express the shape as a search, and write the command in the reply.** A
   shape you cannot search for is a shape you cannot show you swept. Where one
   genuinely resists expression as a search, say so and state how the
   enumeration was bounded instead.
3. **Run it over the whole repository.** Scoping the search to the file in hand
   is the third instance above, and the file under edit has no privileged
   status: the rule `G04` was correcting lived in three documents, one of which
   was a dossier nobody was editing.
4. **Itemise every hit** in the reply — found, and for each whether it was fixed
   or deliberately left. A sweep that is claimed but not itemised cannot be
   reviewed.

*When the fix changes a definition* — an actor's capabilities, an asset's roles,
a term, a numbered section:

1. **Re-read every row that cites it** before committing. Tightening `ACT-8`
   should have triggered a re-read of all six `ACT-8` rows; it reached five.
2. Treat the definition's citations as the blast radius of the edit. This is the
   document-level form of `DL-7`'s third assertion: "something changed" is not
   "the change I meant", and here "the cited row is fixed" is not "the document
   is consistent".

**Last recurrence.** Pull request #293 — `69d06b289`, fixed in `d1d909bd7`.

**Status: `Failed`.** The countermeasure was in force, was applied, and the class
recurred. That is this document's definition of `Failed`, and the steps above
have been rewritten in response — which is the moment § "Status values" warns
about, because a revision is exactly what would let an entry return to
`Proposed` and lose its failure.

**It does not.** The status stays `Failed` until the revised steps are exercised
on a later task that offered the defect a chance and did not take it. A
countermeasure is not credited for being rewritten; `DL-7` kept `Proposed`
through a revision because its original tier was proven and only the new tier
was untested, and `DL-8` has nothing proven to stand on. `P02` is the first task
that can exercise the revision, and the outcome there is `Held` if it catches a
recurrence or `Adopted` if the class had its chance and did not appear.

**What this entry cannot become.** No check catches this class. All three
instances passed their acceptance cases, `make check`, and a clean
`git diff --check`; the documents were well-formed at every point and wrong
about the world.

The revision narrows the unmechanised part rather than removing it. A written
search command **is** reproducible — a reviewer can re-run it and see the same
hits, which a claim of diligence never allowed — so the sweep is now checkable
rather than merely reviewable. What stays a judgement is the shape itself, and
whether a hit is genuinely the same defect or a lookalike. § "On whether this
document works" names the reason for scepticism: `DL-2` and `DL-3` were written
down and recurred anyway, and `DL-8` has now done it too.

## On whether this document works

Its own efficacy is unproven, and `DL-2` and `DL-3` are evidence against the
premise: in both cases the lesson was written down and the defect recurred
anyway. What is different here is that this record is in the repository, visible
to every agent and to review, rather than in one agent's private notes.

That is a reason to expect better, not a demonstration of it. **Review this
document when Stage P completes:** entries that have not moved from `Proposed`,
or that sit at `Failed` with an unchanged countermeasure, are evidence the
countermeasure is wrong rather than that the defect is unavoidable.
