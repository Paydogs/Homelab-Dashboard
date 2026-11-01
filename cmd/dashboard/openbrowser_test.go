package main

import (
	"net"
	"testing"
)

func TestDashboardURL(t *testing.T) {
	cases := map[string]string{
		":8080":          "http://localhost:8080",
		"0.0.0.0:9000":   "http://localhost:9000",
		"127.0.0.1:8080": "http://127.0.0.1:8080",
		"8080":           "http://localhost:8080", // bare port
	}
	for listen, want := range cases {
		if got := dashboardURL(listen); got != want {
			t.Errorf("dashboardURL(%q) = %q, want %q", listen, got, want)
		}
	}
}

// waitForListen returns once the address accepts connections
func TestWaitForListen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	waitForListen(ln.Addr().String()) // must not block past its cap nor panic
}

func TestDialAddrWildcard(t *testing.T) {
	if got := dialAddr(":8080"); got != "127.0.0.1:8080" {
		t.Errorf("dialAddr(:8080) = %q, want 127.0.0.1:8080", got)
	}
	if got := dialAddr("192.168.1.5:9000"); got != "192.168.1.5:9000" {
		t.Errorf("dialAddr(bound) = %q, want unchanged", got)
	}
}
