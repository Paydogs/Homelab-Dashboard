package server

// PWA plumbing: the web app manifest and the service worker script. Both
// are must-revalidate responses — the manifest follows the config title on
// every request, and a stale sw.js would keep an old cache name alive. The
// service worker is served at the root (NOT under /static/) so its scope
// covers the whole site.
import (
	"encoding/json"
	"io/fs"
	"net/http"

	"dashboard/web"
)

func (s *Server) manifest(w http.ResponseWriter, r *http.Request) {
	// Hand-marshal instead of a template: html/template would HTML-escape
	// the title, which could produce invalid JSON.
	m := struct {
		Name            string         `json:"name"`
		ShortName       string         `json:"short_name"`
		Description     string         `json:"description"`
		StartURL        string         `json:"start_url"`
		Scope           string         `json:"scope"`
		Display         string         `json:"display"`
		BackgroundColor string         `json:"background_color"`
		ThemeColor      string         `json:"theme_color"`
		Icons           []manifestIcon `json:"icons"`
	}{ // colors match main.css :root (Resurrect 64); a manifest is per-app, not per-style
		Name:            s.cfg.Get().Title,
		ShortName:       s.cfg.Get().Title,
		Description:     "Homelab dashboard — services, health and quick links",
		StartURL:        "/",
		Scope:           "/",
		Display:         "standalone",
		BackgroundColor: "#2e222f",
		ThemeColor:      "#2e222f",
		Icons: []manifestIcon{
			{"/static/icons/icon-192.png", "192x192", "image/png", "any"},
			{"/static/icons/icon-512.png", "512x512", "image/png", "any"},
			{"/static/icons/icon-maskable-512.png", "512x512", "image/png", "maskable"},
		},
	}
	body, err := json.Marshal(m)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(body)
}

type manifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

func (s *Server) swJS(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(web.Assets, "static/js/sw.js")
	if err != nil {
		http.Error(w, "service worker missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(content)
}
