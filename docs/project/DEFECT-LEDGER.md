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
let a ledger of nine defects show no failures at all.

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

**A recurrence is recorded in the pull request that finds it.** Not by a later
task, not by a periodic review. This rule exists because the alternative was
tried and failed measurably: between pull requests #296 and #312 the `DL-8` class
recurred nine times, **each one written up in its own commit message**, and the
entry went on saying "Instances. Four" throughout. `G12` had already stated the
principle at `8a5cddacd` — a finding recorded only in a commit message is not
recorded — and `G07` had already had to correct this entry's own count at
`dda58f462`.

So a task that hits one of these classes updates `Last recurrence` and the
instance record in the same pull request, in the same way it would update any
other document its work made stale. A recurrence noticed and left for later is
the ledger's own `DL-8`.

**State counts so they can be checked, not so they read well.** "Instances.
Four" is a claim with nothing behind it; an itemised list with a count derived
from it cannot drift out of step with what it counts.

**There is no `None`.** An entry only belongs here if the defect has occurred —
that is the first rule in § "What belongs here" — so every entry has an
occurrence to record, and an empty field means the entry has not been kept up to
date rather than that nothing happened. Whether the occurrence left a commit to
cite is a separate question, answered by the instance's own reference or its
justified exemption.

## Entries

| ID | Defect class | Status |
|---|---|---|
| `DL-1` | A check that cannot fail, mistaken for evidence | `Failed` |
| `DL-2` | An acceptance property tested through a proxy | `Proposed` |
| `DL-3` | A factual claim never compared with its source | `Failed` |
| `DL-4` | The record and the branch go unchecked | `Failed` |
| `DL-5` | A check verified only against fixtures | `Adopted` |
| `DL-6` | Shell-hostile command construction | `Proposed` |
| `DL-7` | A substitution that did not do what it claimed | `Failed` |
| `DL-8` | A finding repaired only where it was pointed out | `Failed` |
| `DL-9` | A document that requires work a permission list forbids | `Failed` |

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

**C01-I and C02-I produced instances of this class that never reached this
entry**, and the reason is `DL-9`'s, not inattention: both wrote them up in their
dossiers' § 5.3 and in their pull requests, and no behaviour dossier granted the
path to this file, so no `Cxx` task could record anything here. `G45` fixed the
grant, and the tasks that find recurrences now record their own. The counts
above are known to be low and are **not** reconstructed here — inventing figures
for tasks whose evidence would have to be re-derived is the drift this document
exists against, and the cause is recorded rather than the symptom patched.

**Last recurrence.** `G33` — the container suite's `requireDocker` called
`t.Skip` when the Docker client or daemon was absent, landed at P11b and unread
until G33 moved the suite into `check` and CI. Had it landed in CI as written, a
job whose client install silently failed would have reported a **green** gate:
the skip made *did not run* and *passed* indistinguishable, which is this
entry's headline with the check omitted rather than weak.

**A new gap, and it is about when the countermeasure is applied.** The
countermeasure says to plant a defect and confirm the check reports it. **P11b
could not**: the suite was deferred because no runner could run it, so there was
nowhere to plant anything, and the skip path shipped unexercised behind a target
nothing invoked. A check deferred for want of an environment is never planted
against — and it is the one most likely to be trusted on the day it is finally
switched on, because it has been in the tree looking finished for months. The
rule that follows: **the task that lands a deferred gate plants against it
before trusting its first green**, including the path that decides whether to
run at all. G33 did, in both directions and with the exit codes captured rather
than piped away.

Before that, pull request #323 — `9502f7a66`, and it is the gap below
rather than a weak check. P08's `AC-9` was written in its dossier as two
properties, the second being that every dependent site named in that dossier's
§ 2 is updated. The checker implemented the first and not the second, and its own
comment claimed *"with dependents"*. The case then passed on a document where
`THR-46` contradicted `RSK-4`'s new conclusion. **The claim was in the acceptance
case rather than in the prose this time**, which the entry's gap paragraph did
not anticipate: a case that names two properties and checks one is a completeness
claim with no check behind it, exactly as a sentence would be. Review found it;
the harness could not. Fixed as `AC-9b` in the same pull request.

