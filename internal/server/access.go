package server

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"dashboard/domain"
)

// editAllowed checks the config's editAccess against the requester's IP:
// "local" (= default) only the dashboard host, "lan" any private-network
// client, "any" unrestricted. The `run:` launcher keeps its stricter
// isLocal gate no matter what.
func editAllowed(cfg *domain.Config, r *http.Request) bool {
	switch strings.ToLower(cfg.EditAccess) {
	case "any":
		return true
	case "lan":
		ip := requestIP(r)
		return ip.IsValid() && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
	default:
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
