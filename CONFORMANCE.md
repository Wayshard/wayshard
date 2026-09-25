# Wayshard canonical conformance

Trace of substantive requirements in the six canonicals to implementation and tests.
Status is `done` when code and tests exist in this repository. External-only items are marked `external`.

## Identity and licensing

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Product Wayshard, domain wayshard.dev, org Wayshard/wayshard, MIT | `LICENSE`, `NOTICE`, `go.mod`, canonicals | CI canonicals job | done |
| OpenCode is one-time MIT import, no fork/remote/submodule/API compat | `third_party/opencode-v1.18.31/`, `NOTICE` | no git remote to opencode | done |
| Live clients have no OpenCode SDK/runtime | `clients/web/src/wayshard`, `clients/tui/src`, `@wayshard/sdk` | `rg` clean of `@wayshard/sdk/v2` / OpenCode client | done |

## Server authority

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Go server owns state; clients HTTP/JSON+WS | `cmd/wayshard-server`, `internal/api` | `internal/api/server_test.go`, `internal/app/e2e_test.go` | done |
| SQLite WAL + FK + busy timeout + sole writer | `internal/storage/store.go` | `internal/storage/store_test.go` | done |
| Object store SHA-256 | `internal/storage/objects.go` | `TestObjectStoreContentAddress`, `TestObjectGCKeepsReferenced` | done |
| Event+state same transaction | `storage.WithTx` + `InsertEventJSON` | `TestProjectAndEventTransaction` | done |
| Loopback 127.0.0.1 HTTP/WS, no TLS product | `cmd/wayshard-server --listen`, default `paths.DefaultPort` | listen default in app.Open | done |
| Pairing, advertised URL, per-device verifier, revoke, identity challenge | `internal/auth` | `internal/auth/auth_test.go` | done |
| Web session cookie | `pairingComplete` sets `wayshard_session` | pairing complete path | done |
| Local recovery pairing when no devices | `allowLocalAdmin` limited to `POST /v1/pairing/invitations` | `TestFreshServerAuthBoundary` (all other product APIs return 401 unauthenticated) | done |
| Wayshard credentials in restricted config files (no vault/keyring) | `internal/credentials`, `internal/auth` | `TestPairingRoundTripAndRevoke`, `TestTokenRoundTrip`, `TestTokenEnvTakesPrecedence`, `TestTokenMissingFileIsEmpty` | done |
| APIs never return secret plaintext | `auth.Service`, `api.Server` | `TestAPIsDoNotReturnPlaintext` | done |

## Projects, knowledge, context

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Open/clone/create/locate/remove; open passive | `internal/api/server.go` | `TestOpenProjectIsPassive`, `TestRemoveProjectDoesNotDeleteSource` | done |
| Knowledge discovery, cycles, six-file recognition, no `.wayshard` | `internal/knowledge` | `internal/knowledge/discover_test.go` | done |
| Stage-specific context actually injected into stages + durable manifest of the delivered bundle | `orchestrator.stageBundle`, `ctxengine` | `TestStageContextIsInjected` (real orchestration path; planner/executor/reviewer bundles non-empty, stage-specific, include project instructions and plan criteria; manifest artifact persisted) | done |

## Orchestration

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Assess → plan → execute (fake ACP writes run workspace) → validate → review → READY_TO_INTEGRATE → three-way integrate → COMPLETE | `internal/orchestrator/machine.go`, `internal/harness/exec.go`, `cmd/wayshard-fake-acp` | `TestSourceChangingOrchestrationThroughIntegrate` (`internal/app/source_e2e_test.go`); asserts source unchanged until integrate, dirty `user.txt` stays user-owned, RunDelta is agent-only, journal present, complete only after publish | done |
| Deterministic budgets bound repair/attempt/stage growth; permanent failure reaches BLOCKED/FAILED | `internal/orchestrator/budget.go`, `machine.go` | `TestRepairBudgetTerminates`, `TestInfrastructureBudgetTerminates` | done |
| Stage status leaves `running` when its attempt ends | `storage.UpdateStageStatus` | `TestRepairBudgetTerminates`/`TestInfrastructureBudgetTerminates` assert no stale running stage | done |
| Real cancellation interrupts active harness and process tree | `scheduler.Cancel`, `acp` driver shutdown | `TestCancellationInterruptsActiveHarness` (process-tree check) | done |
| Infrastructure fallback executes next viable candidate; quality failure does not | `runStage` attempt ladder | covered by `TestInfrastructureBudgetTerminates` (no candidate => bounded fail); quality path via completion policy | partial |
| Live WebSocket push of committed events | `storage.EventHook` + `events.Hub.Broadcast` | `TestLiveWebSocketEvents` | done |
| Durable notifications derived from committed events | `internal/notifications` | `TestDeriveRunBlocked`, `TestBlockedRunCreatesAttentionNotification` | done |
| Approvals durable, resolvable from any device; ACP permission APPROVE and DENY proven end-to-end through the real permission path | `internal/api` approvals + `harness.ACPExec` permission hook; fake ACP fixture inspects the selected option kind | `TestApprovalLifecycle`, `TestApprovalApproveEndToEnd`, `TestApprovalDenyEndToEnd`, `TestApprovalCancelWhilePending`, `TestApprovalResolveRequiresAuth`, `TestReconcileCancelsStalePendingApproval` | done (real ACP fixture + real API; denied operation never executes) |
| Integration conflict through orchestrator (not integration package alone) | `conflictBefore` wrapping `IntegrateAdapter` | `TestOrchestratorIntegrationConflictBlocks` | done |
| Append-only attempts | `AppendAttempt` | `TestFailedAttemptNotRewritten` | done |
| NO_VIABLE_ROUTE | `internal/routing` | `TestHardFilterImpossible`, `TestNoViableRouteWithoutHarness` | done |
| Jev DecisionEngine + deterministic fallback | `internal/jev` | routing tests with DeterministicEngine | done |
| Fake ACP scenarios | `cmd/wayshard-fake-acp`, `internal/harness/exec.go` | `TestACPExecPlanViaFakeHarness`, `TestACPExecAuthRequired`, `internal/acp/driver_test.go` | done |
| Invalid stage output bounded retry | `runStage` correction attempt | orchestrator machine | done |
| Validation server-owned, passive discovery | `internal/validation` | `TestPassiveDiscoveryDoesNotExecute` | done |
| Validation commands run as the server OS user in a disposable validation workspace; baseline/final results distinguish pre-existing failures from new regressions | `validation.Runner` | `TestValidationDoesNotContaminateWhenHarnessWritesNothing` | done |
| Validation executes only in disposable validation workspaces; its filesystem side effects never enter RunDelta/integration | `orchestrator.baselineValidationWorkspace`/`finalValidationWorkspace` | `TestValidationDoesNotContaminateWhenHarnessWritesNothing`, `TestValidationDoesNotContaminateAgentDelta`, `TestValidationFailureDoesNotContaminate` | done (Linux black-box) |
| Validation cancellation terminates the check process tree | `validation.Runner` group kill | `TestValidationCancellationKillsDescendants` | done (Linux) |
| Validation baseline vs final distinguishes pre-existing failure from regression | `ensureBaseline`, `CompletionPolicy` | `TestValidationBaselineClassification`, `TestCompletionPolicyHonoursBaseline` | done |
| CompletionPolicy Go-owned | `internal/orchestrator/completion.go` | `completion_test.go` | done |
| Artifact-only complete without integrate | CompletionPolicy + e2e | `TestArtifactOnlyCompletesWithoutIntegration` | done |

## Workspace / Git / files / PTY

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Isolated run workspace, dirty baseline | `internal/workspace` | `TestDirtyBaselineNotAttributedToAgent` | done |
| Three-way integrate, branch block, journal, no auto-stage | `internal/integration` | `integrate_test.go` | done |
| Publication journal crash recovery: prepared-only, partial add/modify/delete, all-written/pre-final, user edit on processed/unprocessed targets, symlink-parent escape, source identity mismatch, idempotent reconcile; startup finalizes or blocks durably with events/notifications | `internal/integration` (`PublishStep`, `ClassifyJournal`, `RecoverPublication`), `internal/recovery/publication.go` | `TestPublicationPreparedOnlyRecovery`, `TestPublicationPartialAddModifyDeleteRecovery`, `TestPublicationUserEditUnprocessedTargetBlocks`, `TestPublicationUserEditPublishedTargetBlocks`, `TestPublicationSymlinkParentEscapeBlocked`, `TestPublicationReconcileIdempotent`, `TestReconcilePublicationPreFinalFinalizes`, `TestReconcilePublicationResumesPartial`, `TestReconcilePublicationBlocksOnUserEdit`, `TestReconcilePublicationIdentityMismatchBlocks`, `TestProcessBoundaryPublicationPartialReconcile` | done (integration + Linux process boundary; multi-file publication is not claimed physically atomic) |
| File save CAS / stale hash | `PUT /v1/projects/{id}/file` | `TestFileSaveConflict` | done |
| Server-owned PTY, disconnect does not kill, restart reports loss | `internal/pty`, `GET /v1/ws/pty` | PTY start uses `exec.Command` not request ctx; 410 on missing | done |

