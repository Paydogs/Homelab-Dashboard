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
  line below), on the right: the **edit config button** (opens an overlay with the raw config.yml
  in a textarea; Save validates server-side and atomic-replaces the file — invalid YAML is
  rejected with the parser's line numbers and the file is untouched; localhost-only), the **style
  picker** (accent theme select), and stats columns (`SERVICES up/total`, `HOSTS = number of
  groups`, `ALERTS = degraded + offline count`). Stats are an htmx fragment (`GET /summary`,
  refreshed on a timer).
- **Filter row:** search box (matches **name, tags, url/ip**) + tag chips (`all` + every distinct
  tag). Search/tag filtering covers all panels at once; empty panels hide while filtering.
- **Panel grid:** one panel per group (2-col responsive grid). Panel header: group icon box,
  name, optional host line, big `up/total` fraction, and a `HOST ONLINE / HOST DEGRADED` label
  (DEGRADED when any service answers slower than its threshold).
- **Service rows** inside each panel: icon, name (link) + url line, tag chips; right side: health
  status block (dot + ONLINE/DEGRADED/OFFLINE label + latency, self-refreshing htmx fragment,
  `GET /health-status/{key}`) or the `run…` confirm button for local executables.
- **Styles (themes) are files, never code:** the only source of styles is `<config-dir>/resources/
  styles/` — **one subfolder per style**, holding `style.yml` plus any files it references.
  `style.yml` has `name`, optional `font:` / `titleFont:` (paths relative to the style folder —
  font files you drop in there; a missing file or a failed load defaults to **Arial**), optional
  `fontSize:` (sets the root font size — the rem-based UI scales with it) and `titleFontSize:`
  (page title only), and `colors` mapping CSS tokens → values (values may be any CSS expression,
  e.g. `color-mix(...)`).
  Currently shipped: `modern/` (indigo ink, `titleFont: DIGITAL TIMELESS DEMO.ttf` — demo
  license, personal use) and `retro/` (warm plum). Font files are served at `/fonts/<id>/<path>`
  (sanitized: style id + no traversal); a style without a font ref inherits the mono token stack;
  `main.css`'s `:root` tokens are the base fallback when the folder is empty.

## Architecture

- `domain/` — the domain models live here and are instantiated by the other packages: the content
  **tree** (Config → Groups → children), the child kinds (Service, App), the health state models
  (Status, ProbeResult) and the HealthCheck spec. config, health and server import these types
  instead of redefining shapes. Presentation projections (ItemView/GroupView/…) stay in the
  server package.

## Domain model (the data layer)

- **Group (parent node):** `title`, optional `subtitle`, `icon`, `host`, plus `[groups]` and
  `[items]`. Groups hold other groups (parents) and items (children) — a recursive tree.
- **Child (end leaf) = Service | App**, both sharing a `Leaf` base: `title`, optional
  `subtitle`, `url` (service/link), `icon`, `[tags]`, and a runtime `status` (set from health
  probes at render time, not stored in YAML).
  - **Service:** a `url` opened in a browser tab; may carry a `health` block
    (`path`, `interval`, `degraded_after`).
  - **App:** a local executable (`run`) launched by the dashboard host behind a confirm step;
    no healthcheck.
- YAML: `items` entries are discriminated by the presence of `url` (Service) or `run` (App);
  both at once, neither, or a healthcheck on a run item are validation errors. Child keys are
  their tree path: `Servers/Server 1/Gitea` (used for health targets, the run endpoint and the
  htmx fragments — routes use `{key...}` wildcards).

## Scope by iteration

- **v1 (current):** Static YAML config → categorized pages/groups of services with links, **tags**
  (displayed on cards, filter/search over them) and healthchecks (HTTP status/latency, periodic).
  Search/filter across services (Compass-style, cheap on static config). Web UI is read-only.
- **v2:** Proxmox integration — VM/container status (running/stopped, CPU/mem at a glance) from the
  Proxmox API. Also more widget types (Docker, network, etc.) as needed.
- **v3:** Web UI editing that writes back to the YAML file (config stays the source of truth).
  A first slice already ships: the masthead edit button round-trips the raw YAML
  (`GET/POST /config-source`, validate-then-atomic-write via `Manager.Save`). Still open:
  structured editing (forms per group/child) instead of the raw textarea.

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

## Config (sketch)

Single YAML file, path given by flag/env (default `config.yml`). Structure roughly:

```yaml
title: Homelab
groups:                          # a recursive tree of panels
  - title: Servers               # group: title, optional subtitle/icon/host
    groups:
      - title: Server 1
        host: 192.168.1.30       # optional; shown in the panel header
        items:
          - title: Gitea         # a Service (leaf): a link opened in the browser
            url: http://192.168.1.30:3000
            icon: …              # optional; URL icons download once to resources/icons/
            tags: [dev]          # shown on the row, filter/search over tags, url, name
            health:              # optional; omit = no healthcheck
              path: /            # omitted path = probe the root
              interval: 60s      # default 60s
              degraded_after: 250ms  # slower answers show DEGRADED instead of ONLINE
          - title: Steam         # an App (leaf): launched on the dashboard host
            run: C:\Program Files (x86)\Steam\steam.exe
```

Leaf kinds (validated: exactly one of `url`/`run`; no healthchecks on run items):

- **Service** — plain link, opened in a new tab. Cover remote services on other VMs.
- **App** — the server launches a locally installed executable (Windows path). It does not
  need to run instantly: clicking shows a **confirm step** ("Run Steam?") before the server
  spawns the process. Only meaningful when the dashboard runs on that machine; refused elsewhere.

Keep the schema minimal and validated at startup — **fail fast with a useful error** (file + line)
on bad config.

## Commands

- `docker compose up -d --build` — **run the app locally and on the VM**. Locally, the
  gitignored `docker-compose.override.yml` points `build:` at this working tree (uncommitted
  changes included); without it (the VM) the build comes straight from the GitHub URL in
  `docker-compose.yml`. Never build/launch a stray exe to "run locally".
- `go build ./...` / `go vet ./...` / `gofmt -w .` — before declaring work done
- `go test ./...` — tests exist for config parsing/validation, healthcheck state, styles and
  the edit-access checks

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