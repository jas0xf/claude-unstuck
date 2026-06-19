<p align="center">
  <img src="docs/readme/hero.svg" width="900" alt="claude-unstuck — Claude Code keeps freezing? On affected networks it's not Claude, it's the IPv6 path. Diagnose in 30 seconds, fix in one command.">
</p>

<p align="center">
  <a href="https://github.com/jas0xf/claude-unstuck/releases"><img src="https://img.shields.io/github/v/release/jas0xf/claude-unstuck?style=for-the-badge&labelColor=0d1117&color=3fb950" alt="latest release"></a>
  &nbsp;
  <a href="#why-its-ipv6--the-evidence"><img src="https://img.shields.io/badge/the_research-CSE_291Y-d29922?style=for-the-badge&labelColor=0d1117" alt="the research behind it"></a>
</p>

> **Claude Code freezing mid-task?** On affected home networks the freeze rides the **IPv6 path to Anthropic** — not Claude itself. `claude-unstuck` pins Claude to IPv4, where it almost never hangs. Two steps below and you're done; the [evidence](#why-its-ipv6--the-evidence) is at the bottom if you want it.

---

## Quick start — stop the freeze

**1 — Install it**

**macOS / Linux**

```sh
curl -fsSL https://raw.githubusercontent.com/jas0xf/claude-unstuck/main/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/jas0xf/claude-unstuck/main/install.ps1 | iex
```

<sub>Installs a single binary (`~/.local/bin` on macOS/Linux) — no admin to install, no background services. Restart your terminal afterward; if `claude-unstuck` isn't found, add that folder to your `PATH`. Prefer to read first? <a href="https://raw.githubusercontent.com/jas0xf/claude-unstuck/main/install.sh">view install.sh</a>.</sub>

**2 — Fix it** → pick your command below (most people want the first one).

## Which command should I run?

**Most people — fix it once, for every app.**

**macOS / Linux** — `sudo` will prompt for your login password (that's expected):

```sh
sudo claude-unstuck on
```

**Windows** — open an **Administrator PowerShell** (Start menu → right-click *PowerShell* → *Run as administrator*); there is no `sudo`:

```powershell
claude-unstuck on
```

Applied once. Afterward plain `claude` just works — no prefix to remember. It only touches Anthropic's API addresses (on Windows, a scoped outbound firewall rule), records exactly what it changed, and survives until you run `off`. Undo anytime with `sudo claude-unstuck off`.

**Just this one session, no admin rights:**

```sh
claude-unstuck          # = claude, but pinned to IPv4 — this terminal only
```

Launch Claude with `claude-unstuck` instead of `claude`. Lasts only for the Claude you start this way; you type it each time. No root, nothing installed, nothing to undo.

> **Rule of thumb:** run `on` once and forget it. Reach for bare `claude-unstuck` only when you can't (or don't want to) use admin rights.

## Prove it's your bug first (optional)

Not sure the freeze is the IPv6 thing? `doctor` runs a couple of **real Claude turns** over each path and prints *your* numbers. It changes nothing and costs a few tokens:

```sh
claude-unstuck doctor
```

```
  claude-unstuck — checking if Claude Code hangs on your connection

  ✔ IPv4 — Claude responded every time (median 3.8s)
  ✘ IPv6 — Claude HUNG (100% of turns froze)

  ➜ DIAGNOSIS  Claude hangs over IPv6 but runs fine over IPv4. Fixable.
```

A plain ping can't reproduce the freeze (it happens *mid-stream*), so `doctor` uses real turns to catch the actual hang. Every fixed session ends with a receipt:

```
[claude-unstuck] running over IPv4: claude
[claude-unstuck] ✅ done — all 10 upstream connections used IPv4
```

## Command reference

**Safe · no root — start here**

| command | what it does |
|---|---|
| `claude-unstuck doctor` | check whether you have the bug (a few tokens, changes nothing) |
| `claude-unstuck` | run Claude over IPv4 for this terminal only (nothing installed) |

**System-wide · needs admin**

| command | what it does |
|---|---|
| `sudo claude-unstuck on` | fix every app at once. **Windows:** `claude-unstuck on` in an Administrator PowerShell (no `sudo`) |
| `sudo claude-unstuck off` | remove the system-wide fix |
| `claude-unstuck status` | show what's installed; warns if Anthropic's IPs rotated since |

> **On Windows:** drop `sudo` from the commands above and run them in an Administrator PowerShell. The no-admin commands (`claude-unstuck`, `doctor`) are identical on every platform.

`on` resolves Anthropic's **current** addresses at apply time (nothing hardcoded) and is fully reversible. Extras: `sudo claude-unstuck on --persist` (survive reboots) · `--for 24h` (self-expiring).

<details>
<summary><b>Why not just edit /etc/hosts or set NODE_OPTIONS?</b></summary>

We tried. Claude Code is a Bun-compiled binary: packet captures show it **silently bypasses `/etc/hosts`** and ignores Node's `--dns-result-order`. The two mechanisms that demonstrably work are `HTTPS_PROXY` (what the per-session command uses) and the OS routing/firewall layer (what `on` uses).
</details>

<details>
<summary><b>Why not disable IPv6 entirely?</b></summary>

Heavy-handed and breaks other things. This touches only Anthropic's API addresses (on Windows, a scoped outbound firewall rule), and `off` restores everything.
</details>

