# Dependency security triage

Triage of the open GitHub Dependabot alerts on `main` before the `v0.2.0`
release line. Findings are grouped by dependency. For each group: owning
manifest/client, production-vs-dev classification, whether Wayshard actually
exercises the vulnerable functionality, the minimum safe version, and the
upgrade risk taken.

Method: `gh api repos/Wayshard/wayshard/dependabot/alerts?state=open`. Reachability
is decided from the real import/link graph (`go list -deps`, `go mod why`,
`cargo tree`), not from the advisory's package name alone.

## Summary

| Dependency | Alerts | Severity | Owning manifest | Class | Vulnerable code reachable? | Min safe version | Action |
|---|---|---|---|---|---|---|---|
| `golang.org/x/crypto` | 13 | 7 critical, 2 high, 4 medium | `go.mod` (Server + CLI, runtime) | production runtime | No — all advisories are in `x/crypto/ssh*`; only `argon2`/`blake2b` are linked | 0.52.0 | upgraded 0.45.0 → 0.52.0 |
| `vite` | 8 | 3 high, 5 medium/low | `clients/web/package.json` (Web build tool) | dev/build only | No — dev-server only; production serves static build output | 7.3.5 | upgraded 7.1.4 → 7.3.5 |
| `glib` | 1 | medium | `clients/desktop/src-tauri/Cargo.lock` (Tauri/gtk, Linux desktop) | production runtime (transitive) | No — Wayshard Rust does not use `glib`/`VariantStrIter` | 0.20.0 (requires gtk-rs 0.20) | accepted, documented (no forced Tauri/gtk stack upgrade) |

## `golang.org/x/crypto` (Server / CLI)

- Owning manifest: `go.mod`; used by `internal/crypto/hash.go` via
  `golang.org/x/crypto/argon2` (device-credential hashing) plus `blake2b`.
- Class: production runtime for Server and CLI.
- Reachability: `go mod why golang.org/x/crypto/ssh` and `go list -deps` show the
  SSH package family is **not imported or linked**. Every one of the 13
  advisories is in the SSH implementation: `ssh`/`ssh/agent`, `ssh/knownhosts`,
  `CertChecker`, FIDO/U2F `sk-*` keys, RSA/DSA host-key parsing, and the SSH
  AES-GCM packet decoder. None touches `argon2` or `blake2b`.
- Minimum safe version: **0.52.0** (all 13 advisories patched there).
- Action taken: upgraded `golang.org/x/crypto` 0.45.0 → 0.52.0. The minimum safe
  version requires **Go 1.25**, so the `go` directive moved `1.24.0 → 1.25.0`
  and `golang.org/x/sys` moved `0.38.0 → 0.45.0` (its required version).
- Upgrade risk: low. `argon2`/`blake2b` APIs are unchanged; only the toolchain
  floor rises. Rationale for acting despite unreachability: these are seven
  critical advisories on a production runtime module bound for a release line,
  and the fix is a minimal in-module version bump.

| Alert | Severity | Advisory | Area (not exercised) |
|---|---|---|---|
| #20 | critical | GHSA-f5wc-c3c7-36mc | ssh agent forwarded-key constraints |
| #19 | critical | GHSA-jppx-rxg9-jmrx | ssh agent keyring `ConfirmBeforeUse` |
| #18 | critical | GHSA-x527-x647-q7gg | ssh server `VerifiedPublicKeyCallback` |
| #17 | critical | GHSA-5cgq-3rg8-m6cv | ssh `@revoked` CA signature key |
| #16 | critical | GHSA-rm3j-f69w-wqmq | ssh channel >4 GiB write overflow |
| #15 | critical | GHSA-89gr-r52h-f8rx | ssh FIDO/U2F user-presence |
| #13 | critical | GHSA-vgwf-h737-ff37 | ssh global-request response flood |
| #14 | high | GHSA-w879-237q-wc7r | ssh RSA/DSA parameter DoS |
| #9 | high | GHSA-q4h4-gmj2-qvw2 | ssh AES-GCM packet decoder panic |
| #21 | medium | GHSA-9m57-25v3-79x9 | ssh ed25519 wire-byte cast panic |
| #12 | medium | GHSA-qpw4-5x99-6vjp | ssh rejected-channel memory leak |
| #11 | medium | GHSA-78mq-xcr3-xm33 | ssh `CertChecker` nil callback panic |
| #10 | medium | GHSA-45gg-vh54-h5m9 | ssh `PartialSuccessError` permissions |

