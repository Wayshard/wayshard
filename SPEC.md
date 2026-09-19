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
- sandbox policy, validation, review/completion policy, recovery, and storage;
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
- native clients use platform-appropriate secure local storage;
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

Wayshard never performs harness or bridge installation through package managers, installers, `npx`, shell downloads, privilege escalation, or similar mechanisms.

Discovery may inspect daemon PATH, safely obtainable login-shell paths, well-known user bin directories, and explicitly configured executable paths. Multiple installations may coexist.

A discovered executable is not routable until it passes version/protocol probing and ACP initialization.

Stable ACP v1 is the baseline protocol. Protocol mechanics are isolated behind an ACP driver; harness-specific quirks belong in harness adapters. Unknown but compliant agents use a Generic ACP adapter.

Optional features such as native session loading, dynamic model selection, extensions, and subagents are never assumed. Native resume is an optimization only; Wayshard reconstructs a new session from durable context/artifacts/workspace when necessary.

ACP stdout is protocol data. Unexpected stdout pollution, malformed frames, oversized frames, capability violations, and process failures are recorded as explicit compatibility/runtime failures.

## 12. Models, providers, and credentials

Harness-managed coding-provider authentication remains with the harness. Wayshard does not scrape or duplicate OpenRouter, OpenAI, Anthropic, Codex, or other harness-owned provider credentials merely because files are accessible.

Wayshard stores credentials only for services the server calls directly, such as Jev, plus explicitly server-managed integrations.

The model catalog is dynamic. Availability can vary by harness installation and project. The automatic routing pool is a user-configured subset of what harnesses expose.

Wayshard may enrich advertised model metadata with context limits, pricing, capabilities, supported effort options, and observed health/latency. Enrichment does not move coding inference away from the harness.

Historical RouteDecision records preserve the model/provider identity and metadata/pricing snapshot available at the time, so later model disappearance does not erase history.

## 13. Routing

Routing is stage-specific.

A route consists of the relevant stage plus a harness installation, optional model selection, optional effort/configuration, security requirements, timeout/budget, and fallback information.

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

Git and filesystem workspace backends are both required so non-Git/empty projects remain supported.

## 15. Integration

Integration is server-controlled and isolated from harnesses.

Wayshard prepares the merge/application in a temporary integration workspace using the run starting snapshot as merge base, run final state as one side, and current source state as the other.

Before publication it revalidates current source state. Publication is journaled with expected before/after hashes. After publication, Wayshard verifies resulting hashes and marks the integration published.

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

## 18. Security and sandboxing

Wayshard treats model-generated actions, repository instructions, tool output, and external content as untrusted.

Sandboxing is layered:

- **Harness Sandbox** constrains the ACP harness itself while permitting its required provider/config access.
- **Tool Sandbox** provides stricter isolation for model-generated commands where the harness/integration supports native or adapter-mediated separation.

Effective tool-execution isolation may be `native`, `adapter_bridge`, or `outer_only` and is part of route capability/health.

A common SandboxPolicy describes filesystem roots, environment, network policy, process/IPC restrictions, resource limits, and scoped secret leases. Platform backends compile that policy into suitable Linux, macOS, and Windows mechanisms.

Failure to establish required containment never silently becomes unrestricted execution. Reduced/unsafe modes, if exposed, are explicit advanced policy choices.

Tool network is denied or brokered by default according to project/run policy. Harness/provider control network is distinct from tool-command network.

External file access normally imports a read-only immutable input snapshot rather than widening sandbox access to arbitrary host paths.

## 19. Secret storage

Wayshard-owned secrets are stored in a versioned encrypted SecretVault, not ordinary plaintext SQLite fields.

A platform-neutral VaultKeyProvider supports native platform storage where practical and headless Linux alternatives such as externally supplied credential material, manual passphrase unlock, or an explicitly protected local key file.

Failure to unlock the existing vault never causes creation of a replacement vault or loss of encrypted state.

Normal APIs reveal secret status/metadata, not plaintext values.

Backups exclude secrets by default. An explicit secret-inclusive backup re-encrypts portable secret material under backup-specific protection rather than copying machine-bound keychain material.

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

Integration uses a publication journal and source hashes so startup recovery can distinguish complete, incomplete, or externally diverged publication.

Server startup reconciles unfinished runs, workspaces, integrations, and orphan processes before normal scheduling resumes.

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

Server backup covers control-plane SQLite state, object store, configuration, and optionally encrypted portable secrets. Source repositories are not included by default and the UI/documentation must say so clearly.

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
