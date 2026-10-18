// Package health runs periodic HTTP healthchecks for dashboard items and
// keeps the latest result of each probe in memory. A failing or unreachable
// service is never fatal: it simply shows as unhealthy on the page.
package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"dashboard/domain"
)

// The health state model (Status, ProbeResult) lives in the domain package;
// re-exported here as aliases so callers read naturally.
type (
	Status = domain.Status
	Result = domain.ProbeResult
)

const (
	StatusUp      = domain.StatusUp
	StatusDown    = domain.StatusDown
	StatusPending = domain.StatusPending
)

// Target configures one healthcheck: Key matches a dashboard service key.
type Target struct {
	Key      string
	BaseURL  string
	Path     string
	Interval time.Duration
}

// Engine owns one goroutine per target, diff-replacing them as the config
// reloads.
type Engine struct {
	client  *http.Client
	mu      sync.Mutex
	workers map[string]*worker
	results map[string]Result
}

type worker struct {
	target Target
	done   chan struct{}
}

// New creates an idle engine.
func New() *Engine {
	return &Engine{
		client:  &http.Client{Timeout: 5 * time.Second},
		workers: map[string]*worker{},
		results: map[string]Result{},
	}
}

// SetTargets syncs the running workers with the config's healthchecks:
// targets no longer present (or with changed settings) are stopped, new
// ones started, and all of them probe once up front.
func (e *Engine) SetTargets(ctx context.Context, targets []Target) {
	e.mu.Lock()
	defer e.mu.Unlock()

	want := make(map[string]Target, len(targets))
	for _, t := range targets {
		want[t.Key] = t
	}
	for key, w := range e.workers {
		live, ok := want[key]
		if !ok || live != w.target {
			close(w.done)
			delete(e.workers, key)
		}
	}
	for key, t := range want {
		if _, running := e.workers[key]; running {
			continue
		}
		w := &worker{target: t, done: make(chan struct{})}
		e.workers[key] = w
		go e.run(ctx, w)
	}
}

func (e *Engine) run(ctx context.Context, w *worker) {
	store := func(r Result) {
		e.mu.Lock()
		e.results[w.target.Key] = r
		e.mu.Unlock()
	}

	store(e.probe(w.target))
	ticker := time.NewTicker(w.target.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			store(e.probe(w.target))
		}
	}
}

func (e *Engine) probe(t Target) Result {
	start := time.Now()
	resp, err := e.client.Get(probeURL(t)) //nolint:gosec // healthcheck URLs are authored in the local config file
	latency := time.Since(start)
	if err != nil {
		return Result{Status: StatusDown, Latency: latency, Checked: time.Now(), Err: err.Error()}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return Result{Status: StatusUp, Latency: latency, Checked: time.Now()}
	}
	return Result{Status: StatusDown, Latency: latency, Checked: time.Now(),
		Err: fmt.Sprintf("status %s", resp.Status)}
}

// probeURL joins the base URL and healthcheck path.
func probeURL(t Target) string {
	base := strings.TrimSuffix(t.BaseURL, "/")
	if t.Path == "" {
		return base
	}
	if !strings.HasPrefix(t.Path, "/") {
		return base + "/" + t.Path
	}
	return base + t.Path
}

// Result returns the latest result for a key; a key nothing probes yet is
// pending.
func (e *Engine) Result(key string) (Result, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.results[key]
	return r, ok
}
