#
# docs clip "remote-exec" — shot 2: dispatch across the fleet.
#
# Needs a shell on the agent, which the E2E image (distroless) does not
# have — the promo overlay swaps in a debian-slim agent runtime for
# exactly this. See scenario/Dockerfile.agent.

Output build/promo/clips/12-exec-run.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Hide
Type `cd assets/promo/scenario; PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl exec run --target 'role:agent' /bin/hostname" Enter
Sleep 5s
