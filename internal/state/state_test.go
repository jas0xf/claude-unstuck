package state

import (
	"testing"
	"time"

	"github.com/jas0xf/claude-unstuck/internal/route"
)

func TestRoundTrip(t *testing.T) {
	t.Setenv(EnvDir, t.TempDir())

	if s, err := Load(); err != nil || s != nil {
		t.Fatalf("Load on empty dir = (%v, %v), want (nil, nil)", s, err)
	}

	in := &State{
		Version:   1,
		AppliedAt: time.Now().Round(time.Second),
		Domains:   []string{"api.anthropic.com"},
		Applied:   []route.Applied{{Method: "blackhole", Target: "2607:6bc0::10"}},
	}
	if err := Save(in); err != nil {
		t.Fatal(err)
	}
	out, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || len(out.Applied) != 1 || out.Applied[0].Target != "2607:6bc0::10" {
		t.Fatalf("Load = %+v", out)
	}
	if err := Clear(); err != nil {
		t.Fatal(err)
	}
	if s, err := Load(); err != nil || s != nil {
		t.Fatalf("Load after Clear = (%v, %v), want (nil, nil)", s, err)
	}
	if err := Clear(); err != nil {
		t.Errorf("double Clear should be a no-op, got %v", err)
	}
}
