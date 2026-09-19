# MEMORY.md

## Purpose

This file preserves durable Wayshard decisions, rationale, terminology, and rejected alternatives that should survive long implementation sessions. It is not a duplicate specification or architecture document. When current behavior/architecture is disputed, consult the canonical document responsible for that concern.

## Identity

- Product: **Wayshard**
- Primary domain: **wayshard.dev**
- GitHub organization: **Wayshard**
- Primary repository: **Wayshard/wayshard**
- License: **MIT**

The name is intentionally not tied to AI, agents, one model provider, or one coding harness.

## Product thesis

Wayshard is a coding control plane, not another coding agent. The important product value is orchestration, routing, context, evidence-driven validation/review, safe workspace isolation, multi-device control, and transparent recovery across existing coding harnesses.

The semantic workflow remains plan -> execute -> validate -> review, with explore/repair/replan inserted as necessary and safe integration before completion for source-changing tasks.

## Server authority

The Go server owns durable workflow truth. Clients are control surfaces. ACP harness sessions are execution details. Jev is a typed judgment service. This separation is foundational and should not erode during implementation.

## Why Go

Go was chosen for the long-lived control-plane daemon because it fits process supervision, networking, concurrency, cross-platform binaries, embedded SQLite, and a modular monolith well. Avoid replacing the server with Node/TypeScript merely because imported clients are TypeScript-based.

## Why SQLite

Wayshard is local/server-owned rather than a multi-tenant hosted platform. SQLite gives simple deployment, transactional durability, migrations in the binary, and no external database service. The server is the sole DB writer. Large immutable content belongs in a content-addressed object store, not large SQLite rows.

Postgres/microservices were deliberately rejected for the intended topology unless future requirements materially change.

## Why Jev

TypeSafe Jev/System One is used as a fast typed/probabilistic judgment layer, not a reasoning/planning engine. It should answer narrow questions about task characteristics and semantic fit, while Go retains deterministic authority for budgets, security, capabilities, stopping conditions, and final route selection.

Keep raw TaskAssessment separate from RouteDecision so old assessments can be replayed against new policy.

Repository text may be adversarial/noisy, so Jev context should prefer compact structured signals over indiscriminate raw files/chat.

## Harness philosophy

Wayshard uses harnesses already installed by the user/server operator. Wayshard never installs harnesses or ACP bridges.

This is a deliberate product boundary, not a missing installer feature.

OpenCode may be used as an ACP harness if the user installed/configured it. Codex may require a separately user-installed ACP bridge depending on ecosystem state. Wayshard discovers, probes, and reports readiness; it does not bootstrap these dependencies itself.

## ACP philosophy

Stable ACP v1 is the initial protocol baseline. Keep protocol mechanics behind a driver and harness quirks behind adapters. Optional ACP features remain optional.

Do not make the product depend on harness subagents. Wayshard's own planner/executor/reviewer are sequential stage roles, not a hidden subagent hierarchy.

Native session resume is useful but not a correctness dependency. Durable artifacts + workspace + context reconstruction must be enough to continue.

## OpenCode 2 client heritage

Wayshard's Web/Desktop/Android graphical client foundation and CLI/TUI foundation may begin from a one-time import of selected MIT-licensed OpenCode 2 source.

This source heritage does **not** create an ongoing OpenCode relationship:

- no GitHub fork relationship;
- no upstream remote;
- no automated sync;
- no mergeability goal;
- no OpenCode branding;
- no OpenCode SDK/API compatibility constraint.

Wayshard should freely restructure imported code around its own server/domain. Preserve legal attribution only.

## Four clients

Wayshard has four full clients:

- Web
- CLI/TUI
- Desktop
- Android

CLI and Desktop must ship on Linux/macOS/Windows. Android is a full client. Web is embedded in Server.

“Full” means core product capability, not forced identical presentation. Do not build nested terminal/editor systems or unrelated convenience features merely to claim parity.

## Networking decision

Wayshard copies the **simplicity** of OpenCode 2's local-server deployment model, not its username/password authentication.

Default server networking is loopback HTTP/WebSocket at `127.0.0.1`. Wayshard does not manage TLS certificates, public remote networking, VPNs, DNS, or reverse proxies.

