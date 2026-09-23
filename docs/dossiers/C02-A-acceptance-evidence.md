# C02-A acceptance evidence

This records the acceptance-contract half of C02. It implements no persistence:
`C02-I` owns the stores, the migrations and the published schema, and removes
each pending entry when its case passes.

Contract package: `test/contract/persistence`

Contract commit: `94e602a97428ff3f12d0427b0b5b03b9bf995ea4`

Contract amendments:

`C04-I` changes AC-1 and the two reuse sites in the executable test body to
require migrations 1 and 2 as an ordered prefix rather than the complete
migration list. C04 necessarily adds migration 3. The frozen identifier,
requirement and `contract_surface.go` do not change, so this is not a contract
amendment and the freeze commit does not move.

- `25b3b50239d92b7283b6f99814c7a41ba761f1cb` — `G42`. `AC-11` required the owner
  and mode of each store **and its directory**, and `C02.md` § 12 puts creating
  directories at C13. The case demanded a property of a thing C02 is forbidden
  to create, so it was unsatisfiable when it was frozen. The requirement now
  names the store file modes; the directory modes and the owning accounts are
  owed by C13.
- `0902f56f13316842cb93abf50fe6fac289786196` — `G43`. `AC-11`'s test was still
  called `TestAC11OwnershipAndModes` and asserted no ownership, because `G42`
  narrowed the requirement and left the name. It is `TestAC11StoreFileModes`.

  Both frozen surfaces were swept for the same defect rather than the one case
  review named. Of twenty-six case names, twenty-five are at least as narrow as
  their requirement and one claimed more. **Direction is the finding.** A name
  narrower than its requirement is harmless because the requirement governs —
  `AC-9` omits *"at its documented path"*, `AC-6` omits *"before publication"* —
  while a name broader than its requirement is taken for the specification by a
  reader who does not open this file.

  **No gate holds that, and none is claimed.** A test name is not mechanically
  comparable to a natural-language requirement; what is enforced is that every
  entry above has a function of that name, so a half-landed rename does not
  compile. The sweep is a reading, recorded here as one.

The freeze commit never moves. Every commit that touches a frozen file after it
is listed here with the task that approved it, and
`contract-immutability-check` enumerates those commits from git and rejects one
that is not declared. **Recording the amendment is what makes it legitimate**,
not the gate: before `G42` the gate ran `git diff --quiet` against the recorded
commit, so any change that also moved the recorded commit passed. Its help text
said it rejected changes to an accepted surface; what it did was re-baseline
them.

## Contract cases

Thirteen cases, `AC-1` through `AC-13`, frozen in
[`contract_surface.go`](../../test/contract/persistence/contract_surface.go) and
registered in `pending-requirements.json`. Each names the property `ADR-0008`
requires, not the mechanism C02-I will choose.

**All thirteen are pending under `C02-I`.** `make pending-contract` runs each and
captures a non-zero exit; ordinary contract execution skips only the cases the
manifest registers, and refuses to skip one it does not.

## Two outputs the dossier expected here, which belong at `C02-I`

**The published-schema fixture.** `C02.md` § 3.1 lists *"the schema as an
operator would read it, frozen so drift is a failure"*. **C02-A cannot produce
it.** `ADR-0008` § 2 gives table *groups* and retention, not columns, so there is
nothing normative to hand-write from.

That is the difference from C01, and it is worth being exact about: C01-A hand-
wrote framing vectors because `ADR-0005` § 1's field table **was** normative —
field order and count were decided, and the encoding followed at G36. No
equivalent exists here, and inventing a schema to freeze would be C02-A deciding
what § 3.3 reserves for the implementation.

So `AC-10` compares the shipped schema against the published one, and both are
`C02-I`'s. The case is registered pending with that as its stated reason.

**The crash harness binary.** Specified in
[`crash_harness.go`](../../test/contract/persistence/crash_harness.go) — its
path, arguments, three halt steps, and the requirement that it terminate
**without unwinding**, since a clean exit would let SQLite finish work a crash
would have lost. `C02-I` builds it, exactly as `C01-I` built the vector binaries
whose case stayed pending until then.

## What the cases cannot detect

- **That the schema is the right schema** for a journey no task has written yet.
  `AC-10` asserts the published and shipped schemas agree; neither is evidence
  that either is correct.
- **That retention is long enough.** `AC-12` asserts a floor is honoured, not
  that the floor is well chosen.
- **That a corrupt store was corrupted by a defect** rather than by a disk.
  `AC-13` asserts refusal, and refusal is all it asserts.

## The two questions asked of every case

C01 produced four defects of one shape — a check that cannot fail on an input
nobody looks at. So each case above was written against two questions:

**Does the fixture contain the input?** `AC-11` is the exposed one: a mode check
whose fixture only ever holds `0600` passes while checking nothing, so it must
carry a wrong mode as well as a right one.

**Does anything read the value rather than the shape?** `AC-10` is the exposed
one there: comparing two schemas that were generated by the same code compares
nothing, which is why the published schema has to be a checked-in artifact and
not a second rendering.

Both are `C02-I`'s to satisfy, and both are stated here so the review has
something to hold the implementation against.

## Reproducing

```text
make contract                 # both packages; persistence skips its thirteen
make pending-contract         # thirteen registered cases, each failing
make contract-immutability-check
```
