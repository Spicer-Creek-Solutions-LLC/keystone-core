#
# docs clip "secrets" — shot 1: write a secret.

Output build/promo/clips/20-secrets-put.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Hide
Type `cd assets/promo/scenario; PS1="\033[38;5;117m$\033[0m "; clear` Enter
Type `kscorectl secrets delete app/db >/dev/null 2>&1 || true` Enter
Type "clear" Enter
Show

Type "kscorectl secrets put app/db --data password=s3cret --data user=kscore" Enter
Sleep 4s
