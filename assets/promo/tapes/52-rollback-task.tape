#
# docs clip "gitops-rollback" — one continuous session.
#
# Propose, see it waiting, reject it, then read the record back. The
# closing `get` is the payload: the decision and its state transition
# are stored, not just applied.
#
# Local engine with a SQLite store, so the hidden setup works in a
# scratch dir under the gitignored build/ tree and clears any prior
# store — deterministic across re-renders, and no .kscore-gitops.db in
# the repo root for clean-check to find.

Output build/promo/clips/52-rollback-task.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Set FontSize 24

Hide
Type `mkdir -p build/promo/scratch && cd build/promo/scratch && rm -f .kscore-gitops.db` Enter
Type `PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl gitops rollback --app payments-api --reason 'bad deploy 14:02' --require-approval" Enter
Sleep 3500ms

Type "kscorectl gitops rollback list" Enter
Sleep 2500ms

Type "R=$(kscorectl gitops rollback list | awk '{print $1}')" Enter
Sleep 800ms

Type "kscorectl gitops rollback reject $R --reason 'rolled forward instead'" Enter
Sleep 4500ms
