# MEMORY.md

## Purpose

This file preserves durable Wayshard decisions, rationale, terminology, and
rejected alternatives across sessions. It records *why*; `SPEC.md`,
`DESIGN.md`, and `ARCHITECTURE.md` record *what*. When current behavior is
disputed, the canonical responsible for that concern wins.

## Identity

- Product: **Wayshard**
- Primary domain: **wayshard.dev**
- GitHub organization: **Wayshard**
- Primary repository: **Wayshard/wayshard**
- License: **MIT**

The name is intentionally not tied to one AI model, agent, or coding harness.

## Product thesis

Wayshard is a coding control plane, not another coding agent. It coordinates
coding harnesses the user already installed through an explicit, evidence-driven
workflow, and owns the durable state around that work: project knowledge,
context assembly, routing, isolated execution, objective validation, semantic
review, safe integration, recovery, usage, and multi-device control.

The workflow is context → assess → plan → execute → validate → review →
(repair / replan / explore as needed) → ready-to-integrate → integrate →
complete, with safe integration before completion for source-changing tasks.

## Server authority

The Go server owns durable workflow truth. Clients are control surfaces;
harnesses are stage workers; Jev is a judgment service. This separation is
foundational and should not erode.

## Why Go

Go fits a long-lived control-plane daemon: process lifecycle, networking,
concurrency, cross-platform single binaries, embedded SQLite, and a modular
monolith. Imported clients are TypeScript; that does not pull the server to
Node.

## Why SQLite

Wayshard is local and server-owned rather than a multi-tenant hosted platform.
SQLite gives simple deployment, transactional durability, migrations inside the
binary, and no external database service, with the server as sole writer. Large
immutable content lives in a content-addressed object store rather than large
rows. Postgres and microservices are rejected for this topology unless a future
requirement materially changes it.

The v0.2 control-plane schema is a single baseline migration (`001_init.sql`,
schema version 1) squashed from the pre-v0.2 migration history. The migration
framework remains for post-v0.2 changes. A database created before v0.2 is not
upgraded: Wayshard refuses to open it and directs the operator to remove the old
data directory and start fresh.

## Why Jev

TypeSafe Jev/System One is a fast typed judgment layer for narrow questions such
as task type, risk, ambiguity, planning/review need, and semantic fit among
already-viable routes. It is not the planner, coding model, source of repository
truth, or project memory. Go keeps deterministic authority for budgets,
capability, policy, stopping conditions, and final route selection. Raw
assessment and `RouteDecision` are persisted separately so historical
assessments can be replayed against newer policy. Repository text can be
adversarial, so Jev context prefers compact structured signals over raw dumps.

## Harness philosophy

Wayshard uses harnesses already installed and available to the OS user running
the server; it does not install harnesses or ACP bridges. The supported
inventory is a versioned TOML catalog (shipped plus a user catalog merged by
stable `id`), and the user catalog is the CRUD interface. A definition declares
how Wayshard recognizes, probes, and invokes a harness family; an installation
is an actual discovered executable. Adding an ordinary compatible ACP harness is
a catalog change, not a code change, and per-name behavior lives in the catalog
rather than in Go adapters.

## ACP philosophy

Stable ACP v1 is the protocol baseline, with protocol mechanics behind the ACP
driver. Capability negotiation is authoritative and optional ACP features stay
optional. The driver is the only ACP-speaking component. Wayshard interposes ACP
terminal and filesystem callbacks so lifecycle and provenance remain
server-owned.

## Execution model

Wayshard trusts the OS user running the server. Discovered harnesses, the tools
they request, validation commands, and discovery probes run as that user with
their normal configuration, authentication, environment, filesystem, and network
access. Approvals, budgets, and routing are server policy: they gate which
server operations proceed, and they are not OS containment. Process cleanup on
cancellation, timeout, and shutdown is best-effort and is never a correctness
dependency.

## Clients

Wayshard has four full clients: Web (served by the server), CLI/TUI, Desktop
(Tauri), and Android. CLI and Desktop ship for Linux, macOS, and Windows.
"Full" means core product capability, not identical widgets; each client uses a
presentation appropriate to its platform.

## Networking

Wayshard is localhost-first: the server binds `127.0.0.1` and serves local
HTTP/WebSocket. Secure remote exposure, TLS, DNS, VPNs, tunnels, and proxies
belong to the user's networking layer — for example Tailscale Serve. Wayshard
ships no certificate manager and no remote-networking product.

## Authentication

Access uses per-device pairing, not shared username/password. Every server has a
stable ServerID and application identity keypair; a trusted client or local
administration issues a short-lived single-use invitation that binds a new
client to the expected identity; each device receives an independently revocable
credential, and the server stores only a verifier.

