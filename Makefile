SHELL := /usr/bin/env bash

ROOT := $(CURDIR)
STATE_DIR ?= $(ROOT)/data
CONFIG_DIR ?= $(ROOT)
BIND_ADDR ?= 127.0.0.1:8091
FRONTEND_DIST ?= $(ROOT)/frontend/dist
RELEASE_DIR ?= $(ROOT)/.release
RELEASE_NAME ?= camera-appliance-$(VERSION)-$(COMMIT)
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

.PHONY: help dev dev-hot backend-dev frontend-dev build backend-build frontend-build release test status discover render-go2rtc compose-config clean

help:
	@echo "Targets:"
	@echo "  make dev             Build frontend + backend, then serve http://$(BIND_ADDR)"
	@echo "  make dev-hot         Run backend API and Vite UI with hot reload"
	@echo "  make test            Run Go tests"
	@echo "  make build           Build frontend and Go binary"
	@echo "  make release         Build a redacted release archive"
	@echo "  make status          Run local status command"
	@echo "  make discover        Run local discovery with short timeouts"
	@echo "  make render-go2rtc   Render local generated go2rtc config"
	@echo "  make compose-config  Validate Docker Compose file"
	@echo "  make clean           Remove local dev state and build output"

dev: build
	./bin/camera-appliance serve

dev-hot: backend-build
	@set -euo pipefail; \
	./bin/camera-appliance serve & \
	api_pid="$$!"; \
	trap 'kill "$$api_pid" 2>/dev/null || true' EXIT INT TERM; \
	cd frontend; npm run dev -- --host 127.0.0.1

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
