package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// a service with a self-signed certificate (Proxmox & co.) probes OFFLINE
// with strict TLS, and ONLINE once the target is marked private
// (health: {private: true}) — TLS verification is then dismissed for it.
func TestPrivateTargetDismissesTLS(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	eng := New()
	strict := eng.probe(Target{Key: "s", BaseURL: ts.URL, Interval: time.Minute})
	if strict.Status != StatusDown {
		t.Fatalf("strict probe against self-signed TLS = %v, want down", strict.Status)
	}
	lax := eng.probe(Target{Key: "s", BaseURL: ts.URL, Interval: time.Minute, Private: true})
	if lax.Status != StatusUp {
		t.Fatalf("private probe against self-signed TLS = %v (%s), want up", lax.Status, lax.Err)
	}
}

// a plain HTTP target probes the same whichever client — private must not
// change non-TLS behavior
func TestPrivateTargetPlainHTTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	eng := New()
	got := eng.probe(Target{Key: "s", BaseURL: ts.URL, Interval: time.Minute, Private: true})
	if got.Status != StatusUp {
		t.Fatalf("plain-HTTP private probe = %v (%s), want up", got.Status, got.Err)
	}
}
