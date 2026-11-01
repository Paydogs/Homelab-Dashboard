package server

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"dashboard"
	"dashboard/internal/appstate"
)

// The data-folder endpoints are host actions on the dashboard machine
// (they decide where the app reads and writes), so they share the security
// boundary of the run: launcher: localhost requests only. On the Docker
// server they are effectively unreachable — exactly right, since switching
// folders inside a container would be meaningless.

// dataDirCurrent reports the active data folder (absolute path).
func (s *Server) dataDirCurrent(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) {
		http.Error(w, "only accepted from the dashboard host", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(s.cfg.DataDir()))
}

// dataDirSet switches the dashboard to a different data folder: the body is
// the target path (created when missing, seeded with the sample config and
// starter styles when empty, and the config must parse — errors come back
// with parser line numbers like the config editor). On success both the
// config and the styles manager follow, and the choice is remembered for
// the next launch.
func (s *Server) dataDirSet(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) {
		http.Error(w, "only accepted from the dashboard host", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		http.Error(w, "reading path: "+err.Error(), http.StatusBadRequest)
		return
	}
	dir := strings.TrimSpace(string(body))
	if dir == "" {
		http.Error(w, "empty folder path", http.StatusBadRequest)
		return
	}
	seed := func(d string) error { return dashboard.Seed(d, s.logger) }
	if err := s.cfg.Retarget(dir, seed); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	current := s.cfg.DataDir()
	s.styles.Retarget(filepath.Join(current, "resources", "styles"))
	if err := appstate.Save(current); err != nil {
		// the switch worked — only the cross-restart memory failed
		s.logger.Printf("remembering data folder: %v", err)
	}
	s.logger.Printf("data folder set to %s", current)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(current))
}
