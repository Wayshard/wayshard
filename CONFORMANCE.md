# Wayshard canonical conformance

Trace of substantive requirements in the six canonicals to implementation and
tests. Status is `done` when code and tests exist in this repository.
External-only items are marked `external`.

## Identity and licensing

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Product Wayshard, domain wayshard.dev, org Wayshard/wayshard, MIT | `LICENSE`, `NOTICE`, `go.mod`, canonicals | CI canonicals job | done |
| OpenCode is a one-time MIT import with no fork, upstream remote, submodule, sync, or API-compat target | `third_party/opencode-v1.18.31/`, `NOTICE` | no git remote to OpenCode; `clients/gui/src/lineage.test.ts` rejects `@opencode-ai/` in live source | done |

## Server authority

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Go server owns state; clients use HTTP/JSON + WebSocket | `cmd/wayshard-server`, `internal/api` | `internal/api/server_test.go`, `internal/app/e2e_test.go` | done |
| SQLite WAL + foreign keys + busy timeout + sole writer | `internal/storage/store.go` | `internal/storage/store_test.go` | done |
| v0.2 baseline schema (version 1) squashed into a single `001_init.sql`; pre-v0.2 databases are refused with guidance; forward-migration framework retained | `internal/storage/migrations/001_init.sql`, `internal/storage/store.go` | `TestFreshInstallCurrentSchema`, `TestReopenUsesBaseline`, `TestRejectPreV02Database`, `TestRefuseNewerSchema`, `TestOpenPathWithSpecialCharacters` | done |
| Object store keyed by SHA-256 | `internal/storage/objects.go` | `TestObjectStoreContentAddress`, `TestObjectGCKeepsReferenced` | done |
| Event and state share one transaction | `storage.WithTx` + `InsertEventJSON` | `TestProjectAndEventTransaction` | done |
| Loopback `127.0.0.1` HTTP/WS; remote exposure is the user's networking layer | `cmd/wayshard-server --listen`, default `paths.DefaultPort` | listen default in `app.Open` | done |
| Pairing, advertised URL, per-device verifier, revoke, identity challenge | `internal/auth` | `internal/auth/auth_test.go` | done |
| Web session cookie | `pairingComplete` sets `wayshard_session` | pairing complete path | done |
| Local recovery pairing when no devices remain | `allowLocalAdmin` limited to `POST /v1/pairing/invitations` | `TestFreshServerAuthBoundary` (all other product APIs return 401 unauthenticated) | done |
| Wayshard credentials in restricted config files | `internal/credentials`, `internal/auth` | `TestPairingRoundTripAndRevoke`, `TestTokenRoundTrip`, `TestTokenEnvTakesPrecedence`, `TestTokenMissingFileIsEmpty` | done |
| APIs return credential metadata, never plaintext | `auth.Service`, `api.Server` | — | partial (metadata-only design; no dedicated plaintext test) |

## Projects, knowledge, context

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Open/clone/create/locate/remove; opening is passive | `internal/api/server.go` | `TestOpenProjectIsPassive`, `TestRemoveProjectDoesNotDeleteSource` | done |
| Knowledge discovery, cycle tolerance, six-file recognition, no `.wayshard` requirement | `internal/knowledge` | `internal/knowledge/discover_test.go` | done |
| Stage-specific context injected into stages with a durable manifest of the delivered bundle | `orchestrator.stageBundle`, `ctxengine` | `TestStageContextIsInjected` (planner/executor/reviewer bundles non-empty, stage-specific, include project instructions and plan criteria; manifest artifact persisted) | done |

