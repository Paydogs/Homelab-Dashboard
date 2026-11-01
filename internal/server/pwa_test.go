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

// newTestServer builds a real Server over a minimal config in a temp dir
// and returns a running httptest server (closed via t.Cleanup).
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	cfgData := `title: Test Dashboard
groups:
  - group: Favourites
    children:
      - service: Gitea
        url: http://192.168.1.30:3000
`
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(cfgData), 0o644); err != nil {
		t.Fatal(err)
	}
	logger := log.New(io.Discard, "", 0)
	mgr, err := config.NewManager(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	styleMgr := styles.NewManager(filepath.Join(dir, "resources", "styles"), logger)
	srv, err := New(mgr, health.New(), styleMgr, logger)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts
}

func TestManifestRoute(t *testing.T) {
	ts := newTestServer(t)
	res, err := http.Get(ts.URL + "/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("manifest status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/manifest+json") {
		t.Errorf("manifest content-type = %q", ct)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("manifest cache-control = %q, want no-cache", cc)
	}
	buf := new(strings.Builder)
	if _, err := io.Copy(buf, res.Body); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"name":"Test Dashboard"`, `"display":"standalone"`, `"purpose":"maskable"`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("manifest missing %s:\n%s", want, buf.String())
		}
	}
}

func TestServiceWorkerRoute(t *testing.T) {
	ts := newTestServer(t)
	res, err := http.Get(ts.URL + "/sw.js")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("sw.js status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("sw.js content-type = %q", ct)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("sw.js cache-control = %q, want no-cache", cc)
	}
}

func TestStaticCacheHeaders(t *testing.T) {
	ts := newTestServer(t)
	res, err := http.Get(ts.URL + "/static/icons/icon-512.png")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("icon status = %d, content-type = %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	n, _ := io.Copy(io.Discard, res.Body)
	if n == 0 {
		t.Error("icon-512.png served empty")
	}

	res2, err := http.Get(ts.URL + "/static/css/main.css")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if cc := res2.Header.Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Errorf("static cache-control = %q", cc)
	}
}
