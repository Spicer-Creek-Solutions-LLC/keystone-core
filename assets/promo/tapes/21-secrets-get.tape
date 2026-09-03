#
# docs clip "secrets" — shot 2: read it back, masked by default.
#
# The masking and the --show-cleartext hint are the point of the shot:
# reading a secret does not spray it across the terminal by accident.

Output build/promo/clips/21-secrets-get.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Hide
Type `cd assets/promo/scenario; PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl secrets get app/db" Enter
Sleep 5s
