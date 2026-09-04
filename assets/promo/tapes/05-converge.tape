#
# Shot 5 (terminal, 6.0s) — "Keystone converges it."
#
# `state drift --fix` detects and remediates in one call, then a second
# bare `drift` proves the host is back in sync. The second call is the
# point of the shot: anyone can print a diff, the claim is convergence.

Output build/promo/clips/05-converge.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Hide
Type `cd assets/promo/scenario; PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl state drift --target localhost --fix state/web.yaml" Enter
Sleep 3s
Type "kscorectl state drift --target localhost state/web.yaml" Enter
Sleep 2500ms
