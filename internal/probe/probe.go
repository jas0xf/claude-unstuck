// Package probe measures the health of the HTTPS path to a host over a single
// address family (IPv4 or IPv6) using a small number of gentle, sequential,
// unauthenticated requests.
//
// Design constraint: probes must stay unauthenticated and few. Bursty or
// credentialed probing of api.anthropic.com can trip provider-side abuse
// heuristics; this package deliberately caps the probe count and paces
// requests.
package probe

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
)

// MaxCount is the hard cap on probes per family. Keep this small on purpose.
const MaxCount = 12

// Options configures a probe run for one family.
type Options struct {
	Host    string
	Count   int
	Timeout time.Duration
	Gap     time.Duration
}

// Attempt records the timing of a single HTTPS request.
type Attempt struct {
	DNS     time.Duration `json:"dns_ns"`
	Connect time.Duration `json:"connect_ns"`
	TLS     time.Duration `json:"tls_ns"`
	TTFB    time.Duration `json:"ttfb_ns"`
	Total   time.Duration `json:"total_ns"`
	Local   string        `json:"local,omitempty"`
	Remote  string        `json:"remote,omitempty"`
	Status  int           `json:"status,omitempty"`
	Err     string        `json:"err,omitempty"`
}

// OK reports whether the attempt completed with an HTTP response.
func (a Attempt) OK() bool { return a.Err == "" && a.Status != 0 }

// Unreachable reports whether the attempt failed instantly because the family
// has no usable path at all (as opposed to a stall on a working path).
func (a Attempt) Unreachable() bool {
	e := strings.ToLower(a.Err)
	return strings.Contains(e, "network is unreachable") ||
		strings.Contains(e, "no route to host") ||
		strings.Contains(e, "no suitable address") ||
		strings.Contains(e, "no records") ||
		strings.Contains(e, "no such host") ||
		// Linux blackhole routes fail connect(2) with EINVAL immediately.
		strings.Contains(e, "invalid argument")
}

// Result aggregates the attempts for one address family.
type Result struct {
	Family   string    `json:"family"` // "IPv4" or "IPv6"
	Network  string    `json:"network"`
	Resolved []string  `json:"resolved,omitempty"`
	Attempts []Attempt `json:"attempts"`
}

// OKCount returns how many attempts completed.
func (r *Result) OKCount() int {
	n := 0
	for _, a := range r.Attempts {
		if a.OK() {
			n++
		}
	}
	return n
}

// StallCount returns attempts that failed on a reachable path (timeouts,
// resets) — the signature this tool exists to detect.
func (r *Result) StallCount() int {
	n := 0
	for _, a := range r.Attempts {
		if !a.OK() && !a.Unreachable() {
			n++
		}
	}
	return n
}

// UnreachableCount returns attempts that failed instantly with no path.
func (r *Result) UnreachableCount() int {
	n := 0
	for _, a := range r.Attempts {
		if a.Unreachable() {
			n++
		}
	}
	return n
}

// Median returns the median of f over completed attempts, or 0 if none.
func (r *Result) Median(f func(Attempt) time.Duration) time.Duration {
	var vals []time.Duration
	for _, a := range r.Attempts {
		if a.OK() {
			vals = append(vals, f(a))
		}
	}
	if len(vals) == 0 {
		return 0
	}
	for i := 1; i < len(vals); i++ {
		for j := i; j > 0 && vals[j] < vals[j-1]; j-- {
			vals[j], vals[j-1] = vals[j-1], vals[j]
		}
	}
	return vals[len(vals)/2]
}

// Run executes opt.Count sequential probes over network ("tcp4" or "tcp6").
// A progress callback, if non-nil, is invoked after each attempt.
func Run(ctx context.Context, network string, opt Options, progress func(i int, a Attempt)) *Result {
	if opt.Count <= 0 {
		opt.Count = 6
	}
	if opt.Count > MaxCount {
		opt.Count = MaxCount
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 10 * time.Second
	}
	if opt.Gap <= 0 {
		opt.Gap = 750 * time.Millisecond
	}
	fam := "IPv4"
	if network == "tcp6" {
		fam = "IPv6"
	}
	res := &Result{Family: fam, Network: network}
	for i := 0; i < opt.Count; i++ {
		if ctx.Err() != nil {
			break
		}
		a := attempt(ctx, network, opt.Host, opt.Timeout)
		res.Attempts = append(res.Attempts, a)
		if a.Remote != "" {
			res.Resolved = appendUnique(res.Resolved, hostOf(a.Remote))
		}
		if progress != nil {
			progress(i, a)
		}
		if i < opt.Count-1 {
			select {
			case <-time.After(opt.Gap):
			case <-ctx.Done():
			}
		}
	}
	return res
}

func attempt(ctx context.Context, network, host string, timeout time.Duration) Attempt {
	var a Attempt
	start := time.Now()
	var dnsStart, connStart, tlsStart time.Time
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:  func(httptrace.DNSDoneInfo) { a.DNS = time.Since(dnsStart) },
		ConnectStart: func(_, _ string) {
			if connStart.IsZero() {
				connStart = time.Now()
			}
		},
		ConnectDone: func(_, addr string, err error) {
			a.Connect = time.Since(connStart)
			if err == nil {
				a.Remote = addr
			}
		},
		TLSHandshakeStart: func() { tlsStart = time.Now() },
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			a.TLS = time.Since(tlsStart)
		},
		GotConn: func(ci httptrace.GotConnInfo) {
			if ci.Conn != nil {
				a.Local = ci.Conn.LocalAddr().String()
			}
		},
		GotFirstResponseByte: func() { a.TTFB = time.Since(start) },
	}
	dialer := &net.Dialer{Timeout: timeout}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
		DisableKeepAlives:   true,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: timeout,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: timeout}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace),
		http.MethodGet, "https://"+host+"/", nil)
	if err != nil {
		a.Err = err.Error()
		return a
	}
	req.Header.Set("User-Agent", "claude-unstuck/doctor (+https://github.com/jas0xf/claude-unstuck)")
	resp, err := client.Do(req)
	a.Total = time.Since(start)
	if err != nil {
		a.Err = err.Error()
		return a
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	a.Status = resp.StatusCode
	return a
}

func hostOf(addrPort string) string {
	h, _, err := net.SplitHostPort(addrPort)
	if err != nil {
		return addrPort
	}
	return h
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
