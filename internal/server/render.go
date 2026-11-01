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

// LevelColumn is one drill-down column (miller-column layout): the
// contents of a single group — its sub-groups as cards and its leaf rows.
// The root groups live in the fixed sidebar (column 1); every group's
// column is rendered here, and the browser keeps visible the chain of
// columns from the selection's root down to it. Column widths divide the
// frame: each at most a third, quarter with a fourth column, and so on.
type LevelColumn struct {
	Path     string
	Name     string
	Fraction string
	Cards    []SidebarView
	Items    []ItemView
	Selected bool // server-side default: the first root's column
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

type viewData struct {
	Title     string
	Summary   SummaryView
	AllTags   []string
	Styles    []domain.Style
	StylesCSS template.CSS

	// Sidebar lists the root groups (column 1); Levels carries every
	// group's drill-down column in PRE-ORDER (parents before their
	// children — columns must read left to right along the drill path),
	// and the browser shows the selection's chain.
	Sidebar []SidebarView
	Levels  []*LevelColumn

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

// leafView projects one child leaf into a row view; the string is its
// health status class ("" when the leaf carries no status — apps, disabled
// or absent healthchecks).
func leafView(mgr *config.Manager, eng *health.Engine, path string, ch domain.Child) (ItemView, string) {
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
	var status string
	if svc, isService := ch.(*domain.Service); isService {
		if svc.Health != nil && !svc.Health.Disabled {
			threshold, err := svc.Health.DegradedD()
			if err != nil {
				threshold = domain.DefaultDegradedAfter // validated at load
			}
			v.Health = healthView(eng, v.Key, threshold, svc.Private)
			status = v.Health.Status
		}
	} else if app, isApp := ch.(*domain.App); isApp {
		v.Kind = "app"
		v.Run = app.Run
	}
	return v, status
}

// buildView walks the config tree into the miller-column layout: the
// sidebar of root groups (with subtree counts), a drill-down column per
// group (its sub-group cards + direct leaf rows), and the masthead stats.
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

	// subtree counts accumulate up the tree
	type counts struct{ ok, degraded, err, total int }
	var visit func(path string, g *domain.Group) counts
	visit = func(path string, g *domain.Group) counts {
		data.Summary.Hosts++
		// pre-order: the column lands in the DOM before its descendants',
		// so drill-down reads left to right; it is a pointer because the
		// cards (with subtree counts) are filled in by the recursion below
		col := &LevelColumn{Path: path, Name: g.Title}
		data.Levels = append(data.Levels, col)
		var c counts
		c.total = len(g.Items)
		for _, ch := range g.Items {
			v, status := leafView(mgr, eng, path, ch)
			switch status {
			case "up":
				c.ok++
			case "degraded":
				c.degraded++
				data.Summary.Alerts++
			case "down":
				c.err++
				data.Summary.Alerts++
			}
			col.Items = append(col.Items, v)
		}
		for i := range g.Groups {
			sub := g.Groups[i]
			subPath := domain.ChildPath(path, sub.Title)
			// recursion builds the sub-group's column AND returns its
			// subtree counts
			sc := visit(subPath, &sub)
			c.ok += sc.ok
			c.err += sc.err
			c.total += sc.total
			col.Cards = append(col.Cards, SidebarView{
				Path: subPath, Name: sub.Title, OK: sc.ok, Error: sc.err, Total: sc.total,
			})
		}
		col.Fraction = itoa(c.ok) + "/" + itoa(c.total)
		return c
	}

	for i := range cfg.Groups {
		c := visit(cfg.Groups[i].Title, &cfg.Groups[i])
		data.Summary.OK += c.ok
		data.Summary.Total += c.total
		// server-side default selection: the first root group; the browser's
		// remembered pick overrides on load
		sel := i == 0
		if sel {
			for j := range data.Levels {
				if data.Levels[j].Path == cfg.Groups[i].Title {
					data.Levels[j].Selected = true
					break
				}
			}
		}
		// sidebar entry per root group
		data.Sidebar = append(data.Sidebar, SidebarView{
			Path:     cfg.Groups[i].Title,
			Name:     cfg.Groups[i].Title,
			OK:       c.ok,
			Error:    c.err,
			Total:    c.total,
			Selected: sel,
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
