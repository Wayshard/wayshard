# ARCHITECTURE.md

## 1. Architectural thesis

Wayshard is a modular Go control-plane server with four clients and pluggable external coding harnesses reached through ACP. The server owns all durable domain state and orchestration authority. External harnesses perform bounded stage work. Jev supplies fast typed/probabilistic judgments. SQLite plus a content-addressed object store persist control-plane state and large immutable artifacts. Source repositories remain authoritative for source code.

The initial implementation is a modular monolith, not a microservice system.

## 2. Topology

```text
Web Client ───────┐
CLI/TUI ──────────┤
Desktop ──────────┼── HTTP/JSON + WebSocket ──> Wayshard Go Server
Android ──────────┘                               │
                                                 ├─ SQLite
                                                 ├─ Object store
                                                 ├─ Repository/workspace filesystem
                                                 ├─ Jev HTTP API
                                                 └─ ACP harness processes
```

Projects and execution live where the server lives. Clients are control surfaces.

Web assets are embedded/served by the matching Go server build. Desktop may provision/manage a local background server. CLI and remote clients connect to an existing server.

## 3. Repository and source heritage

The primary repository is expected to be `Wayshard/wayshard` under the `Wayshard` GitHub organization.

Selected OpenCode 2 client/TUI source may be imported once into the Wayshard monorepo. The imported source is then treated as Wayshard code subject to required MIT attribution. There is no operational upstream relationship, sync mechanism, runtime dependency, or compatibility target with OpenCode.

## 4. Server module boundaries

A representative modular Go layout:

```text
cmd/server
internal/
  api/
  orchestrator/
  scheduler/
  routing/
  context/
  knowledge/
  artifacts/
  workspace/
  integration/
  harness/
  acp/
  validation/
  process/
  credentials/
  auth/
  events/
  storage/
  project/
  notifications/
  recovery/
```

Exact packages may evolve, but the architectural responsibilities should remain separated.

Application services own domain mutations. HTTP handlers validate/translate commands and do not become domain logic containers.

`context.Context` is used pervasively for cancellation, deadline propagation, and process cleanup.

## 5. Persistence

### 5.1 SQLite

SQLite is the authoritative control-plane database.

Operational requirements:

- WAL mode;
- foreign keys enabled;
- suitable busy timeout;
- durability settings appropriate to local control-plane state;
- Go server is the only database writer;
- migrations are built into the server binary;
- v0.2 is the baseline schema (version 1), squashed into a single `001_init.sql`; a database from a pre-v0.2 release is refused with guidance to remove the old data directory and start fresh, and post-v0.2 changes add numbered forward migrations.

Representative tables/entities:

```text
projects
knowledge_documents
knowledge_sections / edges
conversations
messages
tasks
runs
stages
stage_attempts
assessments
route_decisions
artifacts
attachments
workspaces
workspace_snapshots
workspace_checkpoints
run_deltas
integrations
approvals
devices
auth_sessions
notifications
model_usage
events
idempotency_keys
settings
policy_versions
operational metadata
```

### 5.2 Content-addressed object store

Large immutable content is stored outside SQLite by SHA-256:

```text
data/
  app.db
  objects/sha256/<hash>
  runtime/
```

Examples:

- large logs;
- patches/diffs;
- screenshots;
- uploaded attachments;
- large structured artifacts;
- snapshot-related immutable data.

SQLite stores metadata and object hash references.

### 5.3 Source state

The repository/filesystem is authoritative for source code. SQLite never becomes a hidden mirror that supersedes repository truth.

## 6. Domain state model

Core concepts:

```text
Project
├─ ProjectKnowledge
├─ Conversation
│  └─ Message
└─ Task
   └─ Run
      ├─ Workspace / Snapshot / Checkpoints / Delta / Integration
      ├─ TaskAssessment[]
      └─ Stage[]
         ├─ StageAttempt[]
         ├─ RouteDecision
         ├─ HarnessSession
         └─ Artifact[]
```

Important distinctions:

- Task = user intent.
- Run = one attempt to satisfy a Task.
- Stage = semantic workflow step.
- StageAttempt = infrastructure attempt/fallback within a stage.
- Artifact = durable structured inter-stage output.
- HarnessSession = execution detail, not source of truth.

