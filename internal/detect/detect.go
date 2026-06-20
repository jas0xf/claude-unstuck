// Package detect reproduces the real Claude Code hang by running an actual
// `claude -p` turn locked to a single address family, then comparing IPv4 vs
// IPv6. A plain HTTPS GET cannot reproduce the freeze because the withhold
// happens mid-stream, after message_start — only a real completion exercises
// that path. Runs are few and sequential to avoid bursty behavior.
package detect

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/jas0xf/claude-unstuck/internal/proxy"
)

// Outcome classifies one family's real-session behavior.
type Outcome string

const (
	OK       Outcome = "ok"        // session completed with output
	Hung     Outcome = "hung"      // exceeded timeout / killed mid-stream
	Errored  Outcome = "errored"   // claude returned an API error / empty
	NoPath   Outcome = "no_path"   // family has no route at all
	NoClaude Outcome = "no_claude" // claude binary not found / not logged in
)

// TurnResult is one real `claude -p` turn over a fixed family.
type TurnResult struct {
	Outcome   Outcome
	Wall      time.Duration // total time for the turn
	MaxSilent time.Duration // longest single-connection silence (hang signature)
	Detail    string
}

// FamilyResult aggregates the turns for one family.
type FamilyResult struct {
	Family  string // "IPv4" / "IPv6"
	Network string // "tcp4" / "tcp6"
	Turns   []TurnResult
}

// HangRate is the fraction of turns that hung or errored.
func (f *FamilyResult) HangRate() float64 {
	if len(f.Turns) == 0 {
		return 0
	}
	bad := 0
	for _, t := range f.Turns {
		if t.Outcome == Hung || t.Outcome == Errored {
			bad++
		}
	}
	return float64(bad) / float64(len(f.Turns))
}

// HungCount is the number of turns that actually hung (a mid-stream timeout) —
// the IPv6-path signature, as distinct from a generic API error.
func (f *FamilyResult) HungCount() int {
	n := 0
	for _, t := range f.Turns {
		if t.Outcome == Hung {
			n++
		}
	}
	return n
}

// BadCount is the number of turns that hung or errored.
func (f *FamilyResult) BadCount() int {
	n := 0
	for _, t := range f.Turns {
		if t.Outcome == Hung || t.Outcome == Errored {
			n++
		}
	}
	return n
}

// Worst returns the most severe outcome observed for the family.
func (f *FamilyResult) Worst() Outcome {
	rank := map[Outcome]int{OK: 0, Errored: 1, Hung: 2, NoPath: 3, NoClaude: 4}
	worst := OK
	for _, t := range f.Turns {
		if rank[t.Outcome] > rank[worst] {
			worst = t.Outcome
		}
	}
	return worst
}

// MedianWall returns the median wall time across completed turns.
func (f *FamilyResult) MedianWall() time.Duration {
	var v []time.Duration
	for _, t := range f.Turns {
		if t.Outcome == OK {
			v = append(v, t.Wall)
		}
	}
	if len(v) == 0 {
		return 0
	}
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
	return v[len(v)/2]
}

// Options configures a detection run.
type Options struct {
	ClaudePath string        // path to claude binary (default "claude")
	Prompt     string        // tiny prompt for the turn
	Turns      int           // turns per family
	Timeout    time.Duration // per-turn wall timeout (a hang trips this)
	Gap        time.Duration // spacing between turns
}

func (o *Options) withDefaults() {
	if o.ClaudePath == "" {
		o.ClaudePath = "claude"
	}
	if o.Prompt == "" {
		o.Prompt = "Reply with the single word: ok"
	}
	if o.Turns <= 0 {
		o.Turns = 3
	}
	if o.Timeout <= 0 {
		o.Timeout = 60 * time.Second
	}
	if o.Gap <= 0 {
		o.Gap = 1 * time.Second
	}
}

// RunFamily runs Turns real claude sessions over the given network ("tcp4" or
// "tcp6"). progress, if non-nil, is called before each turn with (i, total).
func RunFamily(ctx context.Context, network string, opt Options, progress func(i, total int)) *FamilyResult {
	opt.withDefaults()
	fam := "IPv4"
	if network == "tcp6" {
		fam = "IPv6"
	}
	res := &FamilyResult{Family: fam, Network: network}
	for i := 0; i < opt.Turns; i++ {
		if progress != nil {
			progress(i, opt.Turns)
		}
		res.Turns = append(res.Turns, runTurn(ctx, network, opt))
		if i < opt.Turns-1 {
			select {
			case <-time.After(opt.Gap):
			case <-ctx.Done():
				return res
			}
		}
	}
	return res
}

