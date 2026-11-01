package main

import (
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"dashboard"
)

// seedDefaults copies the shipped default files (sample config and starter
// styles) into the data folder — ONLY when there is no config file yet
// (first boot): an existing dashboard is never touched. The app then
// behaves identically afterwards; everything can be edited through the web
// UI's edit buttons.
func seedDefaults(dataDir, defaultsDir string, logger *log.Logger) error {
	if _, err := os.Stat(defaultsDir); err != nil {
		return nil // no defaults folder given: nothing to seed
	}
	return seedTree(dataDir, os.DirFS(defaultsDir),
		func(rel string) string { return rel }, logger)
}

// seedEmbeddedDefaults is the no-Docker variant: the starter files compiled
// into the binary, so a bare desktop run (Windows/macOS, no /defaults
// folder) still gets the sample config and starter styles on first boot.
func seedEmbeddedDefaults(dataDir string, logger *log.Logger) error {
	return dashboard.Seed(dataDir, logger)
}

// seedTree copies every file in src into dataDir: name maps a slash path in
// src to its data-folder name ("" skips the entry). Existing files are
// never overwritten — only missing pieces of a fresh or partial data folder
// are filled in. A ZERO-BYTE config.yml counts as missing (e.g. a
// disk-full write left a husk) so a broken first boot can re-seed.
func seedTree(dataDir string, src fs.FS, name func(rel string) string, logger *log.Logger) error {
	if st, err := os.Stat(filepath.Join(dataDir, "config.yml")); err == nil && st.Size() > 0 {
		return nil // existing dashboard: leave it alone
	}
	if _, err := fs.ReadDir(src, "."); err != nil {
		return nil // nothing shipped: nothing to seed
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := name(path)
		if rel == "" {
			return nil
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
		in, err := src.Open(path)
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
		logger.Printf("seeded %s", target)
		return nil
	})
}
