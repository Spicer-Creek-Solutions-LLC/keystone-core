# Shared look for every promo shot. Sourced by each tape via `Source`.
#
# FontFamily must name a font that is actually installed. VHS does not
# warn on a miss: it silently falls back to a proportional face, which
# letter-spaces the output and wraps every command line so it overwrites
# itself. DejaVu Sans Mono ships with essentially every Linux desktop
# and is also what the ffmpeg caption pass uses, so the two match.
#
# 1920x1080 at FontSize 30 gives ~99 columns of DejaVu Sans Mono — wide
# enough for the shots' commands and output without wrapping, and large
# enough to read in a timeline preview rather than only fullscreen.
Set Shell bash
Set FontFamily "DejaVu Sans Mono"
Set FontSize 30
Set Width 1920
Set Height 1080
Set Padding 60
Set Theme "Catppuccin Mocha"
# 45ms looked right in isolation but ate 2.6s of a 5s shot just typing
# the command, leaving no room for the output that is the actual point.
Set TypingSpeed 22ms
Set WindowBar Colorful
Set BorderRadius 12