IDs should be sortable unique IDs such as UUIDv7 for domain entities. Durable event stream ordering uses a monotonic integer sequence.

## 7. Run state and workflow

Representative semantic flow:

```text
INTAKE
  -> ASSESS
  -> PLAN
  -> EXECUTE
  -> VALIDATE
  -> REVIEW
  -> REPAIR / REPLAN / EXPLORE as required
  -> READY_TO_INTEGRATE
  -> INTEGRATING
  -> COMPLETE
```

Additional states include BLOCKED, FAILED, CANCELLED, and INTEGRATION_BLOCKED as appropriate.

For source-changing runs, COMPLETE requires successful integration. Artifact-only runs may complete without integration.

Stages and attempts are append-only. Recovery or retry creates a new attempt rather than mutating historical outcome.

## 8. Scheduler

An internal scheduler in the same Go binary manages:

- queued/active runs;
- global and project/resource concurrency;
- integration serialization by concrete source/repository identity;
- shutdown/drain behavior;
- resource pressure;
- blocked/runnable transitions.

Multiple runs may execute concurrently against independent isolated workspaces even when they belong to the same project. Shared-source publication is serialized.

## 9. API and events

### 9.1 HTTP/JSON

Versioned REST-like HTTP/JSON endpoints handle commands and snapshots. No GraphQL/gRPC dependency is required initially.

State-changing requests use idempotency keys where appropriate, stored with device, request hash, and response/result.

Optimistic revisions/concurrency control protect mutable resources/settings across devices.

### 9.2 WebSocket

One multiplexed WebSocket per server carries live domain events and ephemeral streams.

Clients identify their last durable event sequence and subscribe by server/project/conversation/run. If the required history is still retained, events replay; otherwise the client performs snapshot resync then resumes live events.

### 9.3 Durable versus ephemeral

Durable domain events include stage transitions, artifact creation, approval requests/resolution, integration state, and notifications.

Ephemeral streams include token deltas, terminal character output, and transient progress. Important final logs/results may later be compacted/persisted as objects/artifacts.

### 9.4 Transaction invariant

A durable domain mutation and its durable event insertion occur in the same SQLite transaction.

Events are a change feed, not full event-sourced state.

## 10. Authentication

### 10.1 Server identity

Every server has a stable ServerID and application identity keypair stored as server-owned secret material. Server identity is independent of endpoint URL.

The identity is used at the application pairing layer; it is not a TLS/PKI system.

### 10.2 Pairing

A trusted client or local server administration path creates a short-lived, single-use high-entropy pairing invitation containing the intended endpoint and expected Wayshard server identity/fingerprint data.

A new client connects, verifies the application identity challenge/possession, presents the pairing capability, and receives a unique long-lived device credential.

The server stores credential verifiers/hashes only.

### 10.3 Client storage

Native clients store their device credential in a restricted user config file (directory `0700`, file `0600`; user-profile ACL on Windows) or an environment credential. Web uses authenticated server sessions/cookies suitable to the externally exposed transport.

Device revocation invalidates future API/WebSocket access but does not cancel server-owned runs.

## 11. Networking

Wayshard Server binds to `127.0.0.1` by default and serves HTTP/WebSocket locally.

Remote exposure, TLS certificates, private CAs, VPNs/tailnets, DNS, and reverse proxies belong to the user's networking layer. Users expose the loopback service through infrastructure they control. An advertised pairing URL may differ from the local listening URL.

Tailscale Serve is a documented deployment example, not an architectural dependency.

## 12. Wayshard credentials

Wayshard-owned credentials (the server identity private key, Jev/control-plane credentials, device material) are stored in restricted user config files, kept out of ordinary SQLite fields.

```text
Credential store (server data dir)
  credentials/            directory 0700
    server_identity_private   file 0600
```

- The credential directory is created `0700` and each file is written `0600` (the user-profile ACL on Windows).
- Environment variables (for example `TYPESAFE_API_KEY` for Jev) are accepted and take precedence where applicable.
- The server runs on headless, container, and minimal systems without an external credential service.
- Normal APIs expose credential status/metadata, never plaintext values.
- Plaintext secrets are never written to ordinary SQLite fields, logs, artifacts, or diagnostics.

