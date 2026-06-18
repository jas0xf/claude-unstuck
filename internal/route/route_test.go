package route

import (
	"net/netip"
	"reflect"
	"testing"
)

func addrs(t *testing.T, ss ...string) []netip.Addr {
	t.Helper()
	var out []netip.Addr
	for _, s := range ss {
		a, err := netip.ParseAddr(s)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func TestCommandsForDarwin(t *testing.T) {
	cmds, apps, err := CommandsFor("darwin", addrs(t, "2607:6bc0::10"))
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"route", "-n", "add", "-inet6", "2607:6bc0::10", "::1", "-blackhole"}}
	if !reflect.DeepEqual(cmds, want) {
		t.Errorf("cmds = %v, want %v", cmds, want)
	}
	rm, err := RemoveCommandsFor("darwin", apps)
	if err != nil {
		t.Fatal(err)
	}
	wantRm := [][]string{{"route", "-n", "delete", "-inet6", "2607:6bc0::10"}}
	if !reflect.DeepEqual(rm, wantRm) {
		t.Errorf("remove = %v, want %v", rm, wantRm)
	}
}

func TestCommandsForLinux(t *testing.T) {
	cmds, apps, err := CommandsFor("linux", addrs(t, "2607:6bc0::10", "2607:6bc0::11"))
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"ip", "-6", "route", "replace", "blackhole", "2607:6bc0::10/128"},
		{"ip", "-6", "route", "replace", "blackhole", "2607:6bc0::11/128"},
	}
	if !reflect.DeepEqual(cmds, want) {
		t.Errorf("cmds = %v, want %v", cmds, want)
	}
	rm, err := RemoveCommandsFor("linux", apps)
	if err != nil {
		t.Fatal(err)
	}
	if len(rm) != 2 || rm[0][3] != "del" {
		t.Errorf("unexpected remove commands: %v", rm)
	}
}

func TestCommandsForWindows(t *testing.T) {
	cmds, apps, err := CommandsFor("windows", addrs(t, "2607:6bc0::10"))
	if err != nil {
		t.Fatal(err)
	}
	// Scoped outbound firewall block (delete-before-add for idempotency).
	want := [][]string{
		{"netsh", "advfirewall", "firewall", "delete", "rule", "name=claude-unstuck-block-2607:6bc0::10"},
		{"netsh", "advfirewall", "firewall", "add", "rule", "name=claude-unstuck-block-2607:6bc0::10", "dir=out", "action=block", "remoteip=2607:6bc0::10"},
	}
	if !reflect.DeepEqual(cmds, want) {
		t.Errorf("cmds = %v, want %v", cmds, want)
	}
	// Must never touch any system-wide policy/route.
	for _, c := range cmds {
		for _, tok := range c {
			if tok == "::ffff:0:0/96" || tok == "set" || tok == "prefixpolicy" {
				t.Fatalf("windows fix must be a scoped firewall block, not a global change: %v", c)
			}
		}
	}
	rm, err := RemoveCommandsFor("windows", apps)
	if err != nil {
		t.Fatal(err)
	}
	wantRm := [][]string{
		{"netsh", "advfirewall", "firewall", "delete", "rule", "name=claude-unstuck-block-2607:6bc0::10"},
	}
	if !reflect.DeepEqual(rm, wantRm) {
		t.Errorf("remove = %v, want %v", rm, wantRm)
	}
}

func TestCommandsForUnsupported(t *testing.T) {
	if _, _, err := CommandsFor("plan9", addrs(t, "::1")); err == nil {
		t.Error("expected error for unsupported OS")
	}
}

func TestRemoveCommandsForUnknownMethod(t *testing.T) {
	if _, err := RemoveCommandsFor("linux", []Applied{{Method: "bogus", Target: "::1"}}); err == nil {
		t.Error("expected error for unknown method")
	}
}
