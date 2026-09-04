#
# docs clip "manage-secrets" — one continuous session.
#
# Store a credential, then USE it. An earlier cut ended on
# `secrets get --show-cleartext`, which is not something an operator
# does for its own sake — printing a password to a terminal is the
# thing the store exists to avoid.
#
# The realistic shape: the credential is fetched at deploy time into a
# shell variable and passed to `state apply` as a --variable, so it is
# rendered into the application's env file without ever appearing on
# screen or in the committed state file. The closing `state drift`
# confirms convergence without printing the secret — passing the same
# variable, because the check has to render the same declaration.

Output build/promo/clips/51-secrets-task.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Set FontSize 24

Hide
Type `cd assets/promo/scenario; PS1="\033[38;5;117m$\033[0m "; clear` Enter
# Clear BOTH the secret and the rendered file so the apply always shows
# a change. The bind-mounted workdir survives between renders, and an
# already-converged apply reports `unchanged=1`, which is true but makes
# a weaker shot than watching the file get written.
Type `kscorectl secrets delete app/db >/dev/null 2>&1` Enter
Type `rm -f ../../../build/promo/workdir/payments.env; clear` Enter
Show

Type "kscorectl secrets put app/db --data username=app --data password=s3cret" Enter
Sleep 3s

Type "kscorectl secrets get app/db" Enter
Sleep 3500ms

Type "DB_PASS=$(kscorectl secrets get app/db --show-cleartext -o json | jq -r .data.password)" Enter
Sleep 1500ms

# Backtick delimiter, not double quotes: VHS's Type does not support
# \" escaping, and the shell quoting around $DB_PASS is worth keeping.
Type `kscorectl state apply --target localhost --variable db_password="$DB_PASS" state/app-env.yaml` Enter
Sleep 4s

# --target localhost: every state command requires an explicit target,
# because "which host" is not safe to guess. The promo scenario runs in
# the server container, so localhost is the honest answer here; moving
# this clip onto an agent is tracked in the ROADMAP promo entry.
#
# The SAME --variable has to be supplied to the drift check: without it
# the engine renders the declaration against the placeholder default and
# reports a false drift, which would read as "the apply did not work".
Type `kscorectl state drift --target localhost --variable db_password="$DB_PASS" state/app-env.yaml` Enter
Sleep 4s
