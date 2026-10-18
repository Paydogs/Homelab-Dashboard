// Package styles loads UI themes from the styles folder: each style lives
// in its own subfolder (<config-dir>/resources/styles/<id>/) holding
// style.yml (the baseline), an overwrite.yml with the web editor's diffs
// (see loadStyle), and any files the config references — currently fonts,
// addressed by the `font` / `titleFont` properties as paths relative to
// that folder. The folder is the single source of truth; nothing is
// embedded in code.
package styles

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dashboard/domain"
	"gopkg.in/yaml.v3"
)

// fontFallback follows the custom font family (missing files, and webfonts
// that fail to download) — per design decision, a font that is not found
// defaults to Arial.
const fontFallback = `"Arial", sans-serif`

// Manager owns the styles folder. No styles are embedded in the code —
// the folder's contents are the single source of truth.
type Manager struct {
	dir    string
	logger *log.Logger
}

// NewManager creates the folder if it does not exist and is immediately
// usable.
func NewManager(stylesDir string, logger *log.Logger) *Manager {
	m := &Manager{dir: stylesDir, logger: logger}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		logger.Printf("creating styles dir: %v", err)
	}
	return m
}

// All scans the folder for styles — one subfolder per style, holding a
// style.yml (or <name>.yml) plus any files the config references (fonts,
// relative to the style folder). A legacy single *.yml at the top level is
// still supported. Files that fail to parse are logged and skipped; an
// empty folder simply yields no styles, and the page falls back to the
// base tokens in main.css.
func (m *Manager) All() []domain.Style {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		m.logger.Printf("reading styles dir: %v", err)
	}
	var out []domain.Style
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			for _, cfgName := range []string{"style.yml", "style.yaml", name + ".yml", name + ".yaml"} {
				path := filepath.Join(m.dir, name, cfgName)
				if data, err := os.ReadFile(path); err == nil {
					if s, ok := m.loadStyle(data, filepath.Join(m.dir, name), strings.ToLower(name)); ok {
						out = append(out, s)
					}
					break
				}
			}
			continue
		}
		if !(strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")) {
			continue
		}
		// legacy flat file; folder for font refs is the styles root
		data, err := os.ReadFile(filepath.Join(m.dir, name))
		if err != nil {
			m.logger.Printf("reading style %s: %v", name, err)
			continue
		}
		id := strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml"))
		if s, ok := m.loadStyle(data, m.dir, id); ok {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// overwriteName is the editor's per-style override file: style.yml stays
// the baseline, and edits made in the web UI land here — only the diffs.
const overwriteName = "overwrite.yml"

func (m *Manager) loadStyle(data []byte, dir, id string) (domain.Style, bool) {
	var base domain.StyleSpec
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&base); err != nil {
		m.logger.Printf("parsing style %s: %v", id, err)
		return domain.Style{}, false
	}
	if base.Name == "" || len(base.Colors) == 0 {
		m.logger.Printf("style %s needs a name and at least one color", id)
		return domain.Style{}, false
	}

	// every style folder carries an (initially empty) overwrite.yml with
	// the operator's editor-side tweaks; the effective spec is the merge.
	overPath := filepath.Join(dir, overwriteName)
	m.ensureOverwrite(overPath)
	var over domain.StyleSpec
	overData, err := os.ReadFile(overPath)
	if err != nil {
		m.logger.Printf("reading %s for style %s: %v", overwriteName, id, err)
	}
	if len(overData) > 0 {
		overDec := yaml.NewDecoder(strings.NewReader(string(overData)))
		overDec.KnownFields(true)
		if oerr := overDec.Decode(&over); oerr != nil && !errors.Is(oerr, io.EOF) {
			// EOF = a comments-only (effectively empty) overwrite: fine
			m.logger.Printf("parsing %s for style %s: %v (overwrite ignored)", overwriteName, id, oerr)
		}
	}

	merged := base.Effective(&over)
	s := domain.Style{
		ID:       id,
		Dir:      dir,
		Baseline: base,
	}
	s.Name = merged.Name
	s.Font = merged.Font
	s.TitleFont = merged.TitleFont
	s.FontSize = merged.FontSize
	s.TitleFontSize = merged.TitleFontSize
	s.TitleColor = merged.TitleColor
	s.Colors = merged.Colors
	m.resolveFonts(&s)
	return s, true
}