Before that, pull request #306 — `09bed9bdd`, in a form the
countermeasure does not reach. P04's ADR added the sentence "§ 8 enumerates every
grant in § 4 and § 6" while covering one of the monitoring role's three advisory
grants. Nothing checked the sentence, so it read as evidence and could not fail.
`bc4fe417d` records it as this entry's shape, and the shortfall took two rounds
of review because writing the claim is what made it look settled.

Before that, pull request #289 — the planting step missed its target and
the procedure as written could not distinguish that from a check that does not
fire. Recorded as `DL-7`.

**The countermeasure has a gap the latest instance exposes.** It protects checks
that exist: plant a defect, confirm the check reports it. It says nothing about
**a claim a document makes about its own completeness with no check behind it**,
which is the same defect with the check omitted rather than weak. A completeness
claim needs a case that counts one list against the other, or it is prose.

**#323 showed the claim can live inside the acceptance case itself.** An `AC-`
row that states two properties joined by "and" is read as one case, demonstrated
once, and reported as passing — while only one half is implemented. The rule
that follows: **a case whose statement contains "and" is demonstrated once per
conjunct, or split into two cases.** `AC-9` and `AC-9b` are the split.

**Status: `Failed`.** The strengthened procedure has since been applied to five
task dossiers' acceptance cases, which is the occasion the previous status said
was missing — and the class recurred anyway, in the form above. The core measure
has surfaced defective checks repeatedly and keeps doing so; what failed is its
scope.

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

**Last recurrence.** Pull request #327, in `docs/dossiers/P00.md` — and it is the
**oldest** instance this entry has recorded, found sixteen tasks after it was
written. P00's § 5.3 said *"The evidence lands in the same PR as
`docs/dossiers/P00-acceptance-evidence.md`."* It did not: the dossier landed
in #285 and the evidence in #286. The claim was about **the author's own pull
request**, which is the source a claim is least likely to be checked against and
the one a reader can least easily check for themselves.

**It propagated, which is what makes it worth the entry rather than a typo.**
`docs/dossiers/README.md` — the document that defines what a dossier *is* — told
every later reader the record sits *"alongside the dossier"*, and eight
dossiers copied the paragraph's shape without its artifact clause. Review of
pull request #326 then read a dossier exactly as the README instructed,
concluded the acceptance evidence was missing, and declined to approve. **The
reviewer was right about the documents and wrong about the process**, which is
what a false control-document claim buys.

The countermeasure's own words name the failure: *"For anything in a control
document listed in `AGENTS.md` § 7, treat the claim as load-bearing: it licenses
the next agent to act, and that agent has no reason to doubt it."* Ten dossiers
had no reason to doubt it.

**G21 performed the comparison** the countermeasure asks for, over every
dossier: each one's claim about where its record landed was checked against
`git log`, in both directions. **The checker is reproduced in full in the
description of #327**, which is where a `G` task's record lives, and it would
have failed on the day P00's paragraph was written.

**It is not retained in this repository and nothing re-runs it.** No checker
ever has been: `git ls-files` returns no script, `make check` runs three
documentation gates and the capability catalog, and the only checkers that
survive at all are the inert source blocks inside the P-task acceptance-evidence
files. So every verification this project has performed is a **one-time** one,
and the protection against P00's claim coming back is the same protection that
failed for it — someone remembering to look.

**That was almost this entry's own next instance.** The first version of this
paragraph said G21 *encodes* the comparison, in a control document, in the pull
request whose subject is a false control-document claim. Review of #327 found
it. `P11` is where a checker first has somewhere to live and something to run
it, and until then this limitation is the honest description of every one of
them.

