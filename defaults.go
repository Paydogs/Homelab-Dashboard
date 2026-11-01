// Package dashboard exposes the repo's first-boot default files as an
// embedded filesystem so a plain binary run (Windows/macOS/desktop, no
// Docker) can seed its data folder on first boot — same sample config and
// starter styles the Docker image carries under /defaults. One binary, no
// extra files to install.
package dashboard

import (
	"embed"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Defaults holds config.example.yml (seeded as config.yml) plus the
// defaults/ tree (starter styles).
//
//go:embed defaults config.example.yml
var Defaults embed.FS

// Seed copies the embedded first-boot files (sample config as config.yml,
// starter styles) into dataDir — ONLY while no usable config file exists
// there (first boot / freshly chosen folder): existing files are never
// overwritten, but a ZERO-BYTE config.yml counts as missing — e.g. after a
// disk-full write failure — so a broken first boot can re-seed instead of
// being stuck on the boot page forever. Used both at startup without a
// -defaults folder and when the desktop app switches to a new data folder
// at runtime.
func Seed(dataDir string, logger *log.Logger) error {
	if hasConfig(dataDir) {
		return nil // existing dashboard: leave it alone
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(Defaults, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		var rel string
		switch {
		case path == "config.example.yml":
			rel = "config.yml"
		case strings.HasPrefix(path, "defaults/"):
			rel = strings.TrimPrefix(path, "defaults/")
		default:
			return nil // the tree's root and the defaults dir itself
		}
		target := filepath.Join(dataDir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if st, err := os.Stat(target); err == nil {
			if rel != "config.yml" || st.Size() > 0 {
				return nil // never overwrite an existing file
			}
			// a zero-byte config.yml is a husk from a failed write: refill it
		}
		in, err := Defaults.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		if logger != nil {
			logger.Printf("seeded %s", target)
		}
		return nil
	})
}

// hasConfig reports whether dataDir holds a usable config file (a zero-byte
// remnant of a failed write does not count).
func hasConfig(dataDir string) bool {
	st, err := os.Stat(filepath.Join(dataDir, "config.yml"))
	return err == nil && st.Size() > 0
}