## Orchestration

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Assess → plan → execute (fake ACP writes run workspace) → validate → review → READY_TO_INTEGRATE → three-way integrate → COMPLETE | `internal/orchestrator/machine.go`, `internal/harness/exec.go`, `cmd/wayshard-fake-acp` | `TestSourceChangingOrchestrationThroughIntegrate` (source unchanged until integrate, dirty `user.txt` stays user-owned, RunDelta is agent-only, journal present, complete only after publish) | done |
| Deterministic budgets bound repair/attempt/stage growth; permanent failure reaches BLOCKED/FAILED | `internal/orchestrator/budget.go`, `machine.go` | `TestRepairBudgetTerminates`, `TestInfrastructureBudgetTerminates` | done |
| Stage status leaves `running` when its attempt ends | `storage.UpdateStageStatus` | `TestRepairBudgetTerminates`, `TestInfrastructureBudgetTerminates` | done |
| Cancellation interrupts an active harness and yields a terminal cancelled run | `scheduler.Cancel`, `acp` driver shutdown, `orchestrator.ProcessRun` | `TestCancellationInterruptsActiveHarness`, `TestProcessRunCancellationDuringStatusReadConverges`, `TestProcessRunCancellationInsideStageConverges`, `TestProcessRunPreCanceledContextConverges`, `TestProcessRunPreservesNonCancellationFailure` | done |
| Infrastructure fallback executes the next viable candidate; quality failure does not | `runStage` attempt ladder | `TestInfrastructureBudgetTerminates` (no candidate => bounded fail); quality path via completion policy | partial |
| Live WebSocket push of committed events | `storage.EventHook` + `events.Hub.Broadcast` | `TestLiveWebSocketEvents` | done |
| Durable notifications derived from committed events | `internal/notifications` | `TestDeriveRunBlocked`, `TestBlockedRunCreatesAttentionNotification` | done |
| Approvals durable and resolvable from any device; ACP permission APPROVE and DENY proven end-to-end through the real permission path | `internal/api` approvals + `harness.ACPExec` permission hook; fake ACP fixture inspects the selected option kind | `TestApprovalLifecycle`, `TestApprovalApproveEndToEnd`, `TestApprovalDenyEndToEnd`, `TestApprovalCancelWhilePending`, `TestApprovalResolveRequiresAuth` | done (real ACP fixture + real API; denied operation never executes) |
| Integration conflict handled through the orchestrator | `conflictBefore` wrapping `IntegrateAdapter` | `TestOrchestratorIntegrationConflictBlocks` | done |
| Append-only attempts | `AppendAttempt` | `TestFailedAttemptNotRewritten` | done |
| NO_VIABLE_ROUTE | `internal/routing` | `TestHardFilterImpossible`, `TestNoViableRouteWithoutHarness` | done |
| Jev DecisionEngine + deterministic fallback | `internal/jev` | routing tests with `DeterministicEngine` | done |
| Fake ACP scenarios | `cmd/wayshard-fake-acp`, `internal/harness/exec.go` | `TestACPExecPlanViaFakeHarness`, `TestACPExecAuthRequired`, `internal/acp/driver_test.go` | done |
| Invalid stage output bounded retry | `runStage` correction attempt | orchestrator machine | done |
| Validation discovery is passive | `internal/validation` | `TestPassiveDiscoveryDoesNotExecute` | done |
| Validation commands run as the server OS user in a disposable validation workspace; baseline/final results distinguish pre-existing failures from new regressions | `validation.Runner` | `TestValidationDoesNotContaminateWhenHarnessWritesNothing`, `TestValidationDoesNotContaminateAgentDelta`, `TestValidationFailureDoesNotContaminate` | done |
| Validation cancellation terminates the check process tree | `validation.Runner` group kill | `TestNativeTrustedLocalExecution` | partial (process-tree assertion not separately covered) |
| Validation baseline vs final distinguishes pre-existing failure from regression | `ensureBaseline`, `CompletionPolicy` | `TestCompletionPolicyHonoursBaseline`, `TestValidationDoesNotContaminateWhenHarnessWritesNothing` | done |
| CompletionPolicy is Go-owned | `internal/orchestrator/completion.go` | `completion_test.go` | done |
| Artifact-only runs complete without integration | CompletionPolicy + e2e | `TestArtifactOnlyCompletesWithoutIntegration` | done |

