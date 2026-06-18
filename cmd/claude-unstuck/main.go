// claude-unstuck diagnoses and fixes IPv6-path hangs affecting Claude Code
// and other Anthropic API clients.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/jas0xf/claude-unstuck/internal/boot"
	"github.com/jas0xf/claude-unstuck/internal/detect"
	"github.com/jas0xf/claude-unstuck/internal/dnsutil"
	"github.com/jas0xf/claude-unstuck/internal/probe"
	"github.com/jas0xf/claude-unstuck/internal/proxy"
	"github.com/jas0xf/claude-unstuck/internal/report"
	"github.com/jas0xf/claude-unstuck/internal/route"
	"github.com/jas0xf/claude-unstuck/internal/state"
	"github.com/jas0xf/claude-unstuck/internal/ui"
)

var version = "dev" // set via -ldflags at release time

const defaultDomains = "api.anthropic.com,statsig.anthropic.com,console.anthropic.com"

const usage = `claude-unstuck — stop Claude Code freezing on bad IPv6 paths.

  claude-unstuck                  Run Claude Code over IPv4 for this session
                                  (no root, nothing permanent). Just use Claude.
  claude-unstuck -- CMD …         Run any command over IPv4 instead of claude.

  claude-unstuck doctor           Check your connection (runs a couple of real
                                  Claude turns, a few tokens). Changes nothing.

  sudo claude-unstuck on          Install the fix system-wide (every app).
                                    --persist   also survive reboots
  sudo claude-unstuck off         Remove the system-wide fix.
  claude-unstuck status           Show what's installed.

  claude-unstuck version
`

// knownCommands are the subcommands; anything else is treated as a command to
// run over IPv4 (transparent passthrough to `claude`).
var knownCommands = map[string]bool{
	"doctor": true, "on": true, "off": true, "status": true,
	"run": true, "probe": true,
	"version": true, "--version": true, "-v": true,
	"help": true, "--help": true, "-h": true,
}