Harness-owned provider credentials stay with the harness. Wayshard does not read, scrape, or duplicate them.

Backups exclude Wayshard credentials by default: they are local config, not control-plane backup content.

## 13. Harness catalog and discovery

The supported-harness inventory is configuration, not code. Two levels are kept distinct:

```text
HarnessDefinition (declarative catalog knowledge; never asserts an installation exists)
  identity:  id, display name, homepage, enabled, platforms
  discovery: executable aliases, bridge aliases, well-known home-relative dirs, version args
  ACP:       native|bridge, acp args, bridge args, command interposition, model selection

HarnessInstallation (an actual discovered installation)
  definition id + source (shipped|user|overridden)
  resolved executable path(s), bridge path and presence
  version + version-probe result
  ACP result, negotiated capabilities, auth status, resume
  route viability, blocking reason
```

A versioned TOML catalog ships embedded in the binaries (`internal/harness/harnesses.toml`, `schema_version = 1`). A user catalog at the platform config path (`$XDG_CONFIG_HOME/wayshard/harnesses.toml`, with the normal per-platform fallbacks) overrides or extends it. Merge is by stable `id`: user fields override shipped fields by key (arrays replace), `enabled = false` disables a shipped definition, deleting the override restores it, a user-only id creates a custom definition, and duplicate ids within one source are rejected. One invalid entry is isolated so it cannot destroy otherwise-valid definitions; malformed TOML and unsupported future schema versions are rejected with diagnostics. The catalog is loaded at server startup; a restart applies changes (there is no file-watcher subsystem).

**The catalog is a discovery/launch declaration.** Discovered harnesses run as the Wayshard server OS user with their normal configuration, authentication, environment, filesystem, and network access. Executable aliases must be bare names and never package-runner launchers (`npx`, `npm`, `bunx`, …); well-known discovery dirs must be home-relative with no traversal; unknown fields or unsupported enum values fail the entry closed. A generous, bounded limit caps catalog bytes, definition count, per-definition list/argument counts and glob matches.

**Installations are reconciled, not accumulated.** Each discovered installation persists the effective definition's *execution fingerprint* (a deterministic SHA-256 over the fields that materially change what is launched: executables, bridges, well-known discovery, version/ACP/bridge args, native-vs-bridge mode, command-interposition behavior, model selection, platforms). A discovered installation is only routable while the current effective catalog still has an enabled definition with that fingerprint. Refresh atomically replaces the persisted installation set with the latest discovery result, so a disabled, removed or materially changed definition (or a disappeared executable/bridge) cannot leave a stale routable candidate.

Discovery searches the daemon PATH, safely obtained login-shell PATH, Wayshard's well-known bin dirs plus each definition's declared home-relative well-known dirs, and explicit configured paths. It never searches arbitrary project-controlled paths, never installs or updates a harness or bridge, and resolves symlinks and deduplicates physical installations. A well-known search root that is a symlink escaping HOME is dropped, while a resolved executable that is a symlink (for example an NVM shim) is still discovered. Adding an ordinary compatible ACP harness is a TOML addition, not a Go change. For a bridge definition the CLI and bridge are reported independently: a present CLI whose bridge is missing is reported as present with a specific blocking reason, not "harness not installed".

Executable name alone is insufficient. A usable installation must pass launch/version and ACP initialization/capability probing. When a harness is a script/symlink, discovery resolves a narrow launch closure (its package tree plus a PATH-resolved interpreter) so Node-based ACP adapters start with the interpreter on PATH; this is an environment convenience, not a filesystem boundary.

Version and ACP-initialize probes are ordinary local execution as the server OS user: a timeout, bounded combined output, and best-effort process-tree cleanup. Login-shell PATH discovery reads only the shell's required startup files and validates the returned PATH as an absolute-only, deduplicated, bounded list; it is never executed as a command. Discovery/probing runs only after startup recovery completes. A probe does not attempt to resolve harness-owned authentication, so an auth state that cannot be determined is reported honestly rather than assumed.

