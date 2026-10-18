package server

import (
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

// GroupView is one panel in the grid: header info (icon, host) plus the
// aggregate state of its direct children (nested groups render as their own
// panels).
type GroupView struct {
	Name     string
	Subtitle string
	Icon     string
	Host     string
	Items    []ItemView
	OK       int // fast answers
	Degraded int
	Error    int
	Total    int

	// Fraction is the "4/5" header figure — services answering fast.
	Fraction  string
	HostLabel string // "HOST ONLINE" / "HOST DEGRADED" ("" when unhealthchecked)
	HostClass string // up / degraded / down for coloring
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
	Styles    []domain.Style // for the picker options
	StylesCSS template.CSS   // pre-rendered :root[data-style] blocks (operator-authored values)
	Groups    []GroupView

	// DefaultStyle is the resolved style id selected on first visit: the
	// config's defaultStyle if that folder exists, else the first folder.
	DefaultStyle string
}

// index re-reads the config (so a browser refresh picks up edits), syncs
// the healthcheck workers with it, and renders the whole page.
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	s.syncHealth(cfg)
	s.render(w, "page.tmpl", buildView(cfg, s.cfg, s.eng, s.styles))
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
	if svc, isService := child.(*domain.Service); isService && svc.Health != nil {
		if d, err := svc.Health.DegradedD(); err == nil {
			threshold = d
		}
	}
	s.render(w, "components/health-status.tmpl", healthView(s.eng, key, threshold))
}

// syncHealth diffs the running healthcheck workers against the current
// config. It uses the server's own context — workers must outlive requests.
func (s *Server) syncHealth(cfg *domain.Config) {
	targets := make([]health.Target, 0)
	cfg.Walk(func(path string, g *domain.Group) {
		for _, ch := range g.Children {
			svc, isService := ch.(*domain.Service)
			if !isService || svc.Health == nil {
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
			})
		}
	})
	s.eng.SetTargets(s.ctx, targets)
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

// buildView walks the config tree and projects it into the panel grid:
// per-service status blocks, per-group fractions and host labels, and the
// masthead stats. "Up" counts only fast answers; slow ones are DEGRADED and
// join OFFLINE ones in the ALERTS figure.
func buildView(cfg *domain.Config, mgr *config.Manager, eng *health.Engine, styles *styles.Manager) viewData {
	var data viewData
	data.Title = cfg.Title
	data.AllTags = cfg.AllTags()
	data.Styles = styles.All()
	var blocks []string
	for _, st := range data.Styles {
		blocks = append(blocks, st.CSS())
	}
	data.StylesCSS = template.CSS(strings.Join(blocks, "\n"))
	data.DefaultStyle = resolveDefaultStyle(cfg, data.Styles)
	data.Summary.Hosts = 0

	cfg.Walk(func(path string, g *domain.Group) {
		data.Summary.Hosts++
		gv := GroupView{
			Name:     g.Title,
			Subtitle: g.Subtitle,
			Icon:     mgr.ResolveIcon(g.Icon),
			Host:     g.Host,
			Total:    len(g.Children),
		}
		checked := false
		for _, ch := range g.Children {
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
				if svc.Health != nil {
					threshold, err := svc.Health.DegradedD()
					if err != nil {
						threshold = domain.DefaultDegradedAfter // validated at load
					}
					checked = true
					v.Health = healthView(eng, v.Key, threshold)
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
		gv.Fraction = strconv.Itoa(gv.OK) + "/" + strconv.Itoa(gv.Total)
		switch {
		case !checked:
			// group without healthchecks: no host state to speak of
		case gv.Degraded > 0:
			gv.HostLabel, gv.HostClass = "HOST DEGRADED", "degraded"
		default:
			gv.HostLabel, gv.HostClass = "HOST ONLINE", "up"
		}
		data.Summary.OK += gv.OK
		data.Summary.Total += gv.Total
		data.Groups = append(data.Groups, gv)
	})
	return data
}

// healthView classifies the latest probe: fast answers ONLINE, slow answers
// DEGRADED, failures OFFLINE.
func healthView(eng *health.Engine, key string, degradedAfter time.Duration) *HealthView {
	view := &HealthView{Key: key, Status: "pending", Label: "CHECKING", Latency: "—", Title: "checking…"}
	res, ok := eng.Result(key)
	if !ok || res.Status == health.StatusPending {
		return view
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
