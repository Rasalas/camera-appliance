# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`camera-appliance` turns a Linux Mint laptop into a local, offline camera-viewing appliance. A Go manager discovers Tapo/RTSP cameras, stores stable device identities, binds devices to fixed slots (`cam1`–`cam5`), renders a go2rtc config, and serves a German-language kiosk viewer + admin UI on `127.0.0.1:8091`. go2rtc exposes stable stream aliases that the viewer consumes — camera DHCP IPs never reach the viewer.

Data flow: `Tapo cameras → discovery/state → go2rtc stable aliases → Vue viewer`.

## Commands

All commands run from the repo root via the Makefile, which exports `CAMERA_APPLIANCE_*` env vars pointing at local dev paths (state in `./data`, config in repo root, frontend at `./frontend/dist`).

```bash
make dev            # build frontend + backend, serve http://127.0.0.1:8091
make dev-hot        # Go backend on :8091 + Vite hot reload (Vite proxies /api → :8091)
make build          # frontend-build + backend-build
make test           # Go tests (cd camera-manager && go test ./...)
make render-go2rtc  # render local generated go2rtc config
make status         # run local status command
make discover       # run discovery with short timeouts
make compose-config # validate compose.yaml
make clean          # remove ./data, frontend/dist, bin/camera-appliance
make release        # redacted release tar.gz (excludes secrets/.env/data/.private)
```

Run a single Go test: `cd camera-manager && go test ./internal/app -run TestName`.

Frontend type-check happens in the build (`vue-tsc --noEmit && vite build`). There is no separate Go lint target; use `go vet ./...` from `camera-manager/` if needed.

## Architecture

### Go manager (`camera-manager/`)

Single binary built from `cmd/camera-appliance`. Module path is `camera-appliance/camera-manager`. Layered `internal/` packages:

- **`cli`** — Cobra root command wiring every subcommand (`serve`, `status`, `discover`, `assign`, `render-go2rtc`, `restart-go2rtc`, `restart-stack`, `relays`, `admin`, `reset-bindings`, `backup`, `restore`, `support-bundle`, `update`). The `serve` command starts the HTTP server and the watchdog goroutine.
- **`app`** — Core orchestration. `app.Open(ctx)` is the single entry point used by every CLI command and the API: it loads config, opens the SQLite store, loads/upserts slots, and returns an `*App` whose methods (`Status`, `Discover`, `Assign`, `RenderGo2RTC`, viewer/relay/watchdog logic) hold the business logic. The HTTP layer is a thin wrapper over these methods. This package is large and split across files by concern: `app.go`, `viewer.go`, `relays.go`, `watchdog.go`, `display.go`, `auth.go`, `support.go`, `paths.go`.
- **`web/api`** — `net/http` `ServeMux` (Go 1.22 method+pattern routing) under `/api/*`, plus a `/` static handler serving the built frontend. Auth middleware wraps all routes; handlers call `app` methods and serialize results.
- **`state`** — SQLite store (`modernc.org/sqlite`, pure Go) at `state.db`. Schema lives in `migrations/`. Holds devices, slot bindings, settings (a generic key/value table — many features like watchdog tuning and viewer performance mode persist here as dotted keys), events, scan runs, stream checks.
- **`discovery`** — Network scan across local subnets to find cameras; produces device fingerprints and probes RTSP streams.
- **`fingerprint`** — Computes the **stable device ID** from identity attributes, NOT IP. Priority: serial (`manufacturer|model|serial`) → ONVIF endpoint ref → MAC → random fallback. This is the core of "identity is never bound to IP."
- **`go2rtc`** — Renders `go2rtc.yaml` from slots + bindings + resolved credentials/endpoints into the generated config path.
- **`secrets`** — Resolves camera passwords from (in order) env var `TAPO_CAMERA_PASSWORD` → OS keyring → `local.env`. Per-camera passwords via `LoadCamera`. Never logged.
- **`redaction`** — Strips credentials from URLs/text. **All** CLI/API/UI output and error messages must pass through redaction (e.g. `redaction.Text(...)`) before display or persistence.
- **`auth`** — Local login with `admin` and `viewer` roles, PBKDF2-SHA256 password hashing, token sessions.
- **`system`** — Service status checks and Docker Compose restart helpers (`RestartGo2RTC`, `RestartStack`).
- **`backup`/`update`** — tar.gz backup/restore of state + generated config; update applies a release archive with backup, healthcheck, and auto-rollback.

### Frontend (`frontend/`)

Vue 3 + TypeScript + Vue Router, built with Vite. SPA served by the Go binary in production. Routes use **German paths** (`/uebersicht`, `/einrichtung`, `/system`, `/kamera/:id`) with legacy English redirects; `router.beforeEach` enforces `requiresAdmin` / `requiresViewer` via `/api/auth/status`. The viewer (`/`) is the kiosk camera grid; other pages are admin. API access goes through `src/api/client.ts`; shared types in `src/types/index.ts`.

### Runtime topology

Three containers via `compose.yaml`: `go2rtc` (stream aliases, RTSP `:8554`, web `:1984`), `camera-manager` (this app, host network, talks to Docker socket to restart go2rtc), and optional `agentdvr` (gated behind the `agentdvr` profile — not needed for normal operation).

## Conventions & constraints

- **Identity over IP**: never key cameras or viewer config on DHCP IP. Use the slot aliases (`cam1`–`cam5`) and the fingerprint device ID.
- **Redaction is mandatory**: any string that could contain a camera URL/credential must be redacted before it is printed, returned over the API, written to events, or stored.
- **User-facing strings are German**; internal code/identifiers/comments are English. Match this when editing CLI output or UI text.
- **Secrets never committed**: real credentials live in `/etc/camera-appliance/secrets.env` (or keyring/`local.env`), all git-ignored. Copy `.env.example` for local setup.
- **Config resolution** (`config.Load`) falls back from production paths (`/etc/camera-appliance`, `/var/lib/camera-appliance`, `/opt/camera-appliance/compose.yaml`) to repo-local equivalents when the env vars are unset and the prod paths are absent — this is what makes `make dev` work without root.
- **Settings as key/value**: feature flags and tunables are stored in the `state` settings table under dotted keys (e.g. `watchdog.enabled`, `viewer.performance.mode`) rather than as schema columns.
- **Out of scope**: no video recording/NVR in the current target (see `docs/decisions/012`). Don't add recording features without revisiting that decision.

## Decision records

Architectural and product decisions (German) are in `docs/decisions/` — read these before changing identity, go2rtc aliasing, secrets/redaction, auth, layouts, or update/recovery behavior. Index: `docs/decisions/README.md`.
