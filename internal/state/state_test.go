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

	exp := time.Now().Add(24 * time.Hour).Round(time.Second)
	in := &State{
		Version:   1,
		AppliedAt: time.Now().Round(time.Second),
		ExpiresAt: &exp,
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
	if out.ExpiresAt == nil || !out.ExpiresAt.Equal(exp) {
		t.Fatalf("ExpiresAt = %v, want %v", out.ExpiresAt, exp)
	}
	if out.Expired(time.Now()) {
		t.Error("should not be expired yet")
	}
	if !out.Expired(exp.Add(time.Minute)) {
		t.Error("should be expired after expiry time")
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
