// Package server wires the HTTP routes: the page render, the health status
// fragments (refreshed via htmx), static and resource assets, and the
// localhost-only launcher for local runnable items.
package server

import (
	"context"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"dashboard/internal/config"
	"dashboard/internal/health"
	"dashboard/web"
)

// Server owns the parsed templates plus the config and health managers.
type Server struct {
	ctx    context.Context
	cfg    *config.Manager
	eng    *health.Engine
	logger *log.Logger
	tmpl   *template.Template
}

// New parses the embedded templates once. Template changes therefore need a
// restart, but config changes never do.
func New(cfg *config.Manager, eng *health.Engine, logger *log.Logger) (*Server, error) {
	funcs := template.FuncMap{
		"join":  strings.Join,
		"lower": strings.ToLower,
	}
	tfiles, err := fs.Sub(web.Assets, "templates")
	if err != nil {
		return nil, err
	}
	// Register each template under its relative path (e.g.
	// "components/card.tmpl") so component references read like file paths.
	root := template.New("")
	root = root.Funcs(funcs)
	err = fs.WalkDir(tfiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".tmpl") {
			return err
		}
		content, err := fs.ReadFile(tfiles, path)
		if err != nil {
			return err
		}
		_, err = root.New(path).Parse(string(content))
		return err
	})
	if err != nil {
		return nil, err
	}
	s := &Server{ctx: context.Background(), cfg: cfg, eng: eng, logger: logger, tmpl: root}
	s.syncHealth(cfg.Get())
	return s, nil
}

// Routes builds the top-level handler with request logging.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /summary", s.summary)
	mux.HandleFunc("GET /health-status/{key}", s.healthStatus)
	mux.HandleFunc("POST /run/{key}", s.run)
	mux.Handle("GET /static/", staticHandler())
	mux.Handle("GET /resources/", http.StripPrefix("/resources/",
		http.FileServer(http.Dir(s.cfg.ResourcesDir()))))
	return logRequests(s.logger, mux)
}

func staticHandler() http.Handler {
	files, err := fs.Sub(web.Assets, "static")
	if err != nil {
		panic("embedded static assets missing: " + err.Error())
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(files)))
}

func logRequests(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.logger.Printf("rendering %s: %v", name, err)
	}
}
