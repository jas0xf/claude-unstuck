# vhs tape — records a REAL claude session through `claude-unstuck run` on a
# Spectrum residential line (executed on the lab box over SSH; the typed command
# is exactly what a user runs). Regenerate with: vhs docs/assets/demo.tape
Output docs/assets/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 640
Set Padding 16
Set TypingSpeed 30ms

Hide
Type `claude-unstuck() { ssh REDACTED "/tmp/claude-unstuck run -- claude -p 'Why do rivers meander? Answer in one sentence.' < /dev/null"; }; clear`
Enter
Show

Sleep 800ms
Type "claude-unstuck run -- claude -p 'Why do rivers meander? Answer in one sentence.'"
Sleep 600ms
Enter
Sleep 14s
