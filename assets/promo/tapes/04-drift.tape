#
# Shot 4 (terminal, 5.5s) — "The host drifts anyway."
#
# drift.sh mutates the managed files out of band (host side of the
# /srv/promo bind mount), then `kscorectl state drift` reports it.
#
# The report itself is the shot: a per-declaration severity plus the
# content-hash transition. Do NOT add an exit-code beat here --
# `state drift` returns 0 whether or not it finds drift (there is no
# --exit-code flag), so showing `echo exit=$?` would put a claim on
# screen that the CLI does not support.

Output build/promo/clips/04-drift.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Hide
Type `cd assets/promo/scenario; PS1="\033[38;5;117m$\033[0m "; clear` Enter
Type `bash drift.sh >/dev/null` Enter
Type "clear" Enter
Show

Type "kscorectl state drift state/web.yaml" Enter
Sleep 4500ms
