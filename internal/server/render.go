package server

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dashboard/domain"
	"dashboard/internal/config"
	"dashboard/internal/health"
	"dashboard/internal/styles"
)

// SidebarView is one entry of the left column (and, shape-wise, of the
// drill-down tiles): a group with its live subtree counts.
type SidebarView struct {
	Path  string
	Name  string
	OK    int
	Error int
	Total int
	// Selected (used with the sidebar entry) marks the server-side default
	// pick; the browser's remembered selection overrides it on load.
	Selected bool
}

// GroupView is the rendered panel for one group: its direct child rows and
// its sub-groups rendered as titled sections inside the same panel (nested
// groups read as stacked blocks, the way dashboard rows look). Groups are
// structural — the panel header shows title + subtitle only.
type GroupView struct {
	Path      string
	Name      string
	Subtitle  string
	Items     []ItemView
	SubGroups []GroupView

	OK       int
	Degraded int
	Error    int
	Total    int

	Fraction string
	Selected bool
}

// ItemView is one service row; Kind distinguishes a link ("service") from a
// locally runnable app ("app").
type ItemView struct {
	Key      string
	Kind     string
	Name     string
	Subtitle string
	URL      string
	Run      string
	Icon     string
	Tags     []string
	Health   *HealthView
}

// HealthView is the per-service status block: dot + label + latency.
type HealthView struct {
	Key     string
	Status  string // up / degraded / down / pending (CSS class)
	Label   string // ONLINE / DEGRADED / OFFLINE / CHECKING
	Latency string // "3 ms" or "—"
	Title   string // tooltip: full detail
}

// SummaryView feeds the masthead stats: SERVICES x/y, HOSTS, ALERTS.
type SummaryView struct {
	OK     int
	Total  int
	Hosts  int
	Alerts int
}

// CardColumn is the middle column: the sub-groups of one root group as a
// stack of cards (same shape as the sidebar entries). Only the selected
// root's column shows at a time; when the selected group has no sub-groups
// the column collapses away.
type CardColumn struct {
	Root  string
	Cards []SidebarView
}

type viewData struct {
	Title     string
	Summary   SummaryView
	AllTags   []string
	Styles    []domain.Style
	StylesCSS template.CSS

	// Sidebar lists the root groups; CardCols stacks the sub-group cards per
	// root group; Panels carries every group's render (only the selected one
	// is shown, the browser handles selection).
	Sidebar  []SidebarView
	CardCols []CardColumn
	Panels   []GroupView

	DefaultStyle string
	CanEdit      bool
	// CanOpenFolder shows the "open data folder" button: it acts on the
	// dashboard host, so only a viewer on the host itself (localhost) sees it.
	CanOpenFolder bool
	// ConfigError surfaces a broken config: the file on disk failed to parse
	// or validate — the app keeps serving the last good config and shows
	// this as a page banner.
	ConfigError string
}

// index re-reads the config (so a browser refresh picks up edits), syncs
// the healthcheck workers with it, and renders the whole page.
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	s.syncHealth(cfg)
	data := buildView(cfg, s.cfg, s.eng, s.styles)
	data.CanEdit = editAllowed(cfg, r)
	data.CanOpenFolder = isLocal(r)
	if err := s.cfg.LoadError(); err != nil {
		data.ConfigError = fmt.Sprintf("%s has an error: %v — serving the last good version", s.cfg.Path(), err)
	}
	s.render(w, "page.tmpl", data)
}

// groups serves the left column as an htmx fragment so the subtree counts
// stay live; the browser re-applies its selection after each swap.
func (s *Server) groups(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	s.syncHealth(cfg)
	s.render(w, "groups-list.tmpl", buildView(cfg, s.cfg, s.eng, s.styles).Sidebar)
}

// summary serves the masthead stats as an htmx fragment.
func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	s.syncHealth(cfg)
	s.render(w, "summary.tmpl", buildView(cfg, s.cfg, s.eng, s.styles).Summary)
}

