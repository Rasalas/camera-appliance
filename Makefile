SHELL := /usr/bin/env bash

ROOT := $(CURDIR)
STATE_DIR ?= $(ROOT)/data
CONFIG_DIR ?= $(ROOT)
BIND_ADDR ?= 127.0.0.1:8091
FRONTEND_DIST ?= $(ROOT)/frontend/dist
RELEASE_DIR ?= $(ROOT)/.release
RELEASE_NAME ?= camera-appliance-$(VERSION)-$(COMMIT)
GO2RTC_IMAGE ?= alexxit/go2rtc:latest
GO2RTC_CONTAINER ?= camera-appliance-go2rtc-dev
GO2RTC_CONFIG ?= $(STATE_DIR)/generated/go2rtc.yaml
GO2RTC_VERSION ?= v1.9.14
GO2RTC_BIN ?= $(ROOT)/bin/go2rtc
GO2RTC_DEV_CONFIG ?= $(STATE_DIR)/generated/go2rtc-dev.yaml
GO2RTC_PID ?= $(STATE_DIR)/go2rtc-dev.pid
GO2RTC_LOG ?= $(STATE_DIR)/go2rtc-dev.log
SCAN_LIMIT ?= 254
TIMEOUT_MS ?= 500
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo local)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO_LDFLAGS := -X camera-appliance/camera-manager/internal/version.Version=$(VERSION) -X camera-appliance/camera-manager/internal/version.Commit=$(COMMIT) -X camera-appliance/camera-manager/internal/version.BuildTime=$(BUILD_TIME)

export CAMERA_APPLIANCE_STATE_DIR := $(STATE_DIR)
export CAMERA_APPLIANCE_CONFIG_DIR := $(CONFIG_DIR)
export CAMERA_APPLIANCE_BIND_ADDR := $(BIND_ADDR)
export CAMERA_APPLIANCE_FRONTEND_DIST := $(FRONTEND_DIST)
export CAMERA_APPLIANCE_SCAN_LIMIT := $(SCAN_LIMIT)
export CAMERA_APPLIANCE_TIMEOUT_MS := $(TIMEOUT_MS)
export CAMERA_APPLIANCE_DEV_GO2RTC_NATIVE := 1

.PHONY: help dev dev-hot dev-go2rtc stop-dev-go2rtc backend-dev frontend-dev build backend-build frontend-build release test status discover render-go2rtc compose-config clean

help:
	@echo "Targets:"
	@echo "  make dev             Build frontend + backend, then serve http://$(BIND_ADDR)"
	@echo "  make dev-hot         Run backend API and Vite UI with hot reload"
	@echo "  make dev-go2rtc      Start local go2rtc helper"
	@echo "  make stop-dev-go2rtc Stop local go2rtc helper"
	@echo "  make test            Run Go tests"
	@echo "  make build           Build frontend and Go binary"
	@echo "  make release         Build a redacted release archive"
	@echo "  make status          Run local status command"
	@echo "  make discover        Run local discovery with short timeouts"
	@echo "  make render-go2rtc   Render local generated go2rtc config"
	@echo "  make compose-config  Validate Docker Compose file"
	@echo "  make clean           Remove local dev state and build output"

dev: build dev-go2rtc
	./bin/camera-appliance serve

dev-hot: backend-build dev-go2rtc
	@set -euo pipefail; \
	./bin/camera-appliance serve & \
	api_pid="$$!"; \
	trap 'kill "$$api_pid" 2>/dev/null || true' EXIT INT TERM; \
	cd frontend; npm run dev -- --host 127.0.0.1

