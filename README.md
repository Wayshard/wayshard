# Wayshard

Wayshard is a local-first coding control plane. It runs the work *around* your
coding agents — context, routing, validation, review, integration, recovery, and
multi-device control — so a project is no longer tied to a single agent, model,
or chat window.

You keep using the coding harnesses you already trust (OpenCode, Codex, Claude,
Gemini, Cursor, and other ACP-compatible tools). Wayshard discovers what is
installed, routes each stage of a task to a viable harness/model, and owns the
durable state so work survives a closed laptop, a crashed process, or a switch to
another device.

**Website:** `wayshard.dev`  
**GitHub organization:** [`Wayshard`](https://github.com/Wayshard)  
**Primary repository:** [`Wayshard/wayshard`](https://github.com/Wayshard/wayshard)  
**License:** MIT

## Why Wayshard exists

- **Harness independence.** Different harnesses and models are good at different
  stages. Wayshard plans, routes, validates, and reviews across them instead of
  binding a project to one agent.
- **Evidence over claims.** An agent saying "done" is not completion. The server
  runs objective checks and a semantic review, and deterministic Go policy decides
  what happens next.
- **Safe workspaces.** Agents never edit your live tree. Every run works in an
  isolated copy of the exact starting state, and only the proven delta is
  integrated back — conflict-aware, never a silent overwrite.
- **Durable recovery.** Runs, checkpoints, and publication are durable. A crash
  interrupts work but does not corrupt source or erase history.
- **One authority, many surfaces.** The server owns workflow state; Web, Desktop,
  CLI/TUI, and Android are control surfaces you can use interchangeably.

## The workflow

A source-changing task moves through explicit stages:

```text
User request
  -> Context assembly
  -> Jev assessment
  -> Plan
  -> Execute
  -> Validate
  -> Review
  -> Repair / Replan (when needed)
  -> Safe integration
  -> Complete
```

`Plan`, `Explore`, and `Review` are read-only. `Execute` and `Repair` write only
inside the isolated run workspace. `Validate` is server-owned. `Integrate`
publishes the reviewed delta into your source workspace with conflict handling.
Artifact-only tasks (brainstorming, research) can complete without integration.

## Quick start

### Install a release

Linux and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/Wayshard/wayshard/main/scripts/install.sh | sh
```

Windows (PowerShell):

```powershell
powershell -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/Wayshard/wayshard/main/scripts/install.ps1 | iex"
```

The installer resolves the latest stable release, detects the OS/architecture,
downloads the CLI+TUI archive and the matching server binary, verifies both
against the release `SHA256SUMS.txt` (and the minisign signature when the
`minisign` tool is available), and installs atomically into a user-owned bin
directory (`~/.local/bin` on Linux/macOS, `%LOCALAPPDATA%\Wayshard\bin` on
Windows), adding it to your `PATH` idempotently.

Start the server, then the interactive client:

```sh
wayshard-server     # starts on 127.0.0.1:7420
wayshard            # interactive terminal client (TUI)
```

`wayshard` is the full TUI client; scriptable subcommands such as
`wayshard status` talk to the same server.

### Build from source

Prerequisites: Go 1.25+, `bun`, and `git`.

```sh
make build-all                 # builds bin/wayshard-server, bin/wayshard, bin/wayshard-tui
./bin/wayshard-server          # starts on 127.0.0.1:7420
./bin/wayshard status          # scriptable CLI against the local server
./bin/wayshard                 # interactive terminal client (TUI)
```

The server stores its control-plane state (SQLite, object store, credentials)
under the platform data directory:

- Linux: `~/.local/share/wayshard`
- macOS: `~/Library/Application Support/Wayshard`
- Windows: `%APPDATA%\Wayshard`

To connect a client, create a pairing invitation from a loopback client or an
already-trusted device and complete verified pairing:

```sh
wayshard pair --invitation '<pairing card or JSON>'
```

Pairing binds a device to the server's identity. Each device credential is
independently revocable, and revoking a device never cancels a server-owned run.

## Harnesses

Wayshard runs coding harnesses that are already installed for the OS user running
the server. Discovery searches the daemon `PATH`, a safely obtained login-shell
`PATH`, well-known user bin directories, and explicit paths you configure.

- **The catalog is configuration.** Wayshard ships a versioned TOML catalog of
  supported harness definitions and discovers installed members. Add, override,
  disable, or remove definitions by editing the user catalog — the CRUD interface
  — with no API or UI editor:
  - Linux: `$XDG_CONFIG_HOME/wayshard/harnesses.toml` (normally `~/.config/wayshard/harnesses.toml`)
  - macOS: `~/Library/Application Support/wayshard/harnesses.toml`
  - Windows: `%APPDATA%\wayshard\harnesses.toml`
- **Discovery reports readiness.** Each discovered executable is probed for
  version and ACP initialize. A present CLI whose ACP bridge is missing is
  reported as present with a specific reason rather than hidden.
- **Authentication stays with the harness.** Wayshard never reads or copies
  provider credentials. If a probe cannot determine auth state, it reports
  `unknown` instead of guessing.

The user catalog path can be overridden with `wayshard-server --harness-catalog
<path>` or `WAYSHARD_HARNESS_CATALOG`.

## Clients

Wayshard has four full clients that share one server and one domain model:

- **Web** — served directly by the Wayshard Server.
- **CLI / TUI** — a native terminal client for Linux, macOS, and Windows. The
  interactive client is a real TUI; the same binary exposes scriptable
  subcommands.
- **Desktop** — a Tauri graphical client for Linux, macOS, and Windows.
- **Android** — a full mobile client that pairs with a server and controls its
  projects and runs.

The Web, Desktop, and Android clients are one shared graphical application; the
CLI/TUI is a terminal-appropriate client over the same HTTP/JSON and WebSocket
APIs. Download names, signing, and verification are covered in
[`docs/release.md`](./docs/release.md).

## Safe workspaces

Agents never experiment in your live working tree.

Each actionable run begins from an immutable snapshot that captures the exact
meaningful starting state, including pre-existing dirty changes. The agent works
in an isolated run workspace, and its delta is measured from that snapshot — so
your own edits are never mislabeled as agent output.

You can keep editing, committing, rebasing, or using another IDE while Wayshard
works. Integration later performs a conflict-aware three-way reconciliation
against the current source. If the server dies mid-run, the next startup restores
the verified pre-attempt checkpoint and reconciles any interrupted publication
against the real source: recognizing already-published files, resuming only the
safe remainder, and blocking rather than overwriting when the source changed
underneath it.

## Remote access

Wayshard Server is localhost-first. By default it listens on `127.0.0.1` and
serves local HTTP/WebSocket; it does not try to become a networking appliance.

To use Wayshard from other devices, expose the loopback service through
infrastructure you control. A common setup is Tailscale Serve:

```text
Wayshard Server on 127.0.0.1
        -> Tailscale Serve
        -> https://<machine>.<tailnet>.ts.net
        -> Web / Desktop / CLI / Android
```

Tailscale is one example; SSH tunnels and other private networking work equally
well. TLS certificates, DNS, VPNs, and reverse proxies remain your responsibility
and your choice.

## Development

```sh
make ci            # gofmt, vet, tests, build
make test          # Go tests
make build-all     # server + CLI + packaged TUI companion
make release-scripts-test
```

Client checks run from `clients/` (`bun install`, then `bun run typecheck` and the
per-workspace `bun run test`). See [`AGENTS.md`](./AGENTS.md) for the full
implementation rules and quality gates.

## Documentation

- [`AGENTS.md`](./AGENTS.md) — implementation entry point, invariants, and quality gates.
- [`SPEC.md`](./SPEC.md) — authoritative finished-product behavior.
- [`DESIGN.md`](./DESIGN.md) — product, interaction, and client UX design.
- [`ARCHITECTURE.md`](./ARCHITECTURE.md) — technical architecture and boundaries.
- [`MEMORY.md`](./MEMORY.md) — durable rationale and decisions.
- [`CONFORMANCE.md`](./CONFORMANCE.md) — requirements traced to code and tests.
- [`docs/release.md`](./docs/release.md) — downloads, signing, and maintainer setup.
- [`docs/client-source-lineage.md`](./docs/client-source-lineage.md) — imported client source provenance.
- [`docs/dependency-security.md`](./docs/dependency-security.md) — dependency-security triage.

## License

Wayshard is licensed under the MIT License. Imported or bundled third-party
MIT-licensed source remains subject to its required notices; the adapted client
foundation derives from OpenCode 2 and attribution is preserved in
[`NOTICE`](./NOTICE) and [`THIRD_PARTY_NOTICES.md`](./THIRD_PARTY_NOTICES.md).
Attribution does not imply an ongoing upstream relationship.
