# vhs tape — records a real `claude-unstuck run` session for the README demo.
# Run on a machine where `claude` and `claude-unstuck` are installed and the
# IPv6 hang reproduces. Regenerate with: vhs docs/assets/demo.tape
Output docs/assets/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 640
Set Padding 16
Set TypingSpeed 30ms

Type "claude-unstuck run -- claude -p 'Why do rivers meander? Answer in one sentence.'"
Sleep 600ms
Enter
Sleep 14s
