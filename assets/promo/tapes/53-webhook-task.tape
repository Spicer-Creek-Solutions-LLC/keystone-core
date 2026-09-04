#
# docs clip "outbound-webhooks" — one continuous session.
#
# Subscribe, confirm it is stored, fire a delivery, then read the
# delivery record. The receiver is nginx from the promo compose overlay
# on 127.0.0.1:8099 (scenario/webhook-receiver.conf) — nginx's default
# answers a POST with 405, which would make this a picture of a failed
# delivery.

Output build/promo/clips/53-webhook-task.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

# FontSize 20, not the 24 the other task clips use. Two lines here are
# long: the create invocation (~105 chars) and the `test` result, which
# carries TWO full UUIDs plus the status fields (~135). A line past the
# terminal's width does not wrap to the next row — it wraps around and
# overwrites the START of the same line, which reads as a corrupted
# frame. Measured: 105 chars overflows at FontSize 24; ~135 fits at 20.
Set FontSize 20

Hide
Type `mkdir -p build/promo/scratch && cd build/promo/scratch && rm -f .kscore-webhook.db` Enter
Type `PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl webhook outbound create --name prod-alerts --url http://127.0.0.1:8099/hook --events 'state.*'" Enter
Sleep 3500ms

Type "W=$(kscorectl webhook outbound list | awk '{print $1}')" Enter
Sleep 800ms

Type "kscorectl webhook outbound test $W" Enter
Sleep 3500ms

Type "kscorectl webhook outbound history $W" Enter
Sleep 4s