## Security / storage / backup

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Execution trust model: discovered harnesses, ACP-requested tools, validation commands, and discovery probes run as the server OS user with normal config/auth/env/filesystem/network on Linux, macOS, and Windows; there is no sandbox, PID-namespace supervisor, provider broker, or fail-closed platform gate | `internal/harness`, `internal/validation`, `internal/process` | `TestToolRunsInWorkspace`, `TestCustomHarnessDiscovery`, `TestProcessBoundaryExecutorCrashRecovery` | done |
| Provider networking is harness-owned; Wayshard adds no broker or destination policy | removed `internal/provider` | (deleted with the provider package) | done (removed) |
| Routing selects harness/model on health, negotiated capability, policy, and Jev fit only; no provider/isolation gating | `internal/routing/router.go` | `TestRoutePicksReadyCandidate`, `TestRouteBlocksWhenNoViable` | done |
| Real ACP harness interoperability: script/symlink harnesses get PATH-resolved interpreter dirs; the ACP driver supports `session/set_config_option`/`session/set_model` and the structured final-response contract; real harnesses initialize over real ACP and use the server OS user's network | `internal/harness/{closure,discover,exec}.go`, `internal/acp/driver.go` | `TestHarnessClosureForScriptHarness`, `TestDiscoverAdvertisedAuthIsUnknownAndRoutable`, `TestCustomHarnessDiscovery`, `TestNativeProcessEnvironmentAndQuoting` | done (native Linux/macOS/Windows) |
| Tool and validation commands use the server OS user's network (no Wayshard network policy) | `validation.Runner`, `harness.toolManager` | `TestToolRunsInWorkspace`, `TestValidationOrdinaryCommandStillWorks` | done |
| macOS runs harnesses/tools normally as the server OS user (Seatbelt removed; no platform gate) | `internal/harness`, `internal/validation` | `TestNativeTrustedLocalExecution` (CI matrix) | done |
| Windows runs harnesses/tools normally as the server OS user (Job Objects removed; no platform gate) | `internal/harness`, `internal/validation` | `TestNativeTrustedLocalExecution` (CI matrix) | done |
| No fail-closed platform gate remains; execution availability is uniform across Linux/macOS/Windows | removed `internal/sandbox` | (sandbox package deleted) | done (removed) |
| Harness/tool/validation processes run in their own process group where supported and are cleaned up best-effort on cancel/timeout/shutdown | `internal/process`, `internal/harness/toolterm.go`, `internal/validation/discover.go` | `TestToolCancelKillsDescendants`, `TestCancellationInterruptsActiveHarness`, `TestDriverIgnoreCancel` | done |
| Discovery/version/ACP probes run the installed executables normally as the server OS user with a timeout, bounded output, and best-effort cleanup; login-shell PATH is validated (absolute-only, bounded, deduplicated) | `harness.probeOne`, `harness.loginShellPATH`, `harness.runProbeCommand` | `TestCustomHarnessDiscovery`, `TestDiscoverWellKnownDir`, `TestSanitizeLoginPATH`, `TestNativeProbeVersionTimeout`, `TestNativeProbeInitializeTimeout` | done (timeouts on CI matrix) |
| Discovery/probing runs after startup recovery; there is no durable probe-owner table | `internal/recovery/recovery.go`, `app.Open` ordering | `TestStartupOrderingRecoveryBeforeDiscovery` | done |
| ACP client callbacks: fs read/write scoped to the run workspace; permission requests surface durable approvals; terminal/tool execution is server-interposed and runs as the server OS user | `harness.ACPExec` hooks, `harness.toolManager` | `TestApprovalLifecycle`, `TestToolRunsInWorkspace`, `TestToolWritesWorkspace` | done (approval APPROVE and DENY E2E verified) |
| Object GC, workspace retention, disk pressure gate | `internal/storage/gc.go`, `scheduler.tick` | `gc_test.go`; low-disk gate blocks write-heavy runs with `BlockedStorage` | done |
| Backup excludes repos and Wayshard credentials (restricted config files) | `internal/backup` | `backup_test.go` | done |
| Startup reconciliation of interrupted attempts/stages and incomplete publication journals; interrupted write attempts restore from a pre-attempt workspace checkpoint using verify-then-use private staging | `internal/recovery`, `orchestrator.checkpointBeforeWrite`, `workspace.RestoreVerified` | `TestRecoveryRestoresInterruptedWriteCheckpoint`, `TestRecoveryBlocksOnCorruptCheckpoint`, `TestProcessBoundaryExecutorCrashRecovery`, `TestProcessBoundaryRepairCrashRecovery`, `TestProcessBoundaryCancelledNeverResumes` | done |
| Durable pre-attempt workspace checkpoints (schema v3) for Executor/Repair with unambiguous length-prefixed canonical tree hash (version 3), verify-then-use restore staging, component-based path ownership validation, attempt-scoped lineage, checkpoint retention/pinning, and fail-closed corruption/version/material handling | `internal/storage/checkpoints.go`, `internal/storage/checkpoint_lifecycle.go`, `internal/orchestrator/checkpoint.go`, `internal/workspace/treehash.go`, `internal/workspace/verified_restore.go`, `internal/recovery` | `TestCanonicalTreeHashV3Adversarial`, `TestCanonicalTreeHashVectors`, `TestRecoverySelectsAttemptCheckpointNotStageLatest`, `TestRecoveryLegacyNoCheckpointBlocks`, `TestRecoveryHashVersionFailsClosed`, `TestRecoveryIdempotent`, `TestRecoveryDoesNotTouchSourceWorkspace`, `TestRestoreVerifiedRejectsSourceMutationDuringStaging`, `TestRestoreVerifiedUsesStagedTreeAfterVerification`, `TestRestoreVerifiedAbortsBeforeSwap`, `TestRecoveryDetectsCheckpointTreeCorruption`, `TestRecoveryRejectsCheckpointOutsideRuntimeRoot`, `TestCheckpointPathOwnership`, `TestReconcileRejectsReclaimedCheckpoint`, `TestReclaimPinsNonTerminalRunCheckpoints`, `TestReclaimTerminalCheckpoint`, `TestCleanupCheckpointDebris`, `TestWriteAttemptCreatesCheckpoint`, `TestUpgradeFromPriorVersions` | done (integration + Linux process boundary) |
| Transactional cancellation and terminal-run attempt consistency: run status, running attempts, running stages and pending approvals move together, and startup reconciliation closes any leftover running attempt on a terminal run | `internal/storage/cancel.go`, `internal/recovery` | `TestReconcileCancelledRunAttemptConsistency`, `TestCancellationInterruptsActiveHarness`, `TestProcessBoundaryCancelledNeverResumes` | done |
| Native cross-platform execution verification with `wayshard-fake-acp` on Linux/macOS/Windows: discovery -> ACP initialize -> PLAN -> EXECUTE -> VALIDATE -> REVIEW -> INTEGRATE -> COMPLETE, with run-workspace isolation (source untouched before integration), validation, integration publishing only the run delta, and correct SQLite stages/attempts/artifacts/route-decisions/events | `internal/app/native_exec_test.go`, `.github/workflows/ci.yml` (`native-execution`) | `TestNativeTrustedLocalExecution`, `TestSourceChangingOrchestrationThroughIntegrate`, `TestFakeACPIntakeToCompleteArtifactOnly`, `TestOrchestratorIntegrationConflictBlocks` | done (CI matrix) |
| Routing viability independent of sandbox/provider capability: a discovered, ready harness is routable with a zero routing config and model selection stays harness-owned | `internal/routing/router.go`, `internal/app/app.go` | `TestNativeRoutingWithoutSandboxOrProvider` | done (CI matrix) |
| Native cancellation/timeout: cancellation terminates a running harness, hanging version/initialize probes and the ACP handshake are bounded, and descendant cleanup is best-effort (not a process-tree security requirement) | `internal/app/cancel_test.go`, `internal/harness/native_timeout_test.go`, `internal/acp/native_test.go`, `internal/process` | `TestCancellationInterruptsActiveHarness`, `TestNativeProbeVersionTimeout`, `TestNativeProbeInitializeTimeout`, `TestNativeHandshakeTimeout`, `TestDriverTimeout` | done (CI matrix) |

## Trusted local execution (current model)

Wayshard removed the containment/security architecture. Discovered ACP harnesses,
ACP-requested tools, validation commands, and discovery probes run as the server OS
user with their normal configuration, authentication, environment, filesystem, and
network access on Linux, macOS, and Windows. There is no sandbox, no
PID-namespace supervisor, no provider broker/destination policy, no sandbox-based
routing viability, no fail-closed platform gate, and no mandatory SecretVault or OS
keyring. `internal/sandbox`, `internal/provider`, and `internal/secrets` were
deleted; `internal/process` is a small best-effort process-group helper; Wayshard
credentials live in `internal/credentials` (restricted config files or environment
variables). Preserved: isolated run workspaces, snapshots/checkpoints, server-owned
validation and completion decisions, recovery, run-delta provenance, conflict-safe
integration, ACP capability probing, timeouts, cancellation, best-effort process
cleanup, routing, and harness-owned credentials.

## Clients

| Requirement | Code | Tests | Status |
|---|---|---|---|
| OpenCode-derived shared graphical client | `clients/ui` (`@wayshard/ui`, copied+adapted design system), `clients/gui` (adapted `session-ui` + Wayshard app/state) | `clients/gui/src/lineage.test.ts`, `clients/gui/src/wayshard/adapter.test.ts`, `bun run build` (web) | done |
| Web mounts the shared graphical client | `clients/web/src/wayshard/main.tsx` → `@wayshard/gui` | vite build | done |
| Desktop/Android Tauri 2 host the shared GUI | `clients/desktop` (`frontendDist ../../web/dist`), `ANDROID.md` | desktop `tsc` typecheck | done (packaging; on-device runtime unverified) |
| Real OpenTUI TUI (not readline) | `clients/tui` (`@opentui/solid` + imported theme system) | `clients/tui/src/model.test.ts`; TUI launches and renders | done |
| Shared SDK + live events | `clients/sdk` (HTTP/WS), `clients/gui/src/wayshard/state.tsx` | sdk unit test | done |
| Live clients have no OpenCode runtime/import specifier | `clients/{sdk,ui,gui,web,tui}` | `lineage.test.ts` asserts no `@opencode-ai/` in live source | done |
| Source lineage is verifiable | `clients/lineage.manifest.json`, `docs/client-source-lineage.md` | `lineage.test.ts` (subtree sizes/markers) | done |

## CI/CD

