# transition — Generation 1 tracker retirement

Built by reboot task **R04**; run by **R07**. It closes Generation 1 issues as
*superseded, not completed*, and it is deliberately the most conservative tool
in the repository.

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

## The four commands

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
Generation 2 issues will carry a real task ID, which is what makes
generation-aware matching possible in R09.

**A journal makes it resumable.** Commenting is not idempotent on a forge: a
repeated run would post a second comment. The journal records each step as it
completes, so an interrupted run resumes rather than repeats, and a journal
belonging to a different allowlist is refused rather than silently reused.

**The order is fixed: comment, labels, close.** The explanation has to land
before the close so it is visible on an issue nobody is watching, and the labels
before the close so a reader filtering on `status/superseded` finds it. Roll-up
tracker issues close *after* their leaves, so a tracker never claims work is
finished while its children are open.

## Two things worth knowing before R07

**Take the snapshot after R06.** The nightly `ci-full` `deps-outdated` job
rewrites issue #232 at 05:00 UTC daily. Apply fails closed on `updated_at`, so a
snapshot taken before that job is disabled decays every night. Until then, pass
`-exclude 232`.

**Tracker issues are matched on the generator's exact format.** A roll-up is
titled `<bucket> — release tracker` (see `tools/trackerctl/tracker.go`). This
tracker also contains "Reactor engine + event lifecycle tracking" and "Boostrap
PSK consumption: in-memory tracking only", which are ordinary leaves — a
substring match on "track" would order them wrongly.
