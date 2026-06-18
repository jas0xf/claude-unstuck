// Package ui renders animated, TTY-aware progress for the interactive flow.
// On a non-terminal (pipe, CI) it degrades to plain one-line-per-update output
// so logs stay readable.
package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Color codes, blanked when stdout is not a TTY.
type palette struct{ dim, green, red, yellow, bold, reset string }

func paletteFor(tty bool) palette {
	if !tty {
		return palette{}
	}
	return palette{
		dim:    "\033[2m",
		green:  "\033[32m",
		red:    "\033[31m",
		yellow: "\033[33m",
		bold:   "\033[1m",
		reset:  "\033[0m",
	}
}

// Spinner animates a single status line until stopped.
type Spinner struct {
	w       io.Writer
	tty     bool
	pal     palette
	mu      sync.Mutex
	label   string
	start   time.Time
	stop    chan struct{}
	done    chan struct{}
	running bool
}

// NewSpinner returns a spinner writing to w. isTTY controls animation.
func NewSpinner(w io.Writer, isTTY bool) *Spinner {
	return &Spinner{w: w, tty: isTTY, pal: paletteFor(isTTY)}
}

// Start begins animating with the given label.
func (s *Spinner) Start(label string) {
	s.mu.Lock()
	s.label = label
	s.start = nowMonotonic()
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	s.running = true
	s.mu.Unlock()

	if !s.tty {
		fmt.Fprintf(s.w, "  • %s\n", label)
		close(s.done)
		return
	}
	go s.loop()
}

// Update changes the label of a running spinner.
func (s *Spinner) Update(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

func (s *Spinner) loop() {
	defer close(s.done)
	t := time.NewTicker(90 * time.Millisecond)
	defer t.Stop()
	i := 0
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.mu.Lock()
			label, start := s.label, s.start
			s.mu.Unlock()
			el := time.Since(start).Seconds()
			fmt.Fprintf(s.w, "\r\033[K  %s%s%s %s %s(%.0fs)%s",
				s.pal.yellow, frames[i%len(frames)], s.pal.reset,
				label, s.pal.dim, el, s.pal.reset)
			i++
		}
	}
}

// stopAnim halts animation and clears the line (TTY only). It is idempotent:
// calling a terminal method (Succeed/Fail/Info) more than once, or before
// Start, is a safe no-op rather than a "close of closed channel" panic.
func (s *Spinner) stopAnim() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	if !s.tty {
		<-s.done
		return
	}
	close(s.stop)
	<-s.done
	fmt.Fprint(s.w, "\r\033[K")
}

// Succeed stops the spinner and prints a green check line.
func (s *Spinner) Succeed(msg string) {
	s.stopAnim()
	fmt.Fprintf(s.w, "  %s✔%s %s\n", s.pal.green, s.pal.reset, msg)
}

// Fail stops the spinner and prints a red cross line.
func (s *Spinner) Fail(msg string) {
	s.stopAnim()
	fmt.Fprintf(s.w, "  %s✘%s %s\n", s.pal.red, s.pal.reset, msg)
}

// Info stops the spinner and prints a neutral line.
func (s *Spinner) Info(msg string) {
	s.stopAnim()
	fmt.Fprintf(s.w, "  %s•%s %s\n", s.pal.dim, s.pal.reset, msg)
}

// Palette helpers for callers that print their own decorated lines.
func (s *Spinner) Bold(text string) string   { return s.pal.bold + text + s.pal.reset }
func (s *Spinner) Green(text string) string  { return s.pal.green + text + s.pal.reset }
func (s *Spinner) Red(text string) string    { return s.pal.red + text + s.pal.reset }
func (s *Spinner) Yellow(text string) string { return s.pal.yellow + text + s.pal.reset }
func (s *Spinner) Dim(text string) string    { return s.pal.dim + text + s.pal.reset }

// IsTTY reports whether w is a character device.
func IsTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// nowMonotonic returns a monotonic timestamp. Wrapped so tests can avoid the
// banned argless time.Now elsewhere; here it is the legitimate clock source.
func nowMonotonic() time.Time { return time.Now() }
