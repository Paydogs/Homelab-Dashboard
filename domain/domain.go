// Package domain holds the dashboard's domain models: the content tree
// (Config → Groups → children, see nodes.go), the health state model, and
// the healthcheck spec. Other packages import these types instead of
// redefining their own shapes, so a config edit flows through parsing,
// healthchecks and the UI untouched.
package domain

import (
	"fmt"
	"time"
)

// DefaultHealthInterval is used when a child declares a healthcheck
// without a custom interval.
const DefaultHealthInterval = 60 * time.Second

// DefaultDegradedAfter is the latency at or above which a service shows as
// DEGRADED instead of ONLINE.
const DefaultDegradedAfter = 250 * time.Millisecond

// HealthCheck configures a periodic HTTP healthcheck. It is currently
// meaningful only for Service children (URL links). A service that answers
// but slower than DegradedAfter classifies as DEGRADED rather than ONLINE.
type HealthCheck struct {
	Path          string `yaml:"path,omitempty"`
	Interval      string `yaml:"interval,omitempty"`
	DegradedAfter string `yaml:"degraded_after,omitempty"`
}

// IntervalD returns the parsed probe interval (the D suffix avoids clashing
// with the raw Interval field).
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