Filesystem state is authoritative; discovered state is cached in SQLite for UI/history.

Wayshard never installs a harness or bridge.

## 14. ACP architecture

### 14.1 Driver versus adapter

```text
Orchestrator
  -> HarnessAdapter
       -> ProtocolDriver (ACPV1 initially)
            -> ACP process
```

The protocol driver owns JSON-RPC framing, initialize negotiation, session lifecycle, cancellation, permissions, filesystem/terminal callbacks, and extension-safe parsing.

Harness behavior is resolved from the catalog definition (`acp`, `interpose_commands`, `model_selection`) rather than from per-name Go adapters; a route whose definition is missing from the effective catalog is not routable. The protocol driver remains the only ACP-speaking component.

### 14.2 Compatibility

Stable ACP v1 is the baseline. The internal protocol-driver interface allows future ACP versions without changing the orchestrator.

Capability negotiation is authoritative. Optional behavior is never inferred solely from harness name.

Operational compatibility may be classified internally as incompatible, core, routable, or enhanced based on actual abilities rather than marketing labels.

### 14.3 Process lifecycle

A simple default is one ACP process per logical harness session. Process pooling/multiplexing may be added as an optimization, not a correctness assumption.

ACP stdout is protocol-only; stderr is diagnostic. Frame size, pending request count, and buffer limits prevent unbounded memory use.

ACP terminal/tool callbacks (`terminal/create`, `terminal/output`, `terminal/wait_for_exit`, `terminal/kill`, `terminal/release`) are interposed by Wayshard: a harness never executes model-generated commands directly. The Tool manager runs each command as the server OS user with the server environment, defaulting the working directory to the run workspace and owning the resulting process tree.

The ACP process runs in its own process group so cancellation, timeout, and shutdown terminate the process tree best-effort (SIGTERM then SIGKILL on Unix; direct kill on Windows). Cleanup is executor hygiene: durability lives in server state rather than process lifetime, so durable correctness never depends on killing a process.

Unknown extension metadata/methods are tolerated according to ACP semantics. Known useful extensions may be adapter-specific but are not required by core orchestration.

### 14.4 Session continuity

Installations report session recovery capability such as none/reconstruct/native-resume from negotiated capabilities. Correctness depends on server-owned durable context, artifacts, and workspace state, not hidden ACP memory.

## 15. Artifact protocol

Wayshard defines its own stage artifact schemas independent of ACP.

Submission hierarchy:

1. native structured mechanism/negotiated extension when suitable;
2. structured tool/submission channel when supported;
3. structured final-response contract as universal fallback.

Server validates artifact schema. Invalid output gets a small bounded correction attempt in the same stage/session when practical; persistent invalid output becomes a structured stage-output failure.

Workspace state is authoritative for actual code changes; ImplementationReport summarizes rather than replacing it.

## 16. Jev integration

A `DecisionEngine` interface isolates the Jev HTTP API from the orchestrator.

The server sends compact decision context composed primarily of structured signals, typically well below model context limits. Repository raw text is minimized because repository content may be adversarial or semantically noisy.

A TaskAssessment is immutable and stores dimension values/probabilities/confidence, Jev model/version, question-set/policy version, input/usage metadata, and timestamp.

RouteDecision is separately immutable so historical assessments can be replayed against newer routing policy.

Transient rate/overload failures use bounded retry/backoff. Persistent unavailability activates deterministic fallback routing.

## 17. Routing architecture

Separate registries model:

- Harness installations;
- advertised models/model availability;
- provider metadata;
- route profiles/preferences;
- policy versions.

Effective configuration resolves built-in defaults, server, user, project, and run override scopes subject to hard upper constraints.

Routing flow:

```text
resolve config
-> derive stage requirements
-> enumerate harness/model candidates
-> hard capability/budget/health filtering
-> policy preferences
-> optional Jev semantic comparison
-> select route
-> attach explicit infrastructure fallback chain
-> persist RouteDecision
```

Jev never chooses from impossible candidates.

Routes may be dynamic-model or fixed-harness-default routes. Effort/reasoning options belong to the route where supported.