func main() {
	// Bare invocation, or anything that isn't a subcommand, runs Claude (or the
	// given command) over IPv4 for this session.
	if len(os.Args) < 2 || !knownCommands[os.Args[1]] {
		os.Exit(cmdSession(os.Args[1:]))
	}
	var err error
	switch os.Args[1] {
	case "doctor":
		os.Exit(cmdDoctor(os.Args[2:]))
	case "on":
		err = cmdOn(os.Args[2:])
	case "off":
		err = cmdOff(os.Args[2:])
	case "status":
		err = cmdStatus(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "probe":
		err = cmdProbe(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("claude-unstuck %s\n", version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// cmdSession runs Claude Code (or a given command) over IPv4 for this session.
// This is the bare `claude-unstuck` experience: no root, nothing persistent.
func cmdSession(args []string) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	cmdArgs := []string{"claude"}
	if len(args) > 0 {
		// Passthrough: `claude-unstuck -p "hi"` => claude -p "hi";
		// `claude-unstuck -- node x` already stripped the `--` above.
		if looksLikeCommand(args[0]) {
			cmdArgs = args
		} else {
			cmdArgs = append([]string{"claude"}, args...)
		}
	}
	if err := launchOverIPv4(cmdArgs, 0, true); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// looksLikeCommand reports whether tok is a program name (not a flag), so we
// can decide between wrapping `claude <flags>` and running `<tok> …` directly.
func looksLikeCommand(tok string) bool {
	return tok != "" && !strings.HasPrefix(tok, "-")
}

// launchOverIPv4 starts an IPv4-only proxy and runs cmdArgs with HTTPS_PROXY
// pointed at it, then prints a verification receipt. quiet hides per-connection
// log lines (so interactive Claude isn't cluttered).
func launchOverIPv4(cmdArgs []string, port int, quiet bool) error {
	logf := func(format string, a ...any) {
		if !quiet {
			fmt.Fprintf(os.Stderr, "[claude-unstuck] "+format+"\n", a...)
		}
	}
	srv, err := proxy.Listen(fmt.Sprintf("127.0.0.1:%d", port), "tcp4", logf)
	if err != nil {
		return err
	}
	defer srv.Close()
	fmt.Fprintf(os.Stderr, "[claude-unstuck] running over IPv4: %s\n", strings.Join(cmdArgs, " "))

	child := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	child.Env = append(os.Environ(),
		"HTTPS_PROXY="+srv.URL(), "HTTP_PROXY="+srv.URL(),
		"https_proxy="+srv.URL(), "http_proxy="+srv.URL(),
	)
	if err := child.Start(); err != nil {
		return fmt.Errorf("start %s: %w", cmdArgs[0], err)
	}
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	go func() {
		for s := range sigc {
			_ = child.Process.Signal(s)
		}
	}()
	waitErr := child.Wait()
	signal.Stop(sigc)
	close(sigc)

	printRunSummary(srv.Records())
	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		return waitErr
	}
	return nil
}

// cmdRun is the explicit form: `claude-unstuck run [--port P] -- CMD …`.
func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	port := fs.Int("port", 0, "proxy port (0 = ephemeral)")
	quiet := fs.Bool("quiet", false, "suppress per-connection log lines")
	_ = fs.Parse(args)
	cmdArgs := fs.Args()
	if len(cmdArgs) > 0 && cmdArgs[0] == "--" {
		cmdArgs = cmdArgs[1:]
	}
	if len(cmdArgs) == 0 {
		cmdArgs = []string{"claude"}
	}
	return launchOverIPv4(cmdArgs, *port, *quiet)
}

func printRunSummary(recs []proxy.ConnRecord) {
	if len(recs) == 0 {
		fmt.Fprintln(os.Stderr, "[claude-unstuck] no connections were tunneled — the command may not honor HTTPS_PROXY")
		return
	}
	allV4, failed := true, 0
	for _, r := range recs {
		if r.Err != "" {
			failed++
			continue
		}
		host := r.Remote
		if i := strings.LastIndex(host, ":"); i > 0 {
			host = strings.Trim(host[:i], "[]")
		}
		if a, err := netip.ParseAddr(host); err != nil || !a.Is4() {
			allV4 = false
		}
	}
	switch {
	case failed == len(recs):
		fmt.Fprintln(os.Stderr, "[claude-unstuck] ⚠️  all tunneled connections failed")
	case allV4:
		fmt.Fprintf(os.Stderr, "[claude-unstuck] ✅ done — all %d upstream connections used IPv4\n", len(recs))
	default:
		fmt.Fprintln(os.Stderr, "[claude-unstuck] ⚠️  some upstream connections were not IPv4 (unexpected — please report)")
	}
}

// cmdDoctor checks the connection by running a few real Claude turns over each
// address family — the only way to reproduce a mid-stream freeze — and prints a
// plain-language verdict. Changes nothing. Returns a process exit code.
func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	turns := fs.Int("turns", 2, "real Claude turns per family (kept low — uses a few tokens)")
	timeout := fs.Duration("timeout", 60*time.Second, "per-turn timeout (a hang trips this)")
	jsonOut := fs.Bool("json", false, "emit JSON instead of the animated report")
	full := fs.Bool("full", false, "force the full real-session check even when the fix is ON")
	_ = fs.Parse(args)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// When our system-wide fix is ON, IPv6 to Anthropic is intentionally blocked.
	// Running real v6 Claude turns there just hangs to the timeout, so instead do
	// a fast, token-free verification: is IPv4 healthy and is IPv6 redirected?
	if !*full {
		if st, _ := state.Load(); st != nil && len(st.Applied) > 0 {
			return doctorVerifyFixOn(ctx, *jsonOut)
		}
	}

	opt := detect.Options{Turns: *turns, Timeout: *timeout}

	if *jsonOut {
		v4 := detect.RunFamily(ctx, "tcp4", opt, nil)
		v6 := detect.RunFamily(ctx, "tcp6", opt, nil)
		code, summary := detect.Verdict(v4, v6)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"verdict": code, "summary": summary, "ipv4": v4, "ipv6": v6})
		if code == detect.VerdictV6Hangs {
			return 1
		}
		return 0
	}

	sp := ui.NewSpinner(os.Stderr, ui.IsTTY(os.Stderr))
	fmt.Fprintf(os.Stderr, "\n%s\n", sp.Bold("claude-unstuck — checking if Claude Code hangs on your connection"))
	fmt.Fprintf(os.Stderr, "%s\n\n", sp.Dim(fmt.Sprintf("Running %d real Claude turns per path (IPv4 and IPv6). Uses a few tokens.", *turns)))

	v4 := runFamilyAnimated(ctx, sp, "tcp4", "IPv4", opt)
	v6 := runFamilyAnimated(ctx, sp, "tcp6", "IPv6", opt)

	code, summary := detect.Verdict(v4, v6)
	fmt.Fprintln(os.Stderr)
	switch code {
	case detect.VerdictV6Hangs:
		fmt.Fprintf(os.Stderr, "  %s  %s\n", sp.Red("➜ DIAGNOSIS"), sp.Bold(summary))
		fmt.Fprintf(os.Stderr, "\n  %s\n", sp.Bold("Fix it now (no admin, just use Claude normally):"))
		fmt.Fprintf(os.Stderr, "      %s\n", sp.Green("claude-unstuck"))
		fmt.Fprintf(os.Stderr, "\n  %s\n", sp.Dim("Prefer a permanent, system-wide fix? Once you trust it:"))
		fmt.Fprintf(os.Stderr, "      %s\n", sp.Dim("sudo claude-unstuck on --persist"))
		return 1
	case detect.VerdictHealthy:
		fmt.Fprintf(os.Stderr, "  %s  %s\n", sp.Green("➜ ALL GOOD"), summary)
		return 0
	case detect.VerdictNoClaude:
		fmt.Fprintf(os.Stderr, "  %s  %s\n", sp.Yellow("➜ CAN'T CHECK"), summary)
		return 2
	default:
		fmt.Fprintf(os.Stderr, "  %s  %s\n", sp.Yellow("➜ NOTE"), summary)
		return 0
	}
}

// doctorVerifyFixOn is the fast path used when the system-wide fix is ON. It
// does NOT run real Claude turns (which would hang to the timeout over the
// blocked IPv6). Instead it uses quick unauthenticated probes to confirm two
// things: IPv4 to Anthropic is healthy, and IPv6 to Anthropic is blocked (so
// every Claude connection falls back to IPv4). Returns a process exit code.
func doctorVerifyFixOn(ctx context.Context, jsonOut bool) int {
	const host = "api.anthropic.com"
	popt := probe.Options{Host: host, Count: 3, Timeout: 8 * time.Second, Gap: 300 * time.Millisecond}

	if jsonOut {
		v4 := probe.Run(ctx, "tcp4", popt, nil)
		v4ok := v4.OKCount() > 0
		redirected, _ := verifyRedirect(ctx, popt)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"mode": "verify-fix-on", "mechanism": redirectMechanism(),
			"ipv4_healthy": v4ok, "ipv6_redirected": redirected,
			"ok": v4ok && redirected, "ipv4": v4,
		})
		if v4ok && redirected {
			return 0
		}
		return 1
	}

	sp := ui.NewSpinner(os.Stderr, ui.IsTTY(os.Stderr))
	fmt.Fprintf(os.Stderr, "\n%s\n", sp.Bold("claude-unstuck — verifying the system-wide fix"))
	fmt.Fprintf(os.Stderr, "%s\n\n", sp.Dim("Fast check (no Claude turns): is IPv4 healthy and is traffic on IPv4?"))

	sp.Start("Checking the IPv4 path to Anthropic …")
	v4 := probe.Run(ctx, "tcp4", popt, nil)
	v4ok := v4.OKCount() > 0
	if v4ok {
		ms := v4.Median(func(a probe.Attempt) time.Duration { return a.Connect }).Milliseconds()
		sp.Succeed(fmt.Sprintf("IPv4 path healthy (connect ~%dms)", ms))
	} else {
		sp.Fail("IPv4 path is NOT reachable")
	}

	sp.Start("Confirming Claude traffic is on IPv4 …")
	redirected, redirectDetail := verifyRedirect(ctx, popt)
	if redirected {
		sp.Succeed(redirectDetail)
	} else {
		sp.Fail(redirectDetail)
	}

	fmt.Fprintln(os.Stderr)
	switch {
	case v4ok && redirected:
		fmt.Fprintf(os.Stderr, "  %s  Fix is working — every Claude connection will use IPv4.\n", sp.Green("➜ ALL GOOD"))
		return 0
	case v4ok && !redirected:
		fmt.Fprintf(os.Stderr, "  %s  Fix is ON but traffic isn't all on IPv4 — Anthropic's addresses may have rotated.\n", sp.Yellow("➜ REFRESH"))
		fmt.Fprintf(os.Stderr, "      %s\n", sp.Dim("Run `sudo claude-unstuck on` to re-cover them, or `claude-unstuck status` for details."))
		return 1
	default:
		fmt.Fprintf(os.Stderr, "  %s  IPv4 itself is failing — this is beyond IPv6 (local network or an outage).\n", sp.Red("➜ PROBLEM"))
		return 1
	}
}

