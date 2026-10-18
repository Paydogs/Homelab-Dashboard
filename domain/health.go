package domain

import "time"

// Status is the outcome of the latest health probe. DEGRADED is not a probe
// outcome — it is a presentation classification of an Up result that
// answered slowly (see HealthCheck.DegradedThreshold).
type Status string

const (
	StatusUp      Status = "up"
	StatusDown    Status = "down"
	StatusPending Status = "pending"
)

// ProbeResult is the cached outcome of a target's latest probe.
type ProbeResult struct {
	Status  Status
	Latency time.Duration
	Checked time.Time
	Err     string
}