func runTurn(ctx context.Context, network string, opt Options) TurnResult {
	srv, err := proxy.Listen("127.0.0.1:0", network, nil)
	if err != nil {
		return TurnResult{Outcome: Errored, Detail: "proxy: " + err.Error()}
	}
	defer srv.Close()

	turnCtx, cancel := context.WithTimeout(ctx, opt.Timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(turnCtx, opt.ClaudePath, "-p", opt.Prompt)
	cmd.Env = append(envWithoutProxy(), "HTTPS_PROXY="+srv.URL(), "HTTP_PROXY="+srv.URL(),
		"https_proxy="+srv.URL(), "http_proxy="+srv.URL())
	out, err := cmd.CombinedOutput()
	wall := time.Since(start)

	recs := srv.Records()
	outcome, detail := classify(network, err, turnCtx.Err(), recs, string(out), opt.Timeout)
	return TurnResult{Outcome: outcome, Wall: wall, MaxSilent: maxSilent(recs), Detail: detail}
}

// classify turns the raw signals of one turn into an Outcome. It is pure so the
// ordering rules can be unit-tested without a real claude binary.
//
// Order matters:
//  1. NoClaude  — the binary couldn't start, or output shows a login/auth error.
//  2. NoPath    — at least one upstream dial happened and EVERY one failed to
//     connect (e.g. a blackholed family). This MUST come before the deadline
//     check: a blackholed dial fails instantly, but Claude Code retries until
//     our wall-timeout, which would otherwise look like a hang.
//  3. Hung      — the wall-timeout tripped while a connection was actually open
//     (the real mid-stream withhold).
//  4. Errored   — claude exited non-zero, or produced nothing.
//  5. OK.
func classify(network string, execErr, ctxErr error, recs []ConnRecord, out string, timeout time.Duration) (Outcome, string) {
	if execErr != nil && isMissingBinary(execErr) {
		return NoClaude, "claude not found on PATH"
	}
	if looksLikeAuthError(out) {
		return NoClaude, "claude isn't logged in (auth error)"
	}
	if hasUnreachableAPI(recs) || onlyFailedDials(recs) {
		return NoPath, network + " has no usable route to the Anthropic API (connect failed)"
	}
	if errors.Is(ctxErr, context.DeadlineExceeded) {
		return Hung, "no response within " + timeout.String()
	}
	if execErr != nil {
		return Errored, firstLine(out)
	}
	if len(strings.TrimSpace(out)) == 0 {
		return Errored, "empty response"
	}
	return OK, firstLine(out)
}

// ConnRecord is re-exported from the proxy package for classify's signature.
type ConnRecord = proxy.ConnRecord

func looksLikeAuthError(out string) bool {
	s := strings.ToLower(out)
	return strings.Contains(s, "invalid api key") ||
		strings.Contains(s, "not logged in") ||
		strings.Contains(s, "please run /login") ||
		strings.Contains(s, "authentication_error") ||
		strings.Contains(s, "unauthorized")
}

// maxSilent returns the longest single connection that moved little data —
// the hang signature (bytes up, ~zero down, long duration).
func maxSilent(recs []proxy.ConnRecord) time.Duration {
	var m time.Duration
	for _, r := range recs {
		if r.Down < 64 && r.Dur > m {
			m = r.Dur
		}
	}
	return m
}

// hasUnreachableAPI reports whether the connection to an Anthropic API host
// failed with a hard "no route" error (blackhole, network unreachable). This is
// the signature of a missing/blocked family, as opposed to a mid-stream hang
// (where the connection opens fine and then goes silent).
func hasUnreachableAPI(recs []proxy.ConnRecord) bool {
	for _, r := range recs {
		if r.Err == "" || !strings.Contains(r.Host, "anthropic") {
			continue
		}
		if isUnreachableErr(r.Err) {
			return true
		}
	}
	return false
}

func isUnreachableErr(s string) bool {
	s = strings.ToLower(s)
	for _, m := range []string{
		"invalid argument", "network is unreachable", "no route to host",
		"no suitable address", "cannot assign requested address", "host is down",
	} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

func onlyFailedDials(recs []proxy.ConnRecord) bool {
	if len(recs) == 0 {
		return false
	}
	for _, r := range recs {
		if r.Err == "" {
			return false
		}
	}
	return true
}

func isMissingBinary(err error) bool {
	return errors.Is(err, exec.ErrNotFound) ||
		strings.Contains(strings.ToLower(err.Error()), "executable file not found")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:117] + "..."
	}
	return s
}
