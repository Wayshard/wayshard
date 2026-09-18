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
  sandbox/
  secrets/
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
- migrations are built into the server binary.

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

Native clients use platform secure storage. Web uses authenticated server sessions/cookies suitable to the externally exposed transport.

Device revocation invalidates future API/WebSocket access but does not cancel server-owned runs.

## 11. Networking

Wayshard Server binds to `127.0.0.1` by default and serves HTTP/WebSocket locally.

Wayshard deliberately does not implement:

- certificate issuance/renewal;
- private CA management;
- VPN/tailnet operation;
- DNS;
- public remote exposure;
- reverse-proxy management.

Users expose the loopback service through infrastructure they control. An advertised pairing URL may differ from the local listening URL.

Tailscale Serve is a documented deployment example, not an architectural dependency.

## 12. SecretVault

Wayshard-owned secrets use a versioned encrypted vault separate from ordinary SQLite fields.

Conceptual structure:

```text
SecretVault
  -> encrypted secret values
  -> Vault Data Key
  -> wrapped by VaultKeyProvider
```

VaultKeyProvider implementations may include:

- macOS native secure storage;
- Windows native protected storage;
- Linux desktop Secret Service when available;
- external credential material for headless/service deployments;
- manual passphrase unlock;
- explicitly protected local key-file fallback.

Envelope encryption allows wrapping-key/provider migration without re-encrypting every stored secret.

The vault stores server identity private material, Jev/control-plane credentials, and other Wayshard-owned secrets. Harness-owned provider credentials stay with the harness.

If the vault cannot be unlocked, the server enters a restricted locked/recovery state rather than replacing the vault.

## 13. Harness discovery

The discovery service maintains two levels:

```text
HarnessDefinition
  display name
  executable hints
  launch recipe
  known adapter/runtime profile

HarnessInstallation
  definition/custom ID
  executable path
  executable version
  ACP protocol/capabilities
  auth status
  advertised configuration/models
  health
  compatibility observations
```

Discovery sources may include daemon PATH, safely obtained login-shell PATH, well-known user bin directories, and explicit configured paths.

Executable name alone is insufficient. A usable installation must pass launch/version and ACP initialization/capability probing.

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

Known harness adapters map OpenCode/Codex-specific configuration and behavior into Wayshard capabilities. Generic ACP agents use a GenericACPAdapter.

### 14.2 Compatibility

Stable ACP v1 is the baseline. The internal protocol-driver interface allows future ACP versions without changing the orchestrator.

Capability negotiation is authoritative. Optional behavior is never inferred solely from harness name.

Operational compatibility may be classified internally as incompatible, core, routable, or enhanced based on actual abilities rather than marketing labels.

### 14.3 Process lifecycle

A simple default is one ACP process per logical harness session. Process pooling/multiplexing may be added as an optimization, not a correctness assumption.

ACP stdout is protocol-only; stderr is diagnostic. Frame size, pending request count, and buffer limits prevent unbounded memory use.

Unknown extension metadata/methods are tolerated according to ACP semantics. Known useful extensions may be adapter-specific but are not required by core orchestration.

### 14.4 Session continuity

Adapters report session recovery capability such as none/reconstruct/native-resume. Correctness depends on server-owned durable context, artifacts, and workspace state, not hidden ACP memory.

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
-> hard capability/security/budget/health filtering
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

Before publication, verify source has not changed since preparation. Publication writes are journaled with file-level before/after hashes and verified afterward.

Crash recovery uses the journal to classify completed, incomplete, or externally diverged publication.

Do not claim universal physical atomicity for multi-file publication.

## 23. File editing concurrency

Graphical source-file reads return content plus revision/hash. Save is compare-and-set:

```text
write(path, expectedHash, newContent)
```

If the current hash differs, return a conflict rather than overwriting external edits.

## 24. Terminal architecture

Graphical terminal sessions are server-owned PTYs associated with project/source workspaces and authenticated clients. Clients attach/detach over WebSocket streams.

PTY identity and lifecycle are durable enough for reconnect while the server process remains alive, but server restart is not required to preserve a live PTY.

