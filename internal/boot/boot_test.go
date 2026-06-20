package boot

import "testing"

func TestSystemdUnit(t *testing.T) {
	u := SystemdUnit("/usr/local/bin/claude-unstuck", "api.anthropic.com,console.anthropic.com")
	for _, want := range []string{
		"ExecStart=/usr/local/bin/claude-unstuck on --domains api.anthropic.com,console.anthropic.com",
		"ExecStop=/usr/local/bin/claude-unstuck off",
		"WantedBy=multi-user.target",
		"After=network-online.target",
	} {
		if !contains(u, want) {
			t.Errorf("systemd unit missing %q:\n%s", want, u)
		}
	}
}

func TestLaunchdPlist(t *testing.T) {
	p := LaunchdPlist("/usr/local/bin/claude-unstuck", "api.anthropic.com")
	for _, want := range []string{
		"com.claude-unstuck.boot",
		"<string>/usr/local/bin/claude-unstuck</string>",
		"<key>RunAtLoad</key><true/>",
	} {
		if !contains(p, want) {
			t.Errorf("plist missing %q:\n%s", want, p)
		}
	}
}

func TestWindowsTaskArgs(t *testing.T) {
	args := WindowsTaskArgs(`C:\Program Files\claude-unstuck\claude-unstuck.exe`, "api.anthropic.com")
	if args[0] != "schtasks" || args[1] != "/create" {
		t.Fatalf("unexpected argv: %v", args)
	}
	joined := ""
	for _, a := range args {
		joined += a + " "
	}
	for _, want := range []string{"/sc", "onstart", "claude-unstuck-boot", ".exe on --domains"} {
		if !contains(joined, want) {
			t.Errorf("task args missing %q: %v", want, args)
		}
	}
}

func TestInstallRejectsUnsafeDomains(t *testing.T) {
	// These would otherwise be interpolated verbatim into a root-owned boot
	// artifact; Install must refuse them before touching the system.
	for _, bad := range []string{
		"api.anthropic.com\nExecStart=/bin/touch /tmp/pwned",
		"api.anthropic.com console.anthropic.com",
		`api.anthropic.com"; rm -rf /`,
		"api.anthropic.com\tx",
		"api.anthropic.com`id`",
	} {
		if _, err := Install("/usr/local/bin/claude-unstuck", bad); err == nil {
			t.Errorf("Install accepted unsafe domains %q, want error", bad)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
