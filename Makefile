# Wayshard build entry point.
# Official release artifacts are produced by GitHub Actions, not this Makefile.

GO        ?= go
GOFLAGS   ?=
# Release binaries are CGO-free so Linux artifacts are statically linked and run
# on any distro (musl/Alpine, minimal containers) without glibc coupling. The
# server uses the pure-Go modernc.org/sqlite, so nothing requires cgo.
GOBUILD   := CGO_ENABLED=0 $(GO) build
BINDIR    ?= bin
HOSTOS    ?= $(shell $(GO) env GOOS)
HOSTARCH  ?= $(shell $(GO) env GOARCH)
EXE       ?= $(if $(filter windows,$(HOSTOS)),.exe,)
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE      ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   := -s -w -X github.com/Wayshard/wayshard/internal/version.Version=$(VERSION) -X github.com/Wayshard/wayshard/internal/version.Commit=$(COMMIT) -X github.com/Wayshard/wayshard/internal/version.Date=$(DATE)

.PHONY: all help fmt vet test test-race build build-server build-cli build-fake-acp build-tui build-tui-versioned build-cross build-all tidy ci web desktop android ui-render-smoke clean

all: test build

help:
	@echo "Targets: fmt vet test build build-server build-cli build-tui build-tui-versioned build-cross build-all web desktop android ui-render-smoke release-scripts-test ci clean"

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
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-server$(EXE) ./cmd/wayshard-server

build-cli:
	mkdir -p $(BINDIR)
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard$(EXE) ./cmd/wayshard

# Build the interactive companion under its canonical runtime name so that
# bin/wayshard finds bin/wayshard-tui directly (no rename, no override).
build-tui:
	mkdir -p $(BINDIR)
	cd clients && bun install --frozen-lockfile
	cd clients/tui && TUI_OUTFILE=$(CURDIR)/$(BINDIR)/wayshard-tui$(EXE) bun run build.ts

# Optional versioned copy for archival; it never replaces the runtime layout.
build-tui-versioned: build-tui
	cp -a $(BINDIR)/wayshard-tui$(EXE) $(BINDIR)/wayshard-tui-$(HOSTOS)-$(HOSTARCH)$(EXE)

build-fake-acp:
	mkdir -p $(BINDIR)
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-fake-acp$(EXE) ./cmd/wayshard-fake-acp

# Cross-compilation of Go binaries (CGO-free). Desktop/Android packaging is CI-only.
# Artifact names used for GitHub Releases are applied by scripts/release/package-go.sh.
build-cross:
	mkdir -p $(BINDIR)
	GOOS=linux   GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-server-linux-amd64 ./cmd/wayshard-server
	GOOS=linux   GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-server-linux-arm64 ./cmd/wayshard-server
	GOOS=darwin  GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-server-darwin-amd64 ./cmd/wayshard-server
	GOOS=darwin  GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-server-darwin-arm64 ./cmd/wayshard-server
	GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-server-windows-amd64.exe ./cmd/wayshard-server
	GOOS=linux   GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-linux-amd64 ./cmd/wayshard
	GOOS=linux   GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-linux-arm64 ./cmd/wayshard
	GOOS=darwin  GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-darwin-amd64 ./cmd/wayshard
	GOOS=darwin  GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-darwin-arm64 ./cmd/wayshard
	GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINDIR)/wayshard-windows-amd64.exe ./cmd/wayshard

ci: fmt vet test build

# Compile the Tauri Android-target Rust path. This type-checks the
# #[cfg(target_os = "android")] command bodies that host cargo test/check cannot
# see, so a cross-module or Android-only error fails in CI instead of at release.
android-check:
	mkdir -p clients/web/dist
	[ -f clients/web/dist/index.html ] || printf '<!doctype html><title>Wayshard</title>' > clients/web/dist/index.html
	cargo check --manifest-path clients/desktop/src-tauri/Cargo.toml --target aarch64-linux-android

# Build everything for the host platform (Go server/CLI + interactive TUI).
build-all: build build-tui

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
	python3 scripts/release/android-keystore-verify.py --self-test
	bash scripts/release/checksums_test.sh
	bash scripts/release/android_patch_test.sh
	bash scripts/release/android_keystore_patch_test.sh
	bash scripts/release/android_keystore_verify_test.sh
	bash scripts/release/android_jks_test.sh
	bash scripts/release/android_version_properties_test.sh
	bash scripts/release/icon_policy_test.sh
	bash scripts/release/windows_installer_icon_test.sh
	bash scripts/release/linux_static_test.sh
	bash scripts/release/desktop_macos_checksums_test.sh
	bash scripts/release/package_go_test.sh
	bash scripts/release/package_cli_tui_test.sh
	bash scripts/release/windows_pfx_test.sh
	bash scripts/release/minisign_test.sh
	bash scripts/release/appimage_fix_diricon_test.sh
	bash scripts/release/release_policy_test.sh
	bash scripts/release/set_tauri_version_test.sh

# Rendered smoke test for the production graphical client (builds the Web
# client, then drives the installed Chrome over CDP). Skips if no Chrome.
ui-render-smoke:
	cd clients && bun install --frozen-lockfile
	cd clients/web && bun run build
	node scripts/ci/ui_render_smoke.mjs

clean:
	rm -rf $(BINDIR) coverage.out
