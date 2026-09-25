# SPEC.md

## 1. Product definition

Wayshard is a local-first coding control plane that coordinates multiple installed coding harnesses and models through an explicit, evidence-driven workflow. It is not itself a coding model and is not tied to a single agent, model provider, or harness.

Wayshard accepts a user's request, assembles relevant project context, uses Jev for fast typed judgment, routes stages to viable coding harness/model combinations, validates results objectively, reviews semantic correctness, and safely integrates accepted changes back into the user's project.

The product name is **Wayshard**. The primary domain is **wayshard.dev**. The canonical GitHub organization is **Wayshard**. The project and its Wayshard-owned components are MIT licensed.

## 2. Finished-product components

Wayshard consists of one authoritative server and four full clients.

### 2.1 Wayshard Server

A Go daemon that owns:

- project registrations and project knowledge indexes;
- conversations, messages, tasks, runs, stages, attempts, artifacts, approvals, notifications, and usage;
- context assembly and Jev assessment;
- routing and fallback policy;
- ACP harness discovery, compatibility probing, launch, and lifecycle management;
- isolated run workspaces and Git/filesystem integration;
- validation, review/completion policy, recovery, and storage;
- server/device authentication;
- the HTTP/JSON API, WebSocket event stream, and embedded Web client.

The server must continue running independently of any client window. Official Wayshard Server builds are required for Linux, macOS, and Windows.

### 2.2 Web client

A full browser client served by the matching Wayshard Server build.

### 2.3 CLI / TUI client

A full terminal client derived from a one-time import of suitable OpenCode 2 terminal client code, then independently adapted to the Wayshard domain.

Official CLI builds are required for Linux, macOS, and Windows.

### 2.4 Desktop client

A Tauri-based full graphical client for Linux, macOS, and Windows. It may provision/manage a local user-level Wayshard Server but the server remains an independent daemon.

### 2.5 Android client

A full Android client with project/session/run capability parity where meaningful on mobile. It connects to a selected Wayshard Server; it does not host project execution itself.

## 3. OpenCode 2 source relationship

Wayshard may begin by importing selected MIT-licensed OpenCode 2 graphical-client and CLI/TUI source as implementation foundation.

After import:

- Wayshard is an independent repository, not a GitHub fork;
- no OpenCode upstream remote or synchronization workflow is maintained;
- no runtime dependency on OpenCode is required for Wayshard clients;
- no OpenCode API compatibility target constrains Wayshard;
- OpenCode branding is removed;
- future Wayshard architecture and UX may diverge completely;
- required MIT copyright/license attribution remains in appropriate third-party notices.

OpenCode itself may still be used as an installed external coding harness through ACP. That is separate from the client-source heritage.

## 4. Server networking and client authentication

### 4.1 Networking

Wayshard Server is localhost-first.

Default behavior:

```text
bind: 127.0.0.1
transport: HTTP + WebSocket
```

Wayshard does not provide a built-in TLS certificate manager, private CA, DNS service, VPN, tunnel, or remote exposure layer.

Users may expose the local server through networking they control, such as Tailscale Serve, an SSH tunnel, or another suitable mechanism. Tailscale is a recommended deployment example, not a dependency.

A pairing invitation may advertise an externally reachable URL different from the server's loopback address.

### 4.2 Authentication

Wayshard does not use a shared username/password model for normal client access.

Every server has a stable ServerID and application identity keypair. A trusted client or local administrative command can issue a short-lived, single-use pairing invitation. A new client uses that invitation to verify the expected Wayshard server identity and receive its own high-entropy device credential.

Requirements:

- each device/client credential is independently revocable;
- the server stores only a verifier/hash for long-lived device credentials;
- native clients store their device credential in a restricted user config file (directory `0700`, file `0600`; user-profile ACL on Windows) or an explicit environment credential;
- browser clients use secure server sessions/cookies appropriate to the externally exposed transport;
- revoking or disconnecting a client does not cancel server-owned runs;
- local administrative access can create a new invitation if all previously paired devices are lost.

## 5. Project lifecycle

Users can:

- open an existing project directory located on the selected server;
- clone a Git repository onto the selected server;
- create a new project on the selected server.

Opening an existing project is passive discovery. It must not execute project code, create canonical files, alter Git state, or otherwise mutate the repository.

Cloning performs the Git clone server-side, then follows the same passive discovery path.

Creating a new project initializes the chosen directory and optionally Git, but does not immediately create empty canonical documents. The normal flow is to open a conversation, brainstorm/design, and generate meaningful project canonicals when requested.

