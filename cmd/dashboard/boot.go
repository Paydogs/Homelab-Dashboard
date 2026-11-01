package main

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"sync/atomic"

	"dashboard/internal/config"
	"dashboard/internal/health"
	"dashboard/internal/server"
	"dashboard/internal/styles"
)

// buildServer assembles the live dashboard handler: seed first-boot files,
// load+validate the config, wire managers and routes. Any error means "not
// usable yet" — the caller shows it on the boot page instead of serving
// nothing.
func buildServer(dataDir string, seed func() error, logger *log.Logger) (http.Handler, error) {
	if err := seed(); err != nil {
		return nil, err
	}
	mgr, err := config.NewManager(dataDir, logger)
	if err != nil {
		return nil, err
	}
	styleMgr := styles.NewManager(mgr.ResourcesDir()+"/styles", logger)
	srv, err := server.New(mgr, health.New(), styleMgr, logger)
	if err != nil {
		return nil, err
	}
	return srv.Routes(), nil
}

// boot holds the pre-config state: the error the latest attempt produced,
// and the live handler once the config became usable (atomic swaps — the
// retry goroutine writes, every request reads).
type boot struct {
	status atomic.Pointer[string]
	live   atomic.Pointer[http.Handler]
}

func (b *boot) setStatus(msg string) { b.status.Store(&msg) }

func (b *boot) goLive(h http.Handler) { b.live.Store(&h) }

// root delegates to the dashboard once live; before that EVERY request gets
// the boot page naming the config problem — a broken config.yml shows what
// is wrong (with the parser's line numbers) instead of a dead port.
func (b *boot) root(w http.ResponseWriter, r *http.Request) {
	if h := b.live.Load(); h != nil {
		(*h).ServeHTTP(w, r)
		return
	}
	msg := "starting…"
	if p := b.status.Load(); p != nil {
		msg = *p
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, bootPage, html.EscapeString(msg))
}

// bootPage is deliberately self-contained (no /static/, no styles): those
// routes belong to the dashboard, which does not exist yet. Refresh retries.
const bootPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="5">
<title>Homelab — config problem</title>
<style>
  body { background: #2e222f; color: #e8edf2; font: 16px/1.5 ui-monospace, Consolas, monospace;
         display: flex; justify-content: center; padding: 12vh 1rem 1rem; margin: 0; }
  main { max-width: 640px; width: 100%%; }
  .eyebrow { color: #e83b3b; font-size: .75rem; letter-spacing: .2em; text-transform: uppercase; }
  h1 { font-size: 1.3rem; margin: .4rem 0 1rem; font-weight: 600; }
  pre { background: #3a2d3f; border: 1px solid #e83b3b; border-radius: 10px;
        padding: .9rem 1rem; font-size: .8rem; white-space: pre-wrap; word-break: break-word; }
  p.hint { color: #9babb2; font-size: .8rem; margin-top: 1rem; }
  code { color: #30e1b9; }
</style>
</head>
<body>
<main>
  <div class="eyebrow">Homelab dashboard</div>
  <h1>config.yml isn't usable yet</h1>
  <pre>%s</pre>
  <p class="hint">Fix the file (or make the data folder writable) — this page retries every
  5 seconds and turns into the dashboard as soon as the config is good again.</p>
</main>
</body>
</html>
`
