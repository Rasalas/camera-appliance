# Development

## Quick Start

```bash
make dev
```

Open `http://127.0.0.1:8091`.

The Makefile uses local development paths:

- State: `./data`
- Config: repository root
- Frontend build: `./frontend/dist`
- go2rtc config: `./data/generated/go2rtc.yaml`

`make dev` starts a local go2rtc helper before starting the manager. On macOS, go2rtc runs natively from `./bin/go2rtc` because Docker Desktop containers may not reach LAN cameras reliably. The native dev helper keeps the go2rtc API on `127.0.0.1:1984` and uses RTSP `:18554` internally to avoid stale Docker Desktop port forwards. On other platforms, the Makefile falls back to a Docker helper container.

## Hot Reload

```bash
make dev-hot
```

This runs the Go backend on `127.0.0.1:8091` and Vite with API proxying.

## Backend Tests

```bash
make test
```

This runs the Go and frontend tests. Use `go test -race ./...` in
`camera-manager` to check concurrent credential changes and the other Go tests
with the race detector.

The pull request workflow runs the race detector, Go vet, frontend tests and
the production frontend build. Viewer layout transformations live in
`frontend/src/pages/viewerMosaic.ts` and can be tested without a browser.

## Build

```bash
make build
```

## Useful Commands

```bash
make status
make dev-go2rtc
make stop-dev-go2rtc
make discover
make render-go2rtc
make compose-config
make clean
```