**Also pull request #328**, and it is the same shape at a larger scale. Six
documents described **P10** as something it is not: `docs/dossiers/P02.md` named
it *"deployment and operations"*, `ADR-0003` gave it a fleet-rotation procedure
and a channel-separation procedure, `ADR-0005` gave it a deployment mode twice,
`TESTING.md` gave its artifact-redaction work to P09, and `ADR-0008` and the
epic said it would demonstrate a reconstruction. **There is no
deployment-and-operations task in the plan and nothing runs until P11**, so
every one of those was false when written.

**Three residual risks were gated on that belief.** `RSK-13` was carried to P10
as *"an operational runbook"*, and `RSK-14`'s channel-separated deployment mode
was assigned to P10 twice. A false description of a task became a false
placement of accepted risk, which is how far this class travels when the subject
is a task nobody has run yet: **nothing contradicts it until someone tries to do
the work.**

`TESTING.md`'s wrong owner and the plan's correct one were written **in the same
commit**, `37837bd5f`, on 2026-09-08. The numbering never changed; it was an
off-by-one in one sitting that stood for a week and through nine tasks.

Before that, `ac7974c88` — two stale cross-references written into this
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

**Last recurrence.** Pull request #306 — `bc4fe417d`. A commit message described
an evidence edit the diff did not contain: the script asserted after writing,
aborted, and `git add -A` ran anyway. The message was left standing and the
paragraph landed in a follow-up, because rewriting the branch would have removed
the evidence of the mistake.

Before that, pull request #289, repeatedly. A description
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

**Last recurrence.** Pull request #307 — `5b7761c80`, **uncaught**, in the edit
tier. `G13`'s entry was inserted by anchoring on "the next unchecked item after
`G12`'s", and `G12` is the last supporting task, so the next unchecked item is
the first epic acceptance criterion. The entry landed under "Epic acceptance".
The anchor was unique and the file changed; nothing asserted *where* the new text
went, which is the third assertion applied to placement rather than to content.

Before that, pull request #303 — two of P03's evidence plantings satisfied the
first two assertions and not the third: the predicates asserted that text had
changed rather than that the defect was present. Recorded at `1ee451e9c`, which
carried the lesson into P04's dossier.

Before that, pull request #289, six times. The most recent of those was
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

**Status: `Failed`.** The two tiers have diverged and the status describes the
whole, so it follows the weaker one.

The **anchor assertions keep working**. They have rejected stale anchors before
those could report a passing run at `4600547ab`, `2557a4ff7` and `5243daff9`,
across three later tasks — with `DL-6`'s `&&` they are the only measures here
that catch their own class without a reader.

The **third assertion keeps being skipped**, and both recurrences above are that
skip: P03's plantings asserted that text changed rather than that the defect was
present, and `G13`'s edit asserted that text changed rather than where it landed.
The assertion that cannot be delegated to a shape check is the one that keeps not
being written, which is a usability finding about the countermeasure rather than
a gap in it.

### `DL-8` — A finding repaired only where it was pointed out

**Instances.** Fourteen occasions across eleven tasks — **four** occasions on
**three** tasks in the prose below, and **ten** occasions on **eight** tasks in
the table that follows it. Two in `P01`, in consecutive rounds of
the same review on pull request #290; a third in `G04`, on pull request #293,
**after this entry existed and while its countermeasure was being applied**; a
fourth in `G06`, on pull request #295, **after the countermeasure was revised in
response to the third**.

Every timestamp here is UTC and is the commit's own author date, not a
neighbouring row's.

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

*The revision applied, and the class recurred again — differently.* `G06`
reassigned the subject grammar and permission matrix to P04. Its sweep was
expressed as a search and run over the whole repository, as the revision
requires, and still missed two sites in the very file being realigned. Review
found both. The first was **in the sweep's own output and was deleted from it**:
the published command ended `| grep -v 'DL-2'`, a noise filter that removed the
line `the matrix, the permission matrix, the allowlist…`. The second reads
`permission *matrix*` — markdown emphasis between the two words, which a literal
phrase pattern cannot match on a line-based search. `9a8b2e4e1`; fixed in
`1f796e200`.

