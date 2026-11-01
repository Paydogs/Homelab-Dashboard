package server

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"dashboard/internal/config"
	"dashboard/internal/health"
	"dashboard/internal/styles"
)

// Miller-column view: every group renders a drill-down column holding its
// sub-group cards + its leaf rows; the first root's column is the
// server-side default selection.
func TestLevelColumns(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yml"), []byte(`title: T
groups:
  - group: Leafy
    children:
      - service: A
        url: http://a
  - group: Branchy
    groups:
      - group: Mid
        groups:
          - group: Deep
            children:
              - service: D
                url: http://d
      - group: Flat
        children:
          - service: F
            url: http://f
`), 0o644)
	logger := log.New(io.Discard, "", 0)
	mgr, err := config.NewManager(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	styleMgr := styles.NewManager(filepath.Join(dir, "resources", "styles"), logger)
	data := buildView(mgr.Get(), mgr, health.New(), styleMgr)

	cols := map[string]*LevelColumn{}
	for _, l := range data.Levels {
		cols[l.Path] = l
	}
	// every group in the tree got a column, containers included
	for _, want := range []string{"Leafy", "Branchy", "Branchy/Mid", "Branchy/Mid/Deep", "Branchy/Flat"} {
		if cols[want] == nil {
			t.Fatalf("level column %q missing (have %v)", want, keysOf(data.Levels))
		}
	}

	// Leafy: rows only, no cards
	if len(cols["Leafy"].Items) != 1 || len(cols["Leafy"].Cards) != 0 {
		t.Errorf("Leafy column wrong: %+v", cols["Leafy"])
	}
	// Branchy: cards only
	if len(cols["Branchy"].Cards) != 2 || len(cols["Branchy"].Items) != 0 {
		t.Errorf("Branchy column wrong: %+v", cols["Branchy"])
	}
	// subtree counts flow up: Branchy's card for Deep totals 1
	for _, card := range cols["Branchy"].Cards {
		if card.Name == "Deep" && card.Total != 1 {
			t.Errorf("Deep card counts: %+v", card)
		}
	}
	// the first root is the server-side default selection
	if !cols["Leafy"].Selected {
		t.Error("the first root's column should be marked Selected")
	}
	if cols["Branchy"].Selected {
		t.Error("non-default columns must not be marked Selected")
	}

	// columns are in PRE-ORDER: parents before children, so a drill path
	// reads left to right in the DOM
	firstBranchy := -1
	firstMid := -1
	for i, l := range data.Levels {
		switch l.Path {
		case "Branchy":
			firstBranchy = i
		case "Branchy/Mid":
			firstMid = i
		}
	}
	if firstBranchy == -1 || firstMid == -1 || firstBranchy > firstMid {
		t.Errorf("columns out of order: parent Branchy (index %d) must precede Branchy/Mid (%d)",
			firstBranchy, firstMid)
	}
}

func keysOf(levels []*LevelColumn) []string {
	out := make([]string, 0, len(levels))
	for _, l := range levels {
		out = append(out, l.Path)
	}
	return out
}

// health: {disabled: true} probes nothing and the row carries no status —
// identical to omitting the health block, without losing the settings
func TestDisabledHealthcheck(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yml"), []byte(`title: T
groups:
  - group: G
    children:
      - service: Normal
        url: http://a
        health: {path: /}
      - service: Off
        url: http://b
        health: {path: /, disabled: true}
`), 0o644)
	logger := log.New(io.Discard, "", 0)
	mgr, err := config.NewManager(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	styleMgr := styles.NewManager(filepath.Join(dir, "resources", "styles"), logger)
	srv, err := New(mgr, health.New(), styleMgr, logger)
	if err != nil {
		t.Fatal(err)
	}
	srv.syncHealth(mgr.Get())
	data := buildView(mgr.Get(), mgr, srv.eng, styleMgr)

	var normal, off *ItemView
	for i := range data.Levels[0].Items {
		switch data.Levels[0].Items[i].Name {
		case "Normal":
			normal = &data.Levels[0].Items[i]
		case "Off":
			off = &data.Levels[0].Items[i]
		}
	}
	if normal == nil || normal.Health == nil {
		t.Error("enabled healthcheck should render a status block")
	}
	if off == nil || off.Health != nil {
		t.Error("disabled healthcheck must render no status block")
	}
	if _, ok := srv.eng.Result("G/Off"); ok {
		t.Error("disabled healthcheck must not be probed")
	}
}
