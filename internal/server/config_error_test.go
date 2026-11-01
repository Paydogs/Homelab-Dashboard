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

// a config that breaks WHILE RUNNING: the page keeps serving the last good
// version and shows a banner naming the file + parser error; once fixed,
// the banner disappears
func TestConfigErrorBanner(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	os.WriteFile(cfgPath, []byte("title: T\n"), 0o644)
	logger := log.New(io.Discard, "", 0)
	mgr, err := config.NewManager(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(mgr, health.New(), styles.NewManager(filepath.Join(dir, "resources", "styles"), logger), logger)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	get := func() string {
		res, err := http.Get(ts.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return string(body)
	}

	if strings.Contains(get(), `class="config-error"`) {
		t.Fatal("banner should not show on a healthy config")
	}

	// break it (different size, so the reload-on-change check fires)
	os.WriteFile(cfgPath, []byte("title: T\ngroups: ["), 0o644)
	page := get()
	if !strings.Contains(page, `class="config-error"`) {
		t.Fatal("broken config should render the error banner")
	}
	if !strings.Contains(page, "serving the last good version") {
		t.Errorf("banner should explain the stale serving, page: %s", page)
	}

	// and heal again
	os.WriteFile(cfgPath, []byte("title: T\neditable: true\n"), 0o644)
	if strings.Contains(get(), `class="config-error"`) {
		t.Fatal("banner must disappear once the config parses again")
	}
}
