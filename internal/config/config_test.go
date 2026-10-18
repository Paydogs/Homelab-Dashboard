package config

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const valid = `
title: Test
groups:
  - name: Media
    items:
      - name: Jellyfin
        url: http://192.168.1.20:8096
        tags: [media, video]
        health: {path: /health, interval: 30s}
      - name: Steam
        run: C:\Program Files (x86)\Steam\steam.exe
        tags: [games]
`

func write(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseValid(t *testing.T) {
	mgr, err := NewManager(write(t, valid), testLogger(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := mgr.Get()
	if cfg.Title != "Test" {
		t.Errorf("title = %q, want Test", cfg.Title)
	}
	if len(cfg.Groups) != 1 || len(cfg.Groups[0].Items) != 2 {
		t.Fatalf("expected 1 group with 2 items, got %+v", cfg.Groups)
	}
	tags := cfg.AllTags()
	if len(tags) != 3 || tags[0] != "games" {
		t.Errorf("AllTags = %v, want [games media video]", tags)
	}
}

func TestRejectsItemWithoutURLorRun(t *testing.T) {
	bad := strings.Replace(valid, "run: C:\\Program Files (x86)\\Steam\\steam.exe", "icon: extra", 1)
	if _, err := NewManager(write(t, bad), testLogger(t)); err == nil {
		t.Fatal("expected error for item with neither url nor run")
	}
}

func TestRejectsUnknownField(t *testing.T) {
	bad := strings.Replace(valid, "        tags: [games]", "        typoField: [games]", 1)
	if _, err := NewManager(write(t, bad), testLogger(t)); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestRejectsHealthWithoutURL(t *testing.T) {
	bad := strings.Replace(valid, "run: C:\\Program Files (x86)\\Steam\\steam.exe", "run: notepad.exe\n        health: {path: /x}", 1)
	if _, err := NewManager(write(t, bad), testLogger(t)); err == nil {
		t.Fatal("expected error for run item with healthcheck")
	}
}

func TestDuplicateNamesRejected(t *testing.T) {
	dup := strings.Replace(valid, "        tags: [games]", "        tags: [games2]", 1)
	dup = strings.Replace(dup, "      - name: Steam", "      - name: Jellyfin", 1)
	if _, err := NewManager(write(t, dup), testLogger(t)); err == nil {
		t.Fatal("expected error for duplicate item names in a group")
	}
}

// manager reloads when the file's metadata changes: a browser refresh must
// pick up edits with no restart.
func TestReloadOnChange(t *testing.T) {
	path := write(t, valid)
	mgr, err := NewManager(path, testLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(valid, "title: Test", "title: Changed", 1)
	future := time.Now().Add(5 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mgr.Get().Title; got != "Changed" {
		t.Errorf("title after reload = %q, want Changed", got)
	}
}

func testLogger(t *testing.T) *log.Logger {
	t.Helper()
	return log.New(io.Discard, "", 0)
}

func TestIconFilename(t *testing.T) {
	name := iconFilename("https://example.org/icons/logo.png?v=2")
	if !strings.HasSuffix(name, ".png") || len(name) != 16 {
		t.Errorf("iconFilename() = %q, want 12-hex + .png", name)
	}
	if name != iconFilename("https://example.org/icons/logo.png?v=2") {
		t.Error("iconFilename must be deterministic for the same URL")
	}
}
