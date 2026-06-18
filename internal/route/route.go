// Package route installs and removes system-wide IPv6 avoidance for a set of
// addresses. Mechanisms per OS:
//
//   - darwin:  blackhole host routes (route add -inet6 <ip> ::1 -blackhole),
//     so IPv6 connects to those addresses fail instantly and dual-stack
//     clients fall back to IPv4.
//   - linux:   blackhole routes (ip -6 route replace blackhole <ip>/128).
//   - windows: a scoped outbound Windows Firewall rule that blocks each
//     Anthropic IPv6 address (netsh advfirewall ... action=block remoteip=<ip>),
//     so those connections fail and the client falls back to IPv4.
//
// Every mechanism is scoped to the given addresses — nothing system-wide is
// changed. Command construction is pure (CommandsFor/RemoveCommandsFor) so every
// platform's behavior is unit-testable from any platform.
package route

import (
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Applied describes one installed mitigation, recorded in the state file so
// `off` can undo it precisely.
type Applied struct {
	Method string `json:"method"` // "blackhole" (macOS/Linux) | "firewall" (Windows)
	Target string `json:"target"` // the Anthropic IPv6 address
}

// windowsRuleName returns the Windows Firewall rule name used to block IPv6 to
// a specific Anthropic address. It must be deterministic so `off` can remove
// exactly what `on` added.
func windowsRuleName(a netip.Addr) string {
	return "claude-unstuck-block-" + a.String()
}

// CommandsFor returns the commands that install IPv6 avoidance for addrs on
// goos, plus the Applied records needed to undo them later. Every mechanism is
// scoped to the given Anthropic addresses — nothing system-wide is changed.
func CommandsFor(goos string, addrs []netip.Addr) ([][]string, []Applied, error) {
	switch goos {
	case "darwin":
		var cmds [][]string
		var apps []Applied
		for _, a := range addrs {
			cmds = append(cmds, []string{"route", "-n", "add", "-inet6", a.String(), "::1", "-blackhole"})
			apps = append(apps, Applied{Method: "blackhole", Target: a.String()})
		}
		return cmds, apps, nil
	case "linux":
		var cmds [][]string
		var apps []Applied
		for _, a := range addrs {
			cmds = append(cmds, []string{"ip", "-6", "route", "replace", "blackhole", a.String() + "/128"})
			apps = append(apps, Applied{Method: "blackhole", Target: a.String()})
		}
		return cmds, apps, nil
	case "windows":
		// Outbound firewall block, scoped to each Anthropic IPv6 address: the
		// connection to those addresses fails, so the client falls back to IPv4
		// — the same "block v6" behavior as the blackhole on macOS/Linux. Only
		// traffic to Anthropic's addresses is affected; nothing system-wide
		// changes. Delete-before-add keeps it idempotent (no duplicate rules).
		var cmds [][]string
		var apps []Applied
		for _, a := range addrs {
			name := windowsRuleName(a)
			cmds = append(cmds,
				[]string{"netsh", "advfirewall", "firewall", "delete", "rule", "name=" + name},
				[]string{"netsh", "advfirewall", "firewall", "add", "rule", "name=" + name, "dir=out", "action=block", "remoteip=" + a.String()},
			)
			apps = append(apps, Applied{Method: "firewall", Target: a.String()})
		}
		return cmds, apps, nil
	}
	return nil, nil, fmt.Errorf("unsupported OS: %s", goos)
}

// RemoveCommandsFor returns the commands that undo previously applied
// mitigations on goos.
func RemoveCommandsFor(goos string, apps []Applied) ([][]string, error) {
	var cmds [][]string
	for _, ap := range apps {
		switch {
		case goos == "darwin" && ap.Method == "blackhole":
			cmds = append(cmds, []string{"route", "-n", "delete", "-inet6", ap.Target})
		case goos == "linux" && ap.Method == "blackhole":
			cmds = append(cmds, []string{"ip", "-6", "route", "del", "blackhole", ap.Target + "/128"})
		case goos == "windows" && ap.Method == "firewall":
			// Remove only the block rule we added for this address.
			a, err := netip.ParseAddr(ap.Target)
			if err != nil {
				return nil, fmt.Errorf("bad firewall target %q: %w", ap.Target, err)
			}
			cmds = append(cmds, []string{"netsh", "advfirewall", "firewall", "delete", "rule", "name=" + windowsRuleName(a)})
		default:
			return nil, fmt.Errorf("don't know how to undo method %q on %s", ap.Method, goos)
		}
	}
	return cmds, nil
}

// Apply installs IPv6 avoidance for addrs on the current OS and returns the
// records to persist. Idempotent errors ("exists") are tolerated.
func Apply(addrs []netip.Addr) ([]Applied, error) {
	cmds, apps, err := CommandsFor(runtime.GOOS, addrs)
	if err != nil {
		return nil, err
	}
	for _, c := range cmds {
		if err := runTolerant(c, []string{"exists", "file exists", "no rules match"}); err != nil {
			return nil, err
		}
	}
	return apps, nil
}

// Remove undoes previously applied mitigations on the current OS. Errors for
// already-removed entries are tolerated.
func Remove(apps []Applied) error {
	cmds, err := RemoveCommandsFor(runtime.GOOS, apps)
	if err != nil {
		return err
	}
	var firstErr error
	for _, c := range cmds {
		err := runTolerant(c, []string{"not in table", "no such process", "cannot find", "not found", "no such file", "no rules match"})
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// NeedsElevation reports whether the current process lacks the privileges to
// modify system routing/policy. On Windows we cannot cheaply pre-check, so we
// return false and let the command surface an access-denied error.
func NeedsElevation() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	return os.Geteuid() != 0
}

func runTolerant(argv []string, tolerate []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	low := strings.ToLower(string(out))
	for _, t := range tolerate {
		if strings.Contains(low, t) {
			return nil
		}
	}
	return fmt.Errorf("%s: %w\n%s", strings.Join(argv, " "), err, strings.TrimSpace(string(out)))
}
