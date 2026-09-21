# AGENTS.md

## Purpose

This file is the implementation entry point for coding agents working on Wayshard. Read it before changing the repository, then read the canonical documents relevant to the task.

Wayshard is an independent MIT-licensed coding control plane. Its product name is **Wayshard**, its primary domain is **wayshard.dev**, and its canonical GitHub organization is **Wayshard**. The primary repository is expected to be `Wayshard/wayshard`.

## Canonical documents

The repository root contains six canonical documents. They have distinct responsibilities and should not duplicate one another unnecessarily.

- `AGENTS.md` — implementation rules, workflow, quality gates, and agent constraints.
- `SPEC.md` — authoritative finished-product behavior and requirements.
- `DESIGN.md` — authoritative product, interaction, visual, responsive, CLI/TUI, and client UX design.
- `ARCHITECTURE.md` — authoritative technical architecture, boundaries, protocols, persistence, security, execution, and lifecycle design.
- `MEMORY.md` — durable rationale, decisions, terminology, rejected alternatives, and implementation knowledge worth preserving across sessions.
- `README.md` — human-facing overview, setup and development entry point, and links to the deeper canonicals.

When documents appear to conflict, resolve the matter by concern rather than by a single total filename ranking. Product behavior belongs to `SPEC.md`, UX behavior to `DESIGN.md`, technical structure to `ARCHITECTURE.md`, and implementation process to this file. `MEMORY.md` preserves rationale but does not override the document responsible for a concern.

If an intentional implementation change materially alters a canonical decision, update the relevant canonical document in the same change set.

## Non-negotiable product invariants

1. **Wayshard is not operationally connected to OpenCode.** Selected MIT-licensed OpenCode 2 client/TUI source may be imported once as implementation foundation. Wayshard must not maintain a GitHub fork relationship, upstream remote, synchronization workflow, OpenCode API compatibility target, runtime dependency, or OpenCode branding. Preserve required MIT attribution in third-party notices.
2. **Wayshard has four full clients:** Web, CLI/TUI, Desktop, and Android. CLI and Desktop ship for Linux, macOS, and Windows. Web is served by the Wayshard Server. Android is a full client, not a companion app.
3. **The Go server is the authority.** Clients are control surfaces. Harnesses are stage workers. Jev is a judgment layer. None of them own authoritative workflow state outside the server.
4. **Wayshard never installs user coding harnesses or ACP bridges.** Discover and use what is already installed and available to the OS user running the server. Installation and bridge setup are the user's responsibility.
5. **Stable ACP v1 is the protocol baseline.** Keep protocol mechanics separate from harness-specific adapters. Do not make core orchestration depend on optional ACP features, extensions, or subagent systems.
6. **Harness transcripts are ephemeral; artifacts and workspace state are durable.** Correctness must not depend on hidden harness memory or a still-live ACP session.
7. **Agents never experiment directly in the user's source working tree.** Every run uses an isolated run workspace created from the exact meaningful source state captured at task intake.
8. **Pre-existing user changes are baseline, never agent output.** Run delta is `starting snapshot -> run final`, not `HEAD -> run final`.
9. **The server owns final validation and completion decisions.** An agent saying “done” is not evidence of completion.
10. **Permissions are policy/UX; OS containment is security.** Never silently downgrade a requested sandbox into unrestricted execution.
11. **Wayshard networking is localhost-first and intentionally simple.** The server binds to `127.0.0.1` by default and serves local HTTP/WebSocket. Secure remote exposure, TLS, DNS, VPNs, tunnels, and proxies are handled by the user's networking layer, such as Tailscale Serve. Do not add a built-in certificate-management or remote-networking product.
12. **Authentication uses pairing and per-device credentials, not shared username/password authentication.** Pairing invitations are short-lived; each device credential is independently revocable.
13. **Repository discovery is passive.** Opening or indexing a project must not execute project code or silently modify project files.
14. **Project knowledge remains repository-owned.** Do not invent a required `.wayshard` repository configuration format. Server-local settings remain server state.
15. **General Git remains the user's tool.** Wayshard's Changes experience is for run/workspace diff inspection. Do not turn Wayshard into a full Git GUI unless inherited UI already has a suitable capability and it remains relevant.
16. **Official releases come from the public GitHub repository through CI/CD.** Normal fork-PR CI must not require production secrets, paid model access, or installed third-party harnesses.

## Core workflow

The semantic workflow is:

```text
USER REQUEST
  -> CONTEXT ASSEMBLY
  -> JEV ASSESSMENT
  -> PLAN
  -> EXECUTE
  -> VALIDATE
  -> REVIEW
  -> repair/replan as necessary
  -> READY_TO_INTEGRATE
  -> INTEGRATE
  -> COMPLETE
```

`EXPLORE` may be inserted before or between planning stages when confidence or information is insufficient. Artifact-only tasks such as brainstorming or research may complete without source integration.

Stages and stage attempts are append-only history. Never rewrite failed or interrupted attempts into successful ones.

## Stage rules

### Plan / Explore / Review

