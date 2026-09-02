#
# Shot 3 (terminal, 5.0s) — "Declare the state."
#
# Real `kscorectl state apply` against the promo topology. Hand-written
# rather than templated: it runs commands, and no fact from the repo
# belongs in it.
#
# Requires KSCORE_API_KEY exported by the runner (make promo).

Output build/promo/clips/03-state-apply.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Hide
Type `PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl state apply assets/promo/scenario/state/web.yaml" Enter
Sleep 4s
