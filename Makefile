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
	@echo "  make release         Build a redacted release archive; suggests VERSION with tagger"
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
	  # Nur den Dev-Container entfernen – niemals den Produktiv-Container
	  # "go2rtc" aus compose.yaml.
	  docker rm -f "$(GO2RTC_CONTAINER)" >/dev/null 2>&1 || true; \
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

release:
	@set -euo pipefail; \
	release_version="$$(VERSION_ORIGIN="$(origin VERSION)" VERSION_VALUE="$(VERSION)" "$(ROOT)/scripts/release-version")"; \
	release_name="camera-appliance-$${release_version}-$(COMMIT)"; \
	"$(MAKE)" build VERSION="$$release_version" COMMIT="$(COMMIT)" BUILD_TIME="$(BUILD_TIME)"; \
	export COPYFILE_DISABLE=1; \
	rm -rf "$(RELEASE_DIR)/$${release_name}"; \
	mkdir -p "$(RELEASE_DIR)/$${release_name}"; \
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
	  --exclude "private" \
	  --exclude ".claude" \
	  --exclude ".github" \
	  --exclude "CLAUDE.md" \
	  --exclude "codex-handoff.md" \
	  --exclude "docs" \
	  ./ "$(RELEASE_DIR)/$${release_name}/"; \
	printf '{\n  "version": "%s",\n  "commit": "%s",\n  "build_time": "%s"\n}\n' "$$release_version" "$(COMMIT)" "$(BUILD_TIME)" > "$(RELEASE_DIR)/$${release_name}/manifest.json"; \
	printf 'VERSION=%s\nCOMMIT=%s\nBUILD_TIME=%s\n' "$$release_version" "$(COMMIT)" "$(BUILD_TIME)" > "$(RELEASE_DIR)/$${release_name}/release.env"; \
	tar -czf "$(RELEASE_DIR)/$${release_name}.tar.gz" -C "$(RELEASE_DIR)" "$$release_name"; \
	cp "$(RELEASE_DIR)/$${release_name}.tar.gz" "$(RELEASE_DIR)/camera-appliance-latest.tar.gz"; \
	echo "$(RELEASE_DIR)/$${release_name}.tar.gz"

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

SERVE_PORT ?= 8099
NAS_HOST ?=
NAS_PORT ?= 8091

# Build a release locally and serve it on the LAN so an appliance can update
# over HTTP (requires CAMERA_APPLIANCE_ALLOW_INSECURE_UPDATE=1 on the device).
serve-update:
	@set -euo pipefail; \
	"$(MAKE)" -s release; \
	digest="$$(shasum -a 256 "$(RELEASE_DIR)/camera-appliance-latest.tar.gz" | awk '{print $$1}')"; \
	ip="$$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || hostname -I 2>/dev/null | awk '{print $$1}' || true)"; \
	echo; \
	echo "Release wird bereitgestellt:"; \
	echo "  URL:    http://$${ip:-<LAN-IP>}:$(SERVE_PORT)/camera-appliance-latest.tar.gz"; \
	echo "  Digest: sha256:$$digest"; \
	echo; \
	echo "Auf dem Gerät einmalig nötig:"; \
	echo "  Environment=CAMERA_APPLIANCE_ALLOW_INSECURE_UPDATE=1"; \
	echo "  (danach systemctl daemon-reload && systemctl restart camera-appliance)"; \
	echo "Strg+C beendet den Server."; \
	python3 -m http.server $(SERVE_PORT) --bind 0.0.0.0 --directory "$(RELEASE_DIR)"

# One-shot: build, serve temporarily and trigger the update on a NAS appliance
# via its admin API. Usage:
#   make update-nas NAS_HOST=192.168.178.11 NAS_PASSWORD=...
update-nas:
	@set -euo pipefail; \
	if [[ -z "$(NAS_HOST)" ]]; then echo "Usage: make update-nas NAS_HOST=<ip> [NAS_PORT=8091] [NAS_PASSWORD=...] [VERSION=x.y.z]"; exit 1; fi; \
	pass="$$([[ -n '$(NAS_PASSWORD)' ]] && printf '%s' '$(NAS_PASSWORD)' || read -rsp 'Admin-Passwort für $(NAS_HOST): ' pw && printf '%s' "$$pw")"; \
	"$(MAKE)" -s release; \
	digest="$$(shasum -a 256 "$(RELEASE_DIR)/camera-appliance-latest.tar.gz" | awk '{print $$1}')"; \
	ip="$$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || true)"; \
	if [[ -z "$$ip" ]]; then echo "Keine LAN-IP gefunden." >&2; exit 1; fi; \
	tmp="$$(mktemp -d)"; trap 'rm -rf "$$tmp"' EXIT; \
	python3 -m http.server $(SERVE_PORT) --bind 0.0.0.0 --directory "$(RELEASE_DIR)" >/dev/null 2>&1 & serve_pid=$$!; \
	trap 'kill $$serve_pid 2>/dev/null; rm -rf "$$tmp"' EXIT; \
	status_url="http://$(NAS_HOST):$(NAS_PORT)/api/health"; \
	curl -fsS --max-time 5 "$$status_url" >/dev/null || { echo "Gerät nicht erreichbar: $$status_url" >&2; exit 1; }; \
	code=$$(curl -s -o "$$tmp/login.json" -w '%{http_code}' -c "$$tmp/cookies" -X POST "http://$(NAS_HOST):$(NAS_PORT)/api/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"'"$$pass"'","remember":false}'); \
	if [[ "$$code" != "200" ]]; then echo "Login fehlgeschlagen ($$code)." >&2; exit 1; fi; \
	code=$$(curl -s -o "$$tmp/update.json" -w '%{http_code}' -b "$$tmp/cookies" -X POST "http://$(NAS_HOST):$(NAS_PORT)/api/system/update" -H 'Content-Type: application/json' -d '{"url":"http://'"$$ip"':$(SERVE_PORT)/camera-appliance-latest.tar.gz","digest":"sha256:'$$digest'"}'); \
	if [[ "$$code" != "202" ]]; then echo "Update-Aufruf fehlgeschlagen ($$code):" >&2; cat "$$tmp/update.json" >&2; exit 1; fi; \
	echo "Update gestartet. Digest: sha256:$$digest"; \
	echo "Das Gerät lädt jetzt von diesem Rechner und startet anschließend neu."