- Read-only by default.
- Use relevant project knowledge, current request, applicable artifacts, and fresh workspace facts.
- Do not mutate the run workspace unless the stage explicitly requires a server-controlled output artifact.
- Planning must produce explicit acceptance criteria and a validation plan sufficient for later deterministic evaluation.

### Execute / Repair

- Write only inside the isolated run workspace.
- Follow the accepted PlanArtifact / TaskContract unless a replan is required.
- Do not perform destructive Git administration such as commit, push, reset, rebase, clean, branch switching, or integration into the user's live source tree unless a server-controlled product operation explicitly calls for it.
- Produce an ImplementationReport describing what changed, relevant deviations, and expected validation.

### Validate

- Final validation is server-controlled.
- Project-defined mandatory checks cannot be removed by an agent.
- Validation records actual command, working directory, environment fingerprint, exit status, duration, and durable evidence/log references.
- Distinguish pre-existing failures from new regressions whenever practical.
- Discovery of validation commands is passive; do not execute project code merely to discover what checks exist.

### Review

- Read-only critic by default.
- Review against the user request, TaskContract, run delta, ImplementationReport, validation evidence, and applicable canonical/project rules.
- Findings must be concrete, scoped, and evidence-based.
- A reviewer cannot override deterministic failed gates.

## Artifacts

Prefer structured durable artifacts over prose transcripts. Important artifact types include:

- PlanArtifact / TaskContract
- InvestigationArtifact
- ImplementationReport
- ValidationArtifact
- ReviewArtifact
- TestReport
- ResearchArtifact
- FailureArtifact
- DiffSummary
- RecoveryRecord

When a harness lacks a native structured submission mechanism, use the universal structured final-response contract and validate it server-side. Invalid stage output gets a bounded correction attempt; it must not silently become accepted state.

## Context rules

- Build target-specific context bundles; do not dump the entire repository or conversation into every stage.
- Treat current user intent and hard task constraints as higher priority than older conversation summaries.
- Respect project-owned authority and native instruction-family scoping.
- Do not automatically inject every harness-specific instruction file into every other harness.
- Prefer structured repository signals over raw repository prose for Jev.
- Preserve context provenance and hashes so delivered context is inspectable and reproducible.
- Native harness memory is an optimization only. Critical new context must still be explicitly delivered.

## Harness and routing rules

- Discover installed harness executables; probe actual ACP behavior before marking them routable, always under the discovery ProbePolicy.
- Capability negotiation is authoritative. Never assume optional ACP features from a harness name.
- Separate protocol driver code from known-harness adapter code.
- Generic ACP harnesses must remain usable when they satisfy the minimum core contract.
- Coding-provider credentials remain harness-owned whenever the harness owns the provider connection. Do not scrape provider credentials from harness files.
- The router first filters impossible routes deterministically, then applies policy, health, and Jev semantic fit among viable candidates.
- If no viable route exists, return a structured blocked state such as `NO_VIABLE_ROUTE`; never invent an impossible route.
- Infrastructure failure may retry or use an explicit fallback. Quality failure triggers repair or replan, not arbitrary model switching.

## Security rules

- Platform secrets such as the TypeSafe/Jev key never enter run workspaces or harness context.
- Harness-owned credentials stay in the harness trust domain.
- Separate outer Harness Sandbox and stricter Tool Sandbox where the integration supports it.
- Known adapters may provide native or adapter-mediated tool isolation; generic harnesses may be outer-only. Report effective isolation honestly.
- ACP terminal/tool callbacks are interposed by the server; a harness never executes model-generated commands directly.
- Harness discovery, version, ACP-initialize, and login-shell PATH probes are untrusted execution and run under a dedicated ProbePolicy. If the platform cannot enforce it, report the probe unavailable rather than run unrestricted. Login-shell PATH discovery reads only required per-shell startup files. Probes run only after startup recovery and each probe tree is durably owned for reconciliation.
- Startup reconciliation terminates stale server-owned process trees by ownership token before restoring any workspace; never match or kill by PID alone.
- External file approvals should normally import immutable read-only inputs rather than widen host filesystem access.
- Network for model/provider control traffic is distinct from tool-command network access.
- A provider-capable harness must receive provider-only network capability where the platform can enforce it (isolated network environment plus a Wayshard-controlled broker that admits only authorized, validated provider destinations); never raw host networking. Provider routes require permission, real capability, harness transport compatibility, and a destination policy. Tool and validation network remain denied by default.
- Tool network is denied or brokered according to policy; server-owned credentials must not leak through network or process environment.
- Symlinks must not bypass filesystem boundaries.
- Resource limits and process-tree cleanup are part of execution safety.
- Never persist plaintext secrets in ordinary SQLite fields, logs, artifacts, or diagnostics.

## Git and workspace rules

