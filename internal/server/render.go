package server

import (
	"net/http"
	"strconv"
	"time"

	"dashboard/domain"
	"dashboard/internal/config"
	"dashboard/internal/health"
)

// GroupView is one panel in the grid: header info (icon, host) plus the
// aggregate state of its items.
type GroupView struct {
	Name     string
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

type ItemView struct {
	Key    string
	Name   string
	URL    string
	Run    string
	Icon   string
	Tags   []string
	Health *HealthView
}

// HealthView is the per-service status block: dot + label + latency.
type HealthView struct {
	Key     string
	Status  string // up / degraded / down / pending (CSS class)
	Label   string // ONLINE / DEGRADED / OFFLINE / …
	Latency string // "3 ms" or "—"
	Title   string // tooltip: full detail
}

// SummaryView feeds the header stats: SERVICES x/y, HOSTS, ALERTS.
type SummaryView struct {
	OK     int
	Total  int
	Hosts  int
	Alerts int
}

type viewData struct {
	Title   string
	Summary SummaryView
	AllTags []string
	Groups  []GroupView
}

// index re-reads the config (so a browser refresh picks up edits), syncs
// the healthcheck workers with it, and renders the whole page.
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	s.syncHealth(cfg)
	s.render(w, "page.tmpl", buildView(cfg, s.cfg, s.eng))
}

// summary serves the header stats as an htmx fragment.
func (s *Server) summary(w http.ResponseWriter, r *http.Request) { //nolint:dupl // fragment mirrors index
	cfg := s.cfg.Get()
	s.syncHealth(cfg)
	s.render(w, "summary.tmpl", buildView(cfg, s.cfg, s.eng).Summary)
}

// healthStatus serves a single service row's status block as an htmx
// fragment; each block re-requests itself on a timer.
func (s *Server) healthStatus(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	_, item, ok := s.cfg.Get().FindService(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.syncHealth(s.cfg.Get())
	if item.Health != nil {
		if threshold, err := item.Health.DegradedD(); err == nil {
			s.render(w, "components/health-status.tmpl", healthView(s.eng, key, threshold))
			return
		}
	}
	s.render(w, "components/health-status.tmpl", healthView(s.eng, key, domain.DefaultDegradedAfter))
}

// syncHealth diffs the running healthcheck workers against the current
// config. It uses the server's own context — workers must outlive requests.
func (s *Server) syncHealth(cfg *domain.Config) {
	targets := make([]health.Target, 0)
	for _, g := range cfg.Groups {
		for _, it := range g.Items {
			if it.Health == nil {
				continue
			}
			interval, err := it.Health.IntervalD()
			if err != nil {
				continue // validated at load; cannot happen with a good config
			}
			targets = append(targets, health.Target{
				Key:      it.Key(g.Name),
				BaseURL:  it.URL,
				Path:     it.Health.Path,
				Interval: interval,
			})
		}
	}
	s.eng.SetTargets(s.ctx, targets)
}

// buildView projects the config + live health results into the panel grid:
// per-service status blocks, per-group fractions and host labels, and the
// header stats. Strictly-speaking "up" counts only fast answers; slow ones
// are DEGRADED and join OFFLINE ones in the ALERTS figure.
func buildView(cfg *domain.Config, mgr *config.Manager, eng *health.Engine) viewData {
	data := viewData{Title: cfg.Title, AllTags: cfg.AllTags()}
	data.Summary.Hosts = len(cfg.Groups)
	for _, g := range cfg.Groups {
		gv := GroupView{
			Name:  g.Name,
			Icon:  mgr.ResolveIcon(g.Icon),
			Host:  g.Host,
			Total: len(g.Items),
		}
		checked := false
		for i := range g.Items {
			it := &g.Items[i]
			v := ItemView{
				Key:  it.Key(g.Name),
				Name: it.Name,
				URL:  it.URL,
				Run:  it.Run,
				Tags: it.Tags,
				Icon: mgr.ResolveIcon(it.Icon),
			}
			if it.Health != nil {
				threshold, err := it.Health.DegradedD()
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
	}
	return data
}

// healthView classifies the latest probe: fast answers ONLINE, slow answers
// DEGRADED, no service OFFLINE.
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