## Workspace / Git / files / PTY

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Isolated run workspace with dirty baseline | `internal/workspace` | `TestDirtyBaselineNotAttributedToAgent` | done |
| Three-way integrate, branch block, journal, no auto-stage | `internal/integration` | `integrate_test.go` | done |
| Publication journal crash recovery: prepared-only, partial add/modify/delete, all-written/pre-final, user edit on processed/unprocessed targets, symlink-parent escape, source identity mismatch, idempotent reconcile; startup finalizes or blocks durably with events/notifications | `internal/integration` (`PublishStep`, `ClassifyJournal`, `RecoverPublication`), `internal/recovery/publication.go` | `TestPublicationPreparedOnlyRecovery`, `TestPublicationPartialAddModifyDeleteRecovery`, `TestPublicationUserEditUnprocessedTargetBlocks`, `TestPublicationUserEditPublishedTargetBlocks`, `TestPublicationSymlinkParentEscapeBlocked`, `TestPublicationReconcileIdempotent`, `TestReconcilePublicationPreFinalFinalizes`, `TestReconcilePublicationResumesPartial`, `TestReconcilePublicationBlocksOnUserEdit`, `TestReconcilePublicationIdentityMismatchBlocks`, `TestProcessBoundaryPublicationPartialReconcile` | done (integration + Linux process boundary; multi-file publication is not claimed physically atomic) |
| File save CAS / stale hash | `PUT /v1/projects/{id}/file` | `TestFileSaveConflict` | done |
| Server-owned PTY; disconnect does not kill; restart reports loss | `internal/pty`, `GET /v1/ws/pty` | PTY start uses `exec.Command` not request ctx; 410 on missing | done |

