// Package appstate remembers the desktop app's one cross-restart choice —
// the data folder picked in the web UI — in the OS's per-user config
// location (%AppData% on Windows, ~/Library/Application Support on macOS,
// ~/.config on Linux). It is the only state living OUTSIDE the data folder:
// everything else the dashboard owns stays in the data folder itself.
package appstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Path is the state file's location for the current user.
func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding the user config dir: %w", err)
	}
	return filepath.Join(base, "dashboard", "data-dir"), nil
}

// Load returns the saved data folder, if one was ever chosen and still exists.
func Load() (string, bool) {
	p, err := Path()
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", false
	}
	dir := strings.TrimSpace(string(data))
	if dir == "" {
		return "", false
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", false // gone since — ignore rather than fail
	}
	return dir, true
}

// Save remembers the data folder for the next launch.
func Save(dir string) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(dir+"\n"), 0o600)
}