The expected remote pattern can be:

```text
Wayshard on 127.0.0.1
-> Tailscale Serve
-> *.ts.net
-> clients
```

but Tailscale is not a dependency. SSH tunnels or other user-controlled networking are equally valid.

Earlier ideas for built-in private CA/direct TLS/certificate management are superseded and must not reappear accidentally.

## Authentication decision

Do **not** copy OpenCode's username/password auth.

Wayshard retains the stronger previously designed model:

- stable ServerID and server application identity keypair;
- short-lived, single-use pairing invitation from trusted/local authority;
- unique high-entropy per-device credential;
- independently revocable devices;
- server stores credential verifier/hash;
- native clients use secure storage;
- web uses secure authenticated session behavior.

Server identity is application-layer identity, not a built-in PKI/TLS system.

## Project canonical system

For new projects designed through Wayshard, the default root canonical set is exactly:

- AGENTS.md
- DESIGN.md
- SPEC.md
- ARCHITECTURE.md
- MEMORY.md
- README.md

Do not create empty placeholders when a project is created. Generate meaningful canonicals after brainstorming when the user requests it.

Existing projects are never forced into this scheme. Discover and respect their existing knowledge systems.

## Knowledge authority

Do not invent a universal filename precedence across AGENTS/CLAUDE/GEMINI/Cursor/etc. Each known ecosystem retains its own scope semantics. Project authority is explicit and domain-specific.

Explicit declarations beat semantic inference. Jev can identify possible conflict but cannot decide authority.

MEMORY.md preserves rationale; it does not override SPEC/ARCHITECTURE/DESIGN on concerns owned by those files.

Do not create a required `.wayshard` repository config format as an implementation convenience.

## Context philosophy

No giant universal context blob. Build Jev/Planner/Executor/Reviewer/Repair/Explore bundles independently.

Repository index is structural orientation, not a replacement for harness exploration. Start without requiring a vector database; keep retrieval interfaces open to later hybrid/semantic retrieval.

Conversation summaries are compression, not authority. Regenerate them from durable records instead of recursive summary-of-summary drift.

## Workspace invariant

Agents never experiment directly on the user's live working tree.

A run is based on an immutable starting snapshot that includes meaningful dirty state. Agent delta is snapshot -> final, so pre-existing user edits are never attributed to the agent.

The user may continue editing/committing/rebasing while a run executes. Integration is a three-way reconciliation against the current source state.

Branch switching must never cause silent application onto another branch.

## Changes and Git decision

Wayshard follows OpenCode 2's restraint: Changes is for inspection, not a full Git GUI.

Show Run versus Workspace provenance. Keep general Git in terminal/external tools. Add dedicated UI only for Wayshard-specific operations such as integration/revert/conflict resolution.

Default is no auto-stage and no auto-commit. If auto-commit exists, it may include only proven run delta.

## Validation philosophy

Completion is evidence-driven, not confidence-driven.

The planner creates a TaskContract/acceptance criteria. The server owns final validation. The reviewer evaluates semantic correctness against objective evidence. Go CompletionPolicy decides what happens next.

Existing failing tests are legitimate baseline. Distinguish regressions when practical. Unknown/unverified is a real result, not an excuse to invent PASS.

Discovery/indexing must never execute repository code just to figure out tests/tooling.

## Completion-state correction

Do not call a source-changing run COMPLETE merely because implementation/review passed while source integration is pending.

Use a state such as READY_TO_INTEGRATE, then INTEGRATING, then COMPLETE. Integration conflicts produce INTEGRATION_BLOCKED while preserving the validated run result.

Artifact-only tasks can complete without integration.

## Security philosophy

Approvals are UX/policy; OS containment is security.

Separate Harness Sandbox and Tool Sandbox when the integration can enforce it. Harness/provider credentials should not automatically flow into model-generated command processes.

Known adapters can be `native` or `adapter_bridge`; generic ACP may be `outer_only`. Report the actual isolation level and never silently downgrade failed sandbox setup.

Do not require Docker.

External file approval should usually import a read-only immutable snapshot rather than mount arbitrary host paths.

## OpenCode tool-isolation note

