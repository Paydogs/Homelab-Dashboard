package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// seedDefaults copies the shipped default files (sample config and starter
// styles) into the config folder — ONLY when there is no config file yet
// (first boot): an existing dashboard is never touched. The app then
// behaves identically afterwards; everything can be edited through the web
// UI's edit buttons.
func seedDefaults(configPath, defaultsDir string, logger *log.Logger) error {
	if _, err := os.Stat(configPath); err == nil {
		return nil // existing dashboard: leave it alone
	}
	if _, err := os.Stat(defaultsDir); err != nil {
		return nil // no defaults shipped (plain binary run): nothing to seed
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(defaultsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(defaultsDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(filepath.Dir(configPath), rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if _, err := os.Stat(target); err == nil {
			return nil // never overwrite an existing file
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := os.Create(target)
		if err != nil {
			return err
		}
		defer dst.Close()
		if _, err := io.Copy(dst, src); err != nil {
			return err
		}
		logger.Printf("seeded %s", target)
		return nil
	})
}
