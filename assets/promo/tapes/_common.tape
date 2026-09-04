# Shared look for every promo shot. Sourced by each tape via `Source`.
#
# FontFamily must name a font that is actually installed. VHS does not
# warn on a miss: it silently falls back to a proportional face, which
# letter-spaces the output and wraps every command line so it overwrites
# itself. DejaVu Sans Mono ships with essentially every Linux desktop
# and is also what the ffmpeg caption pass uses, so the two match.
#
# 1920x1080 at FontSize 30 gives roughly 90 columns of DejaVu Sans Mono
# — wide enough for the shots' commands and output, and large enough to
# read in a timeline preview rather than only fullscreen.
#
# WATCH THE LINE LENGTH. A line wider than the terminal does not wrap
# onto the next row: it wraps AROUND and overwrites the start of the
# same line, so the frame shows a mangled command with its tail sitting
# on its head. It looks like a rendering bug and it is easy to miss.
# Roughly 90 columns at FontSize 30, ~100 at 24, ~135 at 20 — measured,
# not derived. Either keep the command short or drop the font size for
# that tape and say why.
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
