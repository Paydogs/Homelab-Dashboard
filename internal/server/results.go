package server

import (
	"net/http"
	"strings"

	"dashboard/domain"
)

// results serves the search overview: the leaf rows of the whole tree that
// match the query (`q`, against name/tags/url) or the selected tags
// (repeated `tags=` params), rendered as a 3-wide grid for the tinted box
// between filter row and column layout. Only leaf rows match — never group
// names; the columns layout stays untouched while searching.
func (s *Server) results(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	s.syncHealth(cfg)

	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	var tags []string
	for _, t := range r.URL.Query()["tags"] {
		if t != "" {
			tags = append(tags, t)
		}
	}

	var items []ItemView
	cfg.Walk(func(path string, g *domain.Group) {
		for _, ch := range g.Items {
			v, _ := leafView(s.cfg, s.eng, path, ch)
			if resultMatches(v, q, tags) {
				items = append(items, v)
			}
		}
	})
	s.render(w, "results.tmpl", items)
}

// resultMatches mirrors the browser row rules on the server side: name,
// tags and url/ip text against the query, plus every required tag present.
func resultMatches(v ItemView, q string, tags []string) bool {
	if len(tags) > 0 {
		for _, t := range tags {
			found := false
			for _, vt := range v.Tags {
				if vt == t {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	if q == "" {
		return true
	}
	text := strings.ToLower(v.Name + " " + strings.Join(v.Tags, " "))
	if v.URL != "" {
		text += " " + strings.ToLower(v.URL)
	}
	if v.Run != "" {
		text += " " + strings.ToLower(v.Run)
	}
	return strings.Contains(text, q)
}