// verifyRedirect confirms IPv6 to Anthropic is blocked (so Claude falls back to
// IPv4). Every platform now blocks v6 to Anthropic's addresses — a scoped
// blackhole route on macOS/Linux, a scoped firewall rule on Windows — so the
// check is the same everywhere: a direct IPv6 connect to the API must FAIL.
func verifyRedirect(ctx context.Context, popt probe.Options) (ok bool, detail string) {
	v6 := probe.Run(ctx, "tcp6", popt, nil)
	if v6.OKCount() == 0 {
		return true, "IPv6 to Anthropic is blocked → Claude uses IPv4"
	}
	return false, fmt.Sprintf("IPv6 to Anthropic is STILL reachable (%d/%d) — fix may need a refresh", v6.OKCount(), len(v6.Attempts))
}

func redirectMechanism() string {
	if runtime.GOOS == "windows" {
		return "firewall"
	}
	return "blackhole"
}

func runFamilyAnimated(ctx context.Context, sp *ui.Spinner, network, fam string, opt detect.Options) *detect.FamilyResult {
	sp.Start(fmt.Sprintf("Testing Claude over %s …", fam))
	res := detect.RunFamily(ctx, network, opt, func(i, total int) {
		sp.Update(fmt.Sprintf("Testing Claude over %s … (turn %d/%d)", fam, i+1, total))
	})
	switch res.Worst() {
	case detect.OK:
		sp.Succeed(fmt.Sprintf("%s — Claude responded every time (median %.1fs)", fam, res.MedianWall().Seconds()))
	case detect.Hung:
		sp.Fail(fmt.Sprintf("%s — Claude HUNG (%.0f%% of turns froze)", fam, res.HangRate()*100))
	case detect.Errored:
		sp.Fail(fmt.Sprintf("%s — Claude errored (%.0f%% of turns)", fam, res.HangRate()*100))
	case detect.NoPath:
		sp.Info(fmt.Sprintf("%s — no usable %s route on this machine", fam, fam))
	case detect.NoClaude:
		sp.Fail(fmt.Sprintf("%s — couldn't run the `claude` command", fam))
	}
	return res
}

