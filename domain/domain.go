// Package domain holds the dashboard's domain models: the config schema
// (shared by YAML parsing, validation and rendering) and the health state
// model. Other packages import these types instead of redefining their own
// shapes, so a config edit flows through parsing, healthchecks and the UI
// untouched.
package domain

import (
	"fmt"
	"sort"
	"time"
)

// Config is the root of the YAML config file — the source of truth.
type Config struct {
	Title  string  `yaml:"title"`
	Groups []Group `yaml:"groups"`
}

// Group is a named panel holding a set of services. Icon and Host are
// optional; Host is displayed in the panel header.
type Group struct {
	Name  string    `yaml:"name"`
	Icon  string    `yaml:"icon"`
	Host  string    `yaml:"host"`
	Items []Service `yaml:"items"`
}

// Service is one dashboard entry. It is either a link (URL) or a local
// runnable (executable on the dashboard's host machine) — exactly one of
// the two must be set.
type Service struct {
	Name   string       `yaml:"name"`
	URL    string       `yaml:"url"`
	Run    string       `yaml:"run"`
	Icon   string       `yaml:"icon"`
	Tags   []string     `yaml:"tags"`
	Health *HealthCheck `yaml:"health"`
}

// Key uniquely identifies a service across the whole page (group:name).
func (s *Service) Key(group string) string { return group + ":" + s.Name }

// IsRunnable reports whether the service spawns a local executable.
func (s *Service) IsRunnable() bool { return s.Run != "" }

// IsLink reports whether the service is an opened-in-browser link.
func (s *Service) IsLink() bool { return s.URL != "" }

// HasHealthCheck reports whether periodic probing is configured.
func (s *Service) HasHealthCheck() bool { return s.Health != nil }

// DefaultHealthInterval is used when a service declares a healthcheck
// without a custom interval.
const DefaultHealthInterval = 60 * time.Second

// DefaultDegradedAfter is the latency at or above which a service shows as
// DEGRADED instead of ONLINE.
const DefaultDegradedAfter = 250 * time.Millisecond

// HealthCheck configures a periodic HTTP healthcheck. It requires the
// service to be a URL item. A service that answers but slower than
// DegradedAfter classifies as DEGRADED rather than ONLINE.
type HealthCheck struct {
	Path          string `yaml:"path"`
	Interval      string `yaml:"interval"`
	DegradedAfter string `yaml:"degraded_after"`
}

// IntervalD returns the parsed probe interval (field Interval holds the raw
// YAML string; the D suffix avoids clashing with it).
func (h *HealthCheck) IntervalD() (time.Duration, error) {
	return parseDuration(h.Interval, DefaultHealthInterval, "health interval")
}

// DegradedD returns the parsed DEGRADED latency threshold.
func (h *HealthCheck) DegradedD() (time.Duration, error) {
	return parseDuration(h.DegradedAfter, DefaultDegradedAfter, "degraded_after")
}

func parseDuration(raw string, fallback time.Duration, what string) (time.Duration, error) {
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parsing %s %q: %w", what, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s %q must be positive", what, raw)
	}
	return d, nil
}

// AllTags returns the distinct tags across the whole config, sorted.
func (c *Config) AllTags() []string {
	set := map[string]bool{}
	for _, g := range c.Groups {
		for _, s := range g.Items {
			for _, t := range s.Tags {
				set[t] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// FindService resolves a key (group:name) back to its group and service.
func (c *Config) FindService(key string) (Group, Service, bool) {
	for _, g := range c.Groups {
		for _, s := range g.Items {
			if s.Key(g.Name) == key {
				return g, s, true
			}
		}
	}
	return Group{}, Service{}, false
}