| Requirement | Code | Tests | Status |
|---|---|---|---|
| PR CI no secrets/paid models/harness installs | `.github/workflows/ci.yml` | workflow | done |
| Linux/macOS/Windows server+CLI | ci.go job + `make build-cross` | workflow | done |
| Desktop Linux | `.github/workflows/release.yml` job `desktop` | checksums + minisign; no paid Linux signing | done |
| Desktop macOS ad-hoc sign (identity `-`, no Apple account) | `tauri.conf.json` `bundle.macOS.signingIdentity`, `APPLE_SIGNING_IDENTITY=-`, `scripts/release/macos-verify-adhoc.sh` | `release_policy_test.sh`; live `codesign` on macOS runners only; Gatekeeper warnings expected | done |
| Desktop Windows self-signed Authenticode | `scripts/release/windows-sign.ps1`; secrets `WAYSHARD_WINDOWS_PFX_*` | `windows_pfx_test.sh`; live `signtool` on Windows runners only; SmartScreen/untrusted publisher expected | done |
| Android Tauri APK, maintainer JKS, no Play | job `android`, `scripts/release/android-sign.sh`, `clients/desktop/android/WayshardKeystore.{kt,pro}` | `android_jks_test.sh`, `android_patch_test.sh`, `android_keystore_patch_test.sh`, `android_keystore_verify_test.sh` (real R8); APK verified; unsigned not published as signed | done |
| Canonical branding mark → all platform icons | `assets/branding/wayshard.{png,ico}`, `clients/desktop/src-tauri/icons`, `clients/web/public`, `scripts/release/android-icons.sh`, NSIS `installerIcon` | `icon_policy_test.sh`, `windows_installer_icon_test.sh`, `release_policy_test.sh`; transparent derivatives match the mark exactly, the Android foreground fits the adaptive safe circle, and the built `-setup.exe` embeds the Wayshard installer icon | done |
| Static, CGO-free Linux binaries | `Makefile` `CGO_ENABLED=0` build rules, `tui` job native CLI | `linux_static_test.sh`; release job asserts `statically linked` | done |
| Android APK Signature Scheme v2 + v3 (no v1 claim) | `scripts/release/android-patch-gradle.py`, `package-android.sh` | `android_patch_test.sh`; `package-android.sh` asserts v2 and v3 | done |
| Web social card served by the published server | `clients/web/public/social-share.png`, `clients/web/index.html`, release `release` job embed check | `icon_policy.py`; server returns 200 for `/social-share.png` | done |
| Tauri CLI aligned with the locked crate | `clients/desktop/package.json` `@tauri-apps/cli` == `Cargo.lock` `tauri` major.minor | `icon_policy.py` `check_tauri_alignment` | done |
| Minisign on combined SHA256SUMS.txt | `scripts/release/minisign-sign.sh`; secrets `WAYSHARD_RELEASE_MINISIGN_*`; public key `keys/wayshard-release.minisign.pub` | `minisign_test.sh`; checksums job verifies before upload | done |
| Checksums once per file, all downloadable artifacts | `scripts/release/checksums.py`, jobs `release`/`desktop`/`desktop-macos-checksums`/`android`/`checksums` | `make release-scripts-test` (overlapping globs cannot duplicate server rows); `desktop_macos_checksums_test.sh` proves both macOS DMGs appear exactly once, order-independent | done |
| CycloneDX SBOM (not `go version -m`) | `scripts/release/sbom.sh` | fails the release job on generator/validation error | done |
| Frozen client lockfile on release (and PR client install) | `bun install --frozen-lockfile` in `ci.yml` + `release.yml` | lockfile `clients/bun.lock` | done |
| Release credentials isolated | `environment: release` on publish jobs; `ci.yml` has no `secrets.*` | `release_policy_test.sh`; PR CI remains secret-free | done |
| Web embedded in server | `internal/webembed` | embed dist before `make build-cross` | done |

## External / manual only (not implementation failures)

| Item | Why it cannot be closed in-repo |
|---|---|
| GitHub org/repo administration, Actions enablement, Environment `release` | operator |
| Android JKS, Windows PFX, minisign secret key stored as Environment `release` secrets | operator-generated material; workflow consumes them |
| Commit of `keys/wayshard-release.minisign.pub` | operator |
| `TYPESAFE_API_KEY` live Jev | runtime credential, not Actions |
| User-installed OpenCode/Codex/etc. | product boundary: never install harnesses |
| Apple Developer ID / notarization / commercial Windows CA / Play Console | intentionally not used |
| Tailscale Serve / tunnel | user networking |
| Real-harness compatibility jobs | provisioned environments only |

## Historical (superseded by trusted local execution) — Post-audit remediation (P0 pass)

The forensic audit of `v0.1.0-rc.6` (`325fa20`) found eight P0 defects. This pass fixed and independently verified:

- **Sandbox filesystem confinement**: Linux now applies Landlock rules (read-only roots for system/toolchain, read-write roots for the run workspace and synthetic temp, denied everywhere else) via an in-binary helper, inherited by child/grandchild processes. Black-box canaries confirm host reads, host writes and `/tmp` escapes are denied while workspace reads/writes succeed.
- **Environment confinement**: blacklist replaced by an explicit allowlist per process class; server secrets and unrelated host credentials are absent from harness/tool/probe environments.
- **Network policy**: `none` is enforced by a seccomp-BPF filter installed before exec (Landlock alone cannot mediate UDP or AF_UNIX). It denies `socket(2)` for every domain — AF_INET, AF_INET6, AF_UNIX (filesystem and abstract), AF_NETLINK, AF_PACKET — plus `io_uring_setup(2)`, and rejects the x32 syscall ABI; it is inherited across exec, child and grandchild. `unrestricted` is only available with an explicit unsafe opt-in and is never selected by required-isolation policies. `allowlist`/`brokered` and secure provider-only networking are not implemented and fail closed. Production harnesses and tool/validation commands run with `NetworkNone`; a harness that requires model/provider network is non-viable (`NO_VIABLE_ROUTE`, "secure provider network isolation unavailable") rather than launched under raw host networking. Verified on Linux amd64/arm64.
- **Validation Tool Sandbox**: all repository-controlled checks run confined; ambient secrets and host filesystem are unavailable, while benign commands pass. Validation executes in disposable validation workspaces (materialized from the task-start snapshot for baseline, and from the authoritative run workspace for final), so its writes/deletes/mode changes/symlinks/git mutations never reach the authoritative run workspace, RunDelta, or the user source tree. The copy is an isolated `git clone --no-hardlinks` or byte copy — never hardlinks.
- **Bounded repair/retry**: deterministic Go budgets cap stages, repairs, replans and per-stage attempts. A permanently failing review or repeated infrastructure failure reaches `BLOCKED`/`FAILED` with bounded stages/events (previously 4,523 stages / 22,621 events).
- **Stage lifecycle**: stages now leave `running` on attempt completion; terminal runs have no stale running stage.
- **Real cancellation**: `Sched.Cancel` cancels the stage context, which tears down the ACP process tree; run becomes `CANCELLED` and no orphan remains.
- **Live WebSocket events**: committed durable events are published after commit via `Store.EventHook` → `Hub.Broadcast`; replay remains the source for reconnect. Verified live delivery of a newly committed event.
- **Notifications**: derived from committed events (run complete/blocked/failed, integration conflict, approval required) with attention state.
- **Approvals**: ACP permission requests create durable approvals; any authorized device may resolve; `pendingApprovals` gates completion.
- **Validation baseline**: baseline captured on the untouched snapshot before the first write stage; pre-existing failures are recorded as such and do not trigger repair.
- **Auth boundary**: loopback local-admin is now limited to `POST /v1/pairing/invitations`; all other product APIs return 401 unauthenticated on a fresh or fully-revoked server.
- **Symlink escape**: project file APIs resolve symlinks and deny escapes.
- **Idempotency conflict**: same key with a different body returns 409; same body replays.
- **Low-disk gate**: critical disk blocks write-heavy runs with `BlockedStorage` on both the poll and enqueue paths.
- **Approval cleanup**: cancelling a run invalidates its outstanding approvals instead of leaving them pending.
- **Recovery**: startup reconciles interrupted stages/attempts and stale server-owned process trees before the scheduler starts, then durably reconciles publication journals (resume the safe remainder or block on unexpected source state, finalize run/integration, emit `publication.reconciled`); interrupted write attempts are restored from a pre-attempt checkpoint through private verify-then-use staging; source-identity locking is applied on the enqueue path.
- **Publication recovery**: a durable journal plus partial source state is classified against real source hashes; already-published targets are recognized, safe remainder is resumed, and unexpected user state blocks without overwrite. Verified across a real process restart.
- **Process-boundary checkpoint recovery**: a real compiled server process is SIGKILLed mid-Executor/Repair, a new server process opens the same durable state, the interrupted attempt becomes `interrupted`, the correct attempt-scoped checkpoint is verified and restored into the RunWorkspace (including ACP Tool-callback partial writes), and the retry starts from the restored workspace. Explicitly cancelled runs stay cancelled and are never reactivated.
- **Orphan process reconciliation**: each stage attempt owns a token inherited by its harness and Tool descendants; the token hash is persisted before launch. Startup terminates every surviving descendant (including backgrounded/setsid grandchildren) before any workspace restore, emits `execution.orphan_reconciled`, and fails closed with BLOCKED/RECOVERY if an owned tree cannot be terminated. Synthetic HOME/TEMP is per attempt. Ownership is Linux-verified (PID-namespace supervisor + token hints); macOS and Windows report the capability as unsupported and fail closed for a live run.
- **Checkpoint lifecycle**: checkpoint material for a non-terminal run is pinned; terminal-run material is reclaimed after retention and its metadata is marked `reclaimed`; unreferenced checkpoint directories, restore staging and terminal sandbox directories are cleaned at startup. Recovery rejects reclaimed material.

Still partial or unverified after this pass:

