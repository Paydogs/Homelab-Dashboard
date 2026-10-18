package server

import (
	"net/http"
	"testing"

	"dashboard/domain"
)

func reqFrom(ip string) *http.Request {
	// IPv6 addresses need bracketed ports; bare IPv4 gets ":port" appended
	ipAddr := requestIP(&http.Request{RemoteAddr: ip})
	if ipAddr.Is4() || ipAddr.Is4In6() {
		return &http.Request{RemoteAddr: ip + ":1234"}
	}
	return &http.Request{RemoteAddr: "[" + ip + "]:1234"}
}

func TestEditAllowed(t *testing.T) {
	cfg := func(editAccess string) *domain.Config {
		return &domain.Config{EditAccess: editAccess}
	}
	cases := []struct {
		name   string
		access string
		ip     string
		want   bool
	}{
		{"default local: loopback allowed", "", "127.0.0.1", true},
		{"default local: lan refused", "", "192.168.1.50", false},
		{"lan: private allowed", "lan", "192.168.1.50", true},
		{"lan: 10/8 allowed", "lan", "10.0.0.7", true},
		{"lan: ipv6 ula allowed", "lan", "fd00::3", true},
		{"lan: loopback allowed", "lan", "::1", true},
		{"lan: public refused", "lan", "8.8.8.8", false},
		{"any: public allowed", "any", "8.8.8.8", true},
	}
	for _, c := range cases {
		if got := editAllowed(cfg(c.access), reqFrom(c.ip)); got != c.want {
			t.Errorf("%s: editAllowed(%s from %s) = %v", c.name, c.access, c.ip, got)
		}
	}

	// the launcher (run:) is unaffected by lan/any: it stays host-only
	for _, access := range []string{"", "lan", "any"} {
		if isLocal(reqFrom("192.168.1.50")) {
			t.Errorf("run launcher must never fire for a lan client (editAccess=%q)", access)
		}
	}
	if !isLocal(reqFrom("127.0.0.1")) {
		t.Error("run launcher must work for loopback")
	}
}

func TestRequestIPMalformed(t *testing.T) {
	r := &http.Request{RemoteAddr: "not-an-ip"}
	if ip := requestIP(r); ip.IsValid() {
		t.Errorf("garbage RemoteAddr parsed as %v", ip)
	}
	// malformed addr also fails the stricter checks rather than panicking
	if editAllowed(&domain.Config{EditAccess: "lan"}, r) {
		t.Error("unparsable RemoteAddr must not pass as lan")
	}
}
