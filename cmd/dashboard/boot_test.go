package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dashboard"
)

// before any live handler exists, every request gets the boot page naming
// the config problem; after goLive the dashboard takes transparently over
func TestBootPageThenLive(t *testing.T) {
	b := &boot{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	b.setStatus(`yaml: line 2: field icon not found`)
	b.root(rec, req)
	body, _ := io.ReadAll(rec.Result().Body)
	if !strings.Contains(string(body), "field icon not found") {
		t.Error("boot page does not show the config error")
	}
	if !strings.Contains(string(body), `http-equiv="refresh"`) {
		t.Error("boot page must auto-refresh (the retry loop flips it live)")
	}

	live := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("dashboard"))
	})
	b.goLive(live)
	rec2 := httptest.NewRecorder()
	b.root(rec2, req)
	if got, _ := io.ReadAll(rec2.Result().Body); string(got) != "dashboard" {
		t.Errorf("after goLive the dashboard handler should serve, got %q", got)
	}
}

// buildServer: an empty folder is seeded and builds fine; a folder with a
// broken config reports the parser error — and succeeds once fixed (what
// the retry loop does)
func TestBuildServer(t *testing.T) {
	dir := t.TempDir()
	seed := func() error { return dashboard.Seed(dir, nil) }
	if _, err := buildServer(dir, seed, testLogger()); err != nil {
		t.Fatalf("fresh folder should seed + build: %v", err)
	}

	bad := t.TempDir()
	os.WriteFile(filepath.Join(bad, "config.yml"), []byte("groups: ["), 0o644)
	if _, err := buildServer(bad, func() error { return nil }, testLogger()); err == nil {
		t.Fatal("broken config must fail the build")
	}
	os.WriteFile(filepath.Join(bad, "config.yml"), []byte("title: Fixed\n"), 0o644)
	if _, err := buildServer(bad, func() error { return nil }, testLogger()); err != nil {
		t.Errorf("fixed config should build: %v", err)
	}
}
