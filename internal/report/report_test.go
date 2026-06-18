package report

import (
	"strings"
	"testing"
	"time"

	"github.com/jas0xf/claude-unstuck/internal/probe"
)

func result(family string, ok, stalled, unreachable int) *probe.Result {
	r := &probe.Result{Family: family}
	for i := 0; i < ok; i++ {
		r.Attempts = append(r.Attempts, probe.Attempt{
			Status: 403, Connect: 12 * time.Millisecond,
			TLS: 30 * time.Millisecond, TTFB: 90 * time.Millisecond,
			Local: "[2603:8000:aaaa:bbbb::1]:55555",
		})
	}
	for i := 0; i < stalled; i++ {
		r.Attempts = append(r.Attempts, probe.Attempt{Err: "context deadline exceeded"})
	}
	for i := 0; i < unreachable; i++ {
		r.Attempts = append(r.Attempts, probe.Attempt{Err: "connect: network is unreachable"})
	}
	return r
}

func TestVerdicts(t *testing.T) {
	cases := []struct {
		name   string
		v4, v6 *probe.Result
		want   string
	}{
		{"healthy", result("IPv4", 6, 0, 0), result("IPv6", 6, 0, 0), VerdictHealthy},
		{"v6 degraded", result("IPv4", 6, 0, 0), result("IPv6", 2, 4, 0), VerdictV6Degraded},
		{"no ipv6", result("IPv4", 6, 0, 0), result("IPv6", 0, 0, 6), VerdictNoIPv6},
		{"both degraded", result("IPv4", 3, 3, 0), result("IPv6", 2, 4, 0), VerdictBothDegraded},
		{"v4 degraded", result("IPv4", 2, 4, 0), result("IPv6", 6, 0, 0), VerdictV4Degraded},
		{"single v6 blip is healthy", result("IPv4", 6, 0, 0), result("IPv6", 5, 1, 0), VerdictHealthy},
	}
	for _, c := range cases {
		code, _ := Verdict(c.v4, c.v6)
		if code != c.want {
			t.Errorf("%s: verdict = %s, want %s", c.name, code, c.want)
		}
	}
}

func TestAnonymizedPrefix(t *testing.T) {
	v6 := result("IPv6", 1, 0, 0)
	got := AnonymizedPrefix(v6)
	if got != "2603:8000::/32" {
		t.Errorf("AnonymizedPrefix = %q, want 2603:8000::/32", got)
	}
	// Full address must never leak into the share snippet.
	snip := ShareSnippet("api.anthropic.com", result("IPv4", 6, 0, 0), v6)
	if strings.Contains(snip, "aaaa:bbbb") {
		t.Errorf("share snippet leaks full local address:\n%s", snip)
	}
	if !strings.Contains(snip, "2603:8000::/32") {
		t.Errorf("share snippet missing anonymized prefix:\n%s", snip)
	}
}

func TestDoctorTextRenders(t *testing.T) {
	out := DoctorText("api.anthropic.com", result("IPv4", 6, 0, 0), result("IPv6", 2, 4, 0))
	for _, want := range []string{"IPv4", "IPv6", "stalled", "sudo claude-unstuck on"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor text missing %q:\n%s", want, out)
		}
	}
}
