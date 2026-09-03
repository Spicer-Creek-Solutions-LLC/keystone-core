#
# Shot 6 (terminal, 4.0s) — "Every change, audited."
#
# The apply and the remediation from shots 3-5 are already recorded by
# the time this runs; the evaluation count is produced by them, and
# nothing is staged for the camera.
#
# `audit stats` rather than `audit log` on purpose: the log table is 139
# columns wide, which needs FontSize 20 to fit and is not readable in
# four seconds at any size. stats is 57 columns and reads at the same
# font as every other shot. (`audit list` does not exist -- the
# subcommands are export / log / report / stats.)

Output build/promo/clips/06-audit.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Hide
Type `PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl audit stats --since 30m" Enter
Sleep 3s