*Nine occasions under the second revision, inside a day.* `G07` merged the
revised steps at 19:21 on 13 September. **The first occasion below is at 19:07,
before that merge** — it is a commit on `G07`'s own branch, correcting the
instance count that the same pull request had just made stale, so the second
revision failed before it was published rather than after. The remaining eight
run from `4f51b0d96` at 20:37 that evening to `d0c6e7771` at 14:38 the next day,
which are the first and last rows of the table.

Durations are not stated anywhere in this entry. Every one that was — "one hour
and forty-seven minutes", "eighteen hours", "eleven days" — was computed once,
typed, and wrong or stale by the next commit. A timestamp cited from the commit
it describes cannot drift; a duration derived from two of them silently can.

All nine are recorded in their own commit messages and nowhere else — which is
how this entry came to say "Four" while the class was recurring roughly
hourly.

| When | Task | Commit | What was left standing |
|---|---|---|---|
| 09-13 19:07 | `G07` | `dda58f462` | **this entry's own instance count**, made stale by the pull request that raised it to four |
| 09-13 20:37 | P03 dossier, #301 | `4f51b0d96` | the edit permission had three statements in one file; two were updated |
| 09-14 09:31 | `G12`, #304 | `0710e7fed` | five handoff sites still sent generated artifacts to P04 |
| 09-14 09:40 | `G12`, #304 | `e914a4d43` | four more, in the dossiers' own attributions |
| 09-14 13:06 | `G14`, #309 | `64c03d4a5` | `THR-50` was narrowed; the epic summary kept the wide claim |
| 09-14 13:31 | P05, #310 | `ce7427f3f` | `RSK-6` was widened in § 7; the ADR's own risk row still said narrowed |
| 09-14 13:40 | P05, #310 | `14322fdbe` | fourth round on `RSK-6`; five statements, one fixed per round |
| 09-14 14:13 | `G15`, #311 | `8e06c6378` | the delivery gap stated three ways, and a count in a second document |
| 09-14 14:38 | `G16`, #312 | `d0c6e7771` | the invariant gap, stated a sixth way in a summary table row |
| 09-15 12:03 | P08, #323 | `9502f7a66` | `RSK-4`'s control stopped routing through the server; `THR-46` went on saying it did |

*What the nine have that the first four did not.* The first four were about
**identifiers and rules** — `TD-AGT`, `ACT-8`, a workstream-split rule, a
permission matrix. The dependents tier works on those, because "re-read every
row that cites it" presumes something a reader can search for.

Five of the nine are about a **conclusion**, which has no identifier: `RSK-6`'s
outcome, `THR-50`'s reach, whether the service halves are delivered, the residual
risk counts, and the invariant gap `RFC 0002` has since closed. A conclusion is
restated in
prose, in different words each time, and summary tables exist precisely to
restate it. Step 4 — build the pattern from the rarest single token — has no
token to offer, and what was substituted each time was **the phrasings the author
could remember**, which is enumerating instances one level up from the sites.

The last occasion is the clearest: `G16`'s sweep listed five phrasings of "the
amendment is still owed" and the residual-risk table said it a sixth way.

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

The fourth moved it back again, to the search itself. A search has two
judgements in it beyond where it runs — **what pattern expresses the shape**, and
**which of its hits you look at** — and the first revision addressed neither. In
`G06` the scope was right and the form was wrong: a phrase pattern against
line-wrapped prose carrying markdown emphasis, with a hand-added exclusion
trimming the result.

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
4. **Build the pattern from the rarest single token, not the phrase.** Search is
   line-based and prose is not: a phrase is split silently by a line wrap, by
   markdown emphasis (`permission *matrix*`), and by hyphenation. `matrix` finds
   what `permission matrix` cannot. This repository wraps at eighty columns, so
   any two-word pattern is one reflow away from matching nothing.
