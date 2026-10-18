# Homelab Dashboard

A self-hostable dashboard (start page) for home use: organizes VMs and services across many hosts,
opens them fast, and monitors their health. Technical direction follows projects like
[glance](https://github.com/glanceapp/glance), [dashboard/homepage](https://github.com/gethomepage/homepage),
[compass](https://github.com/adinhodovic/compass), dashy and mafl
— but leaner, Go-only, and shaped for this household.

## Core principles

- **Single Go binary.** One executable serves everything: HTTP server, pages, and API. No Node, no
  TypeScript, no JS build toolchain. Frontend is server-rendered Go `html/template` with
  **HTMX + Alpine.js + plain hand-written CSS** (Compass-style, minus Tailwind/daisyUI, which would
  require a Node build step). The tiny vendored JS libs are embedded with `go:embed` and served
  locally — nothing load from a CDN.
- **YAML config file is the source of truth.** The dashboard layout, services, links, health-check
  settings, etc. all live in one mounted YAML file. No database.
- **No build step, ever.** A config change must never require rebuilding or restarting: a plain
  browser refresh picks up the YAML change (re-read/re-parse on request, cached briefly, with the
  template data rebuilt from the latest config).
- **Fast and lightweight.** Quick startup, quick page loads, small Docker image. Avoid dependencies;
  prefer the Go standard library (net/http, html/template) over web frameworks.
- **Docker-first deployment**, runnable on Debian or a local Windows machine (Docker is already
  installed there).

## UI design (reference: `Examples/*.pdf`, the "Control" mockup)

- **Masthead block:** `TITLE // CONTROL` eyebrow, big live clock (HH:MM + small seconds, date
  line below), the **style picker** (accent theme select on the right), and stats columns
  (`SERVICES up/total`, `HOSTS = number of groups`, `ALERTS = degraded + offline count`).
  Stats are an htmx fragment (`GET /summary`, refreshed on a timer).
- **Filter row:** search box (matches **name, tags, url/ip**) + tag chips (`all` + every distinct
  tag). Search/tag filtering covers all panels at once; empty panels hide while filtering.
- **Panel grid:** one panel per group (2-col responsive grid). Panel header: group icon box,
  name, optional host line, big `up/total` fraction, and a `HOST ONLINE / HOST DEGRADED` label
  (DEGRADED when any service answers slower than its threshold).
- **Service rows** inside each panel: icon, name (link) + url line, tag chips; right side: health
  status block (dot + ONLINE/DEGRADED/OFFLINE label + latency, self-refreshing htmx fragment,
  `GET /health-status/{key}`) or the `run…` confirm button for local executables.
- Colors: Resurrect 64 palette only; monospace type throughout.

## Architecture

- `domain/` — the domain models (Config, Group, Service, HealthCheck, Status, ProbeResult) live
  here, imported by config, health and server instead of each redefining shapes. Presentation
  projections (ItemView/GroupView/…) stay in the server package.

## Scope by iteration

- **v1 (current):** Static YAML config → categorized pages/groups of services with links, **tags**
  (displayed on cards, filter/search over them) and healthchecks (HTTP status/latency, periodic).
  Search/filter across services (Compass-style, cheap on static config). Web UI is read-only.
- **v2:** Proxmox integration — VM/container status (running/stopped, CPU/mem at a glance) from the
  Proxmox API. Also more widget types (Docker, network, etc.) as needed.
- **v3:** Web UI editing that writes back to the YAML file (config stays the source of truth).
  Not required now — the user edits the YAML in an editor.

No service auto-discovery: every entry is hand-configured in YAML.

## Planned layout

```
/
├── CLAUDE.md
├── domain/              # domain models: Config, Group, Service, HealthCheck, health state
├── go.mod
├── cmd/dashboard/       # entry point (main package, flag/env for config path & listen addr)
├── internal/
│   ├── config/          # YAML parse + validate + hot reload on refresh (schema types = domain)
│   ├── server/          # routing, handlers, view projections
│   ├── health/          # healthcheck engine (periodic, cached results; results = domain)
│   └── (proxmox/        # v2)
└── web/
    ├── static/          # css/js/vendor/icons, served and embedded
    └── templates/
        ├── page.tmpl            # shell: masthead, filter row, panel grid
        ├── summary.tmpl         # header stats fragment
        ├── groups-panel.tmpl    # one panel: header + service rows
        └── components/
            ├── card.tmpl              # service row (link / run / status block)
            └── health-status.tmpl     # dot + label + latency fragment
```

UI files live in the separate `web/` folder, structured **component by component**: each reusable
piece of UI (card, tag, search bar, group) is its own template file, composed into pages — keep
new UI as small component templates rather than monolithic page markup.

## Config (v1 sketch)

Single YAML file, path given by flag/env (default `config.yml`). Structure roughly:

```yaml
title: Homelab
groups:                        # a page can have many groups
  - name: Media
    items:
      - name: Jellyfin           # links open in a new browser tab
        url: http://media.lan:8096
        icon: jellyfin           # optional; built-in icon name or raw URL
                                 # URL icons are downloaded once to a local resources folder
        tags: [media, video]     # v1 feature: shown on the card, filter/search on tags
        health:                  # optional; omit = no healthcheck
          path: /health
          interval: 60s
      - name: Steam
        run: C:\Program Files (x86)\Steam\steam.exe   # local runnable (Windows)
```

Two item kinds:

- **`url` items** — plain links, opened in a new tab. Cover remote services on other VMs.
- **`run` items** — the server launches a locally installed executable (Windows path). It does not
  need to run instantly: clicking shows a **confirm step** ("Run Steam?") before the server spawns
  the process. Only meaningful when the dashboard runs on that machine; hidden or refused elsewhere.

Keep the schema minimal and validated at startup — **fail fast with a useful error** (file + line)
on bad config.

## Commands

- `go run ./cmd/dashboard` — run locally for development
- `go build ./...` / `go vet ./...` / `gofmt -w .` — before declaring work done
- `go test ./...` — tests exist for config parsing/validation and healthcheck state
- `docker compose up -d --build` — the supported container deployment (Dockerfile: multi-stage,
  `CGO_ENABLED=0`, `scratch` final image, runs as uid 1000). `./config.yml` is bind-mounted to
  `/config/config.yml` and URL icons cache in `./resources/` — both stay on the host, never baked
  into the image.

## Conventions

- Go only. No `npm`/`node_modules`, no SPA framework, no CSS framework. The only JavaScript is
  vendored minified htmx/alpine.js files copied into `web/static`; all other assets (CSS, icons) are
  hand-written.
- No database, ever, unless the user asks. State (v3 edits) eventually goes back to the YAML file.
- All templates/static assets must be embedded via `go:embed` (the binary alone must serve the UI).
- Docker image: multi-stage build, static binary (`CGO_ENABLED=0`), minimal final base (scratch or
  alpine), listens on one port (default 8080), reads config from `/config`.
- Windows note: keep everything cross-platform; avoid shell-specific plumbing in code. If a small
  script is genuinely needed, write it cross-platform or duplicate for both shells.
- **URL icons are never hot-linked.** On config load, an item whose `icon` is a URL is downloaded
  once into a local resources folder (default `<config-dir>/resources/icons/`, so it survives
  restarts and travels with the bind-mounted config when the path is relative); the dashboard then
  serves it locally. Skip the download if the file already exists; on a failed download, log it and
  fall back to a placeholder icon — never block or delay the page. Icons remain usable offline.
- **UI colors come from the Resurrect 64 palette** (lospec.com/palette-list/resurrect-64).
  `web/static/css/main.css` derives every theme token (bg, surface, border, text, accent, health
  up/down/pending) from palette hexes — when adding UI elements, pick new colors from the palette,
  not arbitrary hexes or defaults.
- **`run` items are a security boundary.** Only accept the launch endpoint from `localhost`
  (127.0.0.1), require the confirm step, and only spawn processes explicitly listed in the YAML;
  never accept arbitrary paths from a request.
- Errors from remote services (healthchecks, later Proxmox) must degrade gracefully in the UI — an
  unreachable service shows as unhealthy, it never crashes or blocks page rendering.