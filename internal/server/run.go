package server

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
)

// run launches a local runnable config item. It is a security boundary:
// only callers from the dashboard's own machine may launch anything, only
// executables explicitly listed in the YAML can ever be spawned, and the
// UI's confirm step precedes the request.
func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) {
		http.Error(w, "run requests are only accepted from the dashboard host", http.StatusForbidden)
		return
	}
	key := r.PathValue("key")
	item, ok := s.cfg.RunService(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	cmd := exec.Command(item.Run) //nolint:gosec,pathcheck // path comes from the local config file, never from the request
	if err := cmd.Start(); err != nil {
		s.logger.Printf("starting %s (%s): %v", item.Name, item.Run, err)
		http.Error(w, fmt.Sprintf("failed to start %s: %v", item.Name, err), http.StatusInternalServerError)
		return
	}
	s.logger.Printf("launched %s (%s)", item.Name, item.Run)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "started %s", item.Name)
}

// isLocal reports whether the request arrived from the dashboard host
// itself (IPv4 or IPv6 loopback, with or without a port).
func isLocal(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