## Project canonicals

For projects designed through Wayshard, the default canonical set is `AGENTS.md`,
`SPEC.md`, `DESIGN.md`, `ARCHITECTURE.md`, `MEMORY.md`, and `README.md`. Generate
them meaningfully when the user asks, not as empty placeholders at creation.
Existing projects keep their own knowledge layout.

## Knowledge authority

There is no universal filename precedence across ecosystems; each retains its
native scoping semantics. Explicit declarations outrank inference. Jev may
surface possible conflicts but never decides authority. Wayshard does not require
a `.wayshard` repository config format.

## Context

Build per-target bundles (Jev, planner, executor, reviewer, repair, explore); do
not maintain one universal context blob. Prefer structured repository signals,
and keep retrieval interfaces open without mandating a vector database.
Conversation summaries are compression regenerated from durable records, never
authority.

## Workspaces and integration

Agents work in an isolated run workspace built from an immutable starting
snapshot that includes meaningful dirty state. Pre-existing user changes are
baseline, so the run delta is snapshot → final and never includes them.
Integration is server-controlled: a journaled, verified three-way reconciliation
against the current source that never silently overwrites divergent user work or
applies onto a changed branch.

## Validation and completion

Completion is evidence-driven. The planner defines acceptance criteria; the
server runs objective checks and owns final validation; the reviewer judges
semantics against the request and evidence; deterministic Go completion policy
decides what happens next. Pre-existing failures are legitimate baseline,
regressions are distinguished where practical, and unverifiable criteria are
reported as such. Source-changing runs complete only after successful
integration.

## Changes and Git

Changes is a review/diff surface, not a full Git client: run delta versus current
workspace, with general Git left to terminal and external tools. Default behavior
does not auto-stage or auto-commit; optional auto-commit may include only the
proved run delta.

## Credentials

Wayshard-owned credentials (server identity private key, Jev/control-plane
credentials, device material) live in restricted config files (directory `0700`,
files `0600`; user-profile ACL on Windows) or environment variables. Plaintext
secrets stay out of ordinary SQLite fields, logs, artifacts, and diagnostics.
Harness-owned provider credentials stay with the harness; Wayshard does not
scrape them.

## Recovery

A crash may interrupt work but must not corrupt source or erase history. Write
attempts checkpoint before they run; recovery restores a verified, attempt-scoped
checkpoint through private staging and an atomic swap, and reconciles publication
journals from actual source state. Recovery reconciles durable state rather than
process trees.

## CLI / TUI

`wayshard` is a native terminal client for Linux, macOS, and Windows that uses
the same server APIs and authentication as the graphical clients, with a
terminal-appropriate presentation and scriptable subcommands. Handoff to
`$EDITOR` is acceptable; it is not a nested IDE.

## Release and signing

Wayshard is a public GitHub project; official artifacts come from GitHub Actions
and GitHub Releases. Release families are Server, CLI, Desktop, and Android APK,
with Web shipped inside Server. Normal fork/PR CI is secret-free and
fixture-backed. Official artifacts are signed with maintainer-generated keys
(ad-hoc macOS, self-signed Windows Authenticode, Android upload keystore, and a
minisign checksum manifest) — Wayshard signing, not Apple/Microsoft platform PKI,
requiring no paid accounts. There is no auto-updater, and third-party harnesses
or bridges are never bundled.

## Branding

`assets/branding/wayshard.png` is the single artwork source; every platform icon
derives from it without redrawing the mark.

## Client source heritage

The graphical client and TUI were adapted once from MIT-licensed OpenCode 2
client/TUI source, which is retained under `third_party/opencode-v1.18.31` for
provenance. Wayshard is an independent project: no fork relationship, upstream
remote, submodule, sync workflow, OpenCode runtime/SDK/API target, or OpenCode
product branding. Attribution lives in `NOTICE`/`THIRD_PARTY_NOTICES.md`, and
lineage is verified by `clients/lineage.manifest.json`,
`docs/client-source-lineage.md`, and `clients/gui/src/lineage.test.ts`.

## Design restraint

The backend is sophisticated; the product UI stays low-chrome and
developer-oriented. Prefer progressive disclosure, preserve the imported
foundation's interaction character, and avoid building secondary systems (an IDE,
a full Git client, a networking appliance, or a model-credential manager) beyond
the product requirements.

## Rejected alternatives

- Postgres or microservices for the control plane.
- Shared username/password authentication.
- Built-in TLS certificate management, private CA, VPN, DNS, or remote exposure.
- Bundling or installing coding harnesses or ACP bridges.
- Paid platform signing accounts and an automatic updater.
- A required repository configuration format.