## Security / storage / backup

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Execution trust model: discovered harnesses, ACP-requested tools, validation commands, and discovery probes run as the server OS user with normal config/auth/env/filesystem/network on Linux, macOS, and Windows | `internal/harness`, `internal/validation`, `internal/process` | `TestToolRunsInWorkspace`, `TestCustomHarnessDiscovery`, `TestProcessBoundaryExecutorCrashRecovery` | done |
| Provider networking follows the server OS user; Wayshard adds no network layer | `internal/harness`, `internal/validation` | `TestNativeTrustedLocalExecution` (CI matrix) | done |
| Routing selects harness/model on health, negotiated capability, policy, and Jev fit | `internal/routing/router.go` | `TestHardFilterImpossible`, `TestUnauthNotRoutable`, `TestNativeRoutingOnHealthAndCapability` | done |
| Real ACP harness interoperability: script/symlink harnesses get PATH-resolved interpreter dirs; the ACP driver supports `session/set_config_option`/`session/set_model` and the structured final-response contract | `internal/harness/{closure,discover,exec}.go`, `internal/acp/driver.go` | `TestHarnessClosureForScriptHarness`, `TestDiscoverAdvertisedAuthIsUnknownAndRoutable`, `TestCustomHarnessDiscovery`, `TestNativeProcessEnvironmentAndQuoting` | done (native Linux/macOS/Windows) |
| Tool and validation commands use the server OS user's network | `validation.Runner`, `harness.toolManager` | `TestToolRunsInWorkspace`, `TestNativeTrustedLocalExecution` | done |
| macOS runs harnesses/tools normally as the server OS user | `internal/harness`, `internal/validation` | `TestNativeTrustedLocalExecution` (CI matrix) | done |
| Windows runs harnesses/tools normally as the server OS user | `internal/harness`, `internal/validation` | `TestNativeTrustedLocalExecution` (CI matrix) | done |
| Execution availability is uniform across Linux/macOS/Windows | `internal/harness`, `internal/validation` | `TestNativeTrustedLocalExecution` (CI matrix) | done |
| Harness/tool/validation processes run in their own process group where supported and are cleaned up best-effort on cancel/timeout/shutdown | `internal/process`, `internal/harness/toolterm.go`, `internal/validation/discover.go` | `TestToolCancelKillsDescendants`, `TestCancellationInterruptsActiveHarness`, `TestDriverIgnoreCancel` | done |
| Discovery/version/ACP probes run the installed executables normally as the server OS user with a timeout, bounded output, and best-effort cleanup; login-shell PATH is validated (absolute-only, bounded, deduplicated) | `harness.probeOne`, `harness.loginShellPATH`, `harness.runProbeCommand` | `TestCustomHarnessDiscovery`, `TestDiscoverWellKnownDir`, `TestNativeProbeVersionTimeout`, `TestNativeProbeInitializeTimeout` | done |
| Discovery/probing runs after startup recovery | `internal/recovery/recovery.go`, `app.Open` ordering | — | partial (ordering implemented in `app.Open`; no dedicated test) |
| ACP client callbacks: fs read/write scoped to the run workspace; permission requests surface durable approvals; terminal/tool execution is server-interposed and runs as the server OS user | `harness.ACPExec` hooks, `harness.toolManager` | `TestApprovalLifecycle`, `TestToolRunsInWorkspace`, `TestToolWritesWorkspace` | done (approval APPROVE and DENY E2E verified) |
| Object GC, workspace retention, disk-pressure gate | `internal/storage/gc.go`, `scheduler.tick` | `gc_test.go`; low-disk gate blocks write-heavy runs with `BlockedStorage` | done |
| Backup excludes repos and Wayshard credentials (restricted config files) | `internal/backup` | `backup_test.go` | done |
| Startup reconciliation of interrupted attempts/stages and incomplete publication journals; interrupted write attempts restore from a pre-attempt workspace checkpoint using verify-then-use private staging | `internal/recovery`, `orchestrator.checkpointBeforeWrite`, `workspace.RestoreVerified` | `TestRecoveryRestoresInterruptedWriteCheckpoint`, `TestRecoveryBlocksOnCorruptCheckpoint`, `TestProcessBoundaryExecutorCrashRecovery`, `TestProcessBoundaryRepairCrashRecovery`, `TestProcessBoundaryCancelledNeverResumes` | done |
| Durable pre-attempt workspace checkpoints (schema v3) for Executor/Repair with a length-prefixed canonical tree hash (version 3), verify-then-use restore staging, component-based path ownership validation, attempt-scoped lineage, checkpoint retention/pinning, and fail-closed corruption/version/material handling | `internal/storage/checkpoints.go`, `internal/storage/checkpoint_lifecycle.go`, `internal/orchestrator/checkpoint.go`, `internal/workspace/treehash.go`, `internal/workspace/verified_restore.go`, `internal/recovery` | `TestCanonicalTreeHashV3Adversarial`, `TestCanonicalTreeHashVectors`, `TestRecoverySelectsAttemptCheckpointNotStageLatest`, `TestRecoveryLegacyNoCheckpointBlocks`, `TestRecoveryHashVersionFailsClosed`, `TestRecoveryIdempotent`, `TestRecoveryDoesNotTouchSourceWorkspace`, `TestRestoreVerifiedRejectsSourceMutationDuringStaging`, `TestRestoreVerifiedUsesStagedTreeAfterVerification`, `TestRestoreVerifiedAbortsBeforeSwap`, `TestRecoveryDetectsCheckpointTreeCorruption`, `TestRecoveryRejectsCheckpointOutsideRuntimeRoot`, `TestReclaimPinsNonTerminalRunCheckpoints`, `TestReclaimTerminalCheckpoint`, `TestCleanupCheckpointDebris`, `TestWriteAttemptCreatesCheckpoint` | done (integration + Linux process boundary) |
| Transactional cancellation and terminal-run attempt consistency: run status, running attempts, running stages, and pending approvals move together, and startup reconciliation closes any leftover running attempt on a terminal run | `internal/storage/cancel.go`, `internal/recovery` | `TestReconcileLeavesCancelledRun`, `TestCancellationInterruptsActiveHarness`, `TestProcessBoundaryCancelledNeverResumes` | done |
| Cancellation convergence: any cancellation observed during an active run performs the same durable run/attempt/stage/approval cancellation transition before returning, so a run always reaches a terminal cancelled state; correctness does not depend on best-effort process cleanup | `internal/orchestrator/machine.go` (`ProcessRun`), `internal/storage/cancel.go` | `TestProcessRunCancellationDuringStatusReadConverges`, `TestProcessRunCancellationInsideStageConverges`, `TestProcessRunPreCanceledContextConverges`, `TestProcessRunPreservesNonCancellationFailure` | done |
| Native cross-platform execution verification with `wayshard-fake-acp` on Linux/macOS/Windows: discovery → ACP initialize → PLAN → EXECUTE → VALIDATE → REVIEW → INTEGRATE → COMPLETE, with run-workspace isolation (source untouched before integration), integration publishing only the run delta, and correct SQLite stages/attempts/artifacts/route-decisions/events | `internal/app/native_exec_test.go`, `.github/workflows/ci.yml` (`native-execution`) | `TestNativeTrustedLocalExecution`, `TestSourceChangingOrchestrationThroughIntegrate`, `TestFakeACPIntakeToCompleteArtifactOnly`, `TestOrchestratorIntegrationConflictBlocks` | done (CI matrix) |
| Native cancellation/timeout: cancellation terminates a running harness; hanging version/initialize probes and the ACP handshake are bounded; descendant cleanup is best-effort | `internal/app/cancel_test.go`, `internal/harness/native_timeout_test.go`, `internal/acp/native_test.go`, `internal/process` | `TestCancellationInterruptsActiveHarness`, `TestNativeProbeVersionTimeout`, `TestNativeProbeInitializeTimeout`, `TestNativeHandshakeTimeout`, `TestDriverTimeout` | done (CI matrix) |

