# Wayshard build entry point.
# Official release artifacts are produced by GitHub Actions, not this Makefile.

GO        ?= go
GOFLAGS   ?=
BINDIR    ?= bin
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE      ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   := -s -w -X github.com/Wayshard/wayshard/internal/version.Version=$(VERSION) -X github.com/Wayshard/wayshard/internal/version.Commit=$(COMMIT) -X github.com/Wayshard/wayshard/internal/version.Date=$(DATE)

.PHONY: all help fmt vet test test-race build build-server build-cli build-fake-acp build-cross tidy ci web desktop android clean

all: test build

help:
	@echo "Targets: fmt vet test build build-server build-cli build-cross web desktop android release-scripts-test ci clean"

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

test:
	$(GO) test $(GOFLAGS) ./...

test-race:
	$(GO) test $(GOFLAGS) -race ./...

build: build-server build-cli build-fake-acp

build-server:
	mkdir -p $(BINDIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-server ./cmd/wayshard-server

build-cli:
	mkdir -p $(BINDIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard ./cmd/wayshard

build-fake-acp:
	mkdir -p $(BINDIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-fake-acp ./cmd/wayshard-fake-acp

# Cross-compilation of Go binaries (CGO-free). Desktop/Android packaging is CI-only.
# Artifact names used for GitHub Releases are applied by scripts/release/package-go.sh.
build-cross:
	mkdir -p $(BINDIR)
	GOOS=linux   GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-server-linux-amd64 ./cmd/wayshard-server
	GOOS=linux   GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-server-linux-arm64 ./cmd/wayshard-server
	GOOS=darwin  GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-server-darwin-amd64 ./cmd/wayshard-server
	GOOS=darwin  GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-server-darwin-arm64 ./cmd/wayshard-server
	GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-server-windows-amd64.exe ./cmd/wayshard-server
	GOOS=linux   GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-linux-amd64 ./cmd/wayshard
	GOOS=linux   GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-linux-arm64 ./cmd/wayshard
	GOOS=darwin  GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-darwin-amd64 ./cmd/wayshard
	GOOS=darwin  GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-darwin-arm64 ./cmd/wayshard
	GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-windows-amd64.exe ./cmd/wayshard

ci: fmt vet test build

web:
	cd clients && bun install --frozen-lockfile && bun run --cwd web build

desktop:
	cd clients && bun install --frozen-lockfile && bun run --cwd web build
	cd clients/desktop && bunx tauri build

# Official Android APKs are signed in GitHub Actions (environment: release).
# Local unsigned/debug iteration:
#   cd clients && bun install --frozen-lockfile && bun run --cwd web build
#   cd clients/desktop && bunx tauri android init && bunx tauri android build --apk
android:
	cd clients && bun install --frozen-lockfile && bun run --cwd web build
	cd clients/desktop && bunx tauri android init --ci
	cd clients/desktop && bunx tauri android build --apk

release-scripts-test:
	chmod +x scripts/release/*.sh scripts/release/*.py
	python3 scripts/release/checksums.py --self-test
	bash scripts/release/checksums_test.sh
	bash scripts/release/android_patch_test.sh
	bash scripts/release/android_jks_test.sh
	bash scripts/release/package_go_test.sh
	bash scripts/release/windows_pfx_test.sh
	bash scripts/release/minisign_test.sh
	bash scripts/release/release_policy_test.sh

clean:
	rm -rf $(BINDIR) coverage.out
