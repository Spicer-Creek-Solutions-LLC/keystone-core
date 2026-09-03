#
# docs clip "fleet" — shot 1: the fleet view.
#
# `agent list` is the first command an operator runs. Real output from
# the promo topology's two agents.

Output build/promo/clips/10-fleet.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Hide
Type `cd assets/promo/scenario; PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl agent list" Enter
Sleep 4s
