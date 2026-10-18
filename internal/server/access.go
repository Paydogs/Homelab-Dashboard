package server

import (
	"net"
	"net/http"
	"net/netip"

	"dashboard/domain"
)

// editAllowed checks the config's switch against the requester's IP.
// Effective mode: an explicitly set editAccess wins; else editable=true
// behaves like "lan"; else default "local" (dashboard host only).
// The `run:` launcher keeps its stricter isLocal gate no matter what.
func editAllowed(cfg *domain.Config, r *http.Request) bool {
	mode := cfg.EditAccess
	if !cfg.EditAccessSet && cfg.Editable {
		mode = "lan"
	}
	switch mode {
	case "any":
		return true
	case "lan":
		ip := requestIP(r)
		return ip.IsValid() && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
	default: // "local"
		return isLocal(r)
	}
}

// requestIP extracts the requester's IP from RemoteAddr.
func requestIP(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return addr
}