// healthStatus serves a single service row's status block as an htmx
// fragment; each block re-requests itself on a timer.
func (s *Server) healthStatus(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	_, child, ok := s.cfg.Get().FindService(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.syncHealth(s.cfg.Get())
	threshold := domain.DefaultDegradedAfter
	private := false
	if svc, isService := child.(*domain.Service); isService {
		private = svc.Private
		if svc.Health != nil {
			if d, err := svc.Health.DegradedD(); err == nil {
				threshold = d
			}
		}
	}
	s.render(w, "components/health-status.tmpl", healthView(s.eng, key, threshold, private))
}

// syncHealth diffs the running healthcheck workers against the current
// config. It uses the server's own context — workers must outlive requests.
func (s *Server) syncHealth(cfg *domain.Config) {
	targets := make([]health.Target, 0)
	cfg.Walk(func(path string, g *domain.Group) {
		for _, ch := range g.Items {
			svc, isService := ch.(*domain.Service)
			if !isService || svc.Health == nil || svc.Health.Disabled {
				continue
			}
			interval, err := svc.Health.IntervalD()
			if err != nil {
				continue // validated at load; cannot happen with a good config
			}
			targets = append(targets, health.Target{
				Key:      domain.ChildPath(path, svc.Title),
				BaseURL:  svc.URL,
				Path:     svc.Health.Path,
				Interval: interval,
				Private:  svc.Private,
			})
		}
	})
	s.eng.SetTargets(s.ctx, targets)
}

// buildView walks the config tree into the two-column layout: the sidebar
// of root groups (with subtree counts), a renderable panel per group (its
// direct rows + sub-group tiles, subtree counts, host label from its own
// direct children), and the masthead stats.
func buildView(cfg *domain.Config, mgr *config.Manager, eng *health.Engine, styleMgr *styles.Manager) viewData {
	var data viewData
	data.Title = cfg.Title
	data.AllTags = cfg.AllTags()
	data.Styles = styleMgr.All()
	var blocks []string
	for _, st := range data.Styles {
		blocks = append(blocks, st.CSS())
	}
	data.StylesCSS = template.CSS(strings.Join(blocks, "\n"))
	data.DefaultStyle = resolveDefaultStyle(cfg, data.Styles)

	var visit func(path string, g *domain.Group) GroupView
	visit = func(path string, g *domain.Group) GroupView {
		data.Summary.Hosts++
		gv := GroupView{
			Path:     path,
			Name:     g.Title,
			Subtitle: g.Subtitle,
			Total:    len(g.Items),
		}
		for _, ch := range g.Items {
			leaf := ch.Core()
			v := ItemView{
				Key:      domain.ChildPath(path, leaf.Title),
				Name:     leaf.Title,
				Subtitle: leaf.Subtitle,
				URL:      leaf.URL,
				Icon:     mgr.ResolveIcon(leaf.Icon),
				Tags:     leaf.Tags,
				Kind:     "service",
			}
			if svc, isService := ch.(*domain.Service); isService {
				if svc.Health != nil && !svc.Health.Disabled {
					threshold, err := svc.Health.DegradedD()
					if err != nil {
						threshold = domain.DefaultDegradedAfter // validated at load
					}
					v.Health = healthView(eng, v.Key, threshold, svc.Private)
					switch v.Health.Status {
					case "up":
						gv.OK++
					case "degraded":
						gv.Degraded++
						data.Summary.Alerts++
					case "down":
						gv.Error++
						data.Summary.Alerts++
					}
				}
			} else if app, isApp := ch.(*domain.App); isApp {
				v.Kind = "app"
				v.Run = app.Run
			}
			gv.Items = append(gv.Items, v)
		}
		for i := range g.Groups {
			sub := g.Groups[i]
			subPath := domain.ChildPath(path, sub.Title)
			// recursion builds the sub-group section AND returns its subtree
			// counts
			subTree := visit(subPath, &sub)
			gv.OK += subTree.OK
			gv.Error += subTree.Error
			gv.Total += subTree.Total
			gv.SubGroups = append(gv.SubGroups, subTree)
		}
		gv.Fraction = itoa(gv.OK) + "/" + itoa(gv.Total)
		// a group holding ONLY sub-groups is a pure container: no composite
		// panel — the middle column's cards are its entire presentation.
		// (An empty group — no items, no sub-groups — keeps its panel so it
		// can show "no services yet".)
		if len(gv.Items) > 0 || len(gv.SubGroups) == 0 {
			data.Panels = append(data.Panels, gv)
		}
		return gv
	}

	for i := range cfg.Groups {
		gv := visit(cfg.Groups[i].Title, &cfg.Groups[i])
		// server-side default selection: the first root group; the browser's
		// remembered pick overrides on load
		for j := range data.Panels {
			if data.Panels[j].Path == gv.Path {
				data.Panels[j].Selected = i == 0
				break
			}
		}
		data.Summary.OK += gv.OK
		data.Summary.Total += gv.Total
		// middle column: the root's sub-groups as a stack of cards
		if len(gv.SubGroups) > 0 {
			col := CardColumn{Root: gv.Path}
			for _, sub := range gv.SubGroups {
				col.Cards = append(col.Cards, SidebarView{
					Path: sub.Path, Name: sub.Name, OK: sub.OK, Error: sub.Error, Total: sub.Total,
				})
			}
			data.CardCols = append(data.CardCols, col)
		}
		// sidebar entry per root group
		data.Sidebar = append(data.Sidebar, SidebarView{
			Path:     gv.Path,
			Name:     gv.Name,
			OK:       gv.OK,
			Error:    gv.Error,
			Total:    gv.Total,
			Selected: i == 0,
		})
	}
	return data
}

// resolveDefaultStyle picks the style selected on first visit: the config's
// defaultStyle (folder name) when it matches a loaded style, falling back to
// the first style folder — or nothing when no styles exist at all.
func resolveDefaultStyle(cfg *domain.Config, styles []domain.Style) string {
	if cfg.DefaultStyle != "" {
		want := strings.ToLower(cfg.DefaultStyle)
		for _, st := range styles {
			if st.ID == want {
				return want
			}
		}
	}
	if len(styles) > 0 {
		return styles[0].ID
	}
	return ""
}

func itoa(n int) string { return strconv.Itoa(n) }

// healthView classifies the latest probe: fast answers ONLINE, slow answers
// DEGRADED, failures OFFLINE. Private targets get a hint in the tooltip that
// TLS verification is dismissed for them (self-signed cert).
func healthView(eng *health.Engine, key string, degradedAfter time.Duration, private bool) *HealthView {
	view := &HealthView{Key: key, Status: "pending", Label: "CHECKING", Latency: "—", Title: "checking…"}
	res, ok := eng.Result(key)
	if !ok || res.Status == health.StatusPending {
		return view
	}
	if private {
		defer func() { view.Title += " · self-signed cert accepted" }()
	}
	checkedAt := res.Checked.Format("15:04:05")
	switch res.Status {
	case health.StatusUp:
		if res.Latency >= degradedAfter {
			view.Status, view.Label = "degraded", "DEGRADED"
		} else {
			view.Status, view.Label = "up", "ONLINE"
		}
		view.Latency = res.Latency.Round(time.Millisecond).String()
		view.Title = view.Label + " · " + view.Latency + " · " + checkedAt
	case health.StatusDown:
		view.Status, view.Label = "down", "OFFLINE"
		if res.Err != "" {
			view.Title = "OFFLINE · " + res.Err + " · " + checkedAt
		} else {
			view.Title = "OFFLINE · " + checkedAt
		}
	}
	return view
}