- Capture starting Git/workspace state before actionable execution.
- Preserve staged, unstaged, meaningful untracked, mode, and symlink state as applicable.
- Agents work in isolated workspaces, not source workspaces.
- Users may continue editing, committing, rebasing, stashing, or using other Git tools while runs execute.
- Integration uses the immutable run start as merge base and the current source state as the third side.
- A branch change prevents silent integration onto a different branch.
- Integration is serialized by concrete source/repository identity, not merely ProjectID.
- Do not auto-stage by default.
- Auto-commit is optional and may include only the proved run delta; never sweep in pre-existing user index or working-tree changes.

## UI and client rules

- Do not bend Wayshard architecture to preserve OpenCode internals.
- Preserve useful OpenCode 2 interaction/visual/TUI foundations where they still serve Wayshard; otherwise replace them.
- Do not add new command-palette, accessibility, editor, source-control, or other convenience systems solely for feature parity. Preserve/adapt what the imported foundation already provides unless Wayshard functionality genuinely requires more.
- The CLI/TUI is a full Wayshard client, but presentation may be terminal-appropriate. Do not build a nested IDE, terminal, or editor purely to match graphical clients.
- Web/Desktop/Android should share graphical client logic where practical; CLI/TUI uses its terminal foundation.
- All clients talk to Wayshard domain HTTP/JSON and WebSocket APIs, never ACP directly.

## Testing and quality gates

Every change must maintain tests appropriate to the affected layer. The intended project test architecture includes:

- Go unit tests for routing, policy, context packing, permission decisions, state machines, knowledge discovery, and artifact validation.
- SQLite/storage integration tests including migration behavior and atomic state/event transactions.
- Workspace/Git integration tests including dirty baselines, branch movement, concurrent source edits, conflict handling, rollback, and interrupted publication recovery.
- Checkpoint integrity and lifecycle tests: canonical v3 tree hash, verify-then-use restore staging, attempt-scoped lineage, component-based path ownership, retention/pinning, debris cleanup, and fail-closed corruption/version/reclaimed handling.
- Real process-boundary recovery tests that launch a compiled server as an OS subprocess, SIGKILL it, and verify a different server process recovers the same durable state for Executor, Repair, orphan reconciliation, and publication; record distinct PIDs.
- Publication crash-point tests: prepared-only, partial add/modify/delete, all-written/pre-final, user-edit conflict on processed and unprocessed targets, path/symlink escape, identity mismatch, and idempotent reconcile.
- Discovery probe sandbox tests: a malicious version fixture and ACP-initialize fixture proving host/project/Wayshard-data/secret/network denial, timeout descendant cleanup, and bounded output.
- Approval approve, deny, cancel-while-pending, and unauthenticated-resolution tests through the real ACP permission path, asserting a denied protected operation never executes.
- Deterministic fake ACP harnesses covering successful runs, permissions, crashes, cancellation failures, malformed frames, invalid stage output, capability violations, auth-required states, and model/config disappearance.
- Real harness compatibility tests only in deliberately provisioned environments; normal CI must not install those harnesses.
- Client tests for API state, reconnect/resume, multi-device transitions, run timeline, approvals, and change provenance.
- End-to-end tests that exercise intake through integration using fake/test harnesses and no paid model dependencies.
- Platform sandbox conformance tests appropriate to Linux, macOS, and Windows.

Before declaring a task complete, run the repository-defined mandatory validation for the affected areas.

## Public repository and release rules

Wayshard is public and MIT licensed.

- Keep build and test workflows under `.github/workflows/`.
- Fork pull requests must run meaningful CI without secrets.
- Official release artifacts are produced by GitHub Actions and published to GitHub Releases.
- Release families include Server, CLI, Desktop, and Android; Web is embedded in Server.
- Preserve third-party MIT attribution for imported OpenCode 2 code and all other bundled dependencies as required.
- Do not bundle or redistribute third-party coding harnesses or ACP bridges as part of Wayshard releases.

## Change discipline

Prefer cohesive changes that preserve the architecture. Avoid speculative frameworks or abstractions unrelated to a concrete requirement. Do not add microservices where the modular Go monolith suffices. Do not add a separate database server where SQLite suffices. Do not turn extension points into mandatory complexity before the product needs them.

When implementation evidence proves a canonical decision is wrong, update the relevant canonical document explicitly rather than silently drifting away from the design.

## Adapted client source rules

- The live graphical client and TUI are adapted from the imported OpenCode 2 client source under `third_party/opencode-v1.18.31` (provenance only). Adapted copies live under Wayshard-owned directories (`clients/ui`, `clients/gui/src/session-ui`, `clients/tui`).
- Do not establish an upstream remote, submodule, sync workflow, or OpenCode API/runtime compatibility layer. Do not import `@opencode-ai/*` into live client source; `clients/gui/src/lineage.test.ts` fails the build if you do.
- Keep `clients/lineage.manifest.json` and `docs/client-source-lineage.md` accurate. If you add or replace adapted source, update the manifest and the lineage test.
- Wayshard architecture and domain always win over inherited UI structure. Adapt or replace an inherited component rather than bending the product architecture.
- Keep the product fully rebranded; required MIT attribution stays in `NOTICE`/`THIRD_PARTY_NOTICES.md` and provenance comments.
