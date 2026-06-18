# vhs tape — records a REAL claude session through claude-unstuck on a
# Spectrum residential line (executed remotely on the lab box; the typed
# command is exactly what a user types there).
Output docs/assets/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 600
Set Padding 16
Set TypingSpeed 30ms

Hide
Type `claude-unstuck() { ssh black4rch "/tmp/claude-unstuck run -- claude -p 'In exactly one sentence, why do rivers meander?' < /dev/null"; }; clear`
Enter
Show

Sleep 800ms
Type "claude-unstuck run -- claude -p 'In exactly one sentence, why do rivers meander?'"
Sleep 600ms
Enter
Sleep 10s
