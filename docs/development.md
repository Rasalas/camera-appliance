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

## Hot Reload

```bash
make dev-hot
```

This runs the Go backend on `127.0.0.1:8091` and Vite with API proxying.

## Backend Tests

```bash
make test
```

## Build

```bash
make build
```

## Useful Commands

```bash
make status
make discover
make render-go2rtc
make compose-config
make clean
```
