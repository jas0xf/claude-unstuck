# Demo for the README: the freeze, diagnosed and fixed — in one take.
# Record on a connection where Claude Code's IPv6 hang reproduces, so the
# `doctor` contrast (IPv6 hangs / IPv4 fine) is real. Passwordless sudo makes
# the recording clean. Regenerate with: vhs docs/assets/demo.tape
Output docs/assets/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 760
Set Padding 16
Set TypingSpeed 30ms

# 1. Prove the bug: doctor drives real turns over each path and shows the split.
Type "claude-unstuck doctor"
Sleep 500ms
Enter
Sleep 22s

# 2. Fix it once, system-wide.
Type "sudo claude-unstuck on"
Sleep 500ms
Enter
Sleep 4s

# 3. Plain claude just works now.
Type "claude -p 'Why do rivers meander? Answer in one sentence.'"
Sleep 500ms
Enter
Sleep 12s