## `vite` (Web build tool)

- Owning manifest: `clients/web/package.json` (devDependency, pinned).
- Class: **dev/build only**. The Vite dev server is used by `bun run dev`;
  production and release serve the static `vite build` output through the Go
  server (`internal/webembed`). CI runs `vite build`, never the dev server.
- Reachability: the advisories affect the **Vite dev server** (`server.fs.deny`
  bypasses, dev-server WebSocket file read, optimized-deps path traversal) and
  `launch-editor` UNC handling (Windows dev only). Not reachable in the shipped
  product.
- Minimum safe version: **7.3.5** (clears all eight; other patched versions are
  7.3.2/7.1.11/7.1.5, all ≤ 7.3.5).
- Action taken: upgraded 7.1.4 → 7.3.5 and regenerated `clients/bun.lock`
  (also pulls esbuild 0.25 → 0.27, required by Vite 7.3.5).
- Upgrade risk: low. Verified with client typecheck, Web production build, and
  the SDK/GUI/TUI test suites.

| Alert | Severity | Advisory | Area |
|---|---|---|---|
| #7 | high | GHSA-fx2h-pf6j-xcff | dev `server.fs.deny` bypass (Windows alt paths) |
| #5 | high | GHSA-v2wj-q39q-566r | dev `server.fs.deny` bypass via queries |
| #4 | high | GHSA-p9ff-h696-f583 | dev-server WebSocket arbitrary file read |
| #8 | medium | GHSA-v6wh-96g9-6wx3 | `launch-editor` NTLMv2 (Windows dev) |
| #6 | medium | GHSA-4w7w-66w2-5vf9 | optimized-deps `.map` path traversal |
| #3 | medium | GHSA-93m4-6634-74q7 | dev `server.fs.deny` backslash bypass (Windows) |
| #2 | low | GHSA-g4jq-h2w9-997c | dev middleware public-dir prefix |
| #1 | low | GHSA-jqfw-vq24-v9c3 | dev `server.fs` HTML handling |

## `glib` (Desktop, Linux) — accepted

- Owning manifest: `clients/desktop/src-tauri/Cargo.lock`.
- Dependency path: `tauri 2.11.6 → wry/tao/muda → gtk 0.18 → glib 0.18.5`
  (also `webkit2gtk`, `gdk`, `atk`, `cairo-rs`).
- Class: production runtime, but transitive and **Linux-desktop only**.
- Reachability: the advisory (GHSA-wrw7-89jp-8q8g, unsound `Iterator` impls for
  `glib::VariantStrIter`) requires calling that iterator. Wayshard's Rust shell
  (`clients/desktop/src-tauri/src`) does not reference `glib` or
  `VariantStrIter` (`grep` finds no use); the gtk stack is driven by Tauri/wry.
- Minimum safe version: `glib 0.20.0`. This is part of the gtk-rs 0.20 stack, so
  it cannot be bumped independently of `gtk 0.18` — it requires a whole
  Tauri/wry/gtk-rs move.
- Decision: **accepted and documented** rather than forcing churn. The vulnerable
  API is not used by Wayshard code, the exposure is Linux-desktop-only, and the
  fix is a broad GUI-stack upgrade with real regression risk. Revisit when a
  Tauri release moves to gtk-rs 0.20.

## Re-running the triage

```sh
gh api "repos/Wayshard/wayshard/dependabot/alerts?state=open&per_page=100" --paginate
go list -deps ./... | grep '^golang.org/x/crypto'
cd clients/desktop/src-tauri && cargo tree -i glib --locked
```
