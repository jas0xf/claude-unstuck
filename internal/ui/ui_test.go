package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestSpinnerTTYAnimates verifies the animated path emits spinner frames and a
// green success line when the output is a TTY.
func TestSpinnerTTYAnimates(t *testing.T) {
	var buf bytes.Buffer
	sp := NewSpinner(&buf, true)
	sp.Start("working")
	time.Sleep(250 * time.Millisecond) // let a few frames render
	sp.Succeed("done")

	out := buf.String()
	frameSeen := false
	for _, f := range frames {
		if strings.Contains(out, f) {
			frameSeen = true
			break
		}
	}
	if !frameSeen {
		t.Errorf("expected an animated spinner frame in output, got:\n%q", out)
	}
	if !strings.Contains(out, "\033[32m✔") {
		t.Errorf("expected green check on success, got:\n%q", out)
	}
	if !strings.Contains(out, "\r") {
		t.Errorf("expected carriage-return redraws on a TTY, got:\n%q", out)
	}
}

// TestSpinnerDoubleStopNoPanic verifies calling two terminal methods after one
// Start (or a terminal before Start) is a safe no-op, not a "close of closed
// channel" panic. Covers both TTY and non-TTY.
func TestSpinnerDoubleStopNoPanic(t *testing.T) {
	for _, tty := range []bool{true, false} {
		var buf bytes.Buffer
		sp := NewSpinner(&buf, tty)
		sp.Start("work")
		sp.Succeed("done")
		sp.Info("extra") // second stop — must not panic
		sp.Fail("again") // third stop — must not panic
		fresh := NewSpinner(&buf, tty)
		fresh.Info("no start") // terminal before Start — must not panic
	}
}

// TestSpinnerNonTTYPlain verifies non-TTY output is clean one-line-per-update
// with no ANSI escapes.
func TestSpinnerNonTTYPlain(t *testing.T) {
	var buf bytes.Buffer
	sp := NewSpinner(&buf, false)
	sp.Start("working")
	sp.Fail("nope")

	out := buf.String()
	if strings.Contains(out, "\033[") {
		t.Errorf("non-TTY output must not contain ANSI escapes, got:\n%q", out)
	}
	if strings.Contains(out, "\r") {
		t.Errorf("non-TTY output must not contain carriage returns, got:\n%q", out)
	}
	for _, want := range []string{"working", "nope"} {
		if !strings.Contains(out, want) {
			t.Errorf("non-TTY output missing %q, got:\n%q", want, out)
		}
	}
}
