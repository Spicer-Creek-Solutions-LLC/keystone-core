# transition — tracker retirement and publication

Built by reboot task **R04**, run by **R07** to retire Generation 1 tracker
state, and extended by **R09** to publish the Generation 2 planning state. It is
deliberately the most conservative tool in the repository.

**This tool is removed at the end of R09.** It exists to mutate a tracker during
a one-time transition, and keeping it afterwards would invite reuse on a tracker
that should be edited by hand. The record of what it did survives it, in
[`../../docs/transition/`](../../docs/transition/).

It never deletes anything — not an issue, not a comment, not a label, not a
milestone, not a project card. The original content of every issue is the
archive's record of what the project once intended, and destroying it to tidy a
tracker would defeat the purpose of preserving Generation 1 at all.

## Its own module

`tools/transition` is a separate Go module with **no dependencies outside the
standard library**. R08 removes the Generation 1 root module; this tool has to
keep building after that, and a tool that mutates a tracker exactly once should
not carry a dependency graph that can fail to resolve years later when someone
is auditing what happened.

The root module's `go test ./...` and `golangci-lint run ./...` do not reach it.
`make transition-check` does, and CI runs that.

## The retirement commands (R07)

```bash
cd tools/transition

# 1. Read the tracker at a cutoff. GET only; changes nothing.
go run . -snapshot snapshot.json snapshot

# 2. Derive the allowlist and print the digest a reviewer approves.
go run . -snapshot snapshot.json -allowlist allowlist.json -exclude 232 plan

# 3. Dry run — reports what it would do, touches nothing. This is the default.
go run . -snapshot snapshot.json -allowlist allowlist.json apply

# 4. Apply, naming the reviewed digest explicitly.
go run . -allowlist allowlist.json \
   --apply --expect-allowlist-sha256 <digest from step 2>  apply

# 5. Confirm postconditions, and that nothing outside the allowlist moved.
go run . -snapshot snapshot.json -allowlist allowlist.json verify
```

Flags precede the subcommand.

## The publication commands (R09)

The same shape — compose, review a digest, apply, verify:

```bash
cd tools/transition

# 1. Compose the plan from the execution plan and the live forge. GET only.
go run . -repo-root ../.. -source ../../docs/project/REBOOT-EXECUTION-PLAN.md \
   -plan ../../docs/transition/r09/publication-plan.json publish-plan

# 2. Dry run — reports every issue it would create, touches nothing.
go run . -plan ../../docs/transition/r09/publication-plan.json publish

# 3. Publish, naming the reviewed digest explicitly.
go run . -plan ../../docs/transition/r09/publication-plan.json \
   -journal ../../docs/transition/r09/journal.json --max-wait 90m \
   --apply --expect-plan-sha256 <digest from step 1> publish

# 4. Confirm postconditions and the deduplication checks.
go run . -plan ../../docs/transition/r09/publication-plan.json publish-verify
```

**Issue bodies are composed at plan time, not apply time.** R07's retirement
comment was generated during the apply, so its dry run showed which issues would
be touched but not the words that would appear on them. Issue bodies are the
most public artifact this reboot produces, so the plan file carries each one
verbatim: the reviewer approves the text, not a promise about the text.

**Epic bodies are extracted from the execution plan, not retyped.** "Every issue
maps to a task" is R09's acceptance criterion; extracting the prose and the
dependency-gate row is what makes that mechanical rather than a claim. A
workstream with no gate row, or a section the regex cannot parse, is an error —
twelve public issues describing nothing is the failure this guards against.

**In-repository links are resolved before anything is published.** The bodies
link to files using absolute forge URLs, which the CI link gate cannot check:
lychee runs offline and skips external hosts. `publish-plan` stats every one
against the working tree instead.

**Identity is a task marker, never a title.** Every Generation 2 issue body
carries `<!-- keystone-core-task: P00 -->`. Matching is on that, so a closed
Generation 1 issue can never suppress or deduplicate a Generation 2 one — and
`publish-verify` proves it did not, by checking that each created issue numbers
above the whole Generation 1 range and reporting any shared title.

**Rate limits are the expected case, not bad luck.** Comment creation is capped
near 16 per five minutes and issue creation is tighter and tiered. A run of this
size being interrupted is normal, so the journal adopts whatever is already on
the forge — the forge is the authority, not the journal — and creates only what
is missing.

## What stops it going wrong

**Dry run is the default.** `apply` without `--apply` reports and returns.

**`--apply` requires `--expect-allowlist-sha256`.** A reviewer approves a
digest, not a file path, so an allowlist regenerated after review cannot be
applied out of habit.

**It refuses to act on the wrong repository.** Host, owner, name *and* the
numeric repository ID must match the manifest. The ID matters because
`owner/name` can be reassigned.

**Preconditions before the first mutation and again before each close.** An
issue whose `state`, `updated_at` or identity has moved since review stops the
run before anything is written. The second check exists because a long apply
gives someone time to comment on an issue between its comment and its close.

**Identity is a fingerprint, not a title.** Generation 1 issues predate the
task-ID convention and carry no stable ID in their bodies, and R04 is not
allowed to mutate them to add one. Identity is therefore a hash over number,
original title and creation time — captured in the snapshot, checked at apply.
Generation 2 issues carry a real task ID in an HTML comment, which is what makes
generation-aware matching possible.

**A journal makes it resumable.** Neither commenting nor creating an issue is
idempotent on a forge: a repeated run would post a second comment, or create a
second public issue. The journal records each step as it completes, so an
interrupted run resumes rather than repeats, and a journal belonging to a
different allowlist or plan is refused rather than silently reused.

**The order is fixed: comment, labels, close.** The explanation has to land
before the close so it is visible on an issue nobody is watching, and the labels
before the close so a reader filtering on `status/superseded` finds it. Roll-up
tracker issues close *after* their leaves, so a tracker never claims work is
finished while its children are open.

## Things that were learned the hard way

**A 500 on a comment POST is ambiguous.** The write may have landed before the
response failed. Retrying is safe for labels and closes, which are idempotent,
and unsafe for comments — so 500 is retried for everything except a comment
post, and a resume checks the forge for an existing comment before writing one.

**Forgejo does not phrase its rate limits consistently.** Issue creation says
`posted 5 issues in under 5 minutes`; comment creation says `posted 16 comments
in 5 minutes`. A pattern written for the first silently fails to match the
second, which leaves `--max-wait` inert and turns a routine pause into a stopped
run. That is exactly what happened on the first R07 apply, 16 comments in.

**Tracker issues were matched on the generator's exact format.** A Generation 1
roll-up was titled `<bucket> — release tracker`. That tracker also contained
"Reactor engine + event lifecycle tracking" and "Boostrap PSK consumption:
in-memory tracking only", which are ordinary leaves — a substring match on
"track" would have ordered them wrongly.

**Nightly automation and reviewed snapshots do not mix.** The `deps-outdated`
job rewrote issue #232 at 05:00 UTC daily, and apply fails closed on
`updated_at`, so a snapshot decayed overnight. R06 disabled the job and R08
deleted the tool behind it; R09 proves closing #232 produced no replacement.
