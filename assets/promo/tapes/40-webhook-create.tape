#
# docs clip "outbound-webhooks" — shot 1: subscribe to events.
#
# Also a local-store command. The receiver is nginx, declared in the
# promo compose overlay on 127.0.0.1:8099 — see scenario/
# webhook-receiver.conf for why it exists (nginx's default answers a
# POST with 405, which would make the next shot a failed delivery).

Output build/promo/clips/40-webhook-create.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Hide
Type `mkdir -p build/promo/scratch && cd build/promo/scratch && rm -f .kscore-webhook.db` Enter
Type `PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl webhook outbound create --name prod-alerts --url http://127.0.0.1:8099/hook --events 'state.*' --secret hunter2" Enter
Sleep 5s
