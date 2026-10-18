package server

import (
	"strings"
	"testing"

	"dashboard/domain"
)

// the editor's read-only panel shows every property filled in: unset
// scalars as "" and unset color tokens with their main.css defaults.
func TestReadOnlyYAMLFillsEverything(t *testing.T) {
	style := domain.Style{
		ID:     "retro",
		Name:   "Retro",
		Colors: map[string]string{"bg": "#2e222f"},
	}
	out := readOnlyYAML(style)

	for _, line := range []string{
		`name: Retro`,
		`font: ""`,
		`titleFont: ""`,
		`fontSize: "0.8125rem"`,
		`titleFontSize: "0.85rem"`,
		`titleColor: "var(--text)"`,
		`  bg: "#2e222f"`,
		`  accent: "#4d9be6"`, // default filled
		`  chip: "color-mix(in srgb, #374e4a 60%, transparent)"`, // default filled
	} {
		if !strings.Contains(out, line) {
			t.Errorf("read-only view missing %q:\n%s", line, out)
		}
	}

	// a fully-populated style keeps its own values and tokens
	style.TitleFont = "DEMO.ttf"
	style.Colors["bg"] = "#484a77"
	style.Colors["extra"] = "#374e4a"
	out = readOnlyYAML(style)
	if !strings.Contains(out, `titleFont: "DEMO.ttf"`) || !strings.Contains(out, `  extra: "#374e4a"`) {
		t.Errorf("custom values lost in read-only view:\n%s", out)
	}
	if !strings.Contains(out, `  bg: "#484a77"`) {
		t.Errorf("overridden token lost:\n%s", out)
	}
}

func TestResolveDefaultStyle(t *testing.T) {
	styles := []domain.Style{{ID: "modern"}, {ID: "retro"}}
	cases := []struct {
		name string
		set  string
		want string
	}{
		{"configured style wins", "retro", "retro"},
		{"case-insensitive", "MODERN", "modern"},
		{"unknown falls back to first folder", "nope", "modern"},
		{"unset falls back to first folder", "", "modern"},
	}
	for _, c := range cases {
		if got := resolveDefaultStyle(&domain.Config{DefaultStyle: c.set}, styles); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if got := resolveDefaultStyle(&domain.Config{DefaultStyle: "retro"}, nil); got != "" {
		t.Errorf("no styles at all should resolve empty, got %q", got)
	}
}
