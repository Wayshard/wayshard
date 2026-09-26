# Wayshard

Wayshard is a local-first coding control plane for orchestrating work across installed coding harnesses and models.

Instead of tying a project to one coding agent, Wayshard owns the workflow around them: project knowledge, context assembly, fast task assessment, stage routing, isolated execution, objective validation, semantic review, safe Git/workspace integration, recovery, usage, and multi-device control.

**Website:** `wayshard.dev`  
**GitHub organization:** [`Wayshard`](https://github.com/Wayshard)  
**Primary repository:** [`Wayshard/wayshard`](https://github.com/Wayshard/wayshard)  
**License:** MIT

## What Wayshard does

A typical source-changing task follows:

```text
User request
  -> Context assembly
  -> Jev assessment
  -> Plan
  -> Execute
  -> Validate
  -> Review
  -> Repair/Replan if needed
  -> Safe integration
  -> Complete
```

Wayshard is not a coding model. It delegates stage work to coding harnesses already installed by the user, communicates with them through ACP, and keeps the durable orchestration state in the Wayshard Server.

## Clients

Wayshard has four full clients:

- **Web** — served by the Wayshard Server.
- **CLI / TUI** — native terminal client for Linux, macOS, and Windows.
- **Desktop** — Tauri-based graphical client for Linux, macOS, and Windows.
- **Android** — full mobile client for controlling server-side projects and runs.

Wayshard's graphical clients and CLI/TUI are built from selected MIT-licensed OpenCode 2 client application source imported once into Wayshard. The graphical client's application composition (Home, project/session layout and sidebar, session page, titlebar, composer, review/files/terminal panels and the narrow/mobile model) is adapted from that source, with the Wayshard domain and server adapted into it. Wayshard is an independent project: it is not maintained as an OpenCode fork, has no upstream-sync relationship, and does not target OpenCode API compatibility. Required third-party MIT attribution is preserved separately.

## Install the CLI / TUI

Official CLI/TUI releases are distributed as one archive per platform. The
archive contains the `wayshard` CLI and its required `wayshard-tui` companion
under the canonical runtime names, plus `LICENSE`, `NOTICE`, and
`THIRD_PARTY_NOTICES.md`:

| Platform | Archive |
|---|---|
| Linux x86-64 | `wayshard-vX-linux-amd64.tar.gz` |
| Linux arm64 | `wayshard-vX-linux-arm64.tar.gz` |
| macOS x86-64 | `wayshard-vX-darwin-amd64.tar.gz` |
| macOS Apple silicon | `wayshard-vX-darwin-arm64.tar.gz` |
| Windows x86-64 | `wayshard-vX-windows-amd64.zip` |

Extract the archive and run the binaries in place. No renaming, no
`WAYSHARD_TUI` override, and no Bun installation are required:

```sh
mkdir wayshard && tar -xzf wayshard-vX-linux-amd64.tar.gz -C wayshard
./wayshard/wayshard          # interactive Wayshard TUI
./wayshard/wayshard help     # scriptable CLI help
```

The raw `wayshard` binary alone is **not** a functional interactive client: it
launches the adjacent `wayshard-tui` companion. Standalone server, CLI, and TUI
binaries are also published for advanced users, but the archive is the normal
installable unit.

## Install Desktop and Android

Desktop and Android host the same shared graphical client.

| Platform | Desktop artifact |
|---|---|
| Linux x86-64 | `wayshard-desktop-vX-linux-amd64.AppImage` and `.deb` |
| macOS Apple silicon | `wayshard-desktop-vX-macos-aarch64.dmg` |
| macOS Intel | `wayshard-desktop-vX-macos-x86_64.dmg` |
| Windows x86-64 | `wayshard-desktop-vX-windows-x64.msi` and `-setup.exe` |

Android ships as `wayshard-vX-android.apk` (minSdk 26; arm64-v8a,
armeabi-v7a, x86, x86_64). It connects to a selected Wayshard Server; it does
not host project execution itself.

**Signing and OS warnings.** Official artifacts are cryptographically signed by
Wayshard with maintainer-generated keys; they are **not** trusted by Apple,
Microsoft, or Google platform PKI. Expect OS warnings until you explicitly trust
the publisher:

- macOS uses ad-hoc signing, so Gatekeeper reports that the developer cannot be
  verified (Privacy & Security → Open Anyway).
- Windows installers use a self-signed Authenticode certificate, so SmartScreen
  and "Unknown publisher" prompts appear.
- Android is a maintainer-signed APK sideloaded without Google Play.

Verify a download with:

```sh
minisign -V -p keys/wayshard-release.minisign.pub -m SHA256SUMS.txt
sha256sum -c SHA256SUMS.txt
```

Details and maintainer signing setup: [`docs/release.md`](./docs/release.md).

## Supported platforms and verification status

Wayshard Server, CLI/TUI, and Desktop are built for Linux (amd64/arm64), macOS
(Intel and Apple silicon), and Windows (x86-64); Android ships as an APK. The Web
client is embedded in Server.

Wayshard trusts the OS user running the server. Discovered ACP harnesses, the
tools they request, validation commands, and discovery probes run as that user
with their normal configuration, authentication, environment, filesystem, and
network access. There is no sandbox or OS containment layer, and behavior is
the same on Linux, macOS, and Windows. Approvals are policy/UX, not
containment. Process cleanup on cancellation, timeout, and shutdown is
best-effort.

Verification status:

- Go server/orchestration/recovery/storage suites and the process-boundary
  recovery tests run in CI.
- Client source lineage is verified against the vendored OpenCode 2 blobs.
- Desktop/Android on-device runtime is not exercised by official CI: release
  artifacts are built, checksummed, and signed, but on-device behavior is not
  machine-verified here.
- Real installed ACP harness interoperability is verified natively (OpenCode,
  codex-acp) and remains fixture-backed in CI.

## Server model

Projects and execution live where **Wayshard Server** runs.

The Go server owns:

- projects and project knowledge;
- conversations/tasks/runs/stages;
- routing and Jev assessment;
- ACP harness discovery and lifecycle;
- isolated run workspaces;
- validation/review/completion policy;
- source integration and recovery;
- SQLite/object-store persistence;
- device pairing/authentication;
- the HTTP/JSON and WebSocket APIs.

Closing a client does not stop the server or cancel a run.

## Networking

Wayshard Server is localhost-first and intentionally does not try to become a networking appliance.

By default it listens on `127.0.0.1` using local HTTP/WebSocket. Secure remote exposure is handled by infrastructure you control.

A common setup is:

```text
Wayshard Server on 127.0.0.1
        -> Tailscale Serve
        -> https://<machine>.<tailnet>.ts.net
        -> Web / Desktop / CLI / Android
```

Tailscale is not required; other tunnels/private networking can expose the same local service. Wayshard itself does not manage TLS certificates, VPNs, public DNS, or reverse proxies.

## Authentication

Wayshard does not use a shared server username/password for normal client access.

New clients pair through a short-lived invitation generated by an already trusted client or local server administration. Each client receives its own revocable device credential. Native clients store their device credential in a restricted user config file (directory `0700`, file `0600`; user-profile ACL on Windows) or supply it through `WAYSHARD_TOKEN`; browser access uses authenticated server sessions.

## Coding harnesses

Wayshard uses coding harnesses already installed and configured for the OS user running the server.

Examples may include OpenCode, Codex through an appropriate user-installed ACP bridge, and other ACP-compatible harnesses.

Wayshard ships a versioned catalog of supported harness definitions (OpenCode, Codex, Claude, Grok, Gemini CLI, GitHub Copilot CLI, Cursor CLI, Kiro CLI, Junie, goose, Cline, Qwen Code, Qoder, Mistral Vibe, Devin CLI, Kilo Code, Factory Droid, Auggie CLI, Amp, Pi, Oh My Pi) and automatically discovers installed members. You can add, override, disable, or remove definitions by editing the user catalog — the CRUD interface — with no API or UI editor:

- Linux: `$XDG_CONFIG_HOME/wayshard/harnesses.toml` (normally `~/.config/wayshard/harnesses.toml`)
- macOS: `~/Library/Application Support/wayshard/harnesses.toml`
- Windows: `%APPDATA%\wayshard\harnesses.toml`

A user entry with the same `id` overrides shipped fields; `enabled = false` disables a shipped definition; deleting the override restores it; a new `id` creates a custom definition. Adding an ordinary compatible ACP harness needs only TOML. The catalog is a discovery/launch declaration, not a containment policy: executable aliases must be bare names (never package runners), well-known discovery dirs must be home-relative, and unknown fields or unsupported enum values are rejected. An installation is only routable while its persisted execution fingerprint still matches the effective definition, so changing a definition's execution-relevant fields invalidates stale installations. Changes apply after a server restart. The path can be overridden with `wayshard-server --harness-catalog <path>` or `WAYSHARD_HARNESS_CATALOG`.

**Wayshard never installs harnesses or ACP bridges for you.** It discovers available executables, probes ACP compatibility/capabilities, and reports whether a harness is ready, unauthenticated, degraded, incompatible, or unavailable — including a present CLI whose required ACP bridge is missing. Discovery probes run the installed binaries as the server OS user; a probe does not resolve harness-owned authentication, so an auth state it cannot determine is reported as unknown.

Provider authentication normally remains owned by the harness. Wayshard does not scrape OpenRouter/OpenAI/Anthropic/etc. credentials out of harness configuration.

## Jev

Wayshard uses TypeSafe Jev/System One as a fast typed judgment layer for decisions such as task risk, ambiguity, planning/review need, and semantic route fit.

Jev is not the planner or coding model. Deterministic security, capability, cost, stopping, and completion policy remains in Go.

If Jev is temporarily unavailable, Wayshard falls back to deterministic routing rather than making the product unusable.

## Safe workspaces

Agents never experiment directly in the user's live source working tree.

Each actionable run starts from an immutable snapshot that includes meaningful pre-existing dirty state. The agent works in an isolated run workspace. Its delta is calculated from that snapshot, so the user's existing modifications are never mislabeled as agent output.

The user can keep editing, committing, rebasing, or using another IDE/Git client while Wayshard works. Integration later performs a conflict-aware three-way reconciliation against the current source state.

If the server process dies mid-run, the next startup verifies and restores the pre-attempt checkpoint and reconciles any interrupted publication against the real source — recognizing already-published files, resuming only the safe remainder, and blocking (never overwriting) if the source changed underneath it.

## Validation and review

Wayshard does not consider “the model said it is done” to be completion evidence.

The planner defines explicit acceptance criteria. The server runs objective project checks. The reviewer judges semantic correctness against the request, run delta, and validation evidence. Deterministic Go policy decides whether the run can complete, must repair/replan, or becomes blocked.

For source-changing tasks, completion includes safe integration into the intended source workspace/branch.

## Project knowledge

Wayshard discovers repository-owned project knowledge and instruction systems rather than forcing one layout on every existing repository.

For newly designed projects, Wayshard's default canonical set is:

```text
AGENTS.md
DESIGN.md
SPEC.md
ARCHITECTURE.md
MEMORY.md
README.md
```

The files are generated meaningfully after project brainstorming when requested, not as empty boilerplate at project creation.

## Storage

Wayshard uses:

- **SQLite** for authoritative control-plane state;
- a **SHA-256 content-addressed object store** for large immutable artifacts/logs/attachments;
- the **repository/filesystem** as authoritative source state.

Wayshard-owned credentials live in restricted config files (`0700` directory, `0600` files; user-profile ACL on Windows) or environment variables, not ordinary plaintext database fields and not a mandatory vault or OS keyring. Harness-owned provider credentials stay with the harness.

## Development model

Wayshard is intended to be developed as a public monorepo under `Wayshard/wayshard`.

Official CI/CD uses GitHub Actions. Normal pull-request CI must work without production secrets, paid model calls, or external harness installations; deterministic fake ACP harnesses cover orchestration behavior.

Official tagged releases are built by CI and published through GitHub Releases.
Maintainer setup (Environment `release`, maintainer-owned signing secrets, permissions) is in [`docs/release.md`](./docs/release.md).

Expected release families:

- Wayshard Server;
- Wayshard CLI;
- Wayshard Desktop;
- Wayshard Android APK;
- checksums, release notes, third-party notices, and appropriate supply-chain metadata.

The Web client ships inside Wayshard Server.

## Application icon source

[`assets/branding/wayshard.png`](./assets/branding/wayshard.png) is the canonical transparent Wayshard application mark. The checked-in Web and desktop icons are derived from it, and Android launcher/adaptive icons are generated from the same source during release initialization. [`docs/assets/wayshard-app-icon-example.png`](./docs/assets/wayshard-app-icon-example.png) shows the mark on the deep background used for opaque and maskable platform variants.

## Canonical documentation

Before implementation work, read the canonical documents:

- [`AGENTS.md`](./AGENTS.md) — implementation rules and quality gates.
- [`SPEC.md`](./SPEC.md) — finished-product requirements.
- [`DESIGN.md`](./DESIGN.md) — client/product interaction design.
- [`ARCHITECTURE.md`](./ARCHITECTURE.md) — technical architecture and boundaries.
- [`MEMORY.md`](./MEMORY.md) — durable rationale and decisions.

These documents are designed to be complementary rather than six copies of the same specification.

## License

Wayshard is licensed under the MIT License.

Imported or bundled third-party MIT-licensed source remains subject to its required copyright/license notices. Wayshard's initial client implementation may contain code derived from OpenCode 2; attribution does not imply an ongoing project or upstream relationship.

## Development

Prerequisites: Go 1.25+, bun, git. Desktop also needs a Rust toolchain and Tauri WebKit libraries. Android needs JDK 17 and an Android SDK.

```sh
make test
make build
./bin/wayshard-server --listen 127.0.0.1:7420
./bin/wayshard status
```

`make build-all` additionally builds the packaged TUI companion under its
canonical runtime name, so `bin/wayshard` and `bin/wayshard-tui` form a directly
runnable pair:

```sh
make build-all
./bin/wayshard        # interactive TUI (launches bin/wayshard-tui)
./bin/wayshard status # scriptable CLI
```

Default data directory:

- Linux: `~/.local/share/wayshard`
- macOS: `~/Library/Application Support/Wayshard`
- Windows: `%APPDATA%\Wayshard`

Pairing: from a loopback client or an already-trusted device, `POST /v1/pairing/invitations` (optional `advertisedUrl` when the reachable client URL differs from loopback). Complete verified pairing with `wayshard pair --invitation '<pairing card/json>'` (or `wayshard pair --server-id <id> --fingerprint <fp> <code>`); a bare code is refused because pairing binds to the expected server identity.

Official release artifacts are produced by GitHub Actions on `Wayshard/wayshard`, not from workstation uploads. Normal PR CI does not require production secrets, paid model calls, or installed third-party harnesses.