## Why it's IPv6 — the evidence

You've restarted Claude. Restarted your terminal. Blamed your Wi-Fi, your VPN, your account. These are three **real sessions**, captured live:

<p align="center">
  <img src="docs/assets/sound-familiar.png" width="720" alt="Three real Claude Code failures: a 32m 36s retry loop for 350 tokens; an API stream-idle-timeout after 3m 23s; a socket connection closed unexpectedly.">
</p>

And it's not just you — it's one of the most-reported bugs on the tracker:

<p align="center">
  <a href="https://github.com/anthropics/claude-code/issues/26224"><img src="https://img.shields.io/badge/%2326224-hanging-8b949e?labelColor=0d1117&style=flat-square" alt="claude-code issue 26224"></a>
  <a href="https://github.com/anthropics/claude-code/issues/13224"><img src="https://img.shields.io/badge/%2313224-hangs%2Ffreezes-8b949e?labelColor=0d1117&style=flat-square" alt="claude-code issue 13224"></a>
  <a href="https://github.com/anthropics/claude-code/issues/31932"><img src="https://img.shields.io/badge/%2331932-hangs%20indefinitely-8b949e?labelColor=0d1117&style=flat-square" alt="claude-code issue 31932"></a>
  <a href="https://github.com/anthropics/claude-code/issues/8658"><img src="https://img.shields.io/badge/%238658-unresponsive-8b949e?labelColor=0d1117&style=flat-square" alt="claude-code issue 8658"></a>
  <a href="https://github.com/anthropics/claude-code/issues/32867"><img src="https://img.shields.io/badge/%2332867-stalls-8b949e?labelColor=0d1117&style=flat-square" alt="claude-code issue 32867"></a>
</p>

We packet-captured these freezes for weeks — decrypted TLS, byte-level, across machines, ISPs and VPNs. The answer was embarrassingly specific: on the affected line, **IPv6 hung 74% of sessions while IPv4 hung 5%** — same machine, same account, same hour (n = 120 per arm).

<p align="center">
  <img src="docs/readme/hangrate.svg" width="900" alt="Claude Code session hang rate on the same machine, hour and account: IPv6 hung 74% (89/120 sessions), IPv4 hung 5% (6/120). Only the address family differs.">
</p>

**On the wire**, every freeze looks identical: the server sends `HTTP 200` and `message_start`, then goes silent — with **zero TCP retransmissions**. The network delivered every byte; the response simply never comes on the degraded IPv6 path. That's why nothing on your side ever fixed it.

<p align="center">
  <img src="docs/readme/wire.svg" width="900" alt="A healthy session streams content deltas to message_stop; a hung IPv6 session sends HTTP 200 and message_start, then silence until the client gives up at 180s with 0 bytes. Zero TCP retransmissions during the silence.">
</p>

And **every other suspect fell**. This began as a CSE 291Y course project — two weeks of decrypted-TLS captures (`mitmproxy` + `tcpdump`, per-event SSE timing). One by one, the app, the client, the region, the VPN, the machine and IP reputation were ruled out; the freeze follows the **network, not your computer** (clean on an AT&T hotspot, broken on Spectrum). The fault domain narrows to the **IPv6 path between the ISP and Cloudflare**.

<p align="center">
  <img src="docs/readme/investigation.svg" width="900" alt="Every other suspect ruled out by measurement — app, client, region/PoP, VPN, machine, IP reputation — leaving IPv6: 54% no-response on large requests vs 0% on IPv4, on the same line.">
</p>

> 📊 **Full deck** — 10-region global probe, two-ISP controls, VPN confound analysis, byte-level captures:
> **[▸ Measuring Claude — open the slides](https://htmlpreview.github.io/?https://github.com/jas0xf/claude-unstuck/blob/main/docs/slides/measuring-claude.html)**
> · [source](docs/slides/measuring-claude.html)

## How it's verified

- **macOS — real session:** all 8 tunneled connections, including the ~1.1 MB context upload, went to IPv4. Session completed normally.
- **Linux — real session + packet capture:** with `sudo claude-unstuck on` active, a plain `claude -p` session produced **0 IPv6 packets and 867 IPv4 packets** to the Anthropic API. `off` left the routing table clean.
- **Windows:** the scoped firewall block/undo is unit-tested on every commit (the command builders run across the CI matrix, Windows included), and the live `netsh` round-trip was validated on real Windows 11.

<details>
<summary><b>FAQ</b></summary>

**Is this Anthropic's fault? My ISP's? Cloudflare's?**
Honestly: unknown. The failure is server-side silence on specific network paths; captures can't see past the TLS endpoint. What's *provable* is the correlation (same machine, account, hour: IPv6 hangs, IPv4 doesn't) and that forcing IPv4 fixes it. Run `doctor` to measure *your* path instead of trusting ours.

**Will IPv4 be slower?**
Same anycast front door for both families. In our measurements IPv4 was equal or faster on affected networks — and it can hardly be slower than a 32-minute stall.

**Doctor says "healthy" but Claude still hangs.**
Hangs can be intermittent — re-run `doctor` *while* it's hanging. If IPv6 is clean even then, your issue is something else (account concurrency and rate limits produce similar symptoms).

**Affiliated with Anthropic?**
No. Independent tool from a university course research project. MIT.
</details>
