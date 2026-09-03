#
# docs clip "remote-exec" — shot 1: resolve the target, dispatch nothing.
#
# --dry-run is a real feature and the right opening beat: it shows the
# selector matching before anything is executed anywhere.

Output build/promo/clips/11-exec-target.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Hide
Type `cd assets/promo/scenario; PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl exec run --target 'role:agent' --dry-run /bin/hostname" Enter
Sleep 3500ms
