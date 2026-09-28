BINARY := orchestrator
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG := github.com/stefenello/agent-orchestrator/internal/buildinfo
LDFLAGS := -ldflags="-s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).BuildTime=$(BUILD_TIME)"

# Cross-compile target for Piave (linux/amd64).
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

.PHONY: build test lint run migrate web-build dev clean

build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/orchestrator

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/orchestrator

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

run: build
	./$(BUILD_DIR)/$(BINARY) serve --config configs/config.example.yaml

migrate: build
	./$(BUILD_DIR)/$(BINARY) migrate --config configs/config.example.yaml

web-build:
	cd web && npm ci && npm run build
	rm -rf internal/web/dashboard
	mkdir -p internal/web/dashboard
	cp -R web/dist/. internal/web/dashboard/

dev: web-build build run

clean:
	rm -rf $(BUILD_DIR)
