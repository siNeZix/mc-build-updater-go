.DEFAULT_GOAL := help

.PHONY: help dev dev-server dev-client build build-server build-client start start-server start-client stop test test-server test-client fmt fmt-server fmt-client

COMPOSE := docker compose -f apps/file-hosting/compose.yaml
CLIENT_VERSION ?= dev
CLIENT_LDFLAGS := -X main.version=$(CLIENT_VERSION)

help:
	@echo "Development: make dev-server | dev-client | dev"
	@echo "Production:  make build-server | build-client | build"
	@echo "Run:         make start-server | start-client | start | stop"
	@echo "Quality:     make test-server | test-client | test | fmt"

# Native development. Air is fetched and cached by Go; no global install needed.
dev-server:
	cd apps/file-hosting && go run github.com/air-verse/air@latest -c .air.toml

dev-client:
	cd apps/mc-build-updater && go run github.com/air-verse/air@latest -c .air.toml

dev:
	$(MAKE) -j2 dev-server dev-client

# file-hosting production artifact is its Docker image.
build-server:
	$(COMPOSE) build

build-client:
	cd apps/mc-build-updater && go build -trimpath -ldflags "$(CLIENT_LDFLAGS)" -o build/mc-build-updater.exe ./cmd/mc-build-updater
	cd apps/mc-build-updater && go build -trimpath -o build/mc-bu-utils.exe ./cmd/mc-bu-utils

build: build-server build-client

start-server:
	$(COMPOSE) up --detach --build

start-client: build-client
	cd apps/mc-build-updater && ./build/mc-build-updater.exe

start: start-server start-client

stop:
	$(COMPOSE) down

test-server:
	cd apps/file-hosting && go test ./...

test-client:
	cd apps/mc-build-updater && go test ./...

test: test-server test-client

fmt-server:
	cd apps/file-hosting && go fmt ./...

fmt-client:
	cd apps/mc-build-updater && go fmt ./...

fmt: fmt-server fmt-client
