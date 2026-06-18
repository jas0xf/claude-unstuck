package detect

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jas0xf/claude-unstuck/internal/proxy"
)

func failedRec() proxy.ConnRecord {
	return proxy.ConnRecord{Host: "api.anthropic.com:443", Err: "dial tcp6 [2607:6bc0::10]:443: connect: invalid argument"}
}

// sideOK is a non-API host that connects fine over v6 (e.g. statsig/datadog),
// to prove NoPath keys on the API endpoint, not "every dial".
func sideOK() proxy.ConnRecord {
	return proxy.ConnRecord{Host: "statsig.anthropic.com:443", Remote: "[2a00::1]:443", Down: 4000}
}
func silentRec() proxy.ConnRecord {
	return proxy.ConnRecord{Host: "api:443", Remote: "[2607:6bc0::10]:443", Up: 900, Down: 0, Dur: 60 * time.Second}
}

func TestClassify(t *testing.T) {
	deadline := context.DeadlineExceeded
	killed := errors.New("signal: killed")
	cases := []struct {
		name    string
		execErr error
		ctxErr  error
		recs    []proxy.ConnRecord
		out     string
		want    Outcome
	}{
		// The regression: blackholed family — every dial fails fast, but claude
		// retries until the wall-timeout. Must be NoPath, NOT Hung.
		{"blackholed v6 (retries to deadline)", killed, deadline, []proxy.ConnRecord{failedRec(), failedRec(), failedRec()}, "", NoPath},
		// API blackholed but side hosts (statsig) reachable over v6 → still NoPath,
		// because the API endpoint specifically has no route.
		{"api blackholed, side hosts ok", killed, deadline, []proxy.ConnRecord{sideOK(), failedRec(), sideOK()}, "", NoPath},
		// A real mid-stream withhold: connection opened, then silence to deadline.
		{"real hang (connection silent)", killed, deadline, []proxy.ConnRecord{silentRec()}, "", Hung},
		{"healthy", nil, nil, []proxy.ConnRecord{{Remote: "160.79.104.10:443", Down: 4000}}, "ok", OK},
		{"not logged in", errors.New("exit 1"), nil, nil, "Invalid API key · Please run /login", NoClaude},
		{"empty output", nil, nil, []proxy.ConnRecord{{Remote: "x", Down: 10}}, "  ", Errored},
	}
	for _, c := range cases {
		got, _ := classify("tcp6", c.execErr, c.ctxErr, c.recs, c.out, 60*time.Second)
		if got != c.want {
			t.Errorf("%s: classify = %s, want %s", c.name, got, c.want)
		}
	}
}

func fam(name string, outcomes ...Outcome) *FamilyResult {
	f := &FamilyResult{Family: name}
	for _, o := range outcomes {
		f.Turns = append(f.Turns, TurnResult{Outcome: o, Wall: time.Second})
	}
	return f
}

func TestVerdict(t *testing.T) {
	cases := []struct {
		name   string
		v4, v6 *FamilyResult
		want   string
	}{
		{"v6 hangs, v4 ok", fam("IPv4", OK, OK, OK), fam("IPv6", Hung, OK, Hung), VerdictV6Hangs},
		{"healthy", fam("IPv4", OK, OK), fam("IPv6", OK, OK), VerdictHealthy},
		{"no v6", fam("IPv4", OK, OK), fam("IPv6", NoPath, NoPath), VerdictNoV6},
		{"both bad", fam("IPv4", Hung, OK), fam("IPv6", Hung, Hung), VerdictBothBad},
		{"v4 hangs", fam("IPv4", Hung, Hung), fam("IPv6", OK, OK), VerdictV4Hangs},
		{"no claude", fam("IPv4", NoClaude), fam("IPv6", NoClaude), VerdictNoClaude},
		{"v6 errors count as bad", fam("IPv4", OK, OK), fam("IPv6", Errored, OK), VerdictV6Hangs},
	}
	for _, c := range cases {
		got, _ := Verdict(c.v4, c.v6)
		if got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestHangRate(t *testing.T) {
	f := fam("IPv6", Hung, OK, OK, Errored)
	if got := f.HangRate(); got != 0.5 {
		t.Errorf("HangRate = %v, want 0.5", got)
	}
	if fam("x").HangRate() != 0 {
		t.Error("empty HangRate should be 0")
	}
}

func TestWorst(t *testing.T) {
	if got := fam("x", OK, Errored, Hung).Worst(); got != Hung {
		t.Errorf("Worst = %v, want hung", got)
	}
	if got := fam("x", OK, OK).Worst(); got != OK {
		t.Errorf("Worst = %v, want ok", got)
	}
}
