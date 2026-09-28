BINARY := orchestrator
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

.PHONY: build test lint run migrate web-build dev clean

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/orchestrator

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
