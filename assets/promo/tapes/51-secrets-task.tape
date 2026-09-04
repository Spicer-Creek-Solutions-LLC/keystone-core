#
# docs clip "manage-secrets" — one continuous session.
#
# Write, list, read masked, then read cleartext deliberately. The
# masked-then-explicit pair is the point: reading a secret does not
# spray it across the terminal unless you ask.

Output build/promo/clips/51-secrets-task.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Set FontSize 24

Hide
Type `cd assets/promo/scenario; PS1="\033[38;5;117m$\033[0m "; clear` Enter
Type `kscorectl secrets delete app/db >/dev/null 2>&1; clear` Enter
Show

Type "kscorectl secrets put app/db --data password=s3cret --data user=kscore" Enter
Sleep 3s

Type "kscorectl secrets list --prefix app" Enter
Sleep 2500ms

Type "kscorectl secrets get app/db" Enter
Sleep 4s

Type "kscorectl secrets get app/db --show-cleartext" Enter
Sleep 4500ms