// ensureOverwrite creates an empty, commented overwrite.yml when missing.
func (m *Manager) ensureOverwrite(path string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	const header = `# Operator overwrites for style.yml — only the diffs you change land
# here. Removing a line reverts that value to style.yml.
`
	if err := os.WriteFile(path, []byte(header), 0o644); err != nil {
		m.logger.Printf("creating overwrite: %v", err)
	}
}

// SaveOverwrite replaces a style's overwrite.yml with the edited spec —
// the web editor's right-hand panel IS the overwrite file (baseline lives
// read-only in style.yml; the effective spec is the merge). Atomic write.
func (m *Manager) SaveOverwrite(id string, edited domain.StyleSpec) error {
	dir, ok := m.StyleDir(id)
	if !ok {
		return fmt.Errorf("unknown style %q", id)
	}
	out, err := yaml.Marshal(&edited)
	if err != nil {
		return fmt.Errorf("serializing overwrite: %w", err)
	}
	path := filepath.Join(dir, overwriteName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replacing overwrite: %w", err)
	}
	return nil
}

// resolveFonts turns the `font` / `titleFont` file references into
// @font-face rules plus --font/--titleFont tokens, but only when the file
// actually exists — a missing font defaults to Arial per design decision.
// References are relative paths into the style's own folder; subfolders
// are fine, escaping the folder is not.
func (m *Manager) resolveFonts(s *domain.Style) {
	var faces []string
	if _, ok := m.inStyleDir(s, s.Font); ok {
		family := s.FontFamily(false)
		faces = append(faces, fontFace(family, s.FontFileURL(s.Font)))
		s.Colors["font"] = `"` + family + `", ` + fontFallback
	} else if s.Font != "" {
		m.logger.Printf("style %s: font file %q not found, defaulting to Arial", s.ID, s.Font)
		s.Colors["font"] = fontFallback
	}
	if _, ok := m.inStyleDir(s, s.TitleFont); ok {
		family := s.FontFamily(true)
		faces = append(faces, fontFace(family, s.FontFileURL(s.TitleFont)))
		s.Colors["titleFont"] = `"` + family + `", ` + fontFallback
	} else if s.TitleFont != "" {
		m.logger.Printf("style %s: titleFont file %q not found, defaulting to Arial", s.ID, s.TitleFont)
		s.Colors["titleFont"] = fontFallback
	}
	s.FacesCSS = strings.Join(faces, "\n")
	if s.FontSize != "" {
		s.Colors["fontSize"] = s.FontSize
	}
	if s.TitleFontSize != "" {
		s.Colors["titleFontSize"] = s.TitleFontSize
	}
	if s.TitleColor != "" {
		s.Colors["titleColor"] = s.TitleColor
	}
}

func fontFace(family, url string) string {
	return fmt.Sprintf("@font-face { font-family: %q; src: url(%q); }", family, url)
}

// inStyleDir resolves ref inside the style's folder and verifies it exists;
// directory traversal is rejected.
func (m *Manager) inStyleDir(s *domain.Style, ref string) (string, bool) {
	if ref == "" {
		return "", false
	}
	clean := filepath.Clean(filepath.Join(s.Dir, filepath.FromSlash(ref)))
	if !strings.HasPrefix(clean, s.Dir+string(filepath.Separator)) &&
		filepath.Clean(s.Dir) != clean {
		m.logger.Printf("style %s: font path %q escapes the style folder — rejected", s.ID, ref)
		return "", false
	}
	if _, err := os.Stat(clean); err != nil {
		return "", false
	}
	return clean, true
}

// StyleDir returns the on-disk folder of a style id (used by the server's
// /fonts/ file route).
func (m *Manager) StyleDir(id string) (string, bool) {
	for _, s := range m.All() {
		if s.ID == id {
			return s.Dir, true
		}
	}
	return "", false
}

// Overwrite returns a style's overwrite.yml contents — the web editor's
// right-hand panel pre-fills from it.
func (m *Manager) Overwrite(id string) (string, error) {
	dir, ok := m.StyleDir(id)
	if !ok {
		return "", fmt.Errorf("unknown style %q", id)
	}
	data, err := os.ReadFile(filepath.Join(dir, overwriteName))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
