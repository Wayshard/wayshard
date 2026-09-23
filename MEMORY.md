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

Wayshard ships a versioned TOML catalog of supported harness definitions and automatically discovers installed members of the effective catalog. Users add, override, disable or remove definitions by editing the user `harnesses.toml` (the CRUD interface); there is no catalog CRUD API or UI editor. A user definition is a first-class supported harness, not a special case: adding an ordinary compatible ACP harness is a TOML change, not a Go change.

A definition describes how to recognize/probe/invoke a harness family; an installation is an actual discovered executable with resolved paths and probe observations. A definition never asserts an installation exists, and an installation refers to the definition it was discovered from.

OpenCode is a native-ACP harness if the user installed/configured it. Codex and others may require a separately user-installed ACP bridge depending on ecosystem state; the CLI and bridge are reported independently, so a present CLI whose bridge is missing is reported present with a specific reason. Wayshard discovers, probes, and reports readiness; it does not bootstrap these dependencies itself.

## ACP philosophy

Stable ACP v1 is the initial protocol baseline. Keep protocol mechanics behind a driver and harness behavioral differences declarative in the catalog (`acp`, `interpose_commands`, `model_selection`, `acp_requires_loopback`) rather than in per-name Go adapters. Optional ACP features remain optional.

Do not make the product depend on harness subagents. Wayshard's own planner/executor/reviewer are sequential stage roles, not a hidden subagent hierarchy.

Native session resume is useful but not a correctness dependency. Durable artifacts + workspace + context reconstruction must be enough to continue.

ACP terminal/tool callbacks are interposed by Wayshard, not executed by the harness: model-generated commands run in the Tool Sandbox under server-owned process trees. Do not let an approval, a route, or a harness name imply a bypass of OS containment.

## Discovery probe isolation

Discovery executes installed binaries before they are trusted, so probes are sandboxed with a dedicated ProbePolicy rather than the writable HarnessPolicy. Probes get NetworkNone, synthetic HOME/TEMP, an allowlisted environment, read-only system and resolved-executable roots, bounded output and descendant cleanup, and no project/SourceWorkspace/Wayshard-runtime/SSH-agent/display access. Required isolation means an unenforceable platform reports the probe unavailable instead of running unrestricted.

A probe cannot read real harness config, so auth state that cannot be determined is reported honestly (not assumed authenticated). The login-shell PATH probe necessarily reads the user's home and `/etc` to source shell startup files; that is a bounded exception, it never inherits ambient secrets, and its output is used only as search directories.

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

Checkpoint integrity is a cryptographic property, not a path property: the v3 tree digest is canonical and length-prefixed, and restore copies the checkpoint into private staging, hashes the staged tree, compares it to the persisted digest, and materializes only that verified staged state through an atomic swap. Checkpoint lookup is scoped to the exact interrupted attempt; there is no "latest checkpoint" shortcut. Material for a non-terminal run is pinned; terminal-run material is reclaimed after retention and marked reclaimed, which recovery refuses.

A crash can outlive a direct child: parent-death signals cover only the immediate process, so each attempt carries a per-attempt ownership token (hash persisted before launch) inherited by harness and Tool descendants. Startup reconciliation terminates surviving owned trees before touching the workspace and never matches by PID alone. A tree that cannot be terminated blocks its run rather than racing it. Synthetic HOME/TEMP is per attempt.