5. **Do not filter the output.** Every hit gets a disposition, and an exclusion
   is justified hit by hit in the reply or not made at all. Trimming a sweep's
   output is editing its evidence, and in the fourth instance above the deleted
   line was one of the two defects.
6. **Itemise every hit** in the reply — found, and for each whether it was fixed
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

*When what changed is a conclusion rather than an identifier* — an outcome, a
claim's reach, whether something is open or closed, a count:

1. **Do not search for the old conclusion's phrasings.** There is no canonical
   wording to find. A list of phrasings is a list of instances one level up, and
   it is what failed on five of the nine occasions above.
2. **Search for the conclusion's *subject* instead**, by the rarest token in it
   — `RSK-6`, `public halves`, or the invariant gap `RFC 0002` closed — and
   require **every hit to carry the new conclusion**. The question is not
   "where is the old wording" but "where is this subject spoken about at all".
3. **Encode that as an assertion over every tracked file**, not as a search you
   perform and describe. This is the one form that has worked: `14322fdbe`,
   `8e06c6378` and `d0c6e7771` each ended their task's recurrence, and each did
   it by writing the sweep as a check rather than running one.
4. **Make a table row its own unit.** A stale row must not be excused by a
   citation elsewhere in the same table — that is exactly how the ninth occasion
   survived a sweep that ran over the whole repository.

**Last recurrence.** Pull request #323 — `9502f7a66`, found in review.

**This is the first occasion where the enumeration was correct and was not
applied.** The other thirteen failed at the sweep: the shape was drawn too
narrowly, the pattern could not match wrapped prose, or the output was filtered.
Here the task's own dossier § 2 listed `THR-46` by name, in a table derived by
word-boundary search precisely because an earlier version of it had missed eight
sites — and the edit went to `RSK-4`'s row and stopped.

**Which is the whole argument for the third tier's step 3.** A sweep written as
prose is a sweep performed once, by an author who then has to remember it at
commit time; a sweep written as an assertion is performed on every run by
something that cannot forget. The dossier did the hard half — deriving the
subject and finding the sites — and left the half a machine does better. The
acceptance case that should have closed the gap named the property and did not
implement it, which is the same pull request's `DL-1`.

**Status: `Failed`.** The countermeasure was in force, was applied, and the class
recurred. That is this document's definition of `Failed`, and the steps above
have been rewritten in response — which is the moment § "Status values" warns
about, because a revision is exactly what would let an entry return to
`Proposed` and lose its failure.

**It does not.** The status stays `Failed` until the revised steps are exercised
on a later task that offered the defect a chance and did not take it. A
countermeasure is not credited for being rewritten, and this entry has now been
revised three times without ever earning a different value. #323 did not
rewrite the steps — for the first time the steps were adequate and the task did
not follow them, which is a different failure and does not call for a fourth
revision.

This entry has now been revised three times, and **every revision has failed,
including the one written last**. The first was in force for the fourth
instance. The second was in force for nine more, beginning with `dda58f462` at
19:07, ahead of its own merge at 19:21. **The third was in force for #323**, and
it failed in the way that is hardest to argue with: the task read the third tier,
derived the subject, enumerated the sites correctly in its dossier — and did not
perform step 3, which is the step that says to stop writing sweeps down and start
asserting them. That is the strongest argument available for why a rewrite does
not earn a status: each of these would have sat at `Proposed` while the defect it
was written against recurred.

The second revision's prediction is worth keeping as a record of how wrong a
confident forecast can be. It said `P02` was "the first task that can exercise
the current steps, and the outcome there is `Held` if they catch a recurrence or
`Adopted` if the class had its chance and did not appear." `P02` did not produce
an instance; every task after it did.