// cmdProbe is the low-level, zero-token connectivity probe (unauthenticated
// HTTPS GETs). It cannot see a mid-stream freeze, so `doctor` is preferred; this
// stays for a quick "is IPv6 even routable" sanity check and a shareable report.
func cmdProbe(args []string) error {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	host := fs.String("host", "api.anthropic.com", "host to probe")
	count := fs.Int("probes", 6, fmt.Sprintf("probes per family (max %d)", probe.MaxCount))
	timeout := fs.Duration("timeout", 10*time.Second, "per-probe timeout")
	jsonOut := fs.Bool("json", false, "emit JSON instead of text")
	_ = fs.Parse(args)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	opt := probe.Options{Host: *host, Count: *count, Timeout: *timeout}

	if !*jsonOut {
		fmt.Fprintf(os.Stderr, "Probing %s over IPv4 then IPv6 (%d each, unauthenticated)...\n", *host, opt.Count)
	}
	v4 := probe.Run(ctx, "tcp4", opt, nil)
	v6 := probe.Run(ctx, "tcp6", opt, nil)

	if *jsonOut {
		code, summary := report.Verdict(v4, v6)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"host": *host, "verdict": code, "summary": summary, "ipv4": v4, "ipv6": v6})
	}
	fmt.Print(report.DoctorText(*host, v4, v6))
	fmt.Print(report.ShareSnippet(*host, v4, v6))
	return nil
}

// cmdOn installs the system-wide IPv4 fix (needs root). With --persist it also
// re-applies at every boot.
func cmdOn(args []string) error {
	fs := flag.NewFlagSet("on", flag.ExitOnError)
	forDur := fs.Duration("for", 0, "auto-expire after this duration (e.g. 24h)")
	domains := fs.String("domains", defaultDomains, "comma-separated domains to cover")
	persist := fs.Bool("persist", false, "also re-apply at every boot (survives reboot)")
	_ = fs.Parse(args)

	if route.NeedsElevation() {
		return fmt.Errorf("`on` needs root — re-run as: sudo claude-unstuck on")
	}
	hosts := splitDomains(*domains)
	ctx := context.Background()
	// Resolve the Anthropic API's IPv6 addresses; every platform's fix is scoped
	// to exactly these addresses (blackhole route, or a per-address prefix
	// policy on Windows). If there are none, there's no IPv6 path to redirect.
	addrs, errs := dnsutil.ResolveAll(ctx, hosts, dnsutil.IPv6)
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", e)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("no AAAA records resolved for %s — there is no IPv6 path to Anthropic to redirect (IPv6 may already be unavailable here)", *domains)
	}
	applied, err := route.Apply(addrs)
	if err != nil {
		return err
	}
	st := &state.State{Version: 1, AppliedAt: time.Now(), Domains: hosts, Applied: applied}
	if *forDur > 0 {
		exp := time.Now().Add(*forDur)
		st.ExpiresAt = &exp
	}
	if err := state.Save(st); err != nil {
		return fmt.Errorf("fix applied but state not saved (undo manually with `sudo claude-unstuck off`): %w", err)
	}
	fmt.Println("✅ System-wide IPv4 fix ON:")
	for _, ap := range applied {
		fmt.Printf("  %-12s %s\n", ap.Method, ap.Target)
	}
	if st.ExpiresAt != nil {
		fmt.Printf("Expires: %s (enforced on next claude-unstuck invocation)\n", st.ExpiresAt.Format(time.RFC1123))
	}
	if *persist {
		if *forDur > 0 {
			fmt.Fprintln(os.Stderr, "warning: --persist with --for is contradictory; skipping persistence")
		} else {
			loc, err := boot.Install(persistentBinPath(), *domains)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: fix applied but boot-persistence failed: %v\n", err)
			} else {
				fmt.Printf("Persistence: re-applies at every boot via %s\n", loc)
				fmt.Println("Note: does NOT refresh on DNS rotation — if Anthropic changes IPs, re-run")
				fmt.Println("`sudo claude-unstuck on`. Check anytime with `claude-unstuck status`.")
			}
		}
	}
	fmt.Println("Turn off anytime: sudo claude-unstuck off")
	return nil
}

