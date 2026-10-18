package server

import (
	"io"
	"net/http"
	"os"
)

// The web editor round-trips the YAML source: GET /config-source returns
// the file as-is, POST /config-source validates it and (only when good)
// replaces the file on disk. Writing the config is a mutation of the
// dashboard's brain, so like the launcher it only serves the dashboard's
// own machine.
const maxConfigSize = 1 << 20 // 1 MiB of YAML is plenty for a homelab

func (s *Server) configSource(w http.ResponseWriter, r *http.Request) {
	if !editAllowed(s.cfg.Get(), r) {
		http.Error(w, "config editing is only available from the dashboard host", http.StatusForbidden)
		return
	}
	data, err := os.ReadFile(s.cfg.Path())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) saveConfig(w http.ResponseWriter, r *http.Request) {
	if !editAllowed(s.cfg.Get(), r) {
		http.Error(w, "config editing is only available from the dashboard host", http.StatusForbidden)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxConfigSize))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.cfg.Save(data); err != nil {
		s.logger.Printf("config save rejected: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.logger.Printf("config saved via web editor (%d bytes)", len(data))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("saved"))
}
