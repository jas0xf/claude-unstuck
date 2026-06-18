// Package state persists what `on` installed so `off` can undo it
// exactly, even across reboots and re-resolutions.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/jas0xf/claude-unstuck/internal/route"
)

// EnvDir overrides the state directory (useful for tests and packaging).
const EnvDir = "CLAUDE_UNSTUCK_STATE_DIR"

// State records the currently installed system-wide mitigation.
type State struct {
	Version   int             `json:"version"`
	AppliedAt time.Time       `json:"applied_at"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
	Domains   []string        `json:"domains"`
	Applied   []route.Applied `json:"applied"`
}

// Expired reports whether the state has an expiry in the past.
func (s *State) Expired(now time.Time) bool {
	return s.ExpiresAt != nil && now.After(*s.ExpiresAt)
}

// Dir returns the directory holding state.json. When running under sudo it
// resolves the invoking user's config dir so that `sudo claude-unstuck fix
// --system` and a later unprivileged `status` agree on the same file.
func Dir() (string, error) {
	if d := os.Getenv(EnvDir); d != "" {
		return d, nil
	}
	if runtime.GOOS != "windows" && os.Geteuid() == 0 {
		if su := os.Getenv("SUDO_USER"); su != "" && su != "root" {
			u, err := user.Lookup(su)
			if err == nil {
				if runtime.GOOS == "darwin" {
					return filepath.Join(u.HomeDir, "Library", "Application Support", "claude-unstuck"), nil
				}
				return filepath.Join(u.HomeDir, ".config", "claude-unstuck"), nil
			}
		}
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(base, "claude-unstuck"), nil
}

func path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "state.json"), nil
}

// Load returns the saved state, or (nil, nil) if none exists.
func Load() (*State, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", p, err)
	}
	return &s, nil
}

// Save writes the state file, creating the directory if needed. When running
// under sudo, ownership is handed back to the invoking user.
func Save(s *State) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	chownToSudoUser(filepath.Dir(p))
	chownToSudoUser(p)
	return nil
}

// Clear removes the state file. Missing files are not an error.
func Clear() error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove state: %w", err)
	}
	return nil
}

func chownToSudoUser(p string) {
	if runtime.GOOS == "windows" || os.Geteuid() != 0 {
		return
	}
	su := os.Getenv("SUDO_USER")
	if su == "" || su == "root" {
		return
	}
	u, err := user.Lookup(su)
	if err != nil {
		return
	}
	uid, err1 := strconv.Atoi(u.Uid)
	gid, err2 := strconv.Atoi(u.Gid)
	if err1 != nil || err2 != nil {
		return
	}
	_ = os.Chown(p, uid, gid)
}
