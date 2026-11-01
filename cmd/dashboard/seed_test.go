package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
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

	configPath := filepath.Join(root, "data", "config.yml")
	if err := seedDefaults(filepath.Dir(configPath), defaults, testLogger()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil || string(data) != "title: Sample\n" {
		t.Fatalf("config seeded = %q, err %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "resources", "styles", "modern", "style.yml")); err != nil {
		t.Errorf("seeded style missing: %v", err)
	}
}

// an existing dashboard (config present) must never be touched
func TestSeedDefaultsSkipsExisting(t *testing.T) {
	root := t.TempDir()
	defaults := filepath.Join(root, "defaults")
	os.WriteFile(filepath.Join(defaults, "config.yml"), []byte("title: Sample\n"), 0o644)

	configPath := filepath.Join(root, "data", "config.yml")
	os.MkdirAll(filepath.Dir(configPath), 0o755)
	os.WriteFile(configPath, []byte("title: Mine\n"), 0o644)

	if err := seedDefaults(filepath.Dir(configPath), defaults, testLogger()); err != nil {
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
	if err := seedDefaults(filepath.Join(root, "data", "config.yml"), filepath.Join(root, "missing"), testLogger()); err != nil {
		t.Errorf("no defaults dir should be a no-op, got %v", err)
	}
}

// the no-Docker desktop run: the defaults compiled into the binary seed the
// sample config (as config.yml, from config.example.yml) and starter styles
func TestSeedEmbeddedDefaults(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data")
	if err := seedEmbeddedDefaults(data, testLogger()); err != nil {
		t.Fatal(err)
	}
	cfg, err := os.ReadFile(filepath.Join(data, "config.yml"))
	if err != nil || len(cfg) == 0 {
		t.Fatalf("embedded config not seeded: %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, "resources", "styles", "modern", "style.yml")); err != nil {
		t.Errorf("embedded modern style missing: %v", err)
	}
	// second run with config present: existing files stay untouched
	os.WriteFile(filepath.Join(data, "config.yml"), []byte("title: Mine\n"), 0o644)
	if err := seedEmbeddedDefaults(data, testLogger()); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(data, "config.yml")); string(got) != "title: Mine\n" {
		t.Errorf("existing config overwritten: %q", got)
	}
}

// a zero-byte config (husk of a failed write, e.g. disk full) must NOT
// block re-seeding — otherwise the app sits on the boot page forever
func TestSeedReseedsEmptyConfig(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data")
	os.MkdirAll(data, 0o755)
	os.WriteFile(filepath.Join(data, "config.yml"), nil, 0o644)
	if err := seedEmbeddedDefaults(data, testLogger()); err != nil {
		t.Fatal(err)
	}
	cfg, _ := os.ReadFile(filepath.Join(data, "config.yml"))
	if len(cfg) == 0 {
		t.Error("empty config was not re-seeded")
	}
	if !strings.Contains(string(cfg), "title:") {
		t.Errorf("re-seeded config looks wrong: %q", cfg)
	}
}