Projects are identified by stable server-side ProjectID plus repository/source identity signals; filesystem path alone is not identity. A moved or unavailable project remains known and can be relocated.

Removing a project from Wayshard must not silently delete the source directory. Source deletion, if supported at all, is a separate deliberate destructive operation.

## 6. Project knowledge

Wayshard discovers and indexes repository-owned knowledge without imposing its own filename hierarchy on existing projects.

It understands common instruction/document systems, including hierarchical/scoped AGENTS files and widely used tool-specific instruction conventions, README/CONTRIBUTING/SECURITY/docs material, and explicitly declared custom entry points.

Knowledge behavior must preserve:

- document kind and semantic role;
- source path and content hash;
- scope/path applicability;
- instruction family/harness applicability;
- explicit references and authority declarations;
- authority domain such as product behavior, architecture, design, implementation rules, validation, security, or usage.

There is no universal rule that one instruction filename always outranks another ecosystem's filename. Known ecosystems retain their own native scoping/precedence semantics. Explicit project declarations outrank semantic inference.

Potential semantic conflicts may be detected with Jev, but Jev does not decide document authority. Material unresolved conflicts are surfaced when relevant to a task.

Knowledge traversal must tolerate cycles and broken references without infinite recursion.

## 7. Default canonicals for new Wayshard-designed projects

When the user asks to generate canonicals for a new project, the default root set is exactly:

- `AGENTS.md`
- `DESIGN.md`
- `SPEC.md`
- `ARCHITECTURE.md`
- `MEMORY.md`
- `README.md`

`AGENTS.md` acts as implementation entry point and explains the roles of the other five.

Generating or synchronizing the set is a normal isolated Wayshard task. The entire set is planned, written, validated, reviewed, and integrated as one coherent change. Existing meaningful files are updated rather than blindly overwritten.

The project may later reorganize these files; Wayshard follows explicit authority/references rather than permanently enforcing the initial layout.

## 8. Conversations, tasks, and runs

A Conversation is durable project discussion context. A Task represents a concrete user intent. A Run is one attempt to satisfy a Task.

Sending an actionable message may create Message + Task + Run transactionally and return immediately while the server scheduler performs work.

A retry may create a new Run or a new StageAttempt depending on whether the user is retrying the overall task or only an infrastructure attempt within a stage.

New sessions/conversations retain project knowledge and selectively retrievable project history but do not inherit all conversational chatter from older sessions.

Long conversations use durable summaries plus a recent message window and pinned decisions/references. Raw history remains the source for summary regeneration according to retention policy.

## 9. Orchestration workflow

The default semantic workflow is:

```text
USER REQUEST
  -> CONTEXT
  -> ASSESS
  -> PLAN
  -> EXECUTE
  -> VALIDATE
  -> REVIEW
  -> REPAIR / REPLAN / EXPLORE as needed
  -> READY_TO_INTEGRATE
  -> INTEGRATE
  -> COMPLETE
```

Stages are append-only history. StageAttempt records infrastructure attempts and fallbacks without rewriting prior failures.

Read-only stages normally include PLAN, EXPLORE, and REVIEW. Write stages include EXECUTE and REPAIR. VALIDATE is server-controlled execution of objective checks. INTEGRATE is server-controlled publication into the source workspace.

Artifact-only tasks with no source delta may complete without integration.

### 9.1 Plan

Planning produces a PlanArtifact containing a TaskContract with at least:

- objective;
- constraints;
- acceptance criteria;
- expected areas/files when known;
- validation plan;
- risk/assumption notes.

A plan may be rejected as insufficient and sent through EXPLORE and REPLAN before execution.

### 9.2 Execute

Execution receives the original request, accepted plan/task contract, relevant canonicals/knowledge, and isolated run workspace. It modifies only the run workspace and produces an ImplementationReport.

### 9.3 Validate

Wayshard, not executor prose, owns final validation. Validation combines project-mandated checks, tooling/config discovery, changed-area knowledge, and planner task-specific checks.

Validation evidence includes actual command/process results, not only model claims.

### 9.4 Review

Review evaluates semantic correctness against the original request, TaskContract, applicable project authority, run delta, implementation report, and validation evidence.

Review findings use structured severity and evidence. Reviewer prose cannot override deterministic hard failures.

### 9.5 Repair and replan

Repair works from the same lineage/workspace using exact failure/review evidence. Replan preserves prior checkpoints and may continue, partially revert, or reset according to the revised plan.

