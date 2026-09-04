#
# docs clip "gitops-rollback" — shot 1: propose a rollback that waits.
#
# `gitops rollback` drives a LOCAL engine with a SQLite store (--store,
# default ./.kscore-gitops.db), so the hidden setup cds into a scratch
# dir under the gitignored build/ tree and clears any prior store. That
# keeps the shot deterministic across re-renders and keeps the store
# out of the repo root, where clean-check would rightly flag it.

Output build/promo/clips/30-rollback-propose.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

# The record line runs ~102 columns, past the 99 that FontSize 30
# gives. 26 yields ~115 and keeps it on one line; a wrapped record
# reads as a rendering fault rather than as output.
Set FontSize 26

Hide
Type `mkdir -p build/promo/scratch && cd build/promo/scratch && rm -f .kscore-gitops.db` Enter
Type `PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl gitops rollback --app payments-api --reason 'bad deploy 14:02' --require-approval" Enter
Sleep 4500ms
