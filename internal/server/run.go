package server

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"

	"dashboard/domain"
)

// run launches a locally runnable App child. It is a security boundary:
// only callers from the dashboard's own machine may launch anything, only
// executables explicitly listed in the config can ever be spawned, and the
// UI's confirm step precedes the request.
func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) {
		http.Error(w, "run requests are only accepted from the dashboard host", http.StatusForbidden)
		return
	}
	key := r.PathValue("key")
	child, ok := s.cfg.RunService(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	app := child.(*domain.App)
	path := app.Run
	cmd := exec.Command(path) //nolint:gosec,pathcheck // path comes from the local config file, never from the request
	if err := cmd.Start(); err != nil {
		s.logger.Printf("starting %s (%s): %v", app.Title, path, err)
		http.Error(w, fmt.Sprintf("failed to start %s: %v", app.Title, err), http.StatusInternalServerError)
		return
	}
	s.logger.Printf("launched %s (%s)", app.Title, path)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "started %s", app.Title)
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