### 9.6 Completion

Go-owned CompletionPolicy decides among COMPLETE, REPAIR, REPLAN, BLOCKED, or failure outcomes based on acceptance criteria, validation evidence, review findings, budgets/stopping conditions, approvals, and integration state.

For source-changing tasks, `COMPLETE` occurs only after successful integration into the intended source workspace/branch. If the implementation is validated/reviewed but not safely integrated, the run remains `READY_TO_INTEGRATE` or `INTEGRATION_BLOCKED`.

## 10. Jev assessment

Wayshard uses the TypeSafe Jev/System One service as a fast typed judgment layer.

Jev may assess narrow dimensions such as:

- task type;
- scope;
- risk;
- ambiguity;
- reasoning difficulty;
- need for planning, exploration, validation, or review;
- semantic fit among already viable routes;
- whether a plan or review condition appears sufficient.

Jev is not the planner, coding model, deterministic policy engine, arithmetic/budget engine, source of repository truth, or project memory.

Jev receives compact structured decision context rather than indiscriminate raw repository/chat dumps. Raw assessment and resulting RouteDecision are persisted separately and immutably.

If Jev is unavailable after bounded retry/backoff, Wayshard uses a deterministic fallback routing policy and records degraded routing.

## 11. Harness discovery and ACP

Wayshard uses coding harnesses already installed and available to the OS account running the server.

Wayshard ships a versioned TOML catalog of supported harness definitions and automatically discovers installed members of the effective catalog. Users add, override, disable, or remove definitions by editing the user catalog (the CRUD interface); there is no catalog CRUD API or UI editor. A definition describes how Wayshard recognizes, probes, and invokes a harness family; an installation is an actual discovered executable. The catalog is a discovery/launch declaration, not a containment policy: discovered harnesses run as the server OS user with their normal configuration, authentication, environment, filesystem, and network access. Adding an ordinary compatible ACP harness requires catalog TOML, not a code change. A route whose definition is missing from the effective catalog is not routable.

Wayshard never performs harness or bridge installation through package managers, installers, `npx`, shell downloads, privilege escalation, or similar mechanisms.

Discovery may inspect daemon PATH, safely obtainable login-shell paths, well-known user bin directories (including definition-declared home-relative directories), and explicitly configured executable paths. Multiple installations may coexist. Project-controlled executable paths are never searched by default.

A discovered executable is not routable until it passes version/protocol probing and ACP initialization.

Discovery is ordinary local execution as the server OS user: version, ACP-initialize, and login-shell PATH probes run the installed executables normally, with timeouts, bounded output, and best-effort descendant cleanup. Login-shell PATH discovery reads only the shell's required startup files and validates the returned PATH. Probes run only after startup recovery completes. A probe does not attempt to resolve harness-owned authentication, so an auth state that cannot be determined is reported honestly rather than assumed.

Stable ACP v1 is the baseline protocol. Protocol mechanics are isolated behind an ACP driver; harness behavioral differences are declarative catalog properties (`acp`, command interposition, model selection) rather than per-name code. A route whose definition is missing from the effective catalog is not routable.

Optional features such as native session loading, dynamic model selection, extensions, and subagents are never assumed. Native resume is an optimization only; Wayshard reconstructs a new session from durable context/artifacts/workspace when necessary.

ACP stdout is protocol data. Unexpected stdout pollution, malformed frames, oversized frames, capability violations, and process failures are recorded as explicit compatibility/runtime failures.

## 12. Models, providers, and credentials

Harness-managed coding-provider authentication remains with the harness. Wayshard does not scrape or duplicate OpenRouter, OpenAI, Anthropic, Codex, or other harness-owned provider credentials merely because files are accessible.

Wayshard stores credentials only for services the server calls directly, such as Jev. These live in restricted user config files or environment variables (see §19), never in ordinary database fields.

The model catalog is dynamic. Availability can vary by harness installation and project. The automatic routing pool is a user-configured subset of what harnesses expose.

Wayshard may enrich advertised model metadata with context limits, pricing, capabilities, supported effort options, and observed health/latency. Enrichment does not move coding inference away from the harness.

Historical RouteDecision records preserve the model/provider identity and metadata/pricing snapshot available at the time, so later model disappearance does not erase history.

## 13. Routing

Routing is stage-specific.

A route consists of the relevant stage plus a harness installation, optional model selection, optional effort/configuration, timeout/budget, and fallback information.

Routing pipeline:

1. resolve effective configuration;
2. derive stage requirements;
3. enumerate available harness/model combinations;
4. hard-filter impossible or forbidden routes;
5. apply routing policy and user/project preferences;
6. account for harness/provider/model health;
7. optionally ask Jev to compare semantic fit among viable candidates;
8. choose route and explicit infrastructure fallbacks;
9. persist immutable RouteDecision.

If no route satisfies hard requirements, the run becomes BLOCKED with a reason such as `NO_VIABLE_ROUTE`.

Infrastructure failures may trigger bounded retry/fallback. Validation or review failure means repair/replan, not silent quality fallback to another model.

## 14. Workspaces and Git

Every actionable source-changing run receives an isolated run workspace based on an immutable starting snapshot.

The snapshot captures meaningful project state including, where applicable:

- Git HEAD/branch;
- staged tracked changes;
- unstaged tracked changes;
- meaningful non-ignored untracked files;
- file modes and symlink identity;
- canonical/project-knowledge revision.

Agent delta is always calculated from the starting snapshot, so pre-existing dirty changes remain user baseline.

The user may continue using the source working tree, Git CLI, IDEs, or other tools while Wayshard executes. Concurrent source edits are reconciled during integration.

A branch change between task intake and completion prevents silent automatic integration onto the newly checked-out branch. The user can explicitly choose an appropriate target or leave the result pending.

Each write-stage attempt receives a durable pre-attempt checkpoint of the run workspace, identified by a canonical v3 tree digest and associated with that exact attempt. Recovery restores through verify-then-use private staging and an atomic workspace swap, never by re-reading the checkpoint after verification and never from "latest checkpoint for the stage/run". Checkpoint material for a non-terminal run is pinned; terminal-run material becomes reclaimable after retention and is marked reclaimed, which recovery refuses to restore.

Git and filesystem workspace backends are both required so non-Git/empty projects remain supported.

## 15. Integration

Integration is server-controlled and isolated from harnesses.

Wayshard prepares the merge/application in a temporary integration workspace using the run starting snapshot as merge base, run final state as one side, and current source state as the other.

Before publication it revalidates current source state. Publication is journaled with expected before/after hashes, operation, mode, and symlink target. After publication, Wayshard verifies resulting hashes and marks the integration published.

Startup reconciliation of a durable journal classifies every target against the actual source: targets already at the intended after state are recognized, targets still at the before state may have their safe remainder resumed, and a target matching neither state blocks the integration without overwriting the user's file. A journal whose targets all match the intended state is finalized without destructive rewrite.

Wayshard does not promise physically atomic multi-file filesystem publication. It does promise to avoid silent overwrite of divergent source and to detect/reconcile partial publication after interruption.

Concurrent integrations targeting the same concrete source/repository identity are serialized.

Default successful behavior does not auto-stage or auto-commit. Optional auto-commit may include only proved integrated run delta and must never capture unrelated pre-existing index/working-tree changes.

## 16. Changes, files, and terminals

### 16.1 Changes

Changes distinguishes:

- **Run changes** — starting snapshot to run final;
- **Workspace changes** — current source working-tree state.

Wayshard is not intended to become a complete Git GUI. General staging, branch, pull, push, rebase, and similar operations remain available through terminal/external tools unless inherited UI already provides an appropriate capability.

### 16.2 File editing

Graphical clients may browse and edit ordinary text files in the source workspace. File reads used for editing carry a content revision/hash. Save includes the expected hash and fails with a conflict if another editor changed the file in the meantime.

Binary files are viewable as metadata/appropriate previews where supported but are not treated as normal text-editable files.

### 16.3 Terminals

Graphical-client terminal sessions are server-owned PTYs associated with a project/source workspace. Client disconnect or refresh does not automatically kill the terminal. Server restart may terminate PTYs and must report that honestly.

CLI/TUI terminal and editing workflows should follow terminal-appropriate inherited behavior rather than building nested terminal/editor systems solely for parity.

## 17. Validation

Validation command discovery is passive. Wayshard may inspect project instructions and standard configuration files but must not execute repository code just to learn what tests/checks exist.

The ValidationPlan combines mandatory project gates with planner-added task-specific checks. Planner-added checks may increase coverage but cannot silently remove mandatory gates.

Check results support at least PASS, FAIL, WARN, SKIPPED, and BLOCKED, plus explicit NOT VERIFIED semantics for acceptance criteria that cannot actually be proven.

Where practical, Wayshard compares baseline and final validation results to distinguish pre-existing failures, resolved failures, new regressions, and unknown-origin failures.

