package server

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"dashboard/domain"
	"gopkg.in/yaml.v3"
)

// The web style editor is a two-panel view of a style: the LEFT panel shows
// the original style.yml (unchangeable), the RIGHT panel is the operator's
// overwrite.yml (starts empty) where overrides are typed. The dashboard
// then runs the style as base merged with overwrite. Like the config
// editor, these endpoints write files — local machine only.
const maxStyleSize = 1 << 16 // 64 KiB of style YAML is plenty

func (s *Server) styleSource(w http.ResponseWriter, r *http.Request) {
	if !editAllowed(s.cfg.Get(), r) {
		http.Error(w, "style editing is only available from the dashboard host", http.StatusForbidden)
		return
	}
	id := r.PathValue("id")
	part := r.PathValue("part")

	var style domain.Style
	found := false
	for _, st := range s.styles.All() {
		if st.ID == id {
			style = st
			found = true
			break
		}
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	switch part {
	case "base":
		text := "# original style.yml — read-only; put overrides in the right panel\n"
		text += readOnlyYAML(style)
		reply(w, text)
	case "overwrite":
		text, err := s.styles.Overwrite(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		reply(w, text)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) saveStyle(w http.ResponseWriter, r *http.Request) {
	if !editAllowed(s.cfg.Get(), r) {
		http.Error(w, "style editing is only available from the dashboard host", http.StatusForbidden)
		return
	}
	id := r.PathValue("id") // route: POST /style-source/{id}/overwrite — the URL segment is the guard
	data, err := io.ReadAll(io.LimitReader(r.Body, maxStyleSize))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	var edited domain.StyleSpec
	if err := dec.Decode(&edited); err != nil {
		s.logger.Printf("style edit rejected: %v", err)
		http.Error(w, "config rejected, overwrite unchanged: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.styles.SaveOverwrite(id, edited); err != nil {
		s.logger.Printf("style save failed: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.logger.Printf("style %s saved via web editor (overwrite.yml updated)", id)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("saved"))
}

// baseTokenDefaults are the token defaults from main.css's :root — the
// read-only panel mirrors them so every property shows a value even when a
// style leaves the token unset. Keep in sync with :root.
var baseTokenDefaults = map[string]string{
	"bg":       "#2e222f",
	"panel":    "#3e3546",
	"inset":    "color-mix(in srgb, #625565 45%, #2e222f)",
	"raised":   "#625565",
	"text":     "#c7dcd0",
	"muted":    "#9babb2",
	"accent":   "#4d9be6",
	"up":       "#30e1b9",
	"down":     "#e83b3b",
	"degraded": "#f79617",
	"pending":  "#9babb2",
	"chip":     "color-mix(in srgb, #374e4a 60%, transparent)",
}

// readOnlyYAML renders the COMPLETE base file for the editor's left panel:
// every property is present — unset scalars appear as "" and unset color
// tokens fall back to the main.css defaults — so the operator sees the full
// shape of a style at once.
func readOnlyYAML(style domain.Style) string {
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\n", style.Name)
	fmt.Fprintf(&b, "font: %s\n", yamlString(style.Font))
	fmt.Fprintf(&b, "titleFont: %s\n", yamlString(style.TitleFont))
	fmt.Fprintf(&b, "fontSize: %s\n", yamlString(or_(style.FontSize, "0.8125rem")))
	fmt.Fprintf(&b, "titleFontSize: %s\n", yamlString(or_(style.TitleFontSize, "0.85rem")))
	fmt.Fprintf(&b, "titleColor: %s\n", yamlString(or_(style.TitleColor, "var(--text)")))
	b.WriteString("colors:\n")

	colors := map[string]string{}
	for k, v := range baseTokenDefaults {
		colors[k] = v
	}
	for k, v := range style.Colors {
		// scalar-derived tokens (font family stacks, sizes) are already
		// shown as their own properties — keep the colors section colors
		if k == "font" || k == "titleFont" || k == "fontSize" ||
			k == "titleFontSize" || k == "titleColor" {
			continue
		}
		colors[k] = v
	}
	for _, k := range sortedTokenKeys(colors) {
		fmt.Fprintf(&b, "  %s: %s\n", k, yamlString(colors[k]))
	}
	return b.String()
}

// yamlString serializes a value as a YAML double-quoted scalar (Go's %q is
// YAML-compatible for the paths/colors we emit).
func yamlString(v string) string { return fmt.Sprintf("%q", v) }

func or_(preferred, fallback string) string {
	if preferred != "" {
		return preferred
	}
	return fallback
}

func sortedTokenKeys(set map[string]string) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func reply(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	_, _ = w.Write([]byte(text))
}
