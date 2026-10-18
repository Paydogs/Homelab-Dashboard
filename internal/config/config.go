// Package config loads and validates the dashboard YAML config and
// re-reads it from disk when the file changes, without a rebuild or
// restart. The tree schema types live in the domain package.
package config

import (
	"bytes"
	"fmt"
	"io/fs"
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

// Path is the absolute location of the YAML file (for the web editor).
func (m *Manager) Path() string { return m.path }

// RunService returns the child behind a path key if it is a runnable App.
func (m *Manager) RunService(key string) (domain.Child, bool) {
	cfg := m.Get()
	m.mu.Lock()
	defer m.mu.Unlock()
	_, child, ok := cfg.FindService(key)
	return child, ok && domain.IsRunnable(child)
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

// Parse validates config content in memory without touching the file on
// disk — used by the web editor to preview changes before saving.
func Parse(data []byte) (*domain.Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
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

func parseFile(path string) (*domain.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Save validates new config content and, only when it is good, atomically
// replaces the YAML file on disk. The in-memory config is left alone: the
// next render's Get() sees the new file metadata and reloads.
func (m *Manager) Save(data []byte) error {
	if _, err := Parse(data); err != nil {
		return fmt.Errorf("config rejected, file unchanged: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tmp := m.path + ".tmp"
	mode := fs.FileMode(0o644)
	if st, err := os.Stat(m.path); err == nil {
		mode = st.Mode()
	}
	if err := os.WriteFile(tmp, data, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, m.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replacing config file: %w", err)
	}
	return nil
}

// validate enforces the invariants the UI and health engine rely on:
// unique child paths across the whole tree, well-formed durations, and
// healthchecks only on link services.
func validate(c *domain.Config) error {
	if c.Title == "" {
		c.Title = "Homelab"
	}
	seen := map[string]bool{}
	var walk func(prefix string, g *domain.Group) error
	walk = func(prefix string, g *domain.Group) error {
		if g.Title == "" {
			return fmt.Errorf("every group needs a title")
		}
		path := g.Title
		if prefix != "" {
			path = prefix + "/" + g.Title
		}
		if len(g.Children) == 0 && len(g.Groups) == 0 {
			return fmt.Errorf("group %q has no items or sub-groups", g.Title)
		}
		for _, ch := range g.Children {
			key := domain.ChildPath(path, ch.Core().Title)
			if ch.Core().Title == "" {
				return fmt.Errorf("every item in group %q needs a title", g.Title)
			}
			if seen[key] {
				return fmt.Errorf("duplicate item %q (under %q): item titles must be unique across the tree", ch.Core().Title, path)
			}
			seen[key] = true

			// Services are validated for URL shape and healthcheck specs;
			// Apps (local runnable) carry neither by construction — the
			// YAML discriminator already rejected url+run mixtures.
			if svc, isService := ch.(*domain.Service); isService {
				if _, err := url.Parse(strings.TrimSpace(svc.URL)); err != nil {
					return fmt.Errorf("item %q: parsing url: %w", svc.Title, err)
				}
				if svc.Health != nil {
					if _, err := svc.Health.IntervalD(); err != nil {
						return fmt.Errorf("item %q: %v", svc.Title, err)
					}
					if _, err := svc.Health.DegradedD(); err != nil {
						return fmt.Errorf("item %q: %v", svc.Title, err)
					}
				}
			}
		}
		for i := range g.Groups {
			if err := walk(path, &g.Groups[i]); err != nil {
				return err
			}
		}
		return nil
	}
	for i := range c.Groups {
		if err := walk("", &c.Groups[i]); err != nil {
			return err
		}
	}
	return nil
}