Publication recovery is driven by the actual source state, not by a trusted stale journal status: already-published targets are recognized, only the safe remainder is resumed, and a target matching neither before nor after state blocks the integration without overwriting user data.

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
- SQLite schema version 4. Migration 004 adds `process_owners` (per-attempt process-tree ownership: run/stage/attempt, token hash, pgid, state) and `workspace_checkpoints.material_state` (`present`/`reclaimed`).
- Process ownership token env var is `WAYSHARD_OWNER_TOKEN`; only its SHA-256 hash is stored. `internal/process` provides token creation and Linux token-hash reconciliation via procfs; non-Linux platforms report ownership unsupported and fail closed for a live run.
- Checkpoint trees are materialized filesystem trees under `runtime/workspaces/<runID>/checkpoints/<stageID>/<ordinal>`; restore staging is `runtime/restore-staging`; publication journals are `runtime/journals/<runID>/<integrationID>/`.
- Notable durable events: `checkpoint.created`/`checkpoint.restored`/`checkpoint.reclaimed`, `execution.orphan_reconciled`, `recovery.blocked`, `publication.reconciled`, `approval.requested`/`approval.resolved`, `notification.created`.
- `integration.Request.CrashAfter` and `integration.Request.PublishStep` are internal test seams only; the production `IntegrateAdapter` never sets them and no HTTP/JSON path constructs an `integration.Request`.
- SQLite schema version 5. Migration 005 adds `probe_owners` (discovery-probe process-tree ownership: kind, token hash, pgid, state).
- Secure provider networking (Linux) lives in `internal/provider`: a per-attempt user+network namespace, loopback proxy shim, and a private per-attempt Unix HTTPS `CONNECT` broker with destination/address validation. The server binary dispatches its own `--wayshard-provider-shim` argv. Server flags `--allow-provider-network` and `--provider-destination host:port,...` configure permission and destination policy; permission alone never enables a route.
- Provider selection is a four-way gate: permission (`AllowProviderNetwork`), runtime-probed platform capability (`provider.Detect`), harness transport compatibility (`domain.TransportHTTPProxy`), and a validated destination policy. Unknown harness transports fail closed; real-harness compatibility is not inferred from an executable name.
- Discovery probes are durably owned (`probe_owners`) and reconciled at startup before discovery runs; probes always run after `recovery.Reconcile`. Login-shell PATH discovery reads only per-shell startup files, never the home directory or a blanket `/etc`, and validates the returned PATH.
- Provider capability is exposed on authenticated `GET /v1/sandbox` as `providerNetwork`.
- Real-harness launch closure (`harness.harnessClosureFor`) grants a script/symlink harness its package tree and PATH-resolved interpreter; `ProcIsolation` gives harness/probe policies a private PID+mount namespace with a scoped procfs (capability-probed, best-effort; `/proc` granted only when the scoped mount succeeded). The ACP driver supports `session/set_config_option`/`session/set_model`, and real harnesses get the universal structured final-response contract.
- Provider transport compatibility is declared from observed evidence only: OpenCode and Codex are HTTP-proxy compatible because their real provider requests traversed the broker (`opencode.ai`/`models.opencode.ai`, `chatgpt.com`). Native real-harness E2E tests are guarded by `WAYSHARD_REAL_HARNESS=1` and skipped in CI.
- OpenCode's ACP needs a local loopback socket; a distinct loopback-only ACP discovery probe (`NetLoopback`, private netns with only `lo`) discovers it route-viable, while version probes stay `NetworkNone`. A configured provider model (`--provider-model`) is applied to provider candidates that do not advertise models. codex-acp initializes over real ACP but the current ChatGPT account is usage-limited for model calls.
- The supported-harness inventory is configuration, not code. The shipped catalog is `internal/harness/harnesses.toml` (embedded, `schema_version = 1`); the user catalog is `$XDG_CONFIG_HOME/wayshard/harnesses.toml` (per-platform config dir fallback), overridable with the server flag `--harness-catalog` (or `app.Config.HarnessCatalogPath`). Merge is by stable `id`: user fields override shipped fields by key (arrays replace), `enabled = false` disables, deleting restores, user-only ids are custom, duplicates within one source are rejected, invalid entries are isolated, and unsupported schema versions/malformed TOML are rejected with diagnostics. Loaded at startup; a restart applies changes.
- The catalog cannot weaken containment: there is no field for host networking, disabling the sandbox/Landlock/seccomp, arbitrary environment, arbitrary host filesystem roots, approvals bypass, Tool/validation `NetworkNone` bypass, or trusting an unverified provider transport. Paths must be home-relative with no traversal (`.`/`./`/`..`/absolute/drive/HOME rejected), aliases are bare names and never package runners, unknown fields/enum values fail the entry closed, and catalog bytes/definitions/list entries/glob matches are bounded. Provider transport trust stays in the Wayshard-owned `verifiedProviderTransports` evidence registry.
- Provider trust is fingerprint-bound, not id-bound. `Definition.ExecutionFingerprint` is a versioned SHA-256 over execution-relevant fields (executables, bridges, well-known, version/ACP/bridge args, native-vs-bridge, loopback, interposition, model selection, config roots, platforms, provider requirement, declared transport); `VerifiedTransport` grants verified transport only when the effective definition's fingerprint equals the trusted shipped definition's. Cosmetic overrides retain trust; any material override loses it and fails closed.
- Config-root trust distinction: shipped roots are trusted product configuration; user-supplied roots must be beneath a platform config/data/state/cache directory or a dot-directory and must not be a sensitive location (`.ssh`, `.aws`, `.gnupg`, `.kube`, …), HOME or a platform base. Roots are canonicalized with component-safe `Lstat` checks before being granted; a symlink escaping HOME or resolving onto a sensitive location is dropped. Well-known search roots get the same treatment, while a resolved executable symlink (NVM shim) is still discovered. Config roots remain read-write because provider harnesses persist auth/session state there.
- Installations are reconciled, not accumulated: each row persists the effective definition's fingerprint, `Refresh` atomically replaces the persisted set, and `Candidates` excludes rows whose definition is missing/disabled/fingerprint-mismatched and re-derives transport from the current definition. Disabling, removing or materially changing a definition (or losing an executable/bridge) removes its route viability.
- `Discover` iterates the effective enabled definitions and resolves executable/bridge aliases over daemon PATH, sanitized login PATH, Wayshard well-known bin dirs plus each definition's home-relative well-known dirs; there is no `switch definition.ID`. A route whose definition is missing fails closed.
- `GET /v1/harness-definitions` reports effective definitions (source/enabled) separately from `GET /v1/harnesses` installations (bridge path/presence, ACP status, blocking reason, provider transport). A malformed/unsupported user catalog is surfaced through those diagnostics rather than log-only. `GET /v1/sandbox` no longer claims provider networking is unavailable when `providerNetwork.available` is true; the always-on confinement report and runtime-probed provider/loopback capabilities are stated separately.
- SQLite schema version 7. Migration 006 adds harness-catalog discovery columns to `harness_installations` (`definition_source`, `bridge_executable`, `bridge_present`, `acp_status`, `blocking_reason`, `provider_transport`, `model_selection`, `requires_provider_network`); migration 007 adds `definition_fingerprint`.
- TOML parsing uses `github.com/BurntSushi/toml` (per-entry diagnostics and duplicate-key rejection).
- Discovery probes use `sandbox.ProbeEnv`, a harness environment with ambient harness-configuration keys removed (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_CACHE_HOME`, `OPENCODE_CONFIG`, `OPENCODE_CONFIG_DIR`, `CODEX_HOME`), so a probe cannot read the user's real harness configuration; real harness runs may still inherit them.
- `TestReclaimTerminalCheckpoint` (`internal/storage`) backdates the checkpoint's `created_at` before the sub-millisecond reclaim assertion, so the retention decision is independent of platform clock resolution (the previous Windows timing flake).
- P1 Pass 1E adapted the imported OpenCode 2 client source into live Wayshard clients. `clients/ui` = OpenCode `packages/ui` (rebranded `@wayshard/ui`); `clients/gui/src/session-ui` = OpenCode `packages/session-ui` with OpenCode SDK/core/client imports replaced by a Wayshard view-model shim; `clients/tui` = OpenTUI/Solid TUI adapting the imported OpenCode TUI dialog/toast/spinner/border framework and theme system. The graphical app shell adapts concrete OpenCode app files (command context, layout, session tab, command palette) and uses the OpenCode navigation hierarchy (primary tabs + dialogs). Signature surfaces: composer = imported `PromptInputV2`; Changes = imported session-ui `File` diff; Files = adapted `file-tree-v2-model`; terminal = imported ghostty-web renderer against the server PTY; TUI palette = adapted `DialogSelect`. `GET /v1/runs/{id}/file?side=run|snapshot` was added as the smallest server capability for real run diffs. `docs/client-source-lineage.md` + `clients/lineage.manifest.json` are authoritative; `clients/gui/src/lineage.test.ts` enforces it. The vendored `third_party/opencode-v1.18.31` is provenance-only and is never a runtime/build input. Deliberate divergences: OpenCode provider/model UI, server/session/provider contexts, Electron shell and branding removed; domain, data path and backend are Wayshard.
- P1 Pass 1E release-path remediation: the official `wayshard` interactive client launches the packaged OpenCode-derived TUI companion (`wayshard-tui`), built from `clients/tui` with `bun build --compile` (OpenTUI Solid plugin). The CLI resolves the companion only from its own install directory (or `WAYSHARD_TUI`), never `PATH`; the retired `wayshard>` loop is gone; scriptable subcommands remain. Release workflow builds a companion per native runner.
- Pairing binds to the trusted invitation identity: `GET /v1/pairing/challenge` requires a fresh nonce and returns the identity public key; clients verify the Ed25519 signature over the nonce plus the fingerprint and server id, complete pairing with `expectedServerId`/`expectedFingerprint`, re-check the response identity, and persist the credential only after all checks. Application identity only; TLS/VPN/tunnel remain user-owned.
- CLI/TUI releases ship a single coherent distribution unit per platform: `scripts/release/package-cli-tui.sh` builds `wayshard-<tag>-<os>-<arch>.tar.gz` (`.zip` on Windows) containing `wayshard`, `wayshard-tui`, `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md` under the canonical runtime names the launcher expects. The release `tui` matrix builds the native CLI plus the self-contained TUI on each runner; raw CLI/TUI binaries remain for advanced users but a raw `wayshard` alone is not a functional interactive client. `make build-all` emits the runnable `bin/wayshard` + `bin/wayshard-tui` pair (`build-tui-versioned` adds a versioned copy separately). `resolveTUI` resolves the running executable through symlinks before taking its directory, so a symlinked launcher finds the companion beside the real install; escaping companion symlinks stay rejected and PATH search stays forbidden. The combined `checksums` job waits for `release`, `tui`, `desktop`, and `android` and fails if any expected bundle is missing before minisigning `SHA256SUMS.txt`. Proof: `scripts/release/package_cli_tui_test.sh`, `scripts/release/release_policy_test.sh`, `scripts/ci/tui_smoke.sh`, `cmd/wayshard/resolve_test.go`, `cmd/wayshard/resolve_symlink_test.go`.
- Importing OpenCode design-system source is not sufficient on its own: the graphical app must also mount the upstream-derived theme token/provider pipeline. The live client needs both the adapted v2 stylesheet import (`@wayshard/ui/v2/styles/tailwind.css` plus the session-ui styles entry, matching upstream `packages/app/src/index.css`) and the adapted runtime `ThemeProvider` (`@wayshard/ui/theme/context`, default theme `oc-2`, dark), matching upstream `packages/app/src/app.tsx`. Without both, v2 tokens are undefined and the app renders light-on-light. The "More" overflow must mount through the shared dialog provider (`dialog.show`), not an inline `<Dialog>`. Narrow widths use an off-canvas sidebar drawer with a titlebar toggle. Rendered regression coverage is `scripts/ci/ui_render_smoke.mjs` (dependency-free CDP + installed Chrome) with source guards in `clients/gui/src/theme-integration.test.ts`.
- Pass 1E application-level port: the graphical application composition (root/home/layout/sidebar/session/titlebar/composer/review/file/terminal) was replaced with descendants of the actual OpenCode application source (`clients/gui/src/app/**`), with Wayshard domain/backend adapted into it. The custom Wayshard shell (`session-shell.tsx`, old `layout.tsx`, unused `session-tab.tsx`) is retired. Routing uses the adapted Wayshard router (`clients/gui/src/app/router.tsx`) because `@solidjs/router` did not advance its reactive location in the adapted provider tree. `clients/lineage.manifest.json` records the application-level descendants and `clients/gui/src/lineage.test.ts` verifies recorded blobs against the vendored source plus production-import reachability. Proof: `scripts/ci/ui_render_smoke.mjs` (adapted composition), `clients/gui/src/lineage.test.ts`.
- Pass 1E application-level port (continued): the narrow/mobile model is the upstream one — the layout context exposes `mobileSidebar` (opened/show/hide/toggle), the app-level titlebar carries the `xl:hidden` toggle, and `pages/layout/sidebar-mobile.tsx` renders the `data-component="sidebar-nav-mobile"` overlay with a scrim; the persistent sidebar is hidden below `xl` and the overlay hides on navigation. `TestCancellationInterruptsActiveHarness` was made deterministic (load-tolerant budgets, cancellation wait with its own deadline, orphan check scoped to the test's built harness path). Rendered smoke: `scripts/ci/ui_render_smoke.mjs`.
- P1 Pass 1E final application-level remediation (after a second independent audit). Earlier stages established macro application ancestry; the final remediation deepened the behavior-bearing session ancestry and removed the document-reload routing bridge. Key facts: (1) `clients/gui/src/app/router.tsx` is a real reactive SPA router (`pushState`/`replaceState` + a location signal; `popstate` syncs back; only absolute external URLs `location.assign`); the selected route view is resolved as a `createMemo` and swapped via `Dynamic` because the adapted provider tree did not propagate the router signal through top-level `Show`/`Switch` children even though effects/text nodes in the same component update. (2) Review/Changes is the adapted session-ui `SessionReview` fed by `review-adapter.ts` (bounded concurrent hydration, binary handling) with per-session scroll/open persistence (`review-view.ts`). (3) Files uses `file-tab-model.ts` (ordered tabs, neighbor close, per-file scroll) + the inherited `File` viewer with a Wayshard compare-and-set editor. (4) Terminal panel has real multi-terminal tabs (list/create/attach/close) against server PTYs; a narrow capability `DELETE /v1/projects/{id}/terminals/{tid}` (`pty.Manager.Kill`, SDK `closeTerminal`) makes close truthful; split-pane resize and client screen serialization are deliberately omitted. (5) Composer has an adapted region controller + request-dock state machine (approval list, once-applied responding lock, resize-observed dock reveal, focus restoration). (6) Timeline projects messages + run stages into one reconciled row model with bottom-follow, scroll preservation, jump-to-latest, per-session state and a bounded mounted window (no `@tanstack/solid-virtual` dependency). (7) New-session submissions go through Wayshard state (`selectConversation` + `send`) so the run registers. (8) `clients/lineage.manifest.json` is v3 with `structuralAncestry` (required defs, comment-stripped inherited anchors present in both live and upstream, live-only markers, min code size, similarity floor) and import-specifier `usage`; a provenance-only wrapper is explicitly rejected. (9) `scripts/ci/ui_render_smoke.mjs` exercises routing (with a per-document reload detector), New Session, Timeline, Changes, Files (CAS save), Terminal, Composer/Approval, desktop 900 and mobile 390 — 55 checks.
- Native runtime security milestone (after rc.8): added a typed sandbox capability model (`sandbox.Feature`, `RequiredFeatures`, `validateRequiredFeatures`) so a `Required` policy fails closed with `ErrRequiredIsolation` unless the backend declares every feature it depends on. **macOS** Seatbelt confinement is now natively verified on macOS 15 arm64 and `macos-15-intel` (filesystem read/write confinement, `NetworkNone` against a host loopback listener, process-group cancellation, synthetic HOME/TMPDIR, fail-before-exec); the profile is passed inline via `sandbox-exec -p` (no on-disk profile) and a relative command under `cmd.Dir` is resolved against `cmd.Dir`. **Windows** Job Objects are process/resource management only; filesystem/network/race-free-tree containment are not enforced, so `Report()` reports the required sandbox unavailable and required harness/tool/probe execution fails closed before any untrusted code runs — Windows stays a full server/client platform but local protected execution is refused. Isolation-unavailable maps to `FailPolicy` (run BLOCKED), not a retryable infrastructure failure. Probe ownership now fails closed for the server lifetime when an active owner cannot be verified (`recovery.ProbeOwnershipReconciled`). `native-security` CI jobs run the native black-box suite on macOS arm64/Intel and Windows. Known honest residual: a deliberately `setsid`-detached macOS descendant can leave the process group and survive cancellation, but it stays Seatbelt-confined and a stale owner blocks the run (fail closed). Native tests: `internal/sandbox/{feature,native_security}_test.go` (re-exec fixture), `internal/validation/tool_sandbox_test.go` now runs on macOS.
- macOS process-tree ownership remediation (after the native-security pass): `FeatureProcessTree` is now backed by authoritative token-based ownership rather than a process group. `internal/process/owner_darwin.go` enumerates same-user processes and reads their initial environment via `sysctl(KERN_PROCARGS2)`, comparing the inherited `WAYSHARD_OWNER_TOKEN` digest so a `setsid`/`setpgid`/double-fork descendant is still found and killed and a reused PID cannot target an unrelated process; the capability is runtime-probed and, if unavailable, `FeatureProcessTree` is not advertised and required macOS execution fails closed. `finishProcessOwner` no longer marks an owner reconciled when ownership is unsupported (it leaves the owner active so recovery fails closed). `StoreProbeOwnerSink.BeginProbe` refuses to start a probe when ownership is unsupported, so no unreconcilable probe owner is created and a clean probe no longer disables future probing after restart. Tool sessions carry a distinct `WAYSHARD_TOOL_TOKEN` and reconcile escaped descendants on kill/release/close (via `process.ReconcileEnvToken`) without terminating the running harness; validation commands carry a per-command token and reconcile after completion/timeout. `process.ReconcileEnvToken` generalizes the Linux/darwin scanners to any env var name. Mandatory native tests (macOS arm64 + Intel): `TestNativeSetsidDescendantReconciled`, `TestNativeCancellationDoesNotKillUnrelated`, `TestDarwinFinishProcessOwnerReconcilesDetached`, `TestDarwinCleanProbeLeavesNoActiveOwner`, `TestDarwinDetachedProbeReconciledOnRestart`, `TestDarwinCleanProbeTwoStart`, `TestDarwinACPExecReconcilesProcessOwner`, `TestDarwinToolReleaseReconcilesDetached`, `TestDarwinValidationReconcilesDetached`; the native-security CI job fails if the macOS setsid/ownership tests skip.
- macOS process ownership — adversarial token-stripping closure: environment ownership tokens (`WAYSHARD_OWNER_TOKEN` / `WAYSHARD_TOOL_TOKEN`) are NOT a security boundary. The untrusted process controls its children's exec environment, so it can strip the token, setsid away from the process group, and evade any `sysctl(KERN_PROCARGS2)` scan; reconciliation would then observe zero owned processes and could mark the durable owner reconciled while a stripped descendant survived. `kqueue` `EVFILT_PROC` + `NOTE_TRACK` is a real kernel fork-tracking primitive, but a PID-reuse-safe (`NOTE_TRACKERR`-safe, start-time-verified) implementation could not be demonstrated reliably within the pass, and a bug there could terminate an unrelated process (P0). Decision: **honest macOS fail-closed** — macOS no longer advertises `FeatureProcessTree`; `DarwinBackend.Report()` is `Available=false`, `Mode=seatbelt_no_ownership`; `ToolPolicy`/`HarnessPolicy`/`ProbePolicy`/`ReadOnlyViewPolicy`/validation all fail with `ErrRequiredIsolation` before untrusted code runs (`TestNativeRequiredPoliciesFailBeforeExec`, `TestNativeFailClosedBeforeExec`, `TestNativeValidationFailsClosed`, gated in the `native-security` CI job). Environment tokens are retained only as cooperative/diagnostic hints (and for Linux recovery); a durable owner is never reconciled merely because no token-bearing process was found on a platform where the token is removable. Linux remains the only platform advertising `FeatureProcessTree`. macOS stays a full Desktop/TUI/CLI/server/control-plane platform. The same environment-marker removability argument also applies to Linux and is recorded for a future Linux-ownership pass; Linux was explicitly out of scope here.
