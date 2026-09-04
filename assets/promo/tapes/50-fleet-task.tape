#
# docs clip "operate-a-fleet" — one continuous session.
#
# ONE tape, not several. Each tape is its own terminal with its own
# `clear`, so splitting a single task across shots wipes the screen
# mid-task and reads as a rendering glitch rather than as a scene
# change. A docs clip showing one task has to accumulate like a real
# session: command, output, next command, output.
#
# FontSize 24 (~125 cols, ~33 rows) rather than the 30 in _common.tape:
# the session ends up ~20 lines deep and the exec table is wide.

Output build/promo/clips/50-fleet-task.mp4

Require kscorectl
Source assets/promo/tapes/_common.tape

Set FontSize 24

Hide
Type `cd assets/promo/scenario; PS1="\033[38;5;117m$\033[0m "; clear` Enter
Show

Type "kscorectl agent list" Enter
Sleep 3s

Type "kscorectl exec run --target 'role:agent' --dry-run /bin/hostname" Enter
Sleep 3s

Type "kscorectl exec run --target 'role:agent' /bin/hostname" Enter
Sleep 4s

Type "B=$(kscorectl exec list | awk 'NR==2{print $1}')" Enter
Sleep 1s

Type "kscorectl exec output $B" Enter
Sleep 4500ms
