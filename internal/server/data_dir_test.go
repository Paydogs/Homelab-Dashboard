package server

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dashboard/internal/config"
	"dashboard/internal/health"
	"dashboard/internal/styles"
)

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// localhost caller: POST switches, GET reports the new folder
func TestDataDirSetLocal(t *testing.T) {
	ts := newTestServer(t)
	target := filepath.Join(t.TempDir(), "new-data")

	res, err := http.Post(ts.URL+"/data-dir", "text/plain", strings.NewReader(target))
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /data-dir status = %d: %s", res.StatusCode, body)
	}
	// the embedded defaults seed the empty folder
	cfg, err := os.ReadFile(filepath.Join(target, "config.yml"))
	if err != nil || len(cfg) == 0 {
		t.Fatalf("target folder was not seeded: %v", err)
	}
	res2, err := http.Get(ts.URL + "/data-dir")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(readBody(t, res2)); !strings.HasSuffix(filepath.ToSlash(got), "new-data") {
		t.Errorf("GET /data-dir = %q, want the switched folder", got)
	}
}

// a LAN viewer must not read or change the host's data folder
func TestDataDirRefusedFromLAN(t *testing.T) {
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
	req := httptest.NewRequest("POST", "/data-dir", strings.NewReader(t.TempDir()))
	req.RemoteAddr = "192.168.1.50:4444"
	rec := httptest.NewRecorder()
	srv.dataDirSet(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("LAN POST /data-dir = %d, want 403", rec.Code)
	}
}
