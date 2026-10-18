# Homelab Dashboard

A self-hostable dashboard (start page) for home use: organizes VMs and services across many hosts,
opens them fast, and monitors their health. Technical direction follows projects like
[glance](https://github.com/glanceapp/glance), [dashboard/homepage](https://github.com/gethomepage/homepage),
dashy and mafl — but leaner, Go-only, and shaped for this household.

## Core principles

- **Single Go binary.** One executable serves everything: HTTP server, pages, and API. No Node, no
  TypeScript, no JS build toolchain. Frontend is Go `html/template` + plain CSS + a small amount of
  vanilla JS (no framework), embedded with `go:embed`.
- **YAML config file is the source of truth.** The dashboard layout, services, links, health-check
  settings, etc. all live in one mounted YAML file. No database.
- **Fast and lightweight.** Quick startup, quick page loads, small Docker image. Avoid dependencies;
  prefer the Go standard library (net/http, html/template) over web frameworks.
- **Docker-first deployment**, runnable on Debian or a local Windows machine (Docker is already
  installed there).

## Scope by iteration

- **v1 (current):** Static YAML config → categorized pages/groups of services with links and
  healthchecks (HTTP status/latency, periodic). Web UI is read-only.
- **v2:** Proxmox integration — VM/container status (running/stopped, CPU/mem at a glance) from the
  Proxmox API. Also more widget types (Docker, network, etc.) as needed.
- **v3:** Web UI editing that writes back to the YAML file (config stays the source of truth).
  Not required now — the user edits the YAML in an editor.

## Planned layout

```
/
├── CLAUDE.md
├── go.mod
├── cmd/dashboard/       # entry point (main package, flag/env for config path & listen addr)
├── internal/
│   ├── config/          # YAML schema, parse + validate
│   ├── server/          # routing, handlers
│   ├── health/          # healthcheck engine (periodic, cached results)
│   └── (proxmox/        # v2)
└── web/
    ├── static/          # css/js, served and embedded
    └── templates/       # html/template pages
```

## Config (v1 sketch)

Single YAML file, path given by flag/env (default `config.yml`). Structure roughly:

```yaml
title: Homelab
groups:                        # a page can have many groups
  - name: Media
    items:
      - name: Jellyfin
        url: http://media.lan:8096
        icon: jellyfin         # optional; built-in icon name or raw URL
        health:                # optional; omit = no healthcheck
          path: /health
          interval: 60s
```

Keep the schema minimal and validated at startup — **fail fast with a useful error** (file + line)
on bad config.

## Commands

- `go run ./cmd/dashboard` — run locally for development
- `go build ./...` / `go vet ./...` / `gofmt -w .` — before declaring work done
- `go test ./...` — tests exist for config parsing/validation and healthcheck state
- `docker build -t dashboard .` and `docker run -v <host-config>:/config -p 8080:8080 dashboard`
  — the only supported container deployment; config is bind-mounted, never baked in

## Conventions

- Go only. No `npm`/`node_modules`, no JavaScript framework, no CSS framework. Hand-written assets
  in `web/static` only.
- No database, ever, unless the user asks. State (v3 edits) eventually goes back to the YAML file.
- All templates/static assets must be embedded via `go:embed` (the binary alone must serve the UI).
- Docker image: multi-stage build, static binary (`CGO_ENABLED=0`), minimal final base (scratch or
  alpine), listens on one port (default 8080), reads config from `/config`.
- Windows note: keep everything cross-platform; avoid shell-specific plumbing in code. If a small
  script is genuinely needed, write it cross-platform or duplicate for both shells.
- Errors from remote services (healthchecks, later Proxmox) must degrade gracefully in the UI — an
  unreachable service shows as unhealthy, it never crashes or blocks page rendering.