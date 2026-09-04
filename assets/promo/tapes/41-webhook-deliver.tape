#
# docs clip "outbound-webhooks" — shot 2: deliver, then show the record.
#
# `test` POSTs a synthetic webhook.test payload and reports the real
# HTTP status; `history` shows the stored delivery record. Two beats in
# one shot because the second is only meaningful next to the first.

Output build/promo/clips/41-webhook-deliver.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

# `test` prints TWO full UUIDs (delivery id + subscription id) plus
# the status fields -- ~135 columns. FontSize 20 gives ~149. The ids
# are the point of the shot, so shrinking the type beats trimming the
# output or swapping the command.
Set FontSize 20

Hide
Type `mkdir -p build/promo/scratch && cd build/promo/scratch && rm -f .kscore-webhook.db` Enter
Type `kscorectl webhook outbound create --name prod-alerts --url http://127.0.0.1:8099/hook --events 'state.*' --secret hunter2 >/dev/null` Enter
Type `ID=$(kscorectl webhook outbound list | awk '{print $1}')` Enter
Type `PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl webhook outbound test $ID" Enter
Sleep 3s
Type "kscorectl webhook outbound history $ID" Enter
Sleep 3s