## Trusted local execution

Discovered ACP harnesses, ACP-requested tools, validation commands, and discovery
probes run as the server OS user with their normal configuration,
authentication, environment, filesystem, and network access on Linux, macOS, and
Windows. `internal/process` provides best-effort process-group cleanup, and
Wayshard credentials live in `internal/credentials` (restricted config files or
environment variables). Wayshard owns approvals, budgets, routing, isolated run
workspaces, snapshots/checkpoints, server-owned validation and completion,
recovery, run-delta provenance, conflict-safe integration, ACP capability
probing, timeouts, and cancellation; harness-owned credentials stay with the
harness.

## Clients

| Requirement | Code | Tests | Status |
|---|---|---|---|
| OpenCode-derived shared graphical client | `clients/ui` (`@wayshard/ui` design system, copied+adapted), `clients/gui` (adapted `session-ui` + Wayshard app/state) | `clients/gui/src/lineage.test.ts`, `clients/gui/src/wayshard/adapter.test.ts`, `bun run build` (web) | done |
| Web mounts the shared graphical client | `clients/web/src/wayshard/main.tsx` → `@wayshard/gui` | vite build | done |
| Desktop/Android Tauri 2 hosts the shared GUI | `clients/desktop` (`frontendDist ../../web/dist`), `ANDROID.md` | desktop `tsc` typecheck | done (packaging; on-device runtime unverified in CI) |
| Real OpenTUI TUI | `clients/tui` (`@opentui/solid` + imported theme system) | `clients/tui/src/model.test.ts`; TUI launches and renders | done |
| Shared SDK + live events | `clients/sdk` (HTTP/WS), `clients/gui/src/wayshard/state.tsx` | sdk unit test | done |
| Live clients carry no OpenCode runtime/import specifier | `clients/{sdk,ui,gui,web,tui}` | `lineage.test.ts` asserts no `@opencode-ai/` in live source | done |
| Source lineage is verifiable | `clients/lineage.manifest.json`, `docs/client-source-lineage.md` | `lineage.test.ts` (subtree sizes/markers) | done |

## CI/CD

