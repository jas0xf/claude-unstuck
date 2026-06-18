// Package report renders doctor results as a terminal table, a verdict, and a
// privacy-preserving shareable snippet.
package report

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jas0xf/claude-unstuck/internal/probe"
)

// Verdict codes.
const (
	VerdictHealthy      = "healthy"
	VerdictV6Degraded   = "v6-degraded"
	VerdictNoIPv6       = "no-ipv6"
	VerdictBothDegraded = "both-degraded"
	VerdictV4Degraded   = "v4-degraded"
)

// Verdict classifies a paired v4/v6 run.
func Verdict(v4, v6 *probe.Result) (code, summary string) {
	v4Stall := v4.StallCount() + v4.UnreachableCount()
	switch {
	case v6.UnreachableCount() == len(v6.Attempts) && len(v6.Attempts) > 0:
		return VerdictNoIPv6, "No IPv6 connectivity on this network — IPv6 cannot be the cause of hangs here."
	case v6.StallCount() >= 2 && v4Stall == 0:
		return VerdictV6Degraded, fmt.Sprintf(
			"Your IPv6 path to this host is degraded (%d/%d probes stalled) while IPv4 is clean. Forcing IPv4 should stop the hangs.",
			v6.StallCount(), len(v6.Attempts))
	case v6.StallCount() >= 2 && v4Stall > 0:
		return VerdictBothDegraded, "Both IPv4 and IPv6 show failures — this looks like a problem beyond address family (local network, upstream outage, or service-side)."
	case v4Stall >= 2 && v6.StallCount() == 0:
		return VerdictV4Degraded, "Unusual: IPv4 is degraded while IPv6 is clean. Forcing IPv4 would make things worse on this network."
	default:
		return VerdictHealthy, "Both address families look healthy right now. If hangs persist, they may be intermittent — re-run doctor while a hang is happening."
	}
}

// fmtMs renders a duration as milliseconds, or "—" for zero.
func fmtMs(d time.Duration) string {
	if d == 0 {
		return "—"
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}

// DoctorText renders the human-readable doctor report.
func DoctorText(host string, v4, v6 *probe.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nTarget: https://%s/  (unauthenticated GET, %d probes per family)\n\n", host, len(v4.Attempts))
	fmt.Fprintf(&b, "  %-6s %-7s %-4s %-18s %-13s %-9s %-10s %s\n",
		"family", "probes", "ok", "stalled", "connect(med)", "tls(med)", "ttfb(med)", "endpoints")
	for _, r := range []*probe.Result{v4, v6} {
		stalled := r.StallCount()
		extra := ""
		if u := r.UnreachableCount(); u > 0 {
			extra = fmt.Sprintf(" (+%d unreachable)", u)
		}
		fmt.Fprintf(&b, "  %-6s %-7d %-4d %-18s %-13s %-9s %-10s %s\n",
			r.Family, len(r.Attempts), r.OKCount(),
			fmt.Sprintf("%d%s", stalled, extra),
			fmtMs(r.Median(func(a probe.Attempt) time.Duration { return a.Connect })),
			fmtMs(r.Median(func(a probe.Attempt) time.Duration { return a.TLS })),
			fmtMs(r.Median(func(a probe.Attempt) time.Duration { return a.TTFB })),
			strings.Join(r.Resolved, ", "))
	}
	code, summary := Verdict(v4, v6)
	icon := map[string]string{
		VerdictHealthy:      "✅",
		VerdictV6Degraded:   "❌",
		VerdictNoIPv6:       "ℹ️ ",
		VerdictBothDegraded: "⚠️ ",
		VerdictV4Degraded:   "⚠️ ",
	}[code]
	fmt.Fprintf(&b, "\n%s %s\n", icon, summary)
	if code == VerdictV6Degraded {
		b.WriteString("\nNext steps:\n")
		b.WriteString("  claude-unstuck            # run Claude over IPv4 for this session (no root)\n")
		b.WriteString("  sudo claude-unstuck on    # fix system-wide (undo: sudo claude-unstuck off)\n")
	}
	return b.String()
}

// ShareSnippet renders an anonymized markdown block users can paste into
// GitHub issues or forums. Only the /32 of the local IPv6 prefix is included.
func ShareSnippet(host string, v4, v6 *probe.Result) string {
	code, _ := Verdict(v4, v6)
	var b strings.Builder
	b.WriteString("\n--- copy below to share (anonymized) ---\n")
	b.WriteString("```\n")
	fmt.Fprintf(&b, "claude-unstuck doctor — %s\n", time.Now().UTC().Format("2006-01-02 15:04 MST"))
	fmt.Fprintf(&b, "target: %s   verdict: %s\n", host, code)
	for _, r := range []*probe.Result{v4, v6} {
		fmt.Fprintf(&b, "%s: ok=%d/%d stalled=%d connect=%s ttfb=%s\n",
			r.Family, r.OKCount(), len(r.Attempts), r.StallCount(),
			fmtMs(r.Median(func(a probe.Attempt) time.Duration { return a.Connect })),
			fmtMs(r.Median(func(a probe.Attempt) time.Duration { return a.TTFB })))
	}
	if p := AnonymizedPrefix(v6); p != "" {
		fmt.Fprintf(&b, "local IPv6 prefix (/32): %s\n", p)
	}
	b.WriteString("via https://github.com/jas0xf/claude-unstuck\n")
	b.WriteString("```\n")
	return b.String()
}

// AnonymizedPrefix reduces the local IPv6 address observed during probing to
// its /32 (first two hextets) — enough to identify an ISP allocation, not a
// household.
func AnonymizedPrefix(v6 *probe.Result) string {
	for _, a := range v6.Attempts {
		if a.Local == "" {
			continue
		}
		hostPart := a.Local
		if i := strings.LastIndex(hostPart, ":"); i > 0 && strings.Contains(hostPart, "]") {
			hostPart = strings.Trim(hostPart[:i], "[]")
		}
		addr, err := netip.ParseAddr(strings.Trim(hostPart, "[]"))
		if err != nil || !addr.Is6() {
			continue
		}
		p, err := addr.Prefix(32)
		if err != nil {
			continue
		}
		return p.String()
	}
	return ""
}
