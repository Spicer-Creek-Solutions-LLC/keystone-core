#
# Shot 6 (terminal, 4.0s) — "Every change, audited."
#
# The apply and the remediation from shots 3-5 are already in the audit
# log by the time this runs; nothing is staged for the camera.

Output build/promo/clips/06-audit.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Hide
Type `PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl audit list --limit 5" Enter
Sleep 3s
