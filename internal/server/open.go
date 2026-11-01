package server

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
)

// openFolder reveals a folder in the host's file manager. A variable so the
// open-data handler can be tested without popping a real window.
var openFolder = func(dir string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	// Start, not Run: file managers detach (explorer.exe also exits nonzero
	// even on success) — fire and forget, don't wait on it.
	return cmd.Start()
}

// openData opens the dashboard's data folder (config.yml, styles, icons) in
// the host's file manager — a convenience for the desktop installs. It is a
// host action just like the run: launcher, so it follows the same security
// boundary: localhost requests only.
func (s *Server) openData(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) {
		http.Error(w, "open requests are only accepted from the dashboard host", http.StatusForbidden)
		return
	}
	dir := s.cfg.DataDir()
	if err := openFolder(dir); err != nil {
		s.logger.Printf("opening data folder %s: %v", dir, err)
		http.Error(w, fmt.Sprintf("failed to open the data folder: %v", err), http.StatusInternalServerError)
		return
	}
	s.logger.Printf("opened data folder %s", dir)
	w.WriteHeader(http.StatusNoContent)
}
