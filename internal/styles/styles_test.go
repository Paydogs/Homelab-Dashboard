package styles

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dashboard/domain"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return NewManager(t.TempDir(), log.New(io.Discard, "", 0))
}

// writeStyle drops a style folder with a style.yml into the styles root.
func writeStyle(t *testing.T, dir, id, yml string) {
	t.Helper()
	styleDir := filepath.Join(dir, id)
	if err := os.MkdirAll(styleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(styleDir, "style.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
}

// the folder is the single source of truth: styles are whatever subfolders
// hold a style.yml; no styles come from code.
func TestLoadsStyleFolders(t *testing.T) {
	dir := newTestManager(t).dir // also creates the folder
	writeStyle(t, dir, "modern", "name: Modern\ncolors:\n  bg: \"#323353\"\n  accent: \"#4d9be6\"\n")
	writeStyle(t, dir, "retro", "name: Retro\ncolors:\n  bg: \"#2e222f\"\n  accent: \"#4d9be6\"\n")

	m := newTestManager(t)
	m.dir = dir
	all := m.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 folder styles, got %d", len(all))
	}
	// sorted by display name
	if all[0].ID != "modern" || all[1].ID != "retro" {
		t.Errorf("IDs = %q, %q; want modern, retro", all[0].ID, all[1].ID)
	}
}

func TestEmptyFolderYieldsNoStyles(t *testing.T) {
	m := newTestManager(t)
	if all := m.All(); len(all) != 0 {
		t.Errorf("expected no styles from an empty folder, got %+v", all)
	}
}

// invalid entries are skipped, valid ones still load
func TestInvalidStyleFilesSkipped(t *testing.T) {
	dir := newTestManager(t).dir
	writeStyle(t, dir, "bad", "noname: true\n")
	writeStyle(t, dir, "broken", "name: [unclosed\n")
	writeStyle(t, dir, "good", "name: Good\ncolors:\n  bg: \"#2e222f\"\n  up: \"#30e1b9\"\n")

	m := newTestManager(t)
	m.dir = dir
	all := m.All()
	if len(all) != 1 || all[0].ID != "good" {
		t.Fatalf("expected only good, got %+v", all)
	}
}

// a style folder references its own font files: when present they become
// an @font-face rule and a --titleFont/--font token; a missing file
// defaults to Arial.
func TestFontResolution(t *testing.T) {
	dir := newTestManager(t).dir
	writeStyle(t, dir, "modern", `name: Modern
titleFont: title.ttf
font: missing.ttf
colors:
  bg: "#323353"
  up: "#30e1b9"
`)
	// only the title font actually exists
	os.WriteFile(filepath.Join(dir, "modern", "title.ttf"), []byte("fake font bytes"), 0o644)

	m := newTestManager(t)
	m.dir = dir
	all := m.All()
	if len(all) != 1 {
		t.Fatalf("got %d styles, want 1", len(all))
	}
	s := all[0]
	if got := s.Colors["titleFont"]; got != `"dash-style-modern-title", "Arial", sans-serif` {
		t.Errorf("--titleFont = %q", got)
	}
	if got := s.Colors["font"]; got != `"Arial", sans-serif` {
		t.Errorf("--font = %q, want Arial for a missing file", got)
	}
	if want := `@font-face { font-family: "dash-style-modern-title"; src: url("/fonts/modern/title.ttf"); }`; s.FacesCSS != want {
		t.Errorf("FacesCSS = %q, want %q", s.FacesCSS, want)
	}

	// the font URL escapes spaces in file names
	if got := s.FontFileURL("DIGITAL TIMELESS DEMO.ttf"); got != "/fonts/modern/DIGITAL%20TIMELESS%20DEMO.ttf" {
		t.Errorf("FontFileURL = %q", got)
	}
}

func TestTraversalRejected(t *testing.T) {
	dir := newTestManager(t).dir
	writeStyle(t, dir, "secret", "name: Secret\ntitleFont: ../other/secret.txt\ncolors:\n  bg: \"#ccc\"\n")
	writeStyle(t, dir, "modern", "name: Modern\ntitleFont: ../../secret/secret.txt\ncolors:\n  bg: \"#323353\"\n")

	m := newTestManager(t)
	m.dir = dir
	for _, s := range m.All() {
		if strings.Contains(s.FacesCSS, "secret") {
			t.Errorf("traversal reference was resolved into an @font-face: %q", s.FacesCSS)
		}
	}
}

func TestStyleDir(t *testing.T) {
	m := newTestManager(t)
	dir := m.dir
	writeStyle(t, dir, "retro", "name: Retro\ncolors:\n  bg: \"#2e222f\"\n")

	got, ok := m.StyleDir("retro")
	if !ok || got != filepath.Join(dir, "retro") {
		t.Errorf("StyleDir(retro) = %q, %v", got, ok)
	}
	if _, ok := m.StyleDir("nope"); ok {
		t.Error("unknown id must not resolve")
	}
}

// fontSize / titleFontSize become the --fontSize/--titleFontSize tokens;
// the title one is separate so display fonts can size independently.
func TestFontSizeTokens(t *testing.T) {
	dir := newTestManager(t).dir
	writeStyle(t, dir, "modern", `name: Modern
fontSize: 14px
titleFontSize: 1.2rem
titleColor: "#f79617"
colors:
  bg: "#323353"
`)
	writeStyle(t, dir, "retro", "name: Retro\ncolors:\n  bg: \"#2e222f\"\n")

	m := newTestManager(t)
	m.dir = dir
	all := m.All()
	var modern, retro domain.Style
	for _, s := range all {
		switch s.ID {
		case "modern":
			modern = s
		case "retro":
			retro = s
		}
	}
	if modern.Colors["fontSize"] != "14px" || modern.Colors["titleFontSize"] != "1.2rem" {
		t.Errorf("font size tokens = %q / %q", modern.Colors["fontSize"], modern.Colors["titleFontSize"])
	}
	if modern.Colors["titleColor"] != "#f79617" {
		t.Errorf("titleColor token = %q", modern.Colors["titleColor"])
	}
	if _, ok := retro.Colors["fontSize"]; ok {
		t.Error("retro without fontSize must not emit a token")
	}
	if strings.Contains(cssOf(retro), "titleFontSize") {
		t.Error("retro without titleFontSize must not emit a token")
	}
}

func cssOf(s domain.Style) string { return s.CSS() }

// style.yml is the baseline; overwrite.yml holds the editor's overrides;
// the effective spec is the merge — and an empty overwrite changes nothing.
func TestOverwriteMerge(t *testing.T) {
	dir := newTestManager(t).dir
	writeStyle(t, dir, "retro", `name: Retro
titleFontSize: 0.85rem
colors:
  bg: "#2e222f"
  panel: "#3e3546"
`)
	// an overwrite changing one token, adding another, clearing one
	os.WriteFile(filepath.Join(dir, "retro", "overwrite.yml"),
		[]byte("colors:\n  bg: \"#313638\"\n  extra: \"#374e4a\"\n  panel: \"\"\n"), 0o644)

	m := newTestManager(t)
	m.dir = dir
	all := m.All()
	if len(all) != 1 {
		t.Fatalf("got %d styles, want 1", len(all))
	}
	s := all[0]
	if s.Colors["bg"] != "#313638" {
		t.Errorf("bg = %q, want overridden #313638", s.Colors["bg"])
	}
	if s.Colors["panel"] != "" {
		t.Errorf("panel = %q, want cleared (empty = back to base CSS default)", s.Colors["panel"])
	}
	if s.Colors["extra"] != "#374e4a" {
		t.Errorf("extra = %q, want added", s.Colors["extra"])
	}
	if s.Baseline.Colors["bg"] != "#2e222f" {
		t.Errorf("baseline bg = %q, want untouched original", s.Baseline.Colors["bg"])
	}
}

// SaveOverwrite writes exactly what the editor's right panel held — the
// round trip is: load base/overwrite, edit, save, reload.
func TestOverwriteRoundTrip(t *testing.T) {
	dir := newTestManager(t).dir
	writeStyle(t, dir, "modern", "name: Modern\ncolors:\n  bg: \"#323353\"\n")

	m := newTestManager(t)
	m.dir = dir
	if err := m.SaveOverwrite("modern", domain.StyleSpec{
		Colors: map[string]string{"bg": "#2e222f"},
	}); err != nil {
		t.Fatal(err)
	}
	text, err := m.Overwrite("modern")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "bg: '#2e222f'") { // yaml.v3 single-quotes marshaled strings
		t.Errorf("saved overwrite = %q", text)
	}
	if err := m.SaveOverwrite("nope", domain.StyleSpec{}); err == nil {
		t.Error("saving an unknown style must fail")
	}
}

func TestCSSBlock(t *testing.T) {
	dir := newTestManager(t).dir
	writeStyle(t, dir, "retro", "name: Retro\ncolors:\n  bg: \"#2e222f\"\n  chip: \"color-mix(in srgb, #374e4a 60%, transparent)\"\n")

	m := newTestManager(t)
	m.dir = dir
	all := m.All()
	css := all[0].CSS()
	want := ":root[data-style=\"retro\"] {\n"
	if got := css[:len(want)]; got != want {
		t.Errorf("CSS() = %q, want prefix %q", got, want)
	}
	if !strings.Contains(css, "--chip: color-mix(in srgb, #374e4a 60%, transparent);") {
		t.Errorf("CSS() drops color-mix values: %q", css)
	}
	if strings.Contains(css, "@font-face") {
		t.Errorf("style without font refs must not emit @font-face: %q", css)
	}
}
