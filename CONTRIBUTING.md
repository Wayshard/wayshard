# Contributing to Wayshard

Wayshard is a public MIT-licensed project. Official binaries are produced by GitHub Actions under the `Wayshard` organization, never from workstation uploads.

## Canonical documents

Read these before changing behavior:

- `AGENTS.md` — implementation rules and quality gates
- `SPEC.md` — finished-product requirements
- `DESIGN.md` — client/product UX
- `ARCHITECTURE.md` — technical architecture
- `MEMORY.md` — durable rationale
- `README.md` — human overview

Do not rewrite a canonical requirement merely to match incomplete code. If implementation evidence proves a canonical decision wrong, update the responsible document in the same change.

## Development

Prerequisites: Go 1.24+, bun, and (for Desktop) a Rust toolchain plus Tauri WebKit dependencies. Android builds need a JDK and Android SDK.

```sh
make test
make build
```

Normal pull-request CI must pass without production secrets, paid model calls, or installed third-party coding harnesses. Orchestration tests use the bundled fake ACP harness.

## Layout

- `cmd/` — Wayshard Server, CLI, and fake ACP harness
- `internal/` — Go control-plane modules
- `clients/` — Web, CLI/TUI, Desktop, Android, and the shared TypeScript SDK
- `.github/workflows/` — CI and official releases

## License

By contributing you agree that your work is licensed under the MIT License. Preserve third-party notices when touching imported OpenCode 2 client/TUI source.