// persistentBinPath copies the running binary to a stable location if it isn't
// already there, so the boot hook references a path that survives /tmp cleanup.
func persistentBinPath() string {
	self, err := os.Executable()
	if err != nil {
		return "claude-unstuck"
	}
	self, _ = filepath.EvalSymlinks(self)
	target := boot.StableBinPath()
	if self == target {
		return target
	}
	data, err := os.ReadFile(self)
	if err != nil {
		return self
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return self
	}
	if err := os.WriteFile(target, data, 0o755); err != nil {
		return self
	}
	return target
}

func cmdOff(args []string) error {
	st, err := state.Load()
	if err != nil {
		return err
	}
	if st == nil || len(st.Applied) == 0 {
		fmt.Println("Nothing to undo — no system-wide fix is installed.")
		return nil
	}
	if route.NeedsElevation() {
		return fmt.Errorf("`off` needs root to remove routes — re-run as: sudo claude-unstuck off")
	}
	if err := route.Remove(st.Applied); err != nil {
		return err
	}
	if err := state.Clear(); err != nil {
		return err
	}
	if boot.Installed() {
		if err := boot.Remove(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: routes removed but boot-persistence cleanup failed: %v\n", err)
		} else {
			fmt.Println("Removed boot persistence.")
		}
	}
	fmt.Printf("✅ System-wide fix OFF — removed %d mitigation(s). Back to default.\n", len(st.Applied))
	return nil
}

func cmdStatus(args []string) error {
	st, err := state.Load()
	if err != nil {
		return err
	}
	if st == nil || len(st.Applied) == 0 {
		fmt.Println("System-wide fix: OFF (not installed).")
		if boot.Installed() {
			fmt.Println("⚠️  but a boot-persistence hook still exists — clean it with: sudo claude-unstuck off")
		}
		return nil
	}
	fmt.Printf("System-wide fix: ON since %s for: %s\n",
		st.AppliedAt.Format(time.RFC1123), strings.Join(st.Domains, ", "))
	for _, ap := range st.Applied {
		fmt.Printf("  %-12s %s\n", ap.Method, ap.Target)
	}
	if boot.Installed() {
		fmt.Println("Persistence: ON (re-applies at every boot)")
	}
	if st.ExpiresAt != nil {
		fmt.Printf("Expires: %s\n", st.ExpiresAt.Format(time.RFC1123))
	}
	if st.Expired(time.Now()) {
		if route.NeedsElevation() {
			fmt.Println("⚠️  fix has EXPIRED — remove it with: sudo claude-unstuck off")
			return nil
		}
		fmt.Println("Fix has expired — removing now...")
		return cmdOff(nil)
	}
	// Detect DNS drift: AAAA records may have rotated since the fix.
	addrs, _ := dnsutil.ResolveAll(context.Background(), st.Domains, dnsutil.IPv6)
	covered := map[string]bool{}
	for _, ap := range st.Applied {
		covered[ap.Target] = true
	}
	var missing []netip.Addr
	for _, a := range addrs {
		if !covered[a.String()] {
			missing = append(missing, a)
		}
	}
	if len(missing) > 0 {
		fmt.Printf("⚠️  DNS drift: %d current AAAA address(es) are NOT covered — re-run: sudo claude-unstuck on\n", len(missing))
		for _, m := range missing {
			fmt.Printf("    %s\n", m)
		}
	} else if len(addrs) > 0 {
		fmt.Println("✅ all current AAAA addresses are covered")
	}
	return nil
}

func splitDomains(s string) []string {
	var out []string
	for _, d := range strings.Split(s, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}
