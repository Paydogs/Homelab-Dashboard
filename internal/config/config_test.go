package config

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dashboard/domain"
)

const valid = `
title: Test
groups:
  - title: Media
    items:
      - title: Jellyfin
        url: http://192.168.1.20:8096
        tags: [media, video]
        health: {path: /health, interval: 30s}
      - title: Steam
        run: C:\Program Files (x86)\Steam\steam.exe
        tags: [games]
`

// write drops a config.yml into a fresh data dir and returns the dir —
// NewManager takes a data directory (config.yml + resources live there).
func write(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func testLogger(t *testing.T) *log.Logger {
	t.Helper()
	return log.New(io.Discard, "", 0)
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
		t.Fatalf("expected 1 group with 2 children, got %+v", cfg.Groups)
	}
	var svc, app domain.Child
	for _, ch := range cfg.Groups[0].Items {
		if _, isApp := ch.(*domain.App); isApp {
			app = ch
		} else {
			svc = ch
		}
	}
	if svc == nil || app == nil {
		t.Fatalf("expected a Service and an App child, got %+v", cfg.Groups[0].Items)
	}

	if cfg.DefaultStyle != "" {
		t.Errorf("DefaultStyle = %q, want empty when unset", cfg.DefaultStyle)
	}

	tags := cfg.AllTags()
	if len(tags) != 3 || tags[0] != "games" {
		t.Errorf("AllTags = %v, want [games media video]", tags)
	}
}

func TestDefaultStyleParsed(t *testing.T) {
	mgr, err := NewManager(write(t, strings.Replace(valid, "title: Test", "title: Test\ndefaultStyle: Modern", 1)), testLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := mgr.Get().DefaultStyle; got != "Modern" {
		t.Errorf("DefaultStyle = %q, want Modern", got)
	}
}

func TestChildTreeSemantics(t *testing.T) {
	nested := `
title: T
groups:
  - title: Servers
    groups:
      - title: Server 1
        items:
          - title: Gitea
            url: http://192.168.1.30:3000
            tags: [dev]
`
	mgr, err := NewManager(write(t, nested), testLogger(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := mgr.Get()
	_, child, ok := cfg.FindService("Servers/Server 1/Gitea")
	if !ok {
		t.Fatal("nested child path did not resolve")
	}
	if _, isApp := child.(*domain.App); isApp {
		t.Error("Gitea should be a Service, not an App")
	}
	if svc := child.(*domain.Service); svc.URL != "http://192.168.1.30:3000" {
		t.Errorf("URL = %q", svc.URL)
	}
}

func TestRunItemIsApp(t *testing.T) {
	mgr, err := NewManager(write(t, valid), testLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	child, ok := mgr.RunService("Media/Steam")
	if !ok {
		t.Fatal("Steam (run item) should resolve as a runnable App")
	}
	app := child.(*domain.App)
	if app.Run != `C:\Program Files (x86)\Steam\steam.exe` {
		t.Errorf("Run = %q", app.Run)
	}
	_, ok = mgr.RunService("Media/Jellyfin")
	if ok {
		t.Error("Jellyfin (url item) must not resolve as runnable")
	}
}

func TestRejectsItemWithoutURLorRun(t *testing.T) {
	bad := strings.Replace(valid, "run: C:\\Program Files (x86)\\Steam\\steam.exe", "icon: extra", 1)
	if _, err := NewManager(write(t, bad), testLogger(t)); err == nil {
		t.Fatal("expected error for item with neither url nor run")
	}
}

func TestRejectsItemWithBoth(t *testing.T) {
	bad := strings.Replace(valid, "run: C:\\Program Files (x86)\\Steam\\steam.exe",
		"run: notepad.exe\n        url: http://x", 1)
	if _, err := NewManager(write(t, bad), testLogger(t)); err == nil {
		t.Fatal("expected error for item with both url and run")
	}
}

func TestRejectsUnknownField(t *testing.T) {
	bad := strings.Replace(valid, "        tags: [games]", "        typoField: [games]", 1)
	if _, err := NewManager(write(t, bad), testLogger(t)); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestRejectsHealthWithoutURL(t *testing.T) {
	bad := strings.Replace(valid, "run: C:\\Program Files (x86)\\Steam\\steam.exe",
		"run: notepad.exe\n        health: {path: /x}", 1)
	// the url+run discriminator fires first here, which also covers the case
	if _, err := NewManager(write(t, bad), testLogger(t)); err == nil {
		t.Fatal("expected error for run item with healthcheck")
	}
}

func TestDuplicateTitlesRejected(t *testing.T) {
	dup := strings.Replace(valid, "      - title: Steam", "      - title: Jellyfin", 1)
	if _, err := NewManager(write(t, dup), testLogger(t)); err == nil {
		t.Fatal("expected error for duplicate item titles in a group")
	}
}

// manager reloads when the file's metadata changes: a browser refresh must
// pick up edits with no restart.
func TestReloadOnChange(t *testing.T) {
	dir := write(t, valid)
	mgr, err := NewManager(dir, testLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(valid, "title: Test", "title: Changed", 1)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mgr.Get().Title; got != "Changed" {
		t.Errorf("title after reload = %q, want Changed", got)
	}
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

// the self-explanatory syntax: blocks declare what they are on their start
// line — group:/service:/app: — with title:/items: still valid aliases.
func TestSelfExplanatorySyntax(t *testing.T) {
	new := `
title: T
groups:
  - group: Favourites
    subtitle: quick access
    children:
      - service: Jellyfin
        url: http://192.168.1.20:8096
        tags: [media]
        health: {path: /health, interval: 30s}
      - app: Steam
        run: C:\Games\steam.exe
        tags: [games]
`
	mgr, err := NewManager(write(t, new), testLogger(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := mgr.Get()
	if cfg.Groups[0].Title != "Favourites" {
		t.Errorf("group name = %q, want Favourites", cfg.Groups[0].Title)
	}
	app, ok := mgr.RunService("Favourites/Steam")
	if !ok {
		t.Fatal("Steam (app: child) should resolve as runnable")
	}
	if app.Core().Title != "Steam" {
		t.Errorf("app title = %q", app.Core().Title)
	}
}

func TestSelfExplanatoryConflictsRejected(t *testing.T) {
	cases := map[string]string{
		"group+title mixed": `
title: T
groups:
  - group: A
    title: A2
`,
		"children+items mixed": `
title: T
groups:
  - title: A
    children:
      - service: X
        url: http://x
    items:
      - app: Y
        run: y.exe
`,
		"service+app mixed": `
title: T
groups:
  - group: A
    children:
      - service: X
        app: X2
        url: http://x
`,
		"service with run": `
title: T
groups:
  - group: A
    children:
      - service: X
        run: x.exe
`,
		"app with url": `
title: T
groups:
  - group: A
    children:
      - app: X
        url: http://x
`,
		"app without run": `
title: T
groups:
  - group: A
    children:
      - app: X
`,
	}
	for name, bad := range cases {
		if _, err := NewManager(write(t, bad), testLogger(t)); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}
