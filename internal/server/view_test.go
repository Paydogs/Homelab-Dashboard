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

// Group rendering rules:
//   - a group with leaf rows renders its own panel (rows + expanded
//     sub-group sections) — Root here
//   - a group holding ONLY sub-groups is a pure container: no panel, only
//     its middle-column cards — Container here
//   - every nested group with rows gets a standalone panel (drill-in target)
func TestNestedGroupPanels(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yml"), []byte(`title: T
groups:
  - group: Root
    children:
      - service: A
        url: http://a
    groups:
      - group: Sub
        children:
          - service: B
            url: http://b
  - group: Container
    groups:
      - group: Inner
        children:
          - service: C
            url: http://c
`), 0o644)
	logger := log.New(io.Discard, "", 0)
	mgr, err := config.NewManager(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	styleMgr := styles.NewManager(filepath.Join(dir, "resources", "styles"), logger)
	data := buildView(mgr.Get(), mgr, health.New(), styleMgr)

	seen := map[string]bool{}
	for _, p := range data.Panels {
		seen[p.Path] = true
	}
	for _, want := range []string{"Root", "Root/Sub", "Container/Inner"} {
		if !seen[want] {
			t.Errorf("panel %q missing from render (got %v)", want, seen)
		}
	}
	if seen["Container"] {
		t.Error("container-only group must NOT render a composite panel")
	}

	// the nested service lands in its panel's rows
	var sub *GroupView
	for i := range data.Panels {
		if data.Panels[i].Path == "Root/Sub" {
			sub = &data.Panels[i]
		}
	}
	if sub == nil || len(sub.Items) != 1 || sub.Items[0].Name != "B" {
		t.Errorf("nested panel rows wrong: %+v", sub)
	}
	// the root panel carries the nested group as a titled section
	var root *GroupView
	for i := range data.Panels {
		if data.Panels[i].Path == "Root" {
			root = &data.Panels[i]
		}
	}
	if root == nil || len(root.SubGroups) != 1 || root.SubGroups[0].Path != "Root/Sub" {
		t.Errorf("root sub-group sections wrong: %+v", root)
	} else if len(root.SubGroups[0].Items) != 1 || root.SubGroups[0].Items[0].Name != "B" {
		t.Errorf("sub-group section rows wrong: %+v", root.SubGroups[0])
	}

	// the middle column: sub-group cards of each root that has sub-groups —
	// including container-only roots (they are nothing BUT their cards)
	if len(data.CardCols) != 2 {
		t.Fatalf("card columns wrong: %+v", data.CardCols)
	}
	if data.CardCols[0].Root != "Root" || len(data.CardCols[0].Cards) != 1 ||
		data.CardCols[0].Cards[0].Path != "Root/Sub" {
		t.Errorf("Root card column wrong: %+v", data.CardCols[0])
	}
	if data.CardCols[1].Root != "Container" || len(data.CardCols[1].Cards) != 1 ||
		data.CardCols[1].Cards[0].Path != "Container/Inner" {
		t.Errorf("Container card column wrong: %+v", data.CardCols[1])
	}
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
	for i := range data.Panels[0].Items {
		switch data.Panels[0].Items[i].Name {
		case "Normal":
			normal = &data.Panels[0].Items[i]
		case "Off":
			off = &data.Panels[0].Items[i]
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