- Real installed ACP harness interoperability (no supported harness was installed; only the deterministic fake was exercised).
- macOS runtime sandbox enforcement is natively verified (Seatbelt filesystem/network/process-tree on macOS arm64 and Intel); Windows honestly fails closed for required isolation (Job Objects are process/resource management only). See "Native runtime security" below.
- ACP terminal/tool callbacks are wired through the Tool Sandbox (Linux process-boundary verified); a true kill-mid-recovery process test remains outstanding.
- Context assembly is injected into orchestration stages (P1); stage bundles and their durable manifest are verified.
- Web/Desktop/Android runtime parity and client completeness (see the Clients table).
- Live Jev, model metadata enrichment, and event retention/pruning.
- Real provider-backed harness interoperability: no supported real harness was verified against the secure provider transport in this pass. The transport is proven with a deterministic malicious ACP fixture and the production shim/broker; a real-harness E2E remains a provisioned-environment task.
- Provider broker idle-timeout/global rate limiting is intentionally minimal (bounded handshake only); the per-attempt broker is closed with its stage.

## Historical (superseded by trusted local execution) — Post-audit remediation (P1 Pass 1C-1)

P1 Pass 1C-1 closed the three non-blocking discovery findings from the Pass-1B audit and implemented an enforceable secure provider-network capability:

- **F1 — login-shell PATH least privilege**: `sandbox.LoginShellPolicy` grants only the shell executable directory and the specific per-shell startup files (`/etc/profile`, enumerated `/etc/profile.d/*`, and the shell's own home startup files); it no longer grants the real home directory or a blanket `/etc`. `SanitizeLoginPATH` validates untrusted output (absolute-only, no cwd/relative/control-character entries, bounded, deduplicated). A malicious profile cannot read `~/.aws-credentials`, `~/.ssh/*`, `~/.git-credentials`, or an unrelated `/etc` file.
- **F2 — startup ordering**: harness discovery/probing now runs after `recovery.Reconcile` (stale process and probe reconciliation, terminal-state repair, checkpoint/publication recovery, cleanup), and the scheduler still starts last. `TestStartupOrderingRecoveryBeforeDiscovery` observes the durable step order.
- **F3 — probe process ownership**: discovery probes run under the trusted PID-namespace supervisor, so a daemonized (setsid) descendant is torn down with the supervisor, and are also registered in a durable `probe_owners` table (schema v5) before launch so the auxiliary token lets startup reconciliation find a surviving supervisor before discovery runs again. `TestProcessBoundaryProbeDescendantReconciled` SIGKILLs a real server whose probe left a setsid grandchild and proves the next server reconciles it while an unrelated process survives.
- **Secure provider networking (Linux)**: a provider route requires permission **and** platform capability **and** harness transport compatibility **and** a destination policy, all independent. The harness runs in a per-attempt user+network namespace whose only reachable endpoint is a per-attempt HTTPS `CONNECT` broker on a private Unix socket; the broker authorizes `host:port`, resolves on the trusted side, and revalidates every resolved address (loopback/private/link-local/multicast/unspecified/broadcast/IPv4-mapped/6to4/Teredo denied) on every connection, closing DNS rebinding and SSRF. Proxy variables are only transport; the namespace is the boundary, so direct sockets to localhost/LAN/Internet, UDP, AF_UNIX, Docker, `AF_NETLINK` and `AF_PACKET` all fail. `TestProviderNetnsEnforcement` proves the full bypass matrix including child/grandchild and DNS. Tool callbacks and validation remain `NetworkNone`.

Still partial or unverified after this pass:

- Real installed ACP harness interoperability against the secure provider transport (deterministic fixture only; no harness is installed by Wayshard).
- macOS/Windows provider networking: unavailable and fail closed.
- macOS runtime sandbox enforcement is natively verified (Seatbelt filesystem/network/process-tree on macOS arm64 and Intel); Windows honestly fails closed for required isolation (Job Objects are process/resource management only). See "Native runtime security" below.

## Historical (superseded by trusted local execution) — P1 Pass 1C-2 — real harness end-to-end

Pass 1C-2 finished real-harness interoperability and fixed the two broker robustness findings from the Pass-1C-1 audit:

- **Launch closure**: `harness.harnessClosureFor` grants a script/symlink harness its package tree and PATH-resolved interpreter (for example an nvm `node`), so Node-based ACP adapters launch; native binaries get only their own directory. The previous `codex` EACCES was the missing vendored-binary package root.
- **Scoped procfs (`ProcIsolation`)**: harness and probe policies run in a private PID + mount namespace with a procfs scoped to that namespace, capability-probed and best-effort. Bun-based harnesses (OpenCode) can read `/proc` without exposing host processes; when the platform cannot mount a scoped procfs, `/proc` is not granted (no host leak) and the launch still succeeds. Tools and validation never receive `/proc`.
- **ACP model selection and artifact contract**: the driver supports `session/set_config_option` (OpenCode) and `session/set_model` (Codex); real harnesses receive the universal structured final-response contract so stage artifacts validate server-side.
- **Auth reporting**: a harness that advertises auth methods during initialize is reported ready with `authStatus: unknown` (authentication is harness-owned), instead of being assumed unauthenticated; real auth/quota errors are classified as policy failures.
- **Broker hardening (AUD-1/AUD-2)**: CONNECT/bearer headers are bounded to 64 KiB (oversized single or aggregate headers → 431) and upstream resolve/dial uses a finite timeout.
- **Provider transport declarations**: OpenCode and Codex are declared HTTP-proxy compatible only from observed evidence (real harness requests traversed the broker to `opencode.ai`/`models.opencode.ai` and `chatgpt.com`).

Native verification performed on this machine (skipped in CI via `WAYSHARD_REAL_HARNESS=1`):

- **OpenCode v2.0.5** (model `opencode/mimo-v2.5-free`) completed a real source-changing task through the full control plane: plan → execute → validate → review → COMPLETE, writing `AGENT_RESULT.txt` containing a project-context canary read from `AGENTS.md` plus `WAYSHARD_REAL_E2E_OK`. Provider traffic traversed the broker (98× `opencode.ai:443`, 5× `models.opencode.ai:443`). Staged, unstaged and untracked user state were preserved, no auto-stage, RunDelta contained only the agent file, integration published, and no provider shim/owner/approval/running attempt remained.
- **codex-acp 1.12.0** initialized over real ACP and its provider request traversed the broker to `chatgpt.com:443`, but the ChatGPT account's Codex usage limit blocked the model response (`usageLimitExceeded`) — a harness-side quota condition, reproduced identically outside Wayshard.

- **Loopback-only ACP discovery probe**: `NetLoopback` gives the ACP initialize probe a private network namespace with only `lo` (no host/LAN/public route, TCP stream only, no UDP/AF_UNIX/AF_NETLINK/AF_PACKET), capability-probed and falling back to `NetworkNone`. Version probes remain `NetworkNone`. This lets harnesses whose ACP server needs local IPC (OpenCode) be discovered route-viable without weakening ordinary probing. `TestLoopbackProbeIsolation` proves the malicious-fixture matrix (isolated loopback works; host loopback/::1/public/LAN/UDP/host IPC denied, including child/grandchild).

Still partial or unverified after Pass 1C-2:

- Real-harness E2E is native/local only; official CI stays fixture-backed and does not require OpenCode, Codex, codex-acp, credentials, or provider access.
- codex-acp model calls are blocked by the account's upstream `usageLimitExceeded` (harness-side); OpenCode free-model calls succeed.
- `grok` and `claude` had executables but no Wayshard definition in Pass 1C-2; both now ship as default catalog definitions (Pass 1D).
- A real-harness integration-conflict run was not performed (deterministic conflict coverage remains).

## P1 Pass 1D — configurable supported-harness catalog

Pass 1D replaces the Go-hardcoded supported-harness inventory with a configuration-driven catalog:

- **Shipped versioned TOML catalog** (`internal/harness/harnesses.toml`, embedded, `schema_version = 1`) defines the supported harness families: OpenCode, Codex, Claude, Grok, Gemini CLI, GitHub Copilot CLI, Cursor CLI, Kiro CLI, Junie, goose, Cline, Qwen Code, Qoder, Mistral Vibe, Devin CLI, Kilo Code, Factory Droid, Auggie CLI, Amp, Pi, and Oh My Pi (plus the deterministic `wayshard-fake-acp` test fixture). ACP invocations are taken from the official Agent Client Protocol registry and official project documentation, never guessed, and the live registry is never imported at runtime.
- **User catalog** at the platform config path (`$XDG_CONFIG_HOME/wayshard/harnesses.toml`, with per-platform fallbacks) is the CRUD interface; there is no catalog CRUD API or editor. Merge is by stable `id`: user fields override shipped fields by key (arrays replace), `enabled = false` disables, deleting restores, user-only ids are custom definitions, duplicates within a source are rejected, and one invalid entry is isolated from otherwise-valid definitions. Unsupported future schema versions and malformed TOML are rejected with diagnostics. Loaded at startup; restart applies changes.
- **Definition vs installation**: `/v1/harness-definitions` reports the effective definitions (source shipped/user/overridden, enabled), while `/v1/harnesses` reports installations (executable, bridge path/presence, version, ACP result, auth, provider transport, blocking reason). A present CLI without its required ACP bridge is reported present with a specific reason, not "harness not installed".
- **Catalog-driven discovery**: `Discover` iterates the effective enabled definitions and resolves executable/bridge aliases over the daemon PATH, sanitized login PATH, Wayshard well-known bin dirs and each definition's home-relative well-known dirs, deduplicating physical paths. There is no `switch definition.ID` and no per-name Go adapter; ordinary compatible ACP harnesses are added with TOML only. A route whose definition is missing from the effective catalog fails closed.
- **The catalog is a discovery/launch declaration, not a containment policy**: there is no sandbox to disable and no provider trust to grant. Well-known paths must be home-relative with no traversal, aliases must be bare names and never package runners (`npx`/`npm`/`bunx`/…), unknown fields and unsupported enum values fail the entry closed, and catalog bytes/definitions/list entries/glob matches are bounded.
- **Diagnostic fix**: `GET /v1/sandbox` no longer claims provider networking is unavailable while `providerNetwork.available = true`; the always-on filesystem/process confinement report and the runtime-probed provider/loopback capabilities are now stated separately.
- **Extensibility proof**: `TestCustomHarnessDiscovery` defines a fake ACP harness only in a temporary user `harnesses.toml`, with no Go change for its id, and proves it is discovered, version-probed and ACP-initialized. `TestSecurityInvalidDeclarationsFailClosed`, `TestUserOverrideFields`, `TestUserDisableShippedAndDeleteRestores`, `TestUserCustomDefinition`, `TestDuplicateUserIDRejected`, `TestMalformedTOMLDiagnosed`, `TestUnsupportedSchemaVersionRejected`, and `TestInvalidEntryIsolatedFromValid` cover the override/merge/security matrix. `TestSandboxDiagnosticConsistency` and `TestHarnessDefinitionsEndpoint` cover the API diagnostics.

Pass 1C-2's independently audited real OpenCode execution path is unchanged; the OpenCode definition preserves its ACP invocation, loopback requirement, command interposition, model selection and config roots.

Still partial or unverified after Pass 1D:

- Grok and Claude are recognized from their definitions and their CLI presence is reported; their real ACP/runtime and provider compatibility are not empirically verified on this machine (only OpenCode and codex-acp have real runtime evidence).
- Catalog changes require a server restart (no live reload/watcher by design).
- Real-harness E2E remains native/local only; official CI stays fixture-backed.

### Pass 1D remediation — independent-audit findings

An independent audit of Pass 1D found three defects; all three are fixed in the same milestone:

- **F1 (was P1) — provider trust bound to the harness id.** Verification now binds to a versioned SHA-256 *execution fingerprint* over the definition's execution-relevant fields (`Definition.ExecutionFingerprint`). `VerifiedTransport` grants the empirically verified transport only when the effective definition's fingerprint equals the trusted shipped definition's; cosmetic overrides retain trust and any material override (executable, bridge, mode, args, loopback, interposition, model selection, config roots, discovery data, platforms, provider requirement, declared transport) loses it and fails closed. Covered by `TestTrustDefaultShipped`, `TestTrustCosmeticOverrideRetains`, `TestTrustMaterialOverridesLose`, `TestTrustCodexMaterialOverridesLose`, `TestTrustCustomReusedTrustedIDLoses`, `TestFingerprintDeterministicAndOrderIndependent`.
- **F2 (was P2) — config roots could be broad or symlink-escaping.** Roots are validated with `validateRootSyntax` (no `.`/`./`/`..`/absolute/drive/HOME) and `validateRootPolicy` (shipped roots are trusted product configuration; user-supplied roots must live beneath a platform config/data/state/cache directory or a dot-directory and must not be a sensitive location, HOME or a platform base). Before granting, `resolveRootSafe` canonicalizes with component-safe `Lstat` checks, rejecting any symlink that escapes HOME or resolves onto a sensitive location. Well-known search roots get the same symlink-safe treatment while executable symlink resolution still works. Covered by `TestConfigRootSyntaxRejects`, `TestConfigRootUserPolicy`, `TestResolveRootSafeSymlinkEscape`, `TestWellKnownSymlinkEscape`.
- **F3 (was P2) — stale installations remained routable.** Each installation persists the effective definition's fingerprint; `storeCandidates.Refresh` atomically replaces the persisted set with the latest discovery result (`ReplaceHarnessInstallations`), and `Candidates` excludes any row whose definition is missing, disabled or fingerprint-mismatched, re-deriving transport from the current effective definition. Covered by `TestReconcileDisabledDefinitionNotRoutable`, `TestReconcileChangedDefinitionNotRoutable`, `TestReconcileRemovedCustomDefinitionNotRoutable`, `TestReconcileTransportDerivedFromCatalogNotRow`, `TestReplaceHarnessInstallationsClearsStale`.

Non-blocking items: F4 (malformed/unsupported user-catalog errors are now surfaced through the harness-definition diagnostics API), F5 (generous catalog parsing/discovery bounds), F6 (Pi homepage corrected to `github.com/badlogic/pi-mono`). Storage schema 7 adds `harness_installations.definition_fingerprint`.

## P1 Pass 1E — OpenCode 2 client foundation completion

The live clients are adapted from the imported OpenCode 2 client source, not recreated:

- **Graphical client**: `clients/ui` is the imported OpenCode `packages/ui` design system (~1,680 files, ~34k LOC) rebranded as `@wayshard/ui`; `clients/gui/src/session-ui` is the imported `packages/session-ui` (~118 files, ~21k LOC) with OpenCode SDK/core/client imports replaced by a Wayshard view-model shim; `clients/gui/src/app` adapts the imported OpenCode graphical **application shell** (`packages/app/src/context/command.tsx` → `command.tsx`, `pages/layout-new.tsx` → `layout.tsx`, `components/session/session-sortable-tab.tsx` → `session-tab.tsx`, `components/dialog-command-palette-v2.tsx` → `command-palette.tsx`) with the OpenCode navigation hierarchy (Session/Changes/Files/Terminal primary tabs; advanced surfaces in dialogs/sheets and the command palette). Web (`clients/web`) mounts it; Desktop and Android (`clients/desktop`, Tauri 2) host the same build.
- **TUI**: `clients/tui` is a real OpenTUI/Solid terminal application (`@opentui/*`) that adapts the imported OpenCode TUI dialog framework (`ui/dialog.tsx`, `ui/dialog-confirm.tsx`, `ui/dialog-prompt.tsx`, `ui/dialog-alert.tsx`), toast (`ui/toast.tsx`), border (`ui/border.ts`), spinner (`ui/spinner.ts`, `component/spinner.tsx`), theme system (`theme/index.ts` + assets), keymap surface and OpenTUI preload (`bunfig.toml`). Surfaces: project/session navigation, session stream, prompt, run/stage timeline, Changes, Files, Routing, Usage, Approvals, Settings/connection. The readline scaffold is gone.
- **Domain**: `@wayshard/sdk` owns the data path (HTTP/JSON + WebSocket events + PTY); there is no OpenCode server, SDK, API, provider or executable dependency.
- **Lineage**: `clients/lineage.manifest.json` (v2) records broad subtrees plus concrete per-file ancestry (upstream path + upstream git blob + live destination + provenance marker); `docs/client-source-lineage.md` documents it; `clients/gui/src/lineage.test.ts` asserts adapted subtrees remain substantial, every per-file ancestry entry exists with its marker, and live source contains no `@opencode-ai/` import specifier.
- **Branding**: live client source has no OpenCode product branding (theme names/namespaces/i18n renamed to Wayshard; provider icon identifiers and provenance comments retained); MIT attribution remains in `NOTICE`/`THIRD_PARTY_NOTICES.md`.
- **Signature surfaces**: the graphical composer renders the imported `PromptInputV2`; Changes renders real diffs through the imported session-ui `File` component (`GET /v1/runs/{id}/file` provides run/snapshot content); Files uses the adapted OpenCode file-tree model; the terminal renders through the imported `ghostty-web` presentation against the server-owned PTY (with server-side PTY resize); Context/Knowledge/Recovery render structured sections (raw JSON only behind the debug inspector); pairing is a guided identity-verify → invitation-code → device-credential flow. The TUI command palette uses the adapted `DialogSelect`.
- **Known incomplete**: Desktop/Android on-device runtime unverified here; the TUI does not adapt every deep OpenCode route (session/home) component.

### Pass 1E release-path and pairing remediation

- **Shipped interactive TUI**: the official `wayshard` binary, invoked with no
  subcommand, launches the packaged OpenCode-derived TUI companion
  (`wayshard-tui`, an OpenTUI/Solid executable built from `clients/tui` via
  `clients/tui/build.ts`). The retired handwritten `wayshard>` loop is removed;
  `cmd/wayshard` resolves the companion only from the running executable's own
  directory (or an explicit `WAYSHARD_TUI` override), never via `PATH`, and fails
  clearly when it is missing. Scriptable subcommands (`status`, `projects`,
  `send`, `run`, `cancel`, `integrate`, …) remain. Proof:
  `cmd/wayshard/launch_test.go`, `scripts/ci/tui_smoke.sh`, the CI `tui` job, and
  the release `tui` matrix (linux amd64/arm64, macOS amd64/arm64, windows amd64).
- **Pairing binds the expected application identity**: the challenge endpoint
  requires a fresh client nonce and returns the identity public key; clients
  verify the Ed25519 signature over the nonce, the fingerprint, and the server
  id against the trusted invitation before completing pairing with
  `expectedServerId`/`expectedFingerprint`, re-check the returned identity, and
  only then persist a credential. This proves application identity; transport
  security remains user-owned. Proof: `internal/api/pairing_identity_test.go`,
  `internal/auth` pairing tests, `clients/sdk/src/identity.test.ts`,
  `cmd/wayshard/pairing_test.go`.

### Pass 1E final release-packaging remediation

- **Runnable CLI+TUI distribution unit.** The release `tui` matrix builds a
  native `wayshard` CLI and a self-contained `wayshard-tui` companion on each
  runner and packages them with `scripts/release/package-cli-tui.sh` into
  `wayshard-<tag>-<os>-<arch>.tar.gz` (`.zip` on Windows). Inside the archive the
  binaries use the canonical runtime names the launcher expects plus `LICENSE`,
  `NOTICE`, and `THIRD_PARTY_NOTICES.md`, so extraction yields a working
  no-argument `wayshard` with no rename and no `WAYSHARD_TUI` override. The raw
  CLI/TUI binaries remain published for advanced users but the raw `wayshard`
  binary alone is not a functional interactive client.
- **Local build parity.** `make build-all` now emits `bin/wayshard` and
  `bin/wayshard-tui` under canonical names; `build-tui-versioned` adds an
  optional versioned copy without breaking the runtime layout.
- **Executable symlink resolution.** `resolveTUI` resolves the running
  executable itself through symlinks before taking its directory, so
  `/usr/local/bin/wayshard -> /opt/wayshard/wayshard` finds
  `/opt/wayshard/wayshard-tui`. A companion symlink escaping the real install
  directory is still rejected and PATH search remains forbidden. Proof:
  `cmd/wayshard/resolve_test.go`, `cmd/wayshard/resolve_symlink_test.go`.
- **Deterministic checksum coverage.** The combined `checksums` job now waits
  for `release`, `tui`, `desktop`, and `android`, and fails if any expected
  CLI+TUI bundle is absent before writing/minisigning `SHA256SUMS.txt`. Guarded
  by `scripts/release/release_policy_test.sh`.
- **Release-script + packaged smoke coverage.** `package_cli_tui_test.sh`
  validates the archive layout, notices, executable mode, and extraction
  behavior; `scripts/ci/tui_smoke.sh` now exercises the real packaging script
  end to end (extract, `wayshard help`, no-argument TUI launch).
- **CLI help accuracy.** `wayshard help` documents verified pairing
  (`--invitation` or `--server-id`/`--fingerprint`); a bare code remains refused.
  Pairing security behavior is unchanged.

### Pass 1E rendered-UI remediation (R1–R3)

Rendered browser validation of the production graphical build reopened Pass 1E:
the live client imported only the general adapted UI Tailwind layer and mounted
no runtime theme provider, so the v2 palette/theme tokens were undefined and the
app rendered white text on a near-white fallback canvas.

- **R1 — restored theme pipeline.** `clients/gui/src/styles.css` now imports the
  adapted `@wayshard/ui/v2/styles/tailwind.css` and the adapted
  `session-ui/styles/index.css`, matching upstream `packages/app/src/index.css`.
  `WayshardApp` mounts the adapted `ThemeProvider` from
  `@wayshard/ui/theme/context` with `defaultTheme="oc-2"` and
  `defaultColorScheme="dark"` (the provider gained a `defaultColorScheme` prop),
  matching upstream `packages/app/src/app.tsx`. Rendered evidence: `data-theme`
  is `oc-2`, `--v2-background-bg-deep` resolves to `#080808ff`, body background
  is `rgb(8,8,8)` with white text (contrast ≈ 20:1). No CSS color patch.
- **R2 — "More" overflow works.** The advanced-surfaces menu is mounted through
  the shared dialog provider (`dialog.show`), which creates the adapted Kobalte
  dialog root/portal. Rendering `<Dialog>` inline from a `moreOpen` signal
  produced no visible dialog. All ten advanced destinations now open visibly,
  selecting one opens its surface, and Escape/reopen work.
- **R3 — narrow/mobile shell.** Below 820px the sidebar is an off-canvas drawer
  toggled from a titlebar button, with a backdrop, close-on-select, and
  horizontally scrollable primary tabs; desktop keeps the fixed sidebar. Geometry
  is asserted at 390/820/900.
- **Evidence.** `scripts/ci/ui_render_smoke.mjs` drives the installed Chrome over
  CDP against the production Web build with a mocked Wayshard API and asserts 39
  rendered conditions (theme tokens/contrast, More dialog visibility, palette
  rows, mobile geometry). Source-level guards live in
  `clients/gui/src/theme-integration.test.ts`. Screenshots are written outside
  the repository. The client is legible and themed by default with no test-only
  overrides.

### Pass 1E application-level port

Rendered validation plus an application-level audit reopened Pass 1E: the
graphical client had strong component/source lineage but its application
composition was a Wayshard-authored shell. The production graphical application
has since been replaced with descendants of the actual OpenCode application
source, adapting Wayshard into the OpenCode application rather than the reverse.

- **Application root / routing**: `gui/src/app/app.tsx` adapts upstream
  `packages/app/src/app.tsx` (provider tree + routes). Routing uses the adapted
  Wayshard router (`gui/src/app/router.tsx`); the inherited `@solidjs/router`
  did not advance its reactive location in the adapted provider tree.
- **Home**: `gui/src/app/pages/home.tsx` + `pages/home/*` adapt upstream
  `pages/home.tsx`, `home-projects-view.tsx`, `home-sessions-view.tsx`.
- **Layout / sidebar**: `gui/src/app/pages/layout.tsx` +
  `pages/layout/sidebar-{shell,project,items}.tsx` adapt upstream
  `pages/layout.tsx` and the layout sidebar family.
- **Session**: `gui/src/app/session-page.tsx` adapts upstream
  `pages/session.tsx`; `components/titlebar.tsx` adapts upstream
  `components/titlebar.tsx` / `titlebar-tab-nav.tsx`;
  `components/composer-region.tsx` adapts
  `pages/session/composer/session-composer-region.tsx`;
  `pages/session/{review-tab,file-tabs,terminal-panel-v2}.tsx` adapt the
  corresponding upstream session panels.
- **Retired**: the custom Wayshard shell (`app/session-shell.tsx`,
  `app/layout.tsx`, `app/session-tab.tsx`) is removed from the production path.
- **Lineage**: `clients/lineage.manifest.json` now records the application-level
  descendants with their upstream git blobs; `clients/gui/src/lineage.test.ts`
  verifies each recorded blob against the vendored upstream source and asserts
  the signature descendants are reachable from the production application root.
  This reachability check found and removed two dead adapted files
  (`session-tab.tsx`, the old `layout.tsx`) and their stale claims.
- **Domain/backend**: Wayshard `@wayshard/sdk`, server authority, PTY ownership,
  pairing verification and event stream are unchanged; the adapted app is a
  control surface.

- **Narrow/mobile model.** Derived from the vendored application, not a custom
  drawer: `context/layout.tsx` exposes `mobileSidebar`
  (`opened/show/hide/toggle`), the app-level `components/titlebar.tsx` carries
  the `xl:hidden` `data-component="mobile-nav-toggle"`, and
  `pages/layout/sidebar-mobile.tsx` renders the
  `data-component="sidebar-nav-mobile"` overlay (fixed `top-10`, max-w 400px,
  slide transition) with a scrim; the persistent sidebar is hidden below `xl`
  and the overlay hides on project/session selection.
- **Behavior-bearing internals (superseded).** The earlier stages moved Wayshard
  behavior into upstream-named containers: `review-tab.tsx` was largely the
  previous Wayshard Changes view, `file-tabs.tsx` the previous Files view,
  `terminal-panel-v2.tsx` a wrapper around the renderer, the composer region a
  custom footer around PromptInputV2, and the timeline a compact custom loop.
  An independent application-level audit reopened Pass 1E for a final
  remediation; see "Pass 1E final application-level remediation" below.
- **Flake.** `TestCancellationInterruptsActiveHarness` was made deterministic
  (load-tolerant context/wait budgets, an own-deadline cancellation wait, and an
  orphan check scoped to the test's built harness path).
- **Verification.** Client typechecks, GUI tests (blob authenticity +
  production reachability), rendered smoke, provenance-offline build with
  `third_party` removed, and Go/release/build gates all pass on the port's SHA.

### Pass 1E final application-level remediation

A second independent application-level audit confirmed the macro composition was
genuinely OpenCode-derived but found the behavior-bearing session internals and
routing still fell short: normal navigation reloaded the document, and the
session panels were relocated Wayshard views rather than behavioral adaptations.
This pass closed those findings without redesigning the accepted parts.

- **R1 — in-place routing.** `gui/src/app/router.tsx` is a real reactive SPA
  router (`pushState`/`replaceState` update a location signal, `popstate`
  synchronizes back; only absolute external URLs use `location.assign`). The
  selected route view is resolved as a memo and swapped via `Dynamic` because the
  adapted provider tree did not propagate the router signal through top-level
  `Show`/`Switch` children. `RouterAdapter`-contract tests prove reactive updates,
  replace, popstate, query/hash preservation and no document reload.
- **R2A — Review/Changes.** The adapted session-ui `SessionReview` is now the
  production review surface, fed by `review-adapter.ts` (run delta + run/snapshot
  reads, bounded concurrent hydration, binary handling, line counts) with
  per-session scroll/open persistence and the upstream user-interaction
  cancellation + `requestAnimationFrame` restore (`review-view.ts`). Run vs
  Workspace provenance and the pre-existing user-baseline label are preserved.
- **R2B — Files.** `file-tab-model.ts` adapts the upstream tab/scroll model
  (ordered open tabs, active file, neighbor-picking close, per-file scroll);
  `file-tabs.tsx` renders the inherited session-ui `File` viewer with a
  Wayshard compare-and-set editor (`expectedHash`) on top. Source safety is
  unchanged.
- **R2C — Terminal panel.** `terminal-panel-v2.tsx` adapts the upstream lifecycle:
  a terminal tab strip with active selection, create/close/open, focus on
  selection, one `terminal-wrapper-<id>` per server PTY, and honest
  loss/reconnect. `terminal.tsx` binds to an existing PTY id and never spawns a
  shell. A narrow server capability (`DELETE /v1/projects/{id}/terminals/{tid}`,
  `pty.Manager.Kill`) makes close truthful; split-pane resize and client screen
  serialization are deliberately omitted (Wayshard's Terminal is a work surface,
  not a docked split) and documented.
- **R2D — Composer.** `session-composer-state.ts` adapts the request-dock state
  machine (open/closing/opening, a once-applied responding lock) over the
  Wayshard approval list; `composer-region-controller.ts` adapts the dock refs,
  resize-observed height, animated max-height reveal, centered/full-width logic
  and focus restoration; `composer-region.tsx` renders the approval dock.
  PromptInputV2 remains the composer; routing profile and artifact-only remain
  the Wayshard prompt controls.
- **R2E — Timeline.** `timeline/model.ts` projects messages and run stages into a
  single reconciled row model with stable keys; `message-timeline.tsx` adds
  bottom-follow, scroll preservation, jump-to-latest, reveal-by-key, per-session
  scroll/expansion state and a bounded mounted window for long sessions. True
  `@tanstack/solid-virtual` windowing is unavailable as a dependency, so an
  explicit bounded window is used instead of faking a virtualizer.
- **R3 — structural lineage.** `clients/lineage.manifest.json` (v3) declares, per
  major descendant, required live definitions, inherited concept anchors that
  must appear in the comment-stripped live AND upstream source, live-only
  markers, a minimum comment-stripped code size and a low structural-similarity
  floor. Usage entries now require a real import specifier. `lineage.test.ts`
  proves a provenance-only wrapper fails the gate.
- **R4 — rendered coverage.** `scripts/ci/ui_render_smoke.mjs` now exercises
  in-place routing (with a per-document reload detector), New Session, Timeline,
  Changes/Review, Files (incl. compare-and-set save), Terminal (create/close,
  PTY data, loss), Composer/Approval, plus desktop 900 and mobile 390 regression
  — 55 rendered checks against the production build with only the backend
  boundary mocked.
- **New-session registration.** New-session submissions now go through Wayshard
  state (`selectConversation` + `send`) so the created run is registered and the
  session surface shows its stages/changes/timeline immediately.


## Historical (superseded by trusted local execution) — Native runtime security (macOS + Windows)

Verification-first native pass closing the remaining native-runtime security
evidence gap. The hard rule is unchanged: approvals are UX/policy, OS
containment is security, and required isolation never silently becomes
unrestricted execution.

- **Feature-level capability model.** `SandboxPolicy` now has typed features and
  `RequiredFeatures`/`validateRequiredFeatures`: a `Required` policy fails closed
  with `ErrRequiredIsolation` unless the backend declares every feature the
  policy depends on (process tree, filesystem read/write, network mode,
  synthetic env). A backend can no longer compile a required policy while
  silently omitting containment. `TestRequiredPolicyCompileInvariant` enforces
  this on every platform.
- **macOS (honest fail-closed).** Seatbelt confinement (filesystem read/write
  confinement and network denial) is real and its profile is passed inline via
  `sandbox-exec -p`; `sandbox-exec` absence fails closed, and a relative command
  under `cmd.Dir` is resolved against `cmd.Dir`. However, macOS has **no
  non-removable OS-backed process-tree ownership boundary**: a process group is
  escaped by `setsid`, and an environment ownership token (`WAYSHARD_OWNER_TOKEN`
  / `WAYSHARD_TOOL_TOKEN`) is controlled by the untrusted process, which can strip
  it from a child's exec environment and evade any environment scan. `kqueue`
  `EVFILT_PROC` + `NOTE_TRACK` is a kernel fork-tracking primitive, but a
  PID-reuse-safe, `NOTE_TRACKERR`-safe implementation could not be demonstrated
  reliably, and a bug there could terminate an unrelated process. macOS therefore
  does **not** advertise `FeatureProcessTree`; `Report()` reports the required
  sandbox unavailable (`Available=false`, `Mode=seatbelt_no_ownership`) and every
  required policy (`ToolPolicy`, `HarnessPolicy`, `ProbePolicy`,
  `ReadOnlyViewPolicy`, validation) fails closed with `ErrRequiredIsolation`
  before untrusted code runs (`TestNativeRequiredPoliciesFailBeforeExec`,
  `TestNativeFailClosedBeforeExec`, `TestNativeValidationFailsClosed`). macOS
  remains a full Desktop/TUI/CLI/server/control-plane platform; only protected
  local harness/tool execution is unavailable.
- **Linux process-tree boundary (non-removable).** Required execution on Linux runs under a trusted in-binary **PID-namespace supervisor** (`__wayshard-sandbox-supervise`) that remains namespace init while the target runs as its child. A process cannot leave its PID namespace, and when namespace init dies the kernel terminates every remaining member, so `KillTree` (kill the direct child) tears down setsid/double-forked descendants too. Server death is detected through a **death pipe** (the parent holds the write end; EOF tears the namespace down) because namespace init sees `getppid() == 0` for a parent outside its namespace. `FeatureProcessTree` on Linux is advertised only when `procIsolationSupported()`; otherwise `Report()` is `Available=false` and required execution fails closed. Adversarial native tests prove a **token-stripped, setsid** descendant is terminated on cancellation, on normal completion, on server death, and under the loopback policy (`TestLinuxNamespaceBoundary*`), and that token reconciliation spares an unrelated process (`TestLinuxReconcileTokenHashSparesUnrelated`). The supervisor also mounts a namespace-scoped procfs only when a policy requests `ProcIsolation`.
- **Environment tokens are auxiliary only.** `WAYSHARD_OWNER_TOKEN` / `WAYSHARD_TOOL_TOKEN` are retained as cooperative/diagnostic hints (and to let Linux recovery find a surviving supervisor), never as proof of complete ownership of a hostile process. Termination authority is the OS boundary (Linux PID namespace; macOS/Windows fail closed). A durable owner is never marked reconciled on the strength of "no token-bearing process found" on a platform where the token is removable.
- **Windows (honest fail-closed).** Job Objects provide process/resource
  management only; filesystem confinement, network denial and race-free
  process-tree containment are not enforced. `Report()` reports the required
  sandbox unavailable with the missing features, and required harness/tool/probe
  policies fail closed with `ErrRequiredIsolation` before any untrusted code
  executes (`TestNativeFailClosedBeforeExec` proves the marker is never written).
  Isolation-unavailable is a policy block (run settles BLOCKED), not a retryable
  infrastructure failure. Job Object tree termination is still exercised for
  non-required management via `TestWindowsJobObjectManagesNonRequiredProcessTree`.
- **Ownership reconciliation invariant.** On Linux (the only platform with an
  authoritative post-hoc ownership mechanism) `finishProcessOwner` marks a
  durable owner reconciled only when no owned process remains; where ownership
  cannot be verified it leaves the owner active rather than erasing recovery
  evidence. A probe is never started when the platform cannot authoritatively
  verify its tree (`StoreProbeOwnerSink.BeginProbe` refuses), so no
  unreconcilable owner is created.
- **CI.** A `native-security` job runs the native suite on macOS arm64,
  `macos-15-intel` and windows-latest. The fail-before-exec and validation
  fail-closed tests are mandatory on those platforms and fail the job if they
  skip. The ordinary `Go` job continues to run `go test ./...` on
  ubuntu/macos/windows.

macOS and Windows join as honest fail-closed platforms: no local protected
harness/tool/probe/validation execution runs, so no ownership marker can be
stripped and no descendant can survive. Linux remains the only platform that
advertises `FeatureProcessTree`, backed by an authoritative post-hoc ownership
mechanism.

## Stable-readiness remediation (rc.9 P2/P3)

Independent audits of `v0.1.0-rc.9` (`778add0`) found no P0/P1 and a set of
P2/P3 items. This pass closed them without changing the signing policy (no paid
or external certificate/service).

- **Credential handling.** Web/Desktop/Android share one graphical client, but
  credential storage is now platform-appropriate. The shared state layer
  (`clients/gui/src/wayshard/state.tsx`) persists only the endpoint in
  `localStorage`; the device credential is never written to web storage. Browser
  clients authenticate with the server's `HttpOnly` `wayshard_session` cookie
  (the SDK sends `credentials: same-origin`). Native clients
  (`clients/gui/src/wayshard/secure-store.ts`) store the credential through Tauri
  commands backed by the OS credential store on desktop (`clients/desktop/src-tauri/src/credentials.rs`,
  `keyring` crate: macOS Keychain, Windows Credential Manager, Linux Secret
  Service) and by Android Keystore-backed AES-256-GCM encryption on Android
  (non-exportable key; only ciphertext in app-private storage). The CLI
  (`cmd/wayshard/credentials.go`) uses the OS keychain and **fails closed** when it
  is unavailable — it never silently downgrades to a plaintext credential file.
  The protected user-private file fallback (0600 on Unix, the user-profile ACL on
  Windows) is used only when the operator explicitly sets `WAYSHARD_HEADLESS=1`;
  `WAYSHARD_TOKEN` always takes precedence.
- **Native WebSocket authentication.** Browsers/webviews cannot set an
  `Authorization` header on a WebSocket and native clients have no cross-origin
  cookie, so `POST /v1/ws/ticket` issues a short-lived, single-use ticket
  (`internal/api/wsticket.go`) that the SDK appends to the handshake URL
  (`WayshardClient.eventURL`). The long-lived credential never appears in a URL.
- **Desktop platform coverage.** The release `desktop` matrix now builds Intel
  macOS (`macos-15-intel`) alongside Apple silicon, producing
  `wayshard-desktop-<tag>-macos-x86_64.dmg` in addition to `-aarch64.dmg`, so
  Desktop matches the CLI/TUI architecture coverage.
- **Clean Go build stamp.** The release workflow's web-embed step preserves the
  tracked `internal/webembed/dist/.gitkeep` instead of deleting the directory, so
  the Go build stamp is `v0.1.0-rc.N` rather than `+dirty`.
- **AppImage `.DirIcon`.** `scripts/release/appimage-fix-diricon.sh` repacks the
  Linux AppImage with a relative `.DirIcon` (preserving the original runtime
  ELF), so the published icon resolves instead of dangling to the build machine.
  Covered by `appimage_fix_diricon_test.sh`.
- **Android v3 signing.** `scripts/release/android-patch-gradle.py` now enables
  APK Signature Scheme v2 and v3 (`enableV3Signing = true`), so the published APK
  supports key rotation.
- **Sandbox robustness.** Capability probes are bounded
  (`capabilityProbeTimeout`, `runProbeTimeout`) so a wedged probe cannot hang
  `Compile`/`Report`. The Linux PID-namespace supervisor forwards `SIGTERM` to
  the target group and grants a bounded grace period before force-killing
  (`gracefulShutdownGrace`), so cancellation allows clean shutdown.
- **Provider lifecycle coverage.** `TestProviderHarnessLifecycleTornDownOnShimDeath`
  proves that a provider harness tree (including a `setsid` descendant) is torn
  down when the shim that owns the harness death pipe dies — the crash/cancel
  lifecycle for provider routes.
- **Docs.** Duplicate canonical section numbers were removed; `README.md` now
  documents Desktop/Android installation, signing/OS warnings, supported
  architectures, fail-closed platform limitations, and verification status.

## rc.10 release-defect remediation

The `v0.1.0-rc.10` release run failed on two packaging defects; both are fixed in
a subsequent source revision (rc.10 is immutable failed history; a corrected
release is cut separately).

- **Android-target Rust gate.** `crate::android_keystore::invoke` was private but
  called from `credentials.rs`, a cross-module call that only compiled on the
  Android target. Host `cargo test`/`cargo check` cfg out the
  `#[cfg(target_os = "android")]` command bodies, so the error escaped to the
  release build. The delete-key call is now a narrow
  `pub(crate) fn android_keystore::delete_key()`, and CI compiles the Android
  target (`make android-check` → `cargo check --target aarch64-linux-android`) so
  Android-only command bodies cannot escape host checks. Guarded by
  `release_policy_test.sh`.
- **AppImage `.DirIcon`.** The fix script scanned for the `hsqs` magic, which
  matched a decoy inside the runtime (offset 36081) before the real SquashFS
  superblock (193728), so `unsquashfs` failed. It now reads the payload offset
  from the AppImage runtime's `--appimage-offset`, validates the numeric offset,
  the file bounds, the SquashFS magic and extraction before repacking, and
  preserves the runtime ELF byte-for-byte. `appimage_fix_diricon_test.sh` uses a
  synthetic AppImage with a decoy `hsqs` before the real payload.

## rc.11 release-defect remediation (cut as rc.12)

The `v0.1.0-rc.11` release was blocked by two defects. Both are fixed here;
rc.11 is immutable history and the corrected release is cut separately as
rc.12. Neither changes product behavior or the signing policy.

- **Android R8 stripped the Keystore helper.** Tauri 2.5.0's generated release
  build type enables `isMinifyEnabled = true` and collects every `**/*.pro`
  under the app module. `dev.wayshard.app.WayshardKeystore` is reached only from
  Rust over JNI (`find_class` + `call_static_method`), so R8 saw no Java/Kotlin
  reference and tree-shook the class (and would rename its
  `encrypt`/`decrypt`/`deleteKey` methods) out of the minified APK, breaking
  credential storage on Android. The fix adds
  `clients/desktop/android/WayshardKeystore.pro` (installed with the package
  substituted by `android-keystore-patch.sh`) with
  `-keep class <pkg>.WayshardKeystore { *; }`. A release-artifact gate,
  `scripts/release/android-keystore-verify.py`, parses the built APK's
  `classes*.dex` (class defs plus defined static methods) and fails closed if the
  class or any required method is absent; it runs on the signed APK in the
  `android` release job. Coverage: the verifier's synthetic-DEX self-test, the
  real-R8 keep/strip test in `android_keystore_verify_test.sh` (mandatory in the
  new `android-keystore-r8` CI job via `WAYSHARD_REQUIRE_R8=1`), the minified
  gradle fixture in `android_patch_test.sh`, and `release_policy_test.sh`.
- **macOS per-component checksum race.** The `desktop` matrix builds Apple
  silicon (`aarch64`) and Intel (`x86_64`) macOS on separate runners, both
  writing/uploading `SHA256SUMS-desktop-macos.txt`; whichever leg uploaded last
  won, so the published file could list only one architecture. The macOS legs no
  longer generate a per-component checksum; a single `desktop-macos-checksums`
  job runs after the whole matrix, downloads both DMGs, and writes
  `SHA256SUMS-desktop-macos.txt` with exactly one `aarch64` and one `x86_64` row
  (`scripts/release/desktop-macos-checksums.sh`, which fails closed on a missing,
  duplicated, or unexpected architecture). The combined `checksums` job now
  waits for it. Covered by `desktop_macos_checksums_test.sh` and
  `release_policy_test.sh`.

## rc.13 branding-integration audit

`assets/branding/wayshard.png` (the canonical mark, moved there in `da2d76c`) is
the single source of every shipped icon. The transparent desktop, Web/PWA and
favicon derivatives are byte-for-byte canonical downscales, and the `.ico`/`.icns`
containers embed the same mark; no OpenCode, Tauri or placeholder icons remain in
shipped paths.

- **Android icon generation was broken.** The release workflow ran
  `bunx tauri icon src-tauri/app-icon.json`, but Tauri's icon *manifest* landed in
  tauri-cli 2.9.0 while the project pins 2.5.0 for its mobile build template; the
  pinned CLI treats the JSON as an image and aborts the `android` job. Fixed by
  `scripts/release/android-icons.sh`, which drives a pinned manifest-capable
  generator (`@tauri-apps/cli@2.11.5`) for the icon step only, leaving the build
  template and toolchain unchanged. It regenerates launcher, adaptive
  foreground, monochrome and background resources into the ephemeral
  `gen/android` tree and fails closed if the adaptive resources are missing.
- **Adaptive foreground would have been cropped.** The manifest used the raw
  canonical mark (content spanning ~98% of the canvas) as the adaptive
  foreground, which Android's 66dp safe-zone mask would clip. Added the derived
  `assets/branding/wayshard-android-fg.png`, which fits the mark inside the safe
  circle, and used it for both the adaptive foreground and the monochrome mask
  with `bg_color` `#0e0f12`.
- **Guards.** `icon_policy.py` verifies the manifest references, the committed
  desktop/Web derivative sizes, the safe-area padding, and that the workflow uses
  the pinned manifest-capable generator; `release_policy_test.sh` asserts the
  wiring. `Makefile` runs both in `release-scripts-test`.
- **Known pre-existing, out of scope.** `clients/web/index.html` references
  `/social-share.png`, which has never existed in `clients/web/public` (a
  pre-existing dangling `og:image`); the only repo social images
  (`clients/ui/src/assets/images/social-share*.png`) are OpenCode-branded and are
  not imported or shipped. The unused `clients/ui/src/assets/favicon/` directory
  (including the `-v3` duplicates) is dead but canonical-consistent.

## rc.14 release-defect remediation: Windows installer icon

The rc.13 audit found the Windows NSIS `-setup.exe` still showed Tauri/NSIS's
default installer icon (the installed app exe and the MSI were correctly
branded). Root cause: `bundle.windows.nsis.installerIcon` was never set, so
Tauri's NSIS template left `MUI_ICON` empty.

- **Icon source.** `assets/branding/wayshard.ico` is derived from the canonical
  mark: 9 BMP frames (16/24/32/48/64/72/96/128/256), all 32-bit with
  transparency, each matching the canonical mark's 8x8 average (the largest
  frame to within 0.4/255).
- **Config.** `clients/desktop/src-tauri/tauri.conf.json` sets
  `bundle.windows.nsis.installerIcon` to
  `../../../assets/branding/wayshard.ico` (resolved by `tauri build`, which
  chdirs to `src-tauri`). Tauri's template emits `!define MUI_ICON` from it.
- **Config/policy test.** `icon_policy.py` validates the ICO frames and their
  derivation from the canonical mark and that `installerIcon` is set and
  resolves to that file; `release_policy_test.sh` fails if `installerIcon` is
  unset or points elsewhere, and asserts the release workflow runs the artifact
  gate.
- **Artifact gate.** `scripts/release/verify-windows-installer-icon.py` fails
  closed unless every `wayshard.ico` frame is embedded byte-for-byte in the
  built `-setup.exe` (NSIS copies the frames verbatim; independently confirmed
  with a `makensis` build). It runs in the Windows desktop release leg.
  `windows_installer_icon_test.sh` builds a branded and a default `makensis`
  installer and proves the gate passes only the branded one.

Out of scope for this change (recorded from the rc.13 audit, addressed in
rc.15): Linux x86-64 dynamic linkage, the `/social-share.png` dangling
reference, Android v1 signing, dead `-v3` favicon duplicates, and Tauri CLI/crate
version alignment.

## rc.15 pre-stable cleanup

Resolves the pre-stable findings carried from the rc.13/rc.14 audits. No product
behavior changes; all existing tags/releases are preserved.

- **Linux linkage.** Release Go builds are now CGO-free
  (`CGO_ENABLED=0` in `make build-cross`/host targets and the `tui` job's native
  CLI), so `linux/amd64` is statically linked like `linux/arm64` instead of
  picking up glibc. The server uses the pure-Go `modernc.org/sqlite`, so nothing
  requires cgo. Guards: `linux_static_test.sh` (release-scripts) and a release
  job assertion that the Linux artifacts are static.
- **Web social card.** Added `clients/web/public/social-share.png` (1200x630,
  canonical mark on `#0e0f12` with the wordmark) and completed the `og:`/
  `twitter:` metadata in `clients/web/index.html`. `icon_policy.py` validates
  the card and its wiring; the release `release` job fails if the embedded Web
  build lacks it. Verified the built server returns `/social-share.png` 200 with
  the committed bytes.
- **Android signing.** `android-patch-gradle.py` now sets
  `enableV1Signing = false` (minSdk 26 never uses the v1/JAR scheme;
  `apksigner verify` reports it `false` at that minSdk) while keeping v2 + v3.
  `package-android.sh` asserts v2 and v3 are present on the packaged APK;
  `android_patch_test.sh` forbids re-enabling v1 or its old "retained" claim.
- **Dead assets.** Removed `clients/ui/src/assets/images/social-share*.png`
  (OpenCode-branded, unshipped) and the obsolete duplicate
  `clients/ui/src/assets/favicon/` tree after proving no production references
  (no imports, not in `lineage.manifest.json`, not exported by `@wayshard/ui`).
  The live favicons are served from `clients/web/public`; the adapted-lineage
  test still passes. `release_policy_test.sh` guards against their return.
- **Tauri alignment.** `@tauri-apps/cli` moved from 2.5.0 to **2.11.5**, matching
  the locked `tauri` 2.11.6 crate (major.minor). `android-icons.sh` now uses the
  repo-pinned CLI (manifest support no longer needs a separate pin);
  `icon_policy.py` fails if the npm CLI and the crate drift. The Android job
  installs `platforms;android-36`/`build-tools;36.0.0` for the aligned template's
  compileSdk 36. The `--harness-catalog`/icon/branding behavior is unchanged.