## 18. Model metadata and usage

Model availability is discovered from harnesses and can be project-specific.

A Model Metadata Service may enrich advertised model IDs with current capability/pricing data and observed health/latency. Metadata has provenance.

Actual usage/cost reported by the harness/provider is authoritative. Otherwise Wayshard may calculate clearly labeled estimates using the pricing snapshot applicable to the run.

Historical route records preserve identifiers/display metadata even after a model disappears.

## 19. Context Engine

The Context Engine accepts a ContextRequest containing project/task/run/stage/target/token budget/referenced paths and returns a structured ContextBundle plus ContextManifest.

Inputs include:

- current request and run constraints;
- applicable instructions/knowledge;
- conversation summary and recent window;
- selected durable run artifacts;
- structural repository hints;
- fresh workspace/Git facts.

Targets include Jev, planner, executor, reviewer, repair, and explore contexts.

Project canonicals/instructions use deterministic authority/scope resolution first; lexical/structural retrieval adds relevant supporting material. A RepositoryRetriever abstraction permits future semantic/hybrid retrieval without making a vector database mandatory.

Context items record provenance, reason, authority, hash/revision, and delivery mode. Native harness context capabilities are used to avoid duplicated injection while critical new context remains explicit.

## 20. Knowledge Engine

Knowledge files are parsed structurally by sections/headings where useful. Knowledge records include kind, family, scope, source, references, authority domain, hash, and status.

Discovery follows known conventions plus explicit references such as Markdown links or declared entry points. Explicit declaration outranks filename/content inference.

The graph supports cycle detection, visited-set/bounded traversal, broken-reference reporting, and filesystem watch/reparse.

Repository files remain source of truth; SQLite stores index/classification/relationship metadata.

## 21. Workspace architecture

A common workspace abstraction supports Git and non-Git projects:

```text
WorkspaceBackend
  GitWorkspaceBackend
  FilesystemWorkspaceBackend
```

### 21.1 Starting snapshot

At actionable intake, capture exact meaningful source state including branch/HEAD, staged/unstaged tracked changes, meaningful untracked files, modes/symlinks, and project knowledge revision.

### 21.2 Isolated run workspace

Materialize HEAD/base plus user dirty baseline into a separate run workspace. Agents never write directly to the source workspace.

Each write-stage attempt receives a durable pre-attempt checkpoint: a materialized filesystem tree with a canonical, length-prefixed v3 digest over path, entry type, executable bit, content, symlink target, and directory membership. Recovery uses verify-then-use staging: the checkpoint is copied into private trusted staging, the staged tree is hashed and compared against the persisted digest, and only then is it materialized into the run workspace through an atomic sibling-temp swap. Checkpoint lookup is scoped to the exact interrupted attempt, never to "latest checkpoint for the stage/run". Checkpoint material for a non-terminal run is pinned; terminal-run material becomes reclaimable after retention and is marked `material_state=reclaimed`, which recovery refuses.

Internal checkpoints record known-good stage boundaries (S0, S1, S2, ...), potentially using hidden Git objects/refs rather than user-visible commits.

### 21.3 User activity

The source workspace may change concurrently due to terminal, IDE, external Git client, or another Wayshard integration. Active runs remain based on their immutable starting snapshot.

## 22. Integration architecture

Integration performs a three-way reconciliation:

```text
base = run starting snapshot
ours/current = current source workspace
run = validated/reviewed run final state
```

Prepare in a temporary integration workspace. If conflicts exist, source remains untouched and run becomes integration blocked.

Before publication, verify source has not changed since preparation. Publication writes are journaled with file-level before/after hashes, op, mode and symlink target, and verified afterward. Publication writes resolve their target through a containment check so a symlinked parent cannot redirect a write outside the source tree.

Startup reconciles a durable journal by classifying every target against the actual source: targets already at the intended after state are recognized, targets still at the before state may have their safe remainder resumed, and a target matching neither state blocks the integration without overwriting the user's file. A journal whose targets all match the intended state is finalized without destructive rewrite. Reconciliation finalizes the run/integration or marks it integration-blocked, emits `publication.reconciled`, and never claims universal physical atomicity for multi-file publication.

