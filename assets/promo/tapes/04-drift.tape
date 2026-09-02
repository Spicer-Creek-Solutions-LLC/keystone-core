#
# Shot 4 (terminal, 5.5s) — "The host drifts anyway."
#
# drift.sh mutates the managed files out of band (host side of the
# /srv/promo bind mount), then `kscorectl state drift` reports it. The
# non-zero exit is shown on purpose: drift detection scripts cleanly,
# which is the difference between a dashboard and a control plane.

Output build/promo/clips/04-drift.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Hide
Type `PS1="\033[38;5;117m$\033[0m "; clear` Enter
Type `bash assets/promo/scenario/drift.sh >/dev/null` Enter
Type "clear" Enter
Show

Type "kscorectl state drift assets/promo/scenario/state/web.yaml" Enter
Sleep 3s
Type "echo exit=$?" Enter
Sleep 1500ms
