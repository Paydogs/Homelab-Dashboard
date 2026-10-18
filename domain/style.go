package domain

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Style is a named UI theme living in its own folder under
// <config-dir>/resources/styles/<id>/ — style.yml plus optional files the
// config references (currently fonts, relative to that folder).
type Style struct {
	// ID is the style's folder base name (lowercased) — the UI's
	// data-style attribute and picker option value. Not in YAML.
	ID string `yaml:"-"`

	// Dir is the style's folder on disk; font paths resolve inside it.
	// Not in YAML.
	Dir string `yaml:"-"`

	// Name is the display name shown in the style picker.
	Name string `yaml:"name"`

	// Font / TitleFont are font files for the body / titles, as paths
	// relative to the style's folder (e.g. "fonts/demo.ttf"). When the
	// file exists on disk, the styles package emits an @font-face rule
	// and tokens --font / --titleFont pointing at it.
	Font      string `yaml:"font,omitempty"`
	TitleFont string `yaml:"titleFont,omitempty"`

	// FontSize / TitleFontSize are CSS font sizes (e.g. "14px", "1.1rem").
	// FontSize scales the whole UI (it sets the root font size); the title
	// one overrides it for the page title — handy for display fonts. When
	// unset, the tokens keep their main.css defaults.
	FontSize      string `yaml:"fontSize,omitempty"`
	TitleFontSize string `yaml:"titleFontSize,omitempty"`

	// TitleColor is the page title's text color (any CSS color); it
	// overrides the --titleColor token when set.
	TitleColor string `yaml:"titleColor,omitempty"`

	// Colors maps CSS custom property names (without the -- prefix) to
	// CSS values, e.g. {bg: "#2e222f", up: "#30e1b9"}.
	Colors map[string]string `yaml:"colors"`

	// Baseline is style.yml's spec (before the overwrite was applied) —
	// the web style editor's read-only panel. Not in YAML.
	Baseline StyleSpec `yaml:"-"`

	// FacesCSS is the pre-built @font-face block (written by the styles
	// package when font files exist). Not in YAML.
	FacesCSS string `yaml:"-"`
}

// StyleSpec is the editable part of a style. style.yml (the operator's
// baseline) and overwrite.yml (the web editor's output) both serialize
// this shape; the effective spec is baseline merged with overwrite.
type StyleSpec struct {
	Name          string            `yaml:"name,omitempty"`
	Font          string            `yaml:"font,omitempty"`
	TitleFont     string            `yaml:"titleFont,omitempty"`
	FontSize      string            `yaml:"fontSize,omitempty"`
	TitleFontSize string            `yaml:"titleFontSize,omitempty"`
	TitleColor    string            `yaml:"titleColor,omitempty"`
	Colors        map[string]string `yaml:"colors,omitempty"`
}

// Effective merges overwrite into the baseline spec: any non-empty field
// replaces the baseline's; in colors, a non-empty value overrides and an
// explicit empty value clears the token (falling back to the base CSS
// defaults).
func (spec StyleSpec) Effective(over *StyleSpec) StyleSpec {
	merged := spec
	if over == nil {
		return merged
	}
	if over.Name != "" {
		merged.Name = over.Name
	}
	if over.Font != "" {
		merged.Font = over.Font
	}
	if over.TitleFont != "" {
		merged.TitleFont = over.TitleFont
	}
	if over.FontSize != "" {
		merged.FontSize = over.FontSize
	}
	if over.TitleFontSize != "" {
		merged.TitleFontSize = over.TitleFontSize
	}
	if over.TitleColor != "" {
		merged.TitleColor = over.TitleColor
	}
	merged.Colors = map[string]string{}
	for k, v := range spec.Colors {
		merged.Colors[k] = v
	}
	for k, v := range over.Colors {
		if v == "" {
			delete(merged.Colors, k)
			continue
		}
		merged.Colors[k] = v
	}
	return merged
}

// FontFamily returns the @font-face family name for the body or title
// font of this style.
func (s *Style) FontFamily(title bool) string {
	kind := "body"
	if title {
		kind = "title"
	}
	return "dash-style-" + s.ID + "-" + kind
}

// FontFileURL returns the /fonts/<id>/<path> URL under which the server
// serves files from this style's folder.
func (s *Style) FontFileURL(ref string) string {
	parts := strings.Split(ref, "/")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = url.PathEscape(p)
	}
	return "/fonts/" + url.PathEscape(s.ID) + "/" + strings.Join(out, "/")
}

// CSS renders this style as @font-face rules (if any) plus a
// :root[data-style=…] token block. The font paths and color values are
// operator-authored config, so they are emitted verbatim —
// html/template's CSS filter would reject legal values like color-mix(…).
func (s *Style) CSS() string {
	var b strings.Builder
	if s.FacesCSS != "" {
		b.WriteString(s.FacesCSS)
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, ":root[data-style=%q] {\n", s.ID)
	keys := make([]string, 0, len(s.Colors))
	for k := range s.Colors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "  --%s: %s;\n", k, s.Colors[k])
	}
	b.WriteString("}")
	return b.String()
}
