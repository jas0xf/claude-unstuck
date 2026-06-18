package detect

import (
	"os"
	"strings"
)

// Verdict codes for a paired IPv4/IPv6 real-session comparison.
const (
	VerdictHealthy      = "healthy"   // both families fine
	VerdictV6Hangs      = "v6-hangs"  // IPv6 hangs, IPv4 ok — the target bug
	VerdictNoV6         = "no-v6"     // no IPv6 path; not the cause here
	VerdictBothBad      = "both-bad"  // both families failing — not family-related
	VerdictV4Hangs      = "v4-hangs"  // unusual: IPv4 worse than IPv6
	VerdictNoClaude     = "no-claude" // claude not available / not logged in
	VerdictInconclusive = "inconclusive"
)

// Verdict compares the two families and returns (code, one-line summary).
func Verdict(v4, v6 *FamilyResult) (string, string) {
	if v4.Worst() == NoClaude || v6.Worst() == NoClaude {
		return VerdictNoClaude, "Claude Code isn't available here — install it and log in, then re-run."
	}
	v6bad := v6.HangRate() > 0
	v4bad := v4.HangRate() > 0

	switch {
	case v6.Worst() == NoPath && v4.Worst() != NoPath:
		return VerdictNoV6, "This machine has no working IPv6, so IPv6 can't be causing your hangs here."
	case v6bad && !v4bad:
		return VerdictV6Hangs, "Confirmed: Claude hangs over IPv6 but runs fine over IPv4. This is the bug — and it's fixable."
	case v6bad && v4bad:
		return VerdictBothBad, "Both IPv4 and IPv6 are failing — this looks like something other than the address family (account limits, local network, or an outage)."
	case v4bad && !v6bad:
		return VerdictV4Hangs, "Unusual: IPv4 is the one struggling here. Forcing IPv4 would not help on this network."
	case !v6bad && !v4bad:
		return VerdictHealthy, "Both IPv4 and IPv6 completed cleanly right now. The hang is intermittent — if you're stuck, run this again while it's happening."
	default:
		return VerdictInconclusive, "Couldn't get a clear read. Try again, ideally while a hang is happening."
	}
}

// envWithoutProxy returns the current environment with any pre-existing proxy
// variables stripped, so our family-locked proxy is the only one in effect.
func envWithoutProxy() []string {
	drop := map[string]bool{"http_proxy": true, "https_proxy": true, "all_proxy": true}
	var out []string
	for _, kv := range os.Environ() {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if drop[strings.ToLower(name)] {
			continue
		}
		out = append(out, kv)
	}
	return out
}