Agent/tool processes are separate from user terminals and follow run/stage cancellation/sandbox rules.

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

## 27. Sandbox architecture

### 27.1 Trust model

Trust the Wayshard server/sandbox implementation and installed harness executable within enforced limits. Do not trust model-generated actions, repository text, tool output, or external content.

### 27.2 Two security domains

```text
Outer Harness Sandbox
  ACP harness
  harness config/auth
  provider/model control network

Tool Sandbox
  model-generated commands
  run workspace
  approved caches/toolchains
  denied/brokered network
  scoped secret leases
```

Adapters report tool-execution isolation mode:

- `native` — harness provides enforceable inner tool sandboxing configurable/verified by Wayshard;
- `adapter_bridge` — Wayshard interposes command execution and runs it in ToolSandbox;
- `outer_only` — no separate inner command domain; effective capability is weaker and reported honestly.

No failed setup silently becomes unsandboxed execution.

### 27.3 Common SandboxPolicy

Platform-neutral policy describes:

- read-only/read-write/denied roots;
- synthetic HOME/TEMP and allowed environment;
- network none/allowlist/unrestricted as policy permits;
- process/IPC/GUI restrictions;
- memory/CPU/wall-time/process/output/disk limits;
- scoped secret leases.

Platform backends compile the policy.

### 27.4 Platform backends

Implementation should use appropriate supported OS primitives and runtime capability probing.

Linux may combine filesystem/process/network/resource primitives such as Landlock/namespaces/no-new-privs/seccomp/cgroups as available.

macOS may use the practical platform sandbox mechanisms available to the process, with runtime probing and no silent fallback.

Windows may use AppContainer/restricted token/process mitigations/Job Objects and newer sandbox APIs when available and proven by conformance tests.

The architecture intentionally avoids requiring Docker.

### 27.5 External inputs

Approved external files normally become immutable read-only snapshots inside the run workspace/object store. Direct external writes should be rare server-controlled publication operations.

## 28. Network separation

Harness control-plane network (for example OpenCode -> provider) is distinct from tool-command network (for example `npm` -> registry).

Tool network is denied by default or mediated through a broker/policy that can grant destinations/scopes after approval. Network activity can be audited by stage/attempt/approval.

## 29. Recovery architecture

Every write-stage attempt starts from a durable pre-attempt checkpoint. Interrupted attempts remain immutable history.

Default recovery of an interrupted write attempt:

1. preserve enough interrupted state for diagnostics;
2. restore known-good pre-attempt checkpoint;
3. create a new StageAttempt.

Read-only stages generally restart. Validation reruns. Native harness session resume is used only when explicitly supported and safe.

Server startup RecoveryManager:

1. open/verify storage;
2. reconcile active runs/stages;
3. reconcile workspaces/checkpoints;
4. reconcile integrations/publication journals;
5. clean identifiable orphan run processes;
6. restore runnable scheduler queue;
7. accept normal work.

## 30. Process supervision

Launched harness/tool processes are associated with run/stage/attempt identity and an OS process-group/job abstraction so cancellation and crash cleanup target descendants, not only a parent PID.

Graceful shutdown stops new work, drains/checkpoints active runs where practical, marks interrupted attempts accurately, and then terminates managed processes.

## 31. Failure taxonomy

Representative categories:

- infrastructure: harness crash, provider error, rate limit, network error, server restart, sandbox failure, timeout, disk full;
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

A backup captures a consistent SQLite snapshot plus all object-store content referenced by that snapshot and relevant configuration. Repositories are excluded by default.

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

Normal CI covers server/storage/routing/context/workspace/fake-ACP/client/build behavior without requiring external harness installations or production credentials.

The fake ACP harness simulates deterministic success, permissions, crashes, malformed protocol, ignored cancellation, invalid artifact output, provider/config errors, and recovery flows.

Real OpenCode/Codex compatibility jobs are optional/scheduled/controlled jobs in deliberately provisioned environments.

Tagged releases produce official Server, CLI, Desktop, and Android artifacts, checksums, third-party notices, and appropriate supply-chain metadata through GitHub Releases.
