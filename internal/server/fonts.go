package server

import (
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

// styleFont serves a file referenced by a style's config (e.g. a font
// shipped inside that style's folder under resources/styles/<id>/). Both
// URL segments are sanitized: the id must match a loaded style's folder and
// the file path may not escape it.
func (s *Server) styleFont(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	file := r.PathValue("file")

	if !validStyleID(id) || !validFontPath(file) {
		http.NotFound(w, r)
		return
	}
	dir, ok := s.styles.StyleDir(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	// rebuild a clean, traversal-safe relative path from the segments
	ref := path.Join(strings.Split(file, "/")...)
	http.ServeFile(w, r, filepath.Join(dir, filepath.FromSlash(ref)))
}

func validStyleID(id string) bool {
	if id == "" {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

func validFontPath(file string) bool {
	if file == "" {
		return false
	}
	for _, segment := range strings.Split(file, "/") {
		if segment == "" || segment == "." || segment == ".." ||
			strings.ContainsAny(segment, `\*?"<>|:`) {
			return false
		}
	}
	return true
}
