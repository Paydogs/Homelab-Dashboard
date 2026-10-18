package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func testLogger() *log.Logger { return log.New(io.Discard, "", 0) }

// first boot: no config exists — the defaults tree is copied under the
// config folder, never overwriting an existing dashboard file.
func TestSeedDefaultsFirstBoot(t *testing.T) {
	root := t.TempDir()
	defaults := filepath.Join(root, "defaults")
	os.MkdirAll(filepath.Join(defaults, "resources", "styles", "modern"), 0o755)
	os.WriteFile(filepath.Join(defaults, "config.yml"), []byte("title: Sample\n"), 0o644)
	os.WriteFile(filepath.Join(defaults, "resources", "styles", "modern", "style.yml"), []byte("name: Modern\n"), 0o644)

	configPath := filepath.Join(root, "config", "config.yml")
	if err := seedDefaults(configPath, defaults, testLogger()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil || string(data) != "title: Sample\n" {
		t.Fatalf("config seeded = %q, err %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "resources", "styles", "modern", "style.yml")); err != nil {
		t.Errorf("seeded style missing: %v", err)
	}
}

// an existing dashboard (config present) must never be touched
func TestSeedDefaultsSkipsExisting(t *testing.T) {
	root := t.TempDir()
	defaults := filepath.Join(root, "defaults")
	os.WriteFile(filepath.Join(defaults, "config.yml"), []byte("title: Sample\n"), 0o644)

	configPath := filepath.Join(root, "config", "config.yml")
	os.MkdirAll(filepath.Dir(configPath), 0o755)
	os.WriteFile(configPath, []byte("title: Mine\n"), 0o644)

	if err := seedDefaults(configPath, defaults, testLogger()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(configPath)
	if string(data) != "title: Mine\n" {
		t.Errorf("existing config overwritten: %q", data)
	}
}

// a plain binary run without a defaults folder seeds nothing and errors not
func TestSeedDefaultsNoFolder(t *testing.T) {
	root := t.TempDir()
	if err := seedDefaults(filepath.Join(root, "config", "config.yml"), filepath.Join(root, "missing"), testLogger()); err != nil {
		t.Errorf("no defaults dir should be a no-op, got %v", err)
	}
}
