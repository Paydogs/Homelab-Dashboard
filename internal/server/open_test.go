package server

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"dashboard/internal/config"
	"dashboard/internal/health"
	"dashboard/internal/styles"
)

// stubOpenFolder swaps the package's file-manager launcher for a recorder
// during a test (the real one would pop an Explorer/Finder window).
func stubOpenFolder(t *testing.T) (got *string) {
	t.Helper()
	var opened string
	orig := openFolder
	openFolder = func(dir string) error { opened = dir; return nil }
	t.Cleanup(func() { openFolder = orig })
	return &opened
}

// localhost caller: the data folder is passed to the file manager
func TestOpenDataLocal(t *testing.T) {
	opened := stubOpenFolder(t)
	ts := newTestServer(t)
	res, err := http.Post(ts.URL+"/open-data", "", nil) // httptest calls come from loopback
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}
	if *opened == "" || !filepath.IsAbs(*opened) {
		t.Errorf("openFolder called with %q, want the absolute data dir", *opened)
	}
}

// a LAN viewer must never trigger a host action: same boundary as run:
func TestOpenDataRefusedFromLAN(t *testing.T) {
	opened := stubOpenFolder(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yml"), []byte("title: T\n"), 0o644)
	logger := log.New(io.Discard, "", 0)
	mgr, err := config.NewManager(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(mgr, health.New(), styles.NewManager(filepath.Join(dir, "resources", "styles"), logger), logger)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/open-data", nil)
	req.RemoteAddr = "192.168.1.50:4444"
	rec := httptest.NewRecorder()
	srv.openData(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("LAN request status = %d, want 403", rec.Code)
	}
	if *opened != "" {
		t.Errorf("LAN request opened %q — the host action must not fire", *opened)
	}
}
