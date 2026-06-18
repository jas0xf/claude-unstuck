# Launch playbook (keep this honest)

Goal: reach the people actually hitting the hang. Every claim must be backed
by the README evidence — HN will verify.

## Pre-launch checklist

- [ ] Push repo to GitHub, enable Issues + Discussions
- [ ] Tag `v0.1.0` → release workflow publishes binaries; verify `install.sh`
      works end-to-end on a clean machine
- [ ] Enable GitHub Pages (or use htmlpreview) so the research slides render:
      `docs/slides/cse291y_final_presentation_slides_jason_yuan.html`
- [ ] Record hero GIF with [vhs](https://github.com/charmbracelet/vhs):
      split-screen `doctor` verdict → `run -- claude` → green receipt
- [ ] Re-read README as a skeptic: every number traceable to a capture

## Channel order (highest-intent first)

1. **Existing GitHub issues on anthropics/claude-code** (the audience is
   pre-qualified and desperate). Comment helpfully — methodology summary +
   `doctor` snippet + link. Relevant threads: search "hang", "stall",
   "socket connection closed", "stuck", "IPv6". Do NOT spam — one quality
   comment per major thread.
2. **r/ClaudeAI** — post the before/after GIF + the 74%→5% chart. Title
   suggestion: "I packet-captured why Claude Code hangs on my home network —
   it's the IPv6 path. Wrote a 30-second diagnostic."
3. **Show HN** — Tue–Thu 7–9am PT. Title: "Show HN: I packet-captured why
   Claude Code hangs — a diagnosis and fix". First comment (yours):
   methodology, what you can and cannot conclude, the caveats. Stay in the
   thread all day.
4. **X/Twitter thread** — the SSE-silence screenshot is the hook; end with
   the install one-liner.

## The growth loop

`doctor` ends with a paste-ready anonymized snippet. Every user who shares
their result in an issue/forum is distribution. Encourage it in replies.

## Rules

- Never claim "Anthropic is broken" — claim what's measured: path-correlated
  server-side silence, fixed by forcing IPv4.
- Never publish raw captures (they contain tokens + prompt content).
  Charts and aggregates only.
- If Anthropic/Cloudflare fix the underlying issue, celebrate and say so in
  the README — the doctor remains useful as a network diagnostic.
