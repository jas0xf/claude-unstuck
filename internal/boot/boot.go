// Package boot persists the system-wide IPv4 fix across reboots by installing a
// boot-time hook that re-runs `claude-unstuck on`. This is the
// "lighter" persistence: it survives reboot but does not periodically refresh,
// so if Anthropic's AAAA records rotate the fix may need re-applying (status
// detects that drift).
//
// Generators are pure (UnitContent / installArgs) so every platform's output
// is testable from any platform.
package boot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	systemdUnitPath  = "/etc/systemd/system/claude-unstuck.service"
	launchdPlistPath = "/Library/LaunchDaemons/com.claude-unstuck.boot.plist"
	windowsTaskName  = "claude-unstuck-boot"
)

// SystemdUnit returns the systemd unit that re-applies the fix at boot.
func SystemdUnit(binPath, domains string) string {
	return fmt.Sprintf(`[Unit]
Description=claude-unstuck — keep Claude Code on IPv4 (avoid degraded IPv6 path)
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=%s on --domains %s
ExecStop=%s off

[Install]
WantedBy=multi-user.target
`, binPath, domains, binPath)
}

// LaunchdPlist returns the launchd plist that re-applies the fix at load/boot.
func LaunchdPlist(binPath, domains string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.claude-unstuck.boot</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string><string>on</string><string>--domains</string><string>%s</string>
  </array>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`, binPath, domains)
}

// WindowsTaskArgs returns the schtasks argv that registers a boot task.
func WindowsTaskArgs(binPath, domains string) []string {
	tr := fmt.Sprintf(`%s on --domains %s`, binPath, domains)
	return []string{"schtasks", "/create", "/tn", windowsTaskName, "/tr", tr,
		"/sc", "onstart", "/ru", "SYSTEM", "/rl", "HIGHEST", "/f"}
}

// Install registers the boot hook for the current OS.
func Install(binPath, domains string) (location string, err error) {
	// Defense in depth: `domains` is embedded verbatim into root-owned boot
	// artifacts, so reject anything that isn't a plain comma-separated hostname
	// list (no whitespace, quotes, or shell/directive metacharacters).
	if strings.ContainsAny(domains, " \t\r\n\"'`;\\") {
		return "", fmt.Errorf("refusing unsafe domains value %q", domains)
	}
	switch runtime.GOOS {
	case "linux":
		if err := os.WriteFile(systemdUnitPath, []byte(SystemdUnit(binPath, domains)), 0o644); err != nil {
			return "", fmt.Errorf("write unit: %w", err)
		}
		if err := run("systemctl", "daemon-reload"); err != nil {
			return "", err
		}
		if err := run("systemctl", "enable", "claude-unstuck.service"); err != nil {
			return "", err
		}
		return systemdUnitPath, nil
	case "darwin":
		if err := os.WriteFile(launchdPlistPath, []byte(LaunchdPlist(binPath, domains)), 0o644); err != nil {
			return "", fmt.Errorf("write plist: %w", err)
		}
		// Best-effort load; ignore "already loaded".
		_ = run("launchctl", "load", "-w", launchdPlistPath)
		return launchdPlistPath, nil
	case "windows":
		if err := run(WindowsTaskArgs(binPath, domains)...); err != nil {
			return "", err
		}
		return "scheduled task " + windowsTaskName, nil
	}
	return "", fmt.Errorf("boot persistence unsupported on %s", runtime.GOOS)
}

// Remove unregisters the boot hook for the current OS. Missing hooks are not
// an error.
func Remove() error {
	switch runtime.GOOS {
	case "linux":
		_ = run("systemctl", "disable", "claude-unstuck.service")
		if err := os.Remove(systemdUnitPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return run("systemctl", "daemon-reload")
	case "darwin":
		_ = run("launchctl", "unload", "-w", launchdPlistPath)
		if err := os.Remove(launchdPlistPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	case "windows":
		_ = run("schtasks", "/delete", "/tn", windowsTaskName, "/f")
		return nil
	}
	return fmt.Errorf("boot persistence unsupported on %s", runtime.GOOS)
}

// Installed reports whether a boot hook is currently present.
func Installed() bool {
	switch runtime.GOOS {
	case "linux":
		return fileExists(systemdUnitPath)
	case "darwin":
		return fileExists(launchdPlistPath)
	case "windows":
		return run("schtasks", "/query", "/tn", windowsTaskName) == nil
	}
	return false
}

// StableBinPath is where the binary should live for a persistent install.
func StableBinPath() string {
	if runtime.GOOS == "windows" {
		pf := os.Getenv("ProgramFiles")
		if pf == "" {
			pf = `C:\Program Files`
		}
		return filepath.Join(pf, "claude-unstuck", "claude-unstuck.exe")
	}
	return "/usr/local/bin/claude-unstuck"
}

func run(argv ...string) error {
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w\n%s", strings.Join(argv, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
