.DEFAULT_GOAL := help

.PHONY: help dev-c dev-s build build-c build-s start-c start-c-dev start-s start-s-dev deploy test test-c test-s fmt fmt-c fmt-s

CLIENT_VERSION ?= dev
CLIENT_LDFLAGS := -X main.version=$(CLIENT_VERSION)

help:
	@echo "Development: make dev-s | dev-c"
	@echo "Build:       make build-s | build-c | build"
	@echo "Run:         make start-s | start-c | start-s-dev | start-c-dev"
	@echo "Deploy:      make deploy"
	@echo "Quality:     make test-s | test-c | test | fmt-s | fmt-c | fmt"

deploy:
	powershell -NoProfile -ExecutionPolicy Bypass -File deploy/deploy.ps1

# Native development. Air is fetched and cached by Go; no global install needed.
dev-s:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/with-env.ps1 -EnvFile .env -WorkingDirectory apps/file-hosting -Command "go run github.com/air-verse/air@latest -c .air.toml"

dev-c:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/with-env.ps1 -EnvFile .env -WorkingDirectory apps/mc-build-updater -Command "go run github.com/air-verse/air@latest -c .air.toml"

# Native build artifacts are placed in the repository-root build directory.
build-s:
	powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path 'build/server' | Out-Null"
	cd apps/file-hosting && go build -trimpath -o ../../build/server/file-hosting.exe ./cmd/file-hosting

build-c:
	powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path 'build/client' | Out-Null"
	cd apps/mc-build-updater && go build -trimpath -ldflags "$(CLIENT_LDFLAGS)" -o ../../build/client/mc-build-updater.exe ./cmd/mc-build-updater
	cd apps/mc-build-updater && go build -trimpath -o ../../build/client/mc-bu-utils.exe ./cmd/mc-bu-utils

build: build-s build-c

start-s: build-s
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/with-env.ps1 -EnvFile .env -WorkingDirectory apps/file-hosting -Command "../../build/server/file-hosting.exe"

start-s-dev: start-s

start-c: build-c
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/with-env.ps1 -EnvFile .env -WorkingDirectory apps/mc-build-updater -Command "../../build/client/mc-build-updater.exe"

start-c-dev: build-c
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/with-env.ps1 -EnvFile .env -WorkingDirectory apps/mc-build-updater -Command "../../build/client/mc-build-updater.exe --dev"

test-s:
	cd apps/file-hosting && go test ./...

test-c:
	cd apps/mc-build-updater && go test ./...

test: test-s test-c

fmt-s:
	cd apps/file-hosting && go fmt ./...

fmt-c:
	cd apps/mc-build-updater && go fmt ./...

fmt: fmt-s fmt-c