**What this entry cannot become.** No check catches this class in general. Every
instance passed its acceptance cases, `make check`, and a clean
`git diff --check`; the documents were well-formed at every point and wrong
about the world. The third tier narrows this: a sweep written as an assertion
*is* a check, and it catches the recurrence of the specific conclusion it was
written for. It does not catch the next conclusion, which has no assertion yet.

The revisions narrow the unmechanised part rather than removing it. What stays
a judgement is the shape, the pattern that expresses it, and whether a hit is
genuinely the same defect or a lookalike — three judgements, and instances three
and four each failed at a different one. The third tier adds a fourth: whether
what changed is an identifier or a conclusion, because the two need different
sweeps and mistaking the second for the first is the whole of the last five
occasions.

**Separate the two things a published command does**, because only one of them
has worked. It has not reliably found the sites: twice now a sweep was run,
published, and incomplete. It *has* made the failure visible to someone else —
in the fourth instance the reviewer re-ran a better-formed search against the
written one and found both missed sites, which no claim of diligence would have
permitted. A countermeasure that fails and is *caught failing* is worth more
than one that fails silently, and that is the honest case for keeping this
approach rather than any claim that it works. § "On whether this
document works" names the reason for scepticism: `DL-2` and `DL-3` were written
down and recurred anyway, and `DL-8` has now done it too.

### `DL-9` — A document that requires work a permission list forbids

**Instances.** Four occasions across four tasks. The first three are one
shape — a task dossier whose decision section required work its **own** § 3.3
did not permit, found only when the implementation began. The fourth widened the
class, which is why this entry is no longer titled for a document's own list: an
obligation can be imposed by a **different** document than the one holding the
permissions.

1. **`C02.md` and `tools/pendingcontract/`.** `D-C02-4` decided that C02 reuse
   C01-A's contract machinery and named the collision that reuse would hit — the
   tool's coupling to a single package. § 3.3, one section away, did not grant
   the path that fix needed. Found at `G39`; granted in `1cd66348c`.
2. **`C02.md` and `cmd/keystone-ledger-crash/`.** `C02-A` specified the crash
   harness binary at that path in `test/contract/persistence/crash_harness.go`.
   § 3.3 listed no path under `cmd/`, so `C02-I` built the binary the
   specification named and was out of scope by the dossier's own rule. Found in
   review of pull request #359; granted at `G41`, `7d8e0f479`.
3. **`C03.md` and `github.com/nats-io/nats.go`.** `D-C03-1` decided how a NATS
   JWT gets **signed** and weighed three ways of doing it. It never weighed how a
   principal **connects**, and § 3.3 permitted only *"the dependencies of
   `D-C03-1`"* — while every one of the thirty-eight `POS` and `NEG` cases
   connects. Found at the start of `C03-I2`, before any code was written;
   granted at `G44`, this pull request.

4. **Every behaviour dossier, and this file.** § "What belongs here" requires the
   task that finds a recurrence to record it in the same pull request. `C01.md`,
   `C02.md` and `C03.md` each name `DEFECT-LEDGER.md` in § 1 as an **input** and
   none grants the path in § 3.3, so every `Cxx` task was required to record a
   recurrence here and forbidden to. Every commit to this file up to `G45` is a
   G or P task, and `DL-1` above says what that cost. Found in review of pull
   request #367; granted at `G45` in
   [`REBOOT-EXECUTION-PLAN.md`](REBOOT-EXECUTION-PLAN.md) § "Required task
   dossier", as one standing statement rather than a line in each dossier.

**This is not `DL-8`.** That class needs a prior finding: someone reports a
defect and it is repaired at the cited site only. Here nothing was reported and
nothing was repaired too narrowly. **Both halves were written by the same author
in the same pull request, reviewed, and merged** — the document was internally
inconsistent on the day it landed, and stayed that way until work started
against it. The detection moment differs too, which is what makes the
countermeasures different: `DL-8` is swept *after* a finding, and this is
checked *before* a document lands.