dev-go2rtc:
	@set -euo pipefail; \
	mkdir -p "$(dir $(GO2RTC_CONFIG))"; \
	if [[ ! -f "$(GO2RTC_CONFIG)" ]]; then printf 'streams: {}\n' > "$(GO2RTC_CONFIG)"; chmod 0600 "$(GO2RTC_CONFIG)"; fi; \
	if [[ "$$(uname -s)" == "Darwin" ]]; then \
	  if [[ ! -x "$(GO2RTC_BIN)" ]]; then \
	    echo "Installiere go2rtc $(GO2RTC_VERSION) nach $(GO2RTC_BIN)"; \
	    GOBIN="$(ROOT)/bin" go install github.com/AlexxIT/go2rtc@$(GO2RTC_VERSION); \
	  fi; \
	  if [[ -f "$(GO2RTC_PID)" ]]; then \
	    kill "$$(cat "$(GO2RTC_PID)")" >/dev/null 2>&1 || true; \
	    rm -f "$(GO2RTC_PID)"; \
	    sleep 1; \
	  fi; \
	  docker rm -f go2rtc "$(GO2RTC_CONTAINER)" >/dev/null 2>&1 || true; \
	  for _ in 1 2 3 4 5; do \
	    if ! lsof -nP -iTCP:1984 -sTCP:LISTEN >/dev/null 2>&1; then break; fi; \
	    sleep 1; \
	  done; \
	  if lsof -nP -iTCP:1984 -sTCP:LISTEN >/dev/null 2>&1; then \
	    echo "Port 1984 ist belegt. Bitte den anderen go2rtc-Prozess stoppen."; \
	    exit 1; \
	  fi; \
	  { printf 'api:\n  listen: ":1984"\nrtsp:\n  listen: ":18554"\nwebrtc:\n  listen: ":18555"\n\n'; sed 's/@host\.docker\.internal:/@127.0.0.1:/g' "$(GO2RTC_CONFIG)"; } > "$(GO2RTC_DEV_CONFIG)"; \
	  mkdir -p "$(STATE_DIR)"; \
	  nohup "$(GO2RTC_BIN)" -c "$(GO2RTC_DEV_CONFIG)" >"$(GO2RTC_LOG)" 2>&1 & \
	  echo "$$!" > "$(GO2RTC_PID)"; \
	  sleep 1; \
	  if curl -fsS http://127.0.0.1:1984/api/streams >/dev/null 2>&1; then \
	    echo "go2rtc nativ gestartet: http://127.0.0.1:1984"; \
	    exit 0; \
	  fi; \
	  echo "go2rtc konnte nicht gestartet werden. Log: $(GO2RTC_LOG)"; \
	  exit 1; \
	fi; \
	if ! docker info >/dev/null 2>&1; then \
	  echo "Docker ist nicht erreichbar. Bitte Docker starten, dann make dev erneut ausführen."; \
	  exit 1; \
	fi; \
	if curl -fsS http://127.0.0.1:1984/api/streams >/dev/null 2>&1; then \
	  echo "go2rtc ist erreichbar: http://127.0.0.1:1984"; \
	elif docker ps --format '{{.Names}}' | grep -qx "$(GO2RTC_CONTAINER)"; then \
	  echo "go2rtc läuft bereits: http://127.0.0.1:1984"; \
	else \
	  docker rm -f "$(GO2RTC_CONTAINER)" >/dev/null 2>&1 || true; \
	  docker run -d --name "$(GO2RTC_CONTAINER)" \
	    -p 127.0.0.1:1984:1984 \
	    -p 127.0.0.1:8554:8554 \
	    -v "$(abspath $(GO2RTC_CONFIG)):/config/go2rtc.yaml:ro" \
	    "$(GO2RTC_IMAGE)" >/dev/null; \
	  echo "go2rtc gestartet: http://127.0.0.1:1984"; \
	fi

stop-dev-go2rtc:
	@if [[ -f "$(GO2RTC_PID)" ]]; then kill "$$(cat "$(GO2RTC_PID)")" >/dev/null 2>&1 || true; rm -f "$(GO2RTC_PID)"; fi
	@docker rm -f "$(GO2RTC_CONTAINER)" >/dev/null 2>&1 || true

backend-dev: backend-build
	./bin/camera-appliance serve

frontend-dev:
	cd frontend && npm run dev -- --host 127.0.0.1

build: frontend-build backend-build

backend-build:
	cd camera-manager && go build -ldflags "$(GO_LDFLAGS)" -o ../bin/camera-appliance ./cmd/camera-appliance

frontend-build:
	cd frontend && npm install && npm run build

release: build
	@set -euo pipefail; \
	export COPYFILE_DISABLE=1; \
	rm -rf "$(RELEASE_DIR)/$(RELEASE_NAME)"; \
	mkdir -p "$(RELEASE_DIR)/$(RELEASE_NAME)"; \
	rsync -a --delete \
	  --exclude ".git" \
	  --exclude ".private" \
	  --exclude ".release" \
	  --exclude "data" \
	  --exclude "node_modules" \
	  --exclude ".DS_Store" \
	  --exclude "._*" \
	  --exclude ".env" \
	  --exclude "local.env" \
	  --exclude "secrets.env" \
	  ./ "$(RELEASE_DIR)/$(RELEASE_NAME)/"; \
	printf '{\n  "version": "%s",\n  "commit": "%s",\n  "build_time": "%s"\n}\n' "$(VERSION)" "$(COMMIT)" "$(BUILD_TIME)" > "$(RELEASE_DIR)/$(RELEASE_NAME)/manifest.json"; \
	tar -czf "$(RELEASE_DIR)/$(RELEASE_NAME).tar.gz" -C "$(RELEASE_DIR)" "$(RELEASE_NAME)"; \
	cp "$(RELEASE_DIR)/$(RELEASE_NAME).tar.gz" "$(RELEASE_DIR)/camera-appliance-latest.tar.gz"; \
	echo "$(RELEASE_DIR)/$(RELEASE_NAME).tar.gz"

test:
	cd camera-manager && go test ./...

status: backend-build
	./bin/camera-appliance status

discover: backend-build
	./bin/camera-appliance discover

render-go2rtc: backend-build
	./bin/camera-appliance render-go2rtc

compose-config:
	docker compose config

clean:
	rm -rf data frontend/dist bin/camera-appliance
