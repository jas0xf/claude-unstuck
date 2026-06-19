<div align="center">

# claude-unstuck

### Claude Code frozen again? It's probably not Claude. It's your network's IPv6 path — and you can prove it in 30 seconds.

[![release](https://img.shields.io/github/v/release/jas0xf/claude-unstuck)](https://github.com/jas0xf/claude-unstuck/releases)
![platforms](https://img.shields.io/badge/macOS%20%7C%20Linux%20%7C%20Windows-blue)
![deps](https://img.shields.io/badge/dependencies-zero-brightgreen)

</div>

---

## Sound familiar?

<p align="center"><img src="docs/assets/hang-32min.png" width="900" alt="Claude Code stuck for 32 minutes 36 seconds, 350 tokens, retrying attempt 5/10"></p>
<p align="center"><sub><b>32 minutes. 350 tokens. Attempt 5 of 10.</b> A real session, captured live.</sub></p>

<p align="center"><img src="docs/assets/err-stream-idle.png" width="900" alt="API Error: Stream idle timeout — partial response received, worked for 3m 23s"></p>

<p align="center"><img src="docs/assets/err-socket-closed.png" width="900" alt="API Error: The socket connection was closed unexpectedly"></p>

You've restarted Claude. Restarted your terminal. Blamed your Wi-Fi, your VPN, your account.
So has everyone else:

<p align="center"><img src="docs/assets/issue-26224.png" width="640" alt="GitHub issue 26224: Claude Code is hanging / freezing / stuck on heaps of prompts for 5-20 minutes or more"></p>

We packet-captured these freezes for weeks — decrypted TLS, byte-level, across
machines, ISPs, and VPNs. On the affected home networks the answer was
embarrassingly specific: **sessions over IPv6 froze constantly; the exact same
machine, account and hour over IPv4 almost never did.**

<p align="center"><img src="docs/evidence/hang-rate.svg" width="620" alt="74% of IPv6 sessions hung vs 5% of IPv4 sessions — 120 paired cycles, same machine"></p>

## The 30-second version

Install it:

```sh
curl -fsSL https://raw.githubusercontent.com/jas0xf/claude-unstuck/main/install.sh | sh
```

<sub><b>Windows:</b> <code>irm https://raw.githubusercontent.com/jas0xf/claude-unstuck/main/install.ps1 | iex</code></sub>

**Check** whether you have the bug (changes nothing, costs a few tokens):

```sh
claude-unstuck doctor
```
```
  claude-unstuck — checking if Claude Code hangs on your connection

  ✔ IPv4 — Claude responded every time (median 3.8s)
  ✘ IPv6 — Claude HUNG (100% of turns froze)

  ➜ DIAGNOSIS  Confirmed: Claude hangs over IPv6 but runs fine over IPv4.
               This is the bug — and it's fixable.

  Fix it now (no admin, just use Claude normally):
      claude-unstuck
```

**Fix** it — just run Claude through `claude-unstuck` instead of `claude`:

```sh
claude-unstuck          # = claude, but pinned to IPv4 for this session
```

`doctor` runs a couple of **real Claude turns** over each path (a plain ping
can't reproduce the freeze — it happens mid-stream), so it catches the actual
hang instead of guessing.

<p align="center"><img src="docs/assets/demo.gif" width="900" alt="A fast Claude session over forced IPv4 through claude-unstuck"></p>

Every session ends with a receipt, so you never have to trust us:

```
[claude-unstuck] running over IPv4: claude
[claude-unstuck] ✅ done — all 10 upstream connections used IPv4
```

> Want a zero-token connectivity sanity check (no Claude turns)?
> `claude-unstuck probe` runs unauthenticated HTTPS probes and prints an
> anonymized, paste-ready snippet for the GitHub issue threads.

## What's actually happening on the wire

This is what every captured freeze looks like, decrypted:

<p align="center"><img src="docs/evidence/sse-timeline.svg" width="820" alt="Healthy sessions stream deltas to message_stop. Hung sessions: HTTP 200, message_start, then silence until the client's 180s idle timeout"></p>

The request arrives. The server says `HTTP 200`, sends `message_start`…
then **silence**. No error. No data. And crucially — **zero TCP
retransmissions** during the silence. The network delivered every byte both
ways; the response simply stops coming on the degraded IPv6 path. That's why
nothing on your side ever fixed it: there was nothing on your side to fix.

Why IPv6? Your home IPv6 address is individually routable (no NAT), and
residential IPv6 can take different — sometimes degraded — ingress paths than
IPv4 into the API's edge. Whatever the upstream root cause, the observation
is boringly reproducible: **force IPv4, hangs stop.**

## What the packet captures showed

Two weeks of decrypted-TLS measurement — `mitmproxy` + `tcpdump`, per-event SSE
timing on real Claude sessions. The freeze isn't random, and every other suspect
was ruled out by measurement, not guessed:

| Suspect | What the captures showed | Verdict |
|---|---|---|
| The app / a stuck process | survives session, process, **and a machine reboot** | not it |
| The client | every failure is **after the upload is fully ACKed** — then `down 0 bytes`, 120 s of pure silence | not it |
| A bad region / PoP | residential vs VPN take different first hops, but the `cf-ray` header lands on the **same LAX PoP** | not it |
| Your IPv6 reputation | a friend's house — same ISP, **different router, different /64** — still hung 50% | not it |
| The model under load | successful IPv6 replies are the **same speed** as IPv4 (TTFT 1701 ms vs 1652 ms) — v6 gets **dropped, not slowed** | not it |
| **IPv6 itself** | on large (>100 KB) requests: **IPv6 = 54% no-response, IPv4 = 0%** — on the very same line | **this** |

And it follows the **network, not your computer**: on a Spectrum line IPv6 hung
**~74%** while IPv4 hung ~5%; move the same machine to an AT&T hotspot and **both
families dropped to 0%**. IPv6 was a flat ~75%-hang band *all day*; IPv4 stayed
clean except at the US-evening peak. The fault domain is well-bounded — something
on the **IPv6 path between the ISP (Charter/Spectrum) and Cloudflare**. The v4 door
through the same PoP is clean.

> 📊 **Full deck** — 10-region global probe, two-ISP controls, VPN confound
> analysis, byte-level captures:
> **[Measuring Claude](docs/slides/measuring-claude.html)**
> ([view rendered](https://htmlpreview.github.io/?https://github.com/jas0xf/claude-unstuck/blob/main/docs/slides/measuring-claude.html))

## The whole command set

| command | what it does | scope | root? |
|---|---|---|---|
| `claude-unstuck` | run Claude over IPv4 (just use Claude) | this session | no |
| `claude-unstuck doctor` | check if you have the bug (a few tokens) | — | no |
| `sudo claude-unstuck on` | install the fix for every app | system-wide | yes |
| `sudo claude-unstuck off` | remove the system-wide fix | system-wide | yes |
| `claude-unstuck status` | show what's installed | — | no |

`on` resolves the **current** AAAA records at apply time (nothing hardcoded),
records exactly what it installed, and is fully reversible. Extras:

```sh
sudo claude-unstuck on --persist    # also survive reboots
sudo claude-unstuck on --for 24h    # self-expiring
claude-unstuck status               # warns if Anthropic's IPs rotated since
```

<details>
<summary><b>Why not just edit /etc/hosts or set NODE_OPTIONS?</b></summary>

We tried. Claude Code is a Bun-compiled binary: packet captures show it
**silently bypasses `/etc/hosts`** and ignores Node's `--dns-result-order`.
The two mechanisms that demonstrably work are `HTTPS_PROXY` (what the session
command uses) and the OS routing/policy layer (what `on` uses).
</details>

<details>
<summary><b>Why not disable IPv6 entirely?</b></summary>

Heavy-handed and breaks other things. This touches only the Anthropic API
addresses (Windows: only address-selection preference), and `off` restores
everything.
</details>

## Verified, not vibes

- **macOS — real session:** all 8 tunneled connections, including the
  ~1.1 MB context upload, went to IPv4. Session completed normally.
- **Linux — real session + packet capture:** with `sudo claude-unstuck on`
  active, a plain `claude -p` session produced **0 IPv6 packets and 867 IPv4
  packets** to the Anthropic API. `off` left the routing table clean.
- **Windows:** the scoped firewall block/undo is unit-tested on every commit
  (the command builders run across the CI matrix, Windows included), and the
  live `netsh` round-trip was validated on real Windows 11. A live
  Claude-session report from a Windows machine would be welcome.

<details>
<summary><b>FAQ</b></summary>

**Is this Anthropic's fault? My ISP's? Cloudflare's?**
Honestly: unknown. The failure is server-side silence on specific network
paths; captures can't see past the TLS endpoint. What's provable is the
correlation (same machine, account, hour: IPv6 hangs, IPv4 doesn't) and the
fix. `doctor` exists so you can measure *your* path instead of trusting ours.

**Will IPv4 be slower?**
Same anycast front door for both families. In our measurements IPv4 was
equal or faster on affected networks — and it can hardly be slower than a
32-minute stall.

**Doctor says "healthy" but Claude still hangs.**
Hangs can be intermittent — re-run `doctor` *while* it's hanging. If IPv6 is
clean even then, your issue is something else (account concurrency and rate
limits produce similar symptoms).

**Affiliated with Anthropic?**
No. Independent tool from a university course research project. MIT.
</details>