**Root cause.** A dossier's decisions and its permission lists are written as
separate acts of authorship, and only the first feels like deciding. § 11 asks
what the task should do; § 3.3 asks what it may touch; and the author who has
just settled a hard question does not re-read the list two sections away to ask
whether it now says no. The defect is invisible to every reader of the decision,
because the decision is right — which is why all three instances survived review
and were found by whoever tried to do the work.

**Countermeasure.** Two steps, at the two moments the inconsistency can be seen.

*Before a dossier lands*, the author derives § 3.3 from the rest of the document
rather than writing it independently: for every output in § 3.1 and § 3.2 and
every decision in § 11, name the paths and dependencies it requires, and check
each against the list. The pull request states that this was done. **Deriving is
the point** — a list written from memory of what the task will touch is a second
copy of the plan, and the copy is what goes stale.

*At the start of every implementation task*, before writing code, enumerate the
paths and dependencies the plan needs and diff them against § 3.3. This is
cheap, it is the moment the mismatch becomes concrete, and it is how the third
instance was found. **A task that discovers the gap here stops and asks** rather
than widening the list under its own approval, which is `AGENTS.md` § 3's rule
and the reason all three repairs are G tasks rather than quiet edits.

*When an obligation arises DURING a task*, the paths it needs were not in the
plan and the step above cannot have checked them. Instance 4 is exactly that: a
task incurs the duty to record a recurrence by **finding** one, which is not
something its plan can enumerate in advance. So an obligation a standing
document places on every task belongs in a standing grant rather than in each
dossier's list, and `G45` made the one this file needs.

Neither step is mechanical, and that is a real limit. What a dossier's decisions
require is prose, and no checker reads it; `archlint` validates the register and
`contract-immutability-check` the frozen surfaces, but nothing compares a
document's intentions with its own permissions.

**Last recurrence.** This pull request — the missing grant to this file in every
behaviour dossier, found in review of pull request #367.

**Status: `Failed`.** The countermeasure was in force, was applied, and did not
prevent instance 4. It was written at `G44` and exercised at `C03-I2`, which
enumerated the paths its plan needed against § 3.3 before writing any code —
and the path to this file was not among them, because **it checks the paths a
plan intends to touch, and the duty to record a recurrence is incurred by
*finding* one, which no plan enumerates in advance.**

The third step above is the response, and it has not been tested by events.

## On whether this document works

Its own efficacy is unproven, and `DL-2` and `DL-3` are evidence against the
premise: in both cases the lesson was written down and the defect recurred
anyway. What is different here is that this record is in the repository, visible
to every agent and to review, rather than in one agent's private notes.

That is a reason to expect better, not a demonstration of it. **Review this
document when Stage P completes:** entries that have not moved from `Proposed`,
or that sit at `Failed` with an unchanged countermeasure, are evidence the
countermeasure is wrong rather than that the defect is unavoidable.

## What the first currency pass found

`G17` swept every commit since this document landed whose message names a `DL-`
identifier, and separated **recurrences** — the defect occurring — from
**citations**, where a countermeasure was applied or caught something. Most
mentions are citations; the ledger would look far worse if they were counted.

Three entries moved. `DL-1` and `DL-7` left `Proposed`, each having been
exercised on later tasks and having recurred there. `DL-4` and `DL-8` were
already `Failed` and gained later recurrences. **Four entries changed by nothing
at all**: this document landed on 13 September and the pass ran on the 14th, so
they are as they were written.

*What that does not prove.* The sweep's signal is a commit message naming an
identifier, which finds only defects whose author recognised the class and said
so. `DL-3` and `DL-6` almost certainly recurred in that period — a factual claim
not compared with its source, and a backtick in a `-m` body — and left no
citation, because those get fixed in the moment and never reach a message.
**Absence of evidence here is evidence about the sweep, not about the defect**,
and an entry left unchanged by this pass has not thereby earned its status.

The general finding is about the record rather than the defects: nine
recurrences of one class were written down nine times, in nine places nobody
reads, while the one place designed to hold them said four.
