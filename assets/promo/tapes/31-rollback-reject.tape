#
# docs clip "gitops-rollback" — shot 2: the human decides.
#
# Self-contained: the hidden setup recreates the pending record and
# captures its id, so this tape does not depend on shot 1 having run
# first or on store state left over from a previous render. The viewer
# sees `$ID`, which is what an operator would actually type.

Output build/promo/clips/31-rollback-reject.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

# Same width story as the propose shot: 26 keeps the record on one
# line so the state transition below it stays the eye's landing spot.
Set FontSize 26

Hide
Type `mkdir -p build/promo/scratch && cd build/promo/scratch && rm -f .kscore-gitops.db` Enter
Type `kscorectl gitops rollback --app payments-api --reason 'bad deploy 14:02' --require-approval >/dev/null` Enter
Type `ID=$(kscorectl gitops rollback list | awk '{print $1}')` Enter
Type `PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl gitops rollback reject $ID --reason 'rolled forward instead'" Enter
Sleep 5s