## 23. File editing concurrency

Graphical source-file reads return content plus revision/hash. Save is compare-and-set:

```text
write(path, expectedHash, newContent)
```

If the current hash differs, return a conflict rather than overwriting external edits.

## 24. Terminal architecture

Graphical terminal sessions are server-owned PTYs associated with project/source workspaces and authenticated clients. Clients attach/detach over WebSocket streams.

PTY identity and lifecycle are durable enough for reconnect while the server process remains alive, but server restart is not required to preserve a live PTY.

Agent/tool processes are separate from user terminals and follow run/stage cancellation and lifecycle rules.

## 25. Validation architecture

ValidationDiscovery inspects project instructions and common configuration formats passively.

ValidationPlan combines mandatory project gates and task-specific planner checks.

Validators may include command/test/build/lint/typecheck/file/Git/documentation classes, with future browser/API validators behind interfaces.

A ValidationCheck records command/kind, required flag, baseline/final status, exit code, duration, summary, environment/workspace fingerprint, and durable log reference.

ValidationArtifact aggregates check outcomes, baseline comparison, blocking failures, warnings, unverified criteria, and evidence references.

Baseline execution is selective based on cost/risk; unknown failure origin is represented honestly.

## 26. Completion and review policy

ReviewArtifact contains verdict, acceptance-criterion evidence, and findings with concrete severity/location/explanation/required fix.

CompletionPolicy is deterministic Go logic over TaskContract, ValidationArtifact, ReviewArtifact, unresolved approvals, budgets/stopping conditions, and integration state.

Hard deterministic gates cannot be overridden by model prose.

## 27. Execution trust model

### 27.1 Trust boundary

Wayshard trusts the OS user running the server. Discovered ACP harnesses, the tools they request, validation commands, and discovery probes all run as that user with their normal configuration, authentication, environment, filesystem, and network access. Approvals, budgets, and routing gate which server operations proceed; they are policy, not OS containment.

Model-generated actions, repository text, tool output, and external content remain untrusted *decision inputs*: the server owns approvals, validation, review, and completion policy. An agent saying it is done is never evidence of completion.

### 27.2 Approvals are policy, not containment

