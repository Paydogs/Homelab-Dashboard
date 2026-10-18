// Package config loads and validates the dashboard YAML config and
// re-reads it from disk when the file changes, without a rebuild or restart.
// The schema types themselves live in the domain package.
package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dashboard/domain"
	"gopkg.in/yaml.v3"
)

// Manager owns the YAML file: it parses it, validates it, watches the file
// metadata and re-reads it on demand. Callers call Get() on every render; a
// browser refresh therefore picks up config edits with no rebuild, no
// restart, and no build step.
type Manager struct {
	path     string
	resPath  string
	logger   *log.Logger
	mu       sync.Mutex
	cfg      *domain.Config
	lastMod  time.Time
	lastSize int64
}

// NewManager loads the config once and fails fast on a bad file so an
// operator gets a useful error immediately, with line numbers from the
// YAML parser.
func NewManager(path string, logger *log.Logger) (*Manager, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving config path: %w", err)
	}
	m := &Manager{path: abs, resPath: filepath.Join(filepath.Dir(abs), "resources"), logger: logger}
	if err := m.reloadLocked(); err != nil {
		return nil, err
	}
	return m, nil
}

// Get returns the current config, re-reading the file first if its metadata
// changed. A failed reload keeps serving the last good config.
func (m *Manager) Get() *domain.Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, err := os.Stat(m.path)
	if err == nil && (st.ModTime() != m.lastMod || st.Size() != m.lastSize) {
		if err := m.reloadLocked(); err != nil {
			m.logger.Printf("config reload failed: %v (keeping previous config)", err)
		}
	}
	return m.cfg
}

// ResourcesDir is the folder where URL icons are cached locally.
func (m *Manager) ResourcesDir() string { return m.resPath }

// RunService returns the service behind a key if that service is a local
// runnable.
func (m *Manager) RunService(key string) (domain.Service, bool) {
	cfg := m.Get()
	m.mu.Lock()
	defer m.mu.Unlock()
	_, svc, ok := cfg.FindService(key)
	return svc, ok && svc.IsRunnable()
}

// reloadLocked parses and validates the file and caches the result. Called
// with the mutex held both at startup and on change. It also kicks off the
// URL icon download; a failed download never fails the reload.
func (m *Manager) reloadLocked() error {
	cfg, err := parseFile(m.path)
	if err != nil {
		return err
	}
	st, statErr := os.Stat(m.path)
	if statErr == nil {
		m.lastMod, m.lastSize = st.ModTime(), st.Size()
	}
	m.cfg = cfg
	m.ensureIcons(cfg)
	return nil
}

func parseFile(path string) (*domain.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true) // fail on unknown YAML keys instead of silently ignoring typos
	cfg := &domain.Config{}
	if err := dec.Decode(cfg); err != nil {
		return nil, err
	}
	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validate enforces the invariants the UI and health engine rely on:
// unique keys, exactly one of url/run per service, well-formed durations,
// healthchecks only on link services.
func validate(c *domain.Config) error {
	if c.Title == "" {
		c.Title = "Homelab"
	}
	seen := map[string]bool{}
	for _, g := range c.Groups {
		if g.Name == "" {
			return fmt.Errorf("every group needs a name")
		}
		if len(g.Items) == 0 {
			return fmt.Errorf("group %q has no items", g.Name)
		}
		for i := range g.Items {
			svc := &g.Items[i]
			key := svc.Key(g.Name)
			if seen[key] {
				return fmt.Errorf("duplicate item %q (group %q): items need unique names per group", svc.Name, g.Name)
			}
			seen[key] = true
			if svc.Name == "" {
				return fmt.Errorf("every item in group %q needs a name", g.Name)
			}
			if svc.IsLink() == svc.IsRunnable() {
				return fmt.Errorf("item %q (group %q): set exactly one of `url` or `run`", svc.Name, g.Name)
			}
			if svc.URL != "" {
				if _, err := url.Parse(strings.TrimSpace(svc.URL)); err != nil {
					return fmt.Errorf("item %q (group %q): parsing url: %w", svc.Name, g.Name, err)
				}
			}
			if svc.HasHealthCheck() {
				if svc.URL == "" {
					return fmt.Errorf("item %q (group %q): healthchecks need a `url` item", svc.Name, g.Name)
				}
				if _, err := svc.Health.IntervalD(); err != nil {
					return fmt.Errorf("item %q (group %q): %v", svc.Name, g.Name, err)
				}
				if _, err := svc.Health.DegradedD(); err != nil {
					return fmt.Errorf("item %q (group %q): %v", svc.Name, g.Name, err)
				}
			}
		}
	}
	return nil
}