| Requirement | Code | Tests | Status |
|---|---|---|---|
| PR CI without secrets, paid models, or installed harnesses | `.github/workflows/ci.yml` | workflow | done |
| Reproducible release builds: commit-derived build date, `-trimpath`, one shared flag set, single CLI build, standalone CLI == bundled CLI per platform plus same-commit rebuild proof | `Makefile`, `scripts/release/verify-reproducible.sh`, `.github/workflows/release.yml` (`reproducible`) | `scripts/release/verify-reproducible.sh` in the release `reproducible` job | done |
| Compiled TUI reports its release identity | `clients/tui/build.ts`, `clients/tui/src/version.ts`, `clients/tui/src/index.tsx` | `clients/tui/src/version.test.ts`, `scripts/ci/tui_smoke.sh` (`--version` assertion) | done |
| CycloneDX SBOM license metadata from real dependency license evidence; unknown licenses fail rather than fabricate | `scripts/release/sbom.sh` | `sbom.sh` post-generation validation (release `release` job) | done |
| First-party installers (Linux/macOS `sh`, Windows `ps1`): latest-stable resolution, OS/arch detection, checksum + optional minisign verification, atomic user install, idempotent PATH | `scripts/install.sh`, `scripts/install.ps1` | `scripts/release/install_sh_test.sh`, `scripts/release/install_ps1_test.ps1`, `.github/workflows/ci.yml` (`installers` matrix) | done |
| GitHub Actions on the current Node runtime; Linux runners pinned to `ubuntu-24.04` | `.github/workflows/ci.yml`, `.github/workflows/release.yml` | `scripts/release/release_policy_test.sh`; workflow review | done |
| Linux/macOS/Windows server + CLI | `ci` go job + `make build-cross` | workflow | done |
| Native execution verification on Linux/macOS/Windows | `.github/workflows/ci.yml` `native-execution` job | fake ACP harness end-to-end | done |
| Desktop Linux | `.github/workflows/release.yml` job `desktop` | checksums + minisign | done |
| Desktop macOS ad-hoc sign (identity `-`) | `tauri.conf.json` `bundle.macOS.signingIdentity`, `APPLE_SIGNING_IDENTITY=-`, `scripts/release/macos-verify-adhoc.sh` | `release_policy_test.sh`; live `codesign` on macOS runners | done |
| Desktop Windows self-signed Authenticode | `scripts/release/windows-sign.ps1`; secrets `WAYSHARD_WINDOWS_PFX_*` | `windows_pfx_test.sh`; live `signtool` on Windows runners | done |
| Android Tauri APK with maintainer JKS, no Google Play | job `android`, `scripts/release/android-sign.sh`, `clients/desktop/android/WayshardKeystore.{kt,pro}` | `android_jks_test.sh`, `android_patch_test.sh`, `android_keystore_patch_test.sh`, `android_keystore_verify_test.sh` (real R8); APK verified | done |
| Canonical branding mark → all platform icons | `assets/branding/wayshard.{png,ico}`, `clients/desktop/src-tauri/icons`, `clients/web/public`, `scripts/release/android-icons.sh`, NSIS `installerIcon` | `icon_policy_test.sh`, `windows_installer_icon_test.sh`, `release_policy_test.sh`; transparent derivatives match the mark, the Android foreground fits the adaptive safe circle, and the built `-setup.exe` embeds the Wayshard installer icon | done |
| Static, CGO-free Linux binaries | `Makefile` `CGO_ENABLED=0` build rules, `tui` job native CLI | `linux_static_test.sh`; release job asserts `statically linked` | done |
| Android APK Signature Scheme v2 + v3 | `scripts/release/android-patch-gradle.py`, `package-android.sh` | `android_patch_test.sh`; `package-android.sh` asserts v2 and v3 | done |
| Web social card served by the published server | `clients/web/public/social-share.png`, `clients/web/index.html`, release `release` job embed check | `icon_policy.py`; server returns 200 for `/social-share.png` | done |
| Tauri CLI aligned with the locked crate | `clients/desktop/package.json` `@tauri-apps/cli` == `Cargo.lock` `tauri` major.minor | `icon_policy.py` `check_tauri_alignment` | done |
| Minisign on combined `SHA256SUMS.txt` | `scripts/release/minisign-sign.sh`; secrets `WAYSHARD_RELEASE_MINISIGN_*`; public key `keys/wayshard-release.minisign.pub` | `minisign_test.sh`; checksums job verifies before upload | done |
| Checksums once per file, all downloadable artifacts | `scripts/release/checksums.py`, jobs `release`/`desktop`/`desktop-macos-checksums`/`android`/`checksums` | `make release-scripts-test`; `desktop_macos_checksums_test.sh` proves both macOS DMGs appear exactly once, order-independent | done |
| CycloneDX SBOM | `scripts/release/sbom.sh` | fails the release job on generator/validation error | done |
| Frozen client lockfile on release and PR install | `bun install --frozen-lockfile` in `ci.yml` + `release.yml` | lockfile `clients/bun.lock` | done |
| Dependency-security triage of Dependabot findings (reachability, minimum safe version, upgrade risk) | `go.mod`, `clients/web/package.json`, `clients/bun.lock`, `clients/desktop/src-tauri/Cargo.lock`; record in `docs/dependency-security.md` | reachability via `go list -deps`/`go mod why`/`cargo tree`; upgrades validated by `make ci` + client typecheck/tests | done (accepted `glib` documented) |
| Release credentials isolated | `environment: release` on publish jobs; `ci.yml` has no `secrets.*` | `release_policy_test.sh`; PR CI remains secret-free | done |
| Web embedded in server | `internal/webembed` | embed dist before `make build-cross` | done |

## External / manual only (not implementation failures)

| Item | Why it cannot be closed in-repo |
|---|---|
| GitHub org/repo administration, Actions enablement, Environment `release` | operator |
| Android JKS, Windows PFX, minisign secret key stored as Environment `release` secrets | operator-generated material; the workflow consumes them |
| Commit of `keys/wayshard-release.minisign.pub` | operator |
| `TYPESAFE_API_KEY` live Jev | runtime credential, not Actions |
| User-installed OpenCode/Codex/etc. | product boundary: Wayshard never installs harnesses |
| Apple Developer ID / notarization / commercial Windows CA / Play Console | intentionally not used |
| Tailscale Serve / tunnel | user networking |
| Real-harness compatibility jobs | provisioned environments only |