Large logs are durable objects referenced by structured ValidationArtifact records.

## 18. Execution trust model

Wayshard trusts the OS user running the server. Discovered ACP harnesses and the tools and validation commands they request run as that user with their normal configuration, authentication, environment, filesystem, and network access. There is no separate sandbox or OS containment layer.

Model-generated actions, repository instructions, tool output, and external content are still treated as untrusted inputs to the orchestration decision: the server owns approvals, validation, review, and completion policy, and an agent's claims are never evidence of completion.

ACP terminal/tool callbacks are interposed by the server: a harness does not execute model-generated commands directly, and each stage attempt's process tree is cleaned up best-effort on cancellation, timeout, and shutdown.

Approvals are policy/UX, not OS containment. A denied approval means the protected server operation does not execute; it does not describe an OS sandbox outcome.

External file access normally imports a read-only immutable input snapshot rather than granting arbitrary host paths.

## 19. Credential storage

Wayshard-owned credentials (for example the server identity private key, or a Jev control-plane credential) are stored in restricted user config files rather than an encrypted vault or OS keyring.

- The server identity directory is created `0700` and each credential file is written `0600` (the user-profile ACL on Windows).
- Credentials may also be supplied through environment variables (for example `TYPESAFE_API_KEY` for Jev), which take precedence where applicable.
- No OS keyring or vault unlock is required, so the server runs on headless and minimal systems.
- Normal APIs reveal credential status/metadata, not plaintext values.
- Plaintext secrets are never written to ordinary SQLite fields, logs, artifacts, or diagnostics.

Harness-owned provider credentials remain owned and stored by the harness; Wayshard does not read, scrape, or duplicate them.

Backups exclude Wayshard credentials by default (they live in the config file, not in control-plane backup content).

## 20. Persistence and events

Wayshard uses SQLite as authoritative control-plane storage, with a content-addressed object store for large immutable content and the repository/filesystem as authoritative source state.

SQLite uses appropriate durability settings such as WAL, foreign keys, and busy-timeout behavior. The Go server is the only database writer.

Important control-plane state includes projects, knowledge, conversations, tasks, runs, stages, attempts, assessments, routes, artifacts, workspaces, snapshots/checkpoints, integrations, approvals, devices/sessions, notifications, usage, settings, policy versions, events, and idempotency records.

Domain state mutation and durable event insertion occur in the same SQLite transaction.

Events are a durable change feed, not the primary state store and not full event sourcing.

Large logs, patches, attachments, screenshots, and large artifacts live in the SHA-256 object store.

## 21. Recovery

A stage is never marked complete until its required artifact/workspace state is durable.

Write stages receive a pre-attempt checkpoint. Interrupted write attempts remain immutable history and normally restart from the known-good pre-attempt checkpoint unless a verified native resume path is explicitly safe.

Read-only stages can generally restart against unchanged durable inputs. Validation can rerun against the same checkpoint.

Integration uses a publication journal and source hashes so startup recovery can distinguish complete, incomplete, or externally diverged publication, resume only the safe remainder, and block on unexpected source state without overwriting it.

Server startup reconciles unfinished runs, workspaces, integrations, and publication journals before normal scheduling resumes. It restores interrupted write attempts from their pre-attempt checkpoint and then reconciles publication journals, then reclaims checkpoint material for terminal runs past retention. Harness discovery/probing runs only after recovery completes; the scheduler never starts before recovery completes.

Critical database/object corruption enters safe/recovery mode rather than continuing orchestration.

## 22. Notifications and attention

Wayshard creates durable notifications from meaningful domain events such as:

- approval required;
- manual validation required;
- run complete;
- run blocked;
- run failed;
- integration conflict;
- server/harness attention required.

Unresolved attention and historical read/unread notification state are distinct concepts.

External push delivery is optional. The server remains the authority even when no push provider exists.

## 23. Settings

Configuration scopes include Server, User, Project, Device, and Run override.

Hard server/security constraints cannot be weakened by lower scopes.

Repository-owned knowledge/configuration is distinct from server-local preferences. Changing a Wayshard setting must not silently rewrite repository docs.

Wayshard must not require a repository `.wayshard` config format. A future optional portable format may be introduced only by an explicit product decision, not as an implementation convenience.

## 24. Multi-device behavior

Runs are server-owned and continue if the initiating client closes, disconnects, or is revoked.

Clients maintain a multiplexed live connection, resume from durable event sequence IDs where possible, and perform full snapshot resync when history is insufficient.