Approvals pause a requested server operation until a client resolves it. A denied approval means the protected operation does not execute. Approvals do not describe an OS sandbox outcome and are not a security boundary against the harness itself (the harness already has the server OS user's full authority).

### 27.3 What Wayshard still controls

- **Isolated run workspaces.** Every run executes against a run workspace materialized from the exact task-start snapshot; agents never experiment directly in the source working tree.
- **Server-owned validation and completion.** Mandatory project checks are discovered passively, run as the server OS user, and cannot be removed by an agent. Validation records command, working directory, exit status, duration, and durable evidence.
- **Run-delta provenance and conflict-safe integration.** Integration uses the immutable run start as merge base and the current source state as the third side; pre-existing user changes are baseline, never agent output.
- **Best-effort process cleanup.** ACP processes and ACP-requested tool commands run in their own process group and are terminated best-effort on cancellation, timeout, and shutdown.
- **Approvals, budgets, and routing.** These are server policy controls and remain authoritative.

### 27.4 External inputs

Approved external files normally become immutable read-only snapshots inside the run workspace/object store. Direct external writes should be rare server-controlled publication operations.

## 28. Network access

Harness, tool, and validation processes use the server OS user's normal network for all traffic, including model/provider control traffic. Wayshard adds no network policy layer; to bound which hosts a harness or tool can reach, use OS or network controls such as a firewall, a container, or Tailscale ACLs.

## 29. Recovery architecture

Every write-stage attempt starts from a durable pre-attempt checkpoint. Interrupted attempts remain immutable history.

Default recovery of an interrupted write attempt:

1. preserve enough interrupted state for diagnostics;
2. restore the verified pre-attempt checkpoint (attempt-scoped, verify-then-use staging);
3. create a new StageAttempt.

Read-only stages generally restart. Validation reruns. Native harness session resume is used only when explicitly supported and safe.

Server startup RecoveryManager:

1. open/verify storage and apply migrations;
2. repair terminal-run consistency (a cancelled run's running attempts become cancelled; a terminal run's leftover pending approvals are invalidated);
3. remove unreferenced checkpoint debris and restore-staging leftovers;
4. restore interrupted write attempts from their verified pre-attempt checkpoint;
5. reconcile publication journals (recognize already-published targets, resume the safe remainder, or block on unexpected source state) and finalize run/integration state;
6. clean disposable validation workspaces;
7. reclaim checkpoint material for terminal runs past retention.

The scheduler starts only after this recovery completes. Harness discovery/probing runs only after recovery; no run is scheduled before recovery finishes.

## 30. Process lifecycle

Launched harness/tool/validation/probe processes run as the server OS user. On Unix they run in their own process group so the whole group can be terminated together; on Windows the direct child is terminated. Cancellation, timeout, and shutdown terminate processes best-effort.

Cleanup is best-effort by design. Durability lives in server state rather than process lifetime, so durable correctness never depends on killing a process. Recovery reconciles durable state (attempts, checkpoints, runs, journals) rather than process trees.

Run cancellation is centralized in `ProcessRun`: when the run context is canceled, or a cancellable operation reports a cancellation error (for example a status read or storage call that observes the canceled context), the engine performs the durable cancellation transition (run, running attempts/stages, pending approvals) before returning. A canceled run therefore always converges to a terminal CANCELLED state, independent of best-effort process cleanup.

Graceful shutdown stops new work, drains/checkpoints active runs where practical, marks interrupted attempts accurately, and then terminates managed processes best-effort.

## 31. Failure taxonomy

Representative categories:

- infrastructure: harness crash, provider error, rate limit, network error, server restart, timeout, disk full;
- task: validation failure, review rejection, impossible requirement, unresolved integration conflict;
- policy: permission denied, budget exceeded, forbidden action;
- user: cancellation, changed requirements;
- data: missing workspace, corrupted artifact, database error, inconsistent state.

Only infrastructure failures normally trigger automatic retry/fallback. Task failures drive repair/replan. Policy/user outcomes require corresponding state/action rather than blind retry.

## 32. Observability

Observability includes:

- structured local logs;
- metrics;
- optional traces/export;
- durable domain history.

Structured log fields include project/run/stage/attempt identity, harness/model/provider, component, event, error kind, and duration. Secrets are redacted before persistence/streaming.

Advanced diagnostics can export a sanitized bundle containing server/platform versions, schema version, route/capability/error metadata, harness versions, and sanitized logs without project source/secrets by default.

## 33. Notifications

The server derives durable notifications/attention from domain state/events. Clients choose presentation and optional native/push delivery.

## 34. Backups and storage GC

A backup captures a consistent SQLite snapshot plus all object-store content referenced by that snapshot and relevant configuration. Repositories are excluded by default. Wayshard credentials live in restricted config files and are excluded from a control-plane backup by default.

Object GC is mark-and-sweep based on durable references plus active run/backup pins. Workspace cleanup follows explicit retention policy rather than object age alone.

Low disk space can suspend new write-heavy runs before critical failure.

## 35. Packaging and version compatibility

Wayshard-owned components share a product release line:

- Server;
- Web embedded in Server;
- CLI;
- Desktop;
- Android.

Clients/server expose product/API compatibility metadata so compatible version skew can operate during rolling upgrades.

Desktop may manage the local user-level server installation but does not couple server lifetime to window lifetime.

## 36. CI/CD architecture

GitHub Actions is the authoritative official CI/CD path.

Normal CI covers server/storage/routing/context/workspace/fake-ACP/client/build behavior without requiring external harness installations or production credentials. A dedicated `native-execution` job runs the deterministic fake harness through discovery, ACP initialize, the full PLAN/EXECUTE/VALIDATE/REVIEW/INTEGRATE/COMPLETE path, routing, cancellation, and probe/handshake timeouts on Linux, macOS, and Windows.

The fake ACP harness simulates deterministic success, permissions, crashes, malformed protocol, ignored cancellation, invalid artifact output, provider/config errors, and recovery flows.

Real OpenCode/Codex compatibility jobs are optional/scheduled/controlled jobs in deliberately provisioned environments.

Tagged releases produce official Server, CLI, Desktop, and Android artifacts, checksums, third-party notices, and appropriate supply-chain metadata through GitHub Releases.

Private signing material is generated by the Wayshard maintainer and stored only in the protected GitHub Environment `release`. Pull-request CI remains secret-free. Forks never receive those credentials. Publishing uses the automatic `GITHUB_TOKEN`.

Signing mechanics:

- Android: maintainer JKS/PKCS12 materialized in `$RUNNER_TEMP`, Gradle-signed APK, verified before publish.
- macOS: Tauri ad-hoc signing with identity `-`. No Apple Developer ID, notarization, or Apple API keys.
- Windows: maintainer password-protected PFX materialized in `$RUNNER_TEMP`, Authenticode on published `.msi`/`.exe`, verified before publish. No commercial CA or Azure signing.
- Linux desktop and Go binaries: SHA-256 checksums (and the signed checksum manifest). No paid Linux signing service.
- Combined `SHA256SUMS.txt` is signed with a maintainer minisign key; the public key is committed under `keys/`.
- No Tauri updater plugin or updater signing keys.

Wayshard-signed is not the same as trusted by Apple or Microsoft platform PKI.

## Client architecture

```text
clients/
  sdk/       @wayshard/sdk        domain types + HTTP/JSON + WebSocket events + PTY
  ui/        @wayshard/ui         OpenCode 2 design system, copied+adapted (rebranded)
  gui/       @wayshard/gui        shared graphical client
             src/session-ui  OpenCode 2 session presentation, copied+adapted
             src/app         OpenCode 2 application composition, adapted:
                             app.tsx (root+routes), pages/home, pages/layout
                             (+ sidebar shell/project/items/mobile), session-page,
                             components/titlebar, components/composer-region,
                             pages/session/{review-tab,file-tabs,terminal-panel-v2,
                             timeline}, pages/new-session; router.tsx (adapted
                             Wayshard router)
             src/wayshard    Wayshard domain state, adapter, event subscription
  web/       Vite host mounting @wayshard/gui (served by the Wayshard Server)
  desktop/   Tauri 2 shell hosting ../../web/dist (Linux/macOS/Windows + Android)
  tui/       OpenTUI/Solid terminal client (imported theme system)
```

The graphical client is one application whose application composition descends
from the imported OpenCode application source; Web, Desktop and Android differ
only by platform adapters (window chrome, clipboard, notifications, external
open, mobile-safe layout). The TUI is a separate terminal-appropriate client
built from the imported OpenCode terminal foundation. All clients speak only
Wayshard domain HTTP/JSON and WebSocket APIs; none speaks ACP or OpenCode. Live
updates use the durable event stream (`/v1/ws`) with reconnect and polling only
as a narrow fallback.

Application-icon assets have one repository-owned source of truth:
`assets/branding/wayshard.png`. Committed desktop derivatives live in
`clients/desktop/src-tauri/icons` and are selected by `tauri.conf.json` for
Linux PNG, macOS ICNS, and Windows ICO packaging. Web favicon, Apple touch, and
PWA `any`/`maskable` derivatives live in `clients/web/public`; Vite copies that
directory into the embedded Web distribution and `site.webmanifest` declares
the installable icons. Android is initialized into an ephemeral
`src-tauri/gen/android` tree during release CI, so the workflow runs
`scripts/release/android-icons.sh` immediately after initialization. That script
drives Tauri's icon generator from `src-tauri/app-icon.json`, which points at the
canonical mark for desktop/browser use and at
`assets/branding/wayshard-android-fg.png` — a safe-area-padded derivative — for
both the adaptive foreground and the monochrome mask, with `bg_color` supplying
the deep background. `@tauri-apps/cli` is pinned to the same major.minor as the
locked `tauri` Rust crate, so the mobile template and the icon manifest are
generated by a matching toolchain.
`clients/web/public/social-share.png` (the canonical mark on the deep background
with the wordmark) backs the Web `og:image`/`twitter:image` metadata and is
asserted in the embedded Web distribution.
