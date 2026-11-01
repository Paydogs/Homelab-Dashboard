package server

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func getResults(t *testing.T, url string) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	return string(body)
}

// /results matches leaf rows only — by name, tag and url — and never
// reveals group names as results
func TestResultsEndpoint(t *testing.T) {
	ts := newTestServer(t)
	base := ts.URL

	// the fixture has one service "Gitea" url http://192.168.1.30:3000
	if body := getResults(t, base+"/results?q=gitea"); !strings.Contains(body, "Gitea") {
		t.Error("searching by name misses the Gitea row")
	}
	if body := getResults(t, base+"/results?q=192.168.1.30"); !strings.Contains(body, "Gitea") {
		t.Error("searching by url/ip misses the Gitea row")
	}
	if body := getResults(t, base+"/results?q=zzznope"); strings.Contains(body, "Gitea") || !strings.Contains(body, "nothing matches") {
		t.Error("a no-hit query must render the empty state only")
	}
}

// tags narrow results (repeated tags=all-must-match)
func TestResultsTagFilter(t *testing.T) {
	ts := newTestServer(t)
	if c := strings.Count(getResults(t, ts.URL+"/results"), `class="row`); c == 0 {
		t.Fatal("no rows rendered without any query — fixtures missing?")
	}
	// a tag nothing carries
	if c := strings.Count(getResults(t, ts.URL+"/results?tags=nowhere"), `class="row`); c != 0 {
		t.Errorf("tag with no rows returned %d rows, want 0", c)
	}
}