State-changing requests use idempotency semantics where appropriate. Conflicting actions from multiple clients resolve through atomic server state transitions; a client never assumes a command succeeded without server acknowledgement.

Offline clients may show cached stale state but cannot simulate offline execution.

## 25. Packaging, updates, and backups

Wayshard-owned components share a release line while tolerating explicitly compatible client/server version skew.

- Web is embedded in Server and therefore version matched.
- Desktop may provision/manage a local Wayshard Server but must not kill active runs just to apply an update.
- Server updates drain/checkpoint active work where practical.
- Android official artifacts are signed with a maintainer-generated JKS/PKCS12 upload key and may be published through GitHub Releases. Wayshard does not require Google Play.
- CLI is distributed as standalone native executables for supported desktop operating systems.

Database migrations ship inside the Go server. Forward migration creates appropriate pre-migration recovery state. Unsupported downgrade does not attempt unsafe automatic reverse migration.

Server backup covers control-plane SQLite state, object store, and relevant configuration. Source repositories are not included by default and the UI/documentation must say so clearly. Wayshard credentials live in restricted config files and are excluded from a control-plane backup by default.

## 26. Public repository, CI/CD, and releases

Wayshard's primary repository is public under the `Wayshard` GitHub organization and licensed MIT.

GitHub Actions is the authoritative official CI/CD environment.

Normal pull-request CI must run without production credentials, paid model calls, or installed external coding harnesses. Deterministic fake ACP harnesses drive orchestration integration tests.

Official release artifacts are created by CI from tagged source and published to GitHub Releases. Release families include:

- Wayshard Server;
- Wayshard CLI;
- Wayshard Desktop;
- Wayshard Android APK;
- checksums, release notes, third-party notices, and other supply-chain metadata appropriate to the release.

Official artifacts are cryptographically signed with maintainer-generated keys. Wayshard does not require a paid Apple Developer account, notarization, a commercial Windows CA, Microsoft/Azure signing, Google Play, or any other external signing account. macOS uses ad-hoc signing (identity `-`); Windows installers use a maintainer self-signed Authenticode certificate; Android uses the maintainer upload keystore; the SHA-256 checksum manifest is signed with a maintainer minisign key. This is Wayshard cryptographic signing, not Apple or Microsoft platform PKI trust. Users may see Gatekeeper and SmartScreen warnings. There is no automatic Tauri updater.

Web ships inside Server rather than as a separate ordinary download.

Third-party coding harnesses and ACP bridges are never bundled or installed by Wayshard releases.

### 26.1 Application icons

`assets/branding/wayshard.png` is the canonical raster source for Wayshard application icons. Web/PWA, Linux, macOS, Windows, and Android releases use derivatives of that same mark at their required sizes and formats. Derivation may add an opaque Wayshard background or safe-area scaling where a platform requires an opaque or maskable icon, but must not redraw, recolor, or replace the mark.

Desktop bundle configuration references committed PNG, ICNS, and ICO derivatives. The Web build publishes favicon, Apple touch, and installable Web-app icon derivatives plus a Web manifest. Because the generated Android project is created during release CI, its launcher and adaptive-icon resources are regenerated from the canonical source immediately after `tauri android init` and before the signed APK build.

## 27. Client implementation foundation (Pass 1E)

The Web, Desktop and Android clients are one shared graphical application adapted
from the imported OpenCode 2 application source, mounted by Web and hosted by
Tauri 2 for Desktop and Android. The production graphical application composition
— application root, Home, project/session layout and sidebar, session page,
titlebar/work-surface navigation, composer region, review, file browser and
terminal panels, run timeline, new-session flow and the narrow/mobile model —
descends from the actual vendored OpenCode application files; Wayshard domain,
data path and server authority are adapted into that composition. Routing uses
the adapted Wayshard router (`clients/gui/src/app/router.tsx`). The CLI/TUI is a
full terminal client adapted from the imported OpenCode terminal foundation
(OpenTUI/Solid), not a readline prompt. All clients use `@wayshard/sdk` and the
Wayshard server's HTTP/JSON and WebSocket APIs; there is no OpenCode runtime, SDK,
API or provider dependency and no OpenCode product branding. Source lineage is
documented and verified by `clients/lineage.manifest.json`,
`docs/client-source-lineage.md` and `clients/gui/src/lineage.test.ts`, which
verify the recorded upstream git blobs against the vendored source and assert
application-level production-import reachability.