OpenCode permission prompts alone are not sufficient containment. The intended Wayshard/OpenCode integration can interpose command execution through a runtime adapter/bridge so generated shell commands execute in Wayshard's ToolSandbox, while OpenCode itself retains only the minimum outer-harness authority it needs.

This is adapter behavior, not modification/installation of the user's OpenCode installation.

## Provider credential boundary

Coding-provider credentials remain harness-owned whenever the harness owns the provider connection. Wayshard must not scrape harness auth stores and then call those providers directly.

Wayshard owns Jev credentials and other services it directly calls.

## Secret vault

Server-owned secrets belong in an encrypted SecretVault. Headless Linux cannot assume a desktop keyring; support external credential material, passphrase unlock, and an honest protected local key-file fallback.

Do not store plaintext API keys in normal SQLite fields.

Failure to unlock an existing vault never creates a new replacement vault.

## Recovery philosophy

A crash may interrupt work but must not corrupt source or erase task history.

Write stages have pre-attempt checkpoints. Interrupted attempts stay visible; retry creates another attempt. Integration is journaled. Startup reconciliation occurs before normal scheduling.

Ephemeral token/terminal stream loss is acceptable; durable state/artifacts/workspaces are not.

## CLI philosophy

`wayshard` should be a real native terminal client, not a thin automation afterthought. It is distributed for Linux/macOS/Windows and talks to the same server APIs/auth as graphical clients.

Interactive TUI and scriptable CLI should share domain semantics. Exact command names can evolve underneath that requirement.

Keep the terminal experience terminal-native. `$EDITOR` is acceptable; do not build an IDE inside the TUI unless inherited capabilities naturally support it.

## Release philosophy

Wayshard is a public GitHub project. Official binaries/installers come from GitHub Actions and GitHub Releases, not manual workstation builds.

Normal fork PR CI must work without secret credentials or paid model calls. Use fake ACP harnesses heavily.

Release families:

- Server
- CLI
- Desktop
- Android APK
- Web embedded in Server

Never bundle external coding harnesses/bridges.

Official signing material is generated and held by the Wayshard maintainer. Wayshard cryptographically signs Android APKs (JKS/PKCS12), Windows installers (self-signed Authenticode PFX), macOS apps (ad-hoc identity `-`), and the SHA-256 checksum manifest (minisign). That is not Apple/Microsoft platform PKI trust and does not require Apple Developer, notarization, a commercial CA, Azure signing, Google Play, or another paid/external signing account. There is no Tauri auto-updater. Private keys live only in the protected GitHub Environment `release`. PR CI is secret-free.

Rejected alternatives: Apple Developer ID and notarization, commercial Windows Authenticode certificates, Azure Artifact Signing, Google Play App Signing, third-party cloud signing, and unused Tauri updater keypairs.

## Design restraint

The backend is sophisticated; the user experience should not advertise that complexity everywhere.

Preserve the restrained OpenCode-inspired workspace/navigation style where useful. Do not add command palettes, accessibility initiatives, rich IDE systems, or other secondary UX systems if they are not already present in the imported foundation and are not required by Wayshard functionality.

## Implementation posture

The complete intended product is specified up front. Implementation should proceed in coherent stages until the product matches the canonicals. Avoid describing implementation as an MVP/v1/v2 product ladder that permanently defers core requirements.

## Implementation notes (code)

- GitHub organization slug used in module paths and CI is `Wayshard`; the primary repository is `Wayshard/wayshard`. The product domain remains `wayshard.dev`.
- One-time OpenCode 2 client/TUI import is tag `v1.18.31` (commit `014614d35b397775e5d397a490fc72368c894ec2`), stored under `third_party/opencode-v1.18.31` for provenance. Adapted copies live in `clients/`. No upstream remote or submodule.
- OpenCode 2's current desktop wrapper is Electron; Wayshard Desktop is Tauri as specified. The imported graphical app is the shared Web/Desktop/Android UI foundation.
- Default HTTP listen address is `127.0.0.1:7420`. Pairing invitations may advertise a different URL.
- Fake ACP harness binary is `wayshard-fake-acp` (`WAYSHARD_FAKE_SCENARIO`, `WAYSHARD_FAKE_STAGE`).
