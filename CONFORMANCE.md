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
| SecretVault envelope encryption, no replace on unlock fail | `internal/secrets` | `internal/secrets/vault_test.go` | done |
| APIs never return secret plaintext | `Vault.Status` | `TestAPIsDoNotReturnPlaintext` | done |

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
| Validation commands run through the Tool Sandbox (no host FS, no ambient secrets) | `validation.Runner` + `sandbox.ToolPolicy` | `TestValidationRunsInToolSandbox`, `TestValidationOrdinaryCommandStillWorks`, `TestValidationNetworkNoneEnforced` | done |
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
| Linux sandbox: Landlock filesystem confinement + seccomp-BPF communication-socket confinement (`socket(2)` all domains, `io_uring_setup(2)`, x32 ABI rejection), process group, pdeathsig, env allowlist. `NetworkNone` is the default for harness and tool/validation execution. Raw `unrestricted` host networking requires an explicit unsafe opt-in and is never selected by required-isolation policies. Secure **provider-only** networking is implemented for Linux: a provider-capable harness runs in a per-attempt user+network namespace whose only reachable endpoint is a per-attempt Wayshard HTTPS `CONNECT` broker; seccomp provider mode permits only `AF_INET`/`AF_INET6` `SOCK_STREAM` and denies UDP, `AF_UNIX`, `AF_NETLINK`, `AF_PACKET`, `io_uring` and the x32 ABI. A distinct **loopback-only** mode (`NetLoopback`) gives the ACP discovery probe a private network namespace with only `lo` (no host/LAN/public route, TCP stream only) for harnesses whose ACP server needs local IPC. Harness/probe policies additionally use a private PID + mount namespace with a scoped procfs (`ProcIsolation`) so runtimes such as Bun can read `/proc` without exposing host processes; both capabilities are runtime-probed and best-effort, and `/proc` is only granted when the scoped mount succeeded. macOS/Windows fail closed. | `internal/sandbox/{linux,landlock_linux,seccomp_linux,helper_linux,helper,netlink_linux,env,policy,compile,login_shell,probe}.go`, `internal/provider/` | `TestLandlockFilesystemConfinement`, `TestLandlockReadOnlyView`, `TestNetworkNoneEnforced`, `TestProductionHarnessPolicyIsNetworkNone`, `TestProviderNetworkPolicyNeverSilentlyUnrestricted`, `TestLoopbackProbeIsolation`, `TestExplicitUnsafeHostNetworkWorks`, `TestUnsupportedNetworkPoliciesFailClosed`, `TestEnvAllowlistDropsHostSecrets`, `TestProviderNetnsEnforcement` | done (Linux black-box, amd64/arm64) |
| Provider-only network capability and broker: per-attempt user+network namespace, loopback proxy, private per-attempt Unix broker, destination authorization (host+port) and per-connection address validation (loopback/private/link-local/multicast/unspecified/broadcast/IPv4-mapped/6to4/Teredo denied), DNS-rebinding revalidation, end-to-end TLS via `CONNECT` only (no MITM CA), per-attempt bearer and cross-run isolation. CONNECT/bearer headers are bounded (64 KiB) and upstream resolve/dial has a finite timeout. Runtime-probed capability; unsupported platforms fail closed. | `internal/provider/{dest,validate,broker,config,capability_linux,shim_linux,netlink_linux}.go` | `TestPolicyAuthorize`, `TestPolicyCheckConfigured`, `TestIsDisallowedAddr`, `TestResolveValidatedRejectsRebindingAndPrivate`, `TestBrokerRejectsBadBearerAndMethods`, `TestBrokerRejectsDisallowedResolvedAddress`, `TestBrokerTunnelsAuthorizedConnect`, `TestBrokerBoundsConnectHeaders`, `TestBrokerCrossRunIsolation`, `TestProviderNetnsEnforcement` | done (Linux black-box, amd64/arm64) |
| Provider routing requires all four independent conditions: user/policy permission, actual platform capability, harness transport compatibility, and a configured destination policy. Raw host networking is never chosen as provider access. Transport compatibility is declared only from observed evidence (a real harness reaching an authorized destination through the broker). | `internal/routing/router.go`, `internal/harness/exec.go`, `internal/app/app.go`, `cmd/wayshard-server` flags | `TestProviderRouteMatrix`, `TestProviderHarnessRouteFailsClosed`, `TestACPExecProviderFailsClosed`, `TestProviderExecLaunchesThroughSecureBroker`, `TestRealHarnessProviderCall` (native) | done (Linux; real-harness transport verified natively) |
| Real ACP harness interoperability: script/symlink harnesses receive a narrow launch closure (package tree + PATH-resolved interpreter), the ACP driver tolerates extension notifications and supports `session/set_config_option`/`session/set_model` model selection, and real harnesses are given the universal structured final-response contract so stage artifacts validate server-side. OpenCode v2.0.5 and codex-acp 1.12.0 initialize over real ACP. The ACP discovery probe uses an isolated loopback-only network namespace when the platform supports it, so harnesses whose ACP server needs local IPC (OpenCode) are discovered route-viable; a configured provider model (`--provider-model`) is applied to provider candidates that do not advertise models. OpenCode completed a real source-changing task end-to-end through normal discovery and routing with secure provider networking. | `internal/harness/{closure,discover,exec}.go`, `internal/acp/driver.go`, `internal/sandbox/{policy,probe,linux,helper_linux,netlink_linux}.go`, `internal/app/app.go` | `TestHarnessClosureForScriptHarness`, `TestHarnessClosureForNativeBinary`, `TestProbeVersionArgs`, `TestDiscoverAdvertisedAuthIsUnknownAndRoutable`, `TestLoopbackProbeIsolation`, `TestRealHarnessProviderCall` (native), `TestRealHarnessSourceChangingE2E` (native) | done (native Linux; CI uses fixture-backed tests only) |
| Tool/validation network defaults to `NetworkNone`; project/tool commands cannot reach TCP/UDP/Unix/docker.sock | `validation.Runner` | `TestValidationNetworkNoneEnforced` | done (Linux black-box) |
| macOS sandbox: `sandbox-exec` seatbelt profile compiled from SandboxPolicy; process group; fail if missing when Required | `internal/sandbox/seatbelt.go`, `darwin.go` | `TestSeatbeltProfileCompilation` (all OS) | code present; native runtime enforcement UNVERIFIED (no macOS execution here) |
| Windows sandbox: Job Objects + new process group; AppContainer not claimed; fail closed if unavailable | `internal/sandbox/windows.go` | cross-compile + policy tests | code present; native runtime enforcement UNVERIFIED |
| Required isolation never silently unrestricted | `LinuxBackend.Compile`/`Constrain` fail closed when Landlock unavailable; unsupported backends error | `TestRequiredIsolationNeverSilent`, `TestReducedSecurityStillDoesNotSilentlyUnrestrict`, `TestUnsupportedConstrainFailsClosed`, `TestProbeNeverClaimsUnrestricted` | done |
| Harness and tool launch apply the compiled filesystem/env policy to the process and descendants | `sandbox` helper re-exec + `harness.ACPExec` + `validation.Runner` | confinement tests (host read/write denied, workspace allowed, child/grandchild confined) | done (Linux) |
| Harness discovery/version/ACP-initialize probes run under a dedicated ProbePolicy: NetworkNone, synthetic HOME/TEMP, env allowlist, read-only system + executable roots, no project/SourceWorkspace/Wayshard-runtime/SSH-agent/display access, bounded output and descendant cleanup; fail closed when isolation is unavailable. Script/symlink harnesses get a narrow read-only launch closure (their package tree plus a PATH-resolved interpreter) and a scoped procfs when the platform can mount one. The login-shell PATH probe is a least-privilege exception: it reads only the shell executable directory and the specific per-shell startup files actually required (never the home directory or a blanket `/etc`), and its output is validated (absolute only, no cwd/relative/control-character entries, bounded, deduplicated). | `sandbox.ProbePolicy`, `sandbox.LoginShellPolicy`, `sandbox.SanitizeLoginPATH`, `sandbox.RunConstrainedOutput`, `harness.closure`, `harness.probeOne`, `harness.loginShellPATH` | `TestProbePolicyConfinesMaliciousVersionProbe`, `TestProbePolicyConfinesACPInitialize`, `TestProbeTimeoutKillsDescendants`, `TestProbeOutputBounded`, `TestLoginShellPolicyDeniesHomeSecrets`, `TestSanitizeLoginPATH`, `TestLoginShellStartupFilesSelection`, `TestHarnessClosureForScriptHarness` | done (Linux verified; macOS/Windows native probe enforcement unverified) |
| Discovery probes are server-owned and reconcilable: each probe process tree receives a durable `probe_owners` record (token hash persisted before launch), completes only when no owned descendant remains, and startup reconciliation terminates any daemonized (setsid) probe descendant by token before discovery runs again. Discovery/probing runs only after startup recovery. | `internal/recovery/recovery.go` (`reconcileProbeOwners`), `internal/storage/probe_owners.go`, `migrations/005_probe_owners.sql`, `harness.StoreProbeOwnerSink`, `app.Open` ordering | `TestProcessBoundaryProbeDescendantReconciled`, `TestStartupOrderingRecoveryBeforeDiscovery`, `TestUpgradeFromPriorVersions` | done (Linux process boundary; other platforms report ownership unsupported and fail closed for a live run) |
| ACP client callbacks: fs read/write scoped to run workspace; permission requests surface durable approvals; terminal/tool execution runs through the Tool Sandbox (NetworkNone, allowlisted env, workspace-scoped, server-owned process tree) | `harness.ACPExec` hooks, `harness.toolManager` | `TestToolRunsInSandbox`, `TestToolWritePermission`, `TestToolCancelKillsDescendants`; `TestApprovalLifecycle`; callback scoping via `withinRoot` | done (Linux process-boundary; approval APPROVE and DENY E2E verified) |
| Object GC, workspace retention, disk pressure gate | `internal/storage/gc.go`, `scheduler.tick` | `gc_test.go`; low-disk gate blocks write-heavy runs with `BlockedStorage` | done |
| Backup excludes repos; optional secrets | `internal/backup` | `backup_test.go` | done |
| Startup reconciliation of interrupted attempts/stages, stale server-owned process trees, and incomplete publication journals; interrupted write attempts restore from a pre-attempt workspace checkpoint using verify-then-use private staging | `internal/recovery`, `orchestrator.checkpointBeforeWrite`, `workspace.RestoreVerified`, `internal/process` | `TestRecoveryRestoresInterruptedWriteCheckpoint`, `TestRecoveryBlocksOnCorruptCheckpoint`, `TestWriteAttemptCreatesCheckpoint`, `TestProcessBoundaryExecutorCrashRecovery`, `TestProcessBoundaryRepairCrashRecovery`, `TestProcessBoundaryCancelledNeverResumes`, `TestReconcileKillsOwnedDescendantsAndSparesUnrelated`, `TestProcessBoundaryOrphanGrandchildReconciled`, `TestProcessBoundaryOrphanBlockedRecovery`, `TestProcessBoundaryUnrelatedProcessSurvives`, `TestProcessBoundaryMultipleRunOrphans` | done (Linux process boundary); orphan ownership is Linux-only (macOS/Windows fail closed for a live run and remain unverified) |
| Durable pre-attempt workspace checkpoints (schema v3) for Executor/Repair with unambiguous length-prefixed canonical tree hash (version 3), verify-then-use restore staging, component-based path ownership validation, attempt-scoped lineage, checkpoint retention/pinning, and fail-closed corruption/version/material handling | `internal/storage/checkpoints.go`, `internal/storage/checkpoint_lifecycle.go`, `internal/orchestrator/checkpoint.go`, `internal/workspace/treehash.go`, `internal/workspace/verified_restore.go`, `internal/recovery` | `TestCanonicalTreeHashV3Adversarial`, `TestCanonicalTreeHashVectors`, `TestRecoverySelectsAttemptCheckpointNotStageLatest`, `TestRecoveryLegacyNoCheckpointBlocks`, `TestRecoveryHashVersionFailsClosed`, `TestRecoveryIdempotent`, `TestRecoveryDoesNotTouchSourceWorkspace`, `TestRestoreVerifiedRejectsSourceMutationDuringStaging`, `TestRestoreVerifiedUsesStagedTreeAfterVerification`, `TestRestoreVerifiedAbortsBeforeSwap`, `TestRecoveryDetectsCheckpointTreeCorruption`, `TestRecoveryRejectsCheckpointOutsideRuntimeRoot`, `TestCheckpointPathOwnership`, `TestReconcileRejectsReclaimedCheckpoint`, `TestReclaimPinsNonTerminalRunCheckpoints`, `TestReclaimTerminalCheckpoint`, `TestCleanupCheckpointDebris`, `TestWriteAttemptCreatesCheckpoint`, `TestUpgradeFromPriorVersions` | done (integration + Linux process boundary) |
| Transactional cancellation and terminal-run attempt consistency: run status, running attempts, running stages and pending approvals move together, and startup reconciliation closes any leftover running attempt on a terminal run | `internal/storage/cancel.go`, `internal/recovery` | `TestReconcileCancelledRunAttemptConsistency`, `TestCancellationInterruptsActiveHarness`, `TestProcessBoundaryCancelledNeverResumes` | done |

## Clients

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Web full surfaces (session/changes/files/terminal + overlays) | `clients/web/src/wayshard` | `app.test.ts`, vite build | partial (raw JSON views, polling not WS, no pairing UI, no diff viewer) |
| CLI/TUI full client same API | `cmd/wayshard` (shipped), `clients/tui` (not packaged) | tui test, go build | partial (shipped CLI is a thin scriptable client; richer bun TUI not built by `make`/`package-go.sh`) |
| Desktop Tauri 2 + local server provision independent of window | `clients/desktop/src-tauri` | Rust `provision_local_server`; `cargo check` | partial (GUI runtime not exercised; no Linux detach/setsid) |
| Android is Tauri 2 of the same web client, not a custom WebView | `clients/desktop` identifier `dev.wayshard.app`, `ANDROID.md`, release `tauri android build` | CI release android job | UNVERIFIED runtime (packaging only; no device/emulator run) |
| Shared SDK | `clients/sdk` | sdk unit test | done |

## CI/CD

| Requirement | Code | Tests | Status |
|---|---|---|---|
| PR CI no secrets/paid models/harness installs | `.github/workflows/ci.yml` | workflow | done |
| Linux/macOS/Windows server+CLI | ci.go job + `make build-cross` | workflow | done |
| Desktop Linux | `.github/workflows/release.yml` job `desktop` | checksums + minisign; no paid Linux signing | done |
| Desktop macOS ad-hoc sign (identity `-`, no Apple account) | `tauri.conf.json` `bundle.macOS.signingIdentity`, `APPLE_SIGNING_IDENTITY=-`, `scripts/release/macos-verify-adhoc.sh` | `release_policy_test.sh`; live `codesign` on macOS runners only; Gatekeeper warnings expected | done |
| Desktop Windows self-signed Authenticode | `scripts/release/windows-sign.ps1`; secrets `WAYSHARD_WINDOWS_PFX_*` | `windows_pfx_test.sh`; live `signtool` on Windows runners only; SmartScreen/untrusted publisher expected | done |
| Android Tauri APK, maintainer JKS, no Play | job `android`, `scripts/release/android-sign.sh` | `android_jks_test.sh`, `android_patch_test.sh`; APK verified; unsigned not published as signed | done |
| Minisign on combined SHA256SUMS.txt | `scripts/release/minisign-sign.sh`; secrets `WAYSHARD_RELEASE_MINISIGN_*`; public key `keys/wayshard-release.minisign.pub` | `minisign_test.sh`; checksums job verifies before upload | done |
| Checksums once per file, all downloadable artifacts | `scripts/release/checksums.py`, jobs `release`/`desktop`/`android`/`checksums` | `make release-scripts-test` (overlapping globs cannot duplicate server rows) | done |
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

## Post-audit remediation (P0 pass)

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
- **Orphan process reconciliation**: each stage attempt owns a token inherited by its harness and Tool descendants; the token hash is persisted before launch. Startup terminates every surviving descendant (including backgrounded/setsid grandchildren) before any workspace restore, emits `execution.orphan_reconciled`, and fails closed with BLOCKED/RECOVERY if an owned tree cannot be terminated. Synthetic HOME/TEMP is per attempt. Ownership is Linux-verified; other platforms report the capability as unsupported and fail closed for a live run.
- **Checkpoint lifecycle**: checkpoint material for a non-terminal run is pinned; terminal-run material is reclaimed after retention and its metadata is marked `reclaimed`; unreferenced checkpoint directories, restore staging and terminal sandbox directories are cleaned at startup. Recovery rejects reclaimed material.

Still partial or unverified after this pass:

- Real installed ACP harness interoperability (no supported harness was installed; only the deterministic fake was exercised).
- macOS/Windows runtime sandbox enforcement and orphan-process reconciliation (code present; Linux-only and not executed natively on other platforms; a live run with an unreconcilable owner fails closed there).
- ACP terminal/tool callbacks are wired through the Tool Sandbox (Linux process-boundary verified); a true kill-mid-recovery process test remains outstanding.
- Context assembly is injected into orchestration stages (P1); stage bundles and their durable manifest are verified.
- Web/Desktop/Android runtime parity and client completeness (see the Clients table).
- Live Jev, model metadata enrichment, and event retention/pruning.
- Real provider-backed harness interoperability: no supported real harness was verified against the secure provider transport in this pass. The transport is proven with a deterministic malicious ACP fixture and the production shim/broker; a real-harness E2E remains a provisioned-environment task.
- Provider broker idle-timeout/global rate limiting is intentionally minimal (bounded handshake only); the per-attempt broker is closed with its stage.

## Post-audit remediation (P1 Pass 1C-1)

P1 Pass 1C-1 closed the three non-blocking discovery findings from the Pass-1B audit and implemented an enforceable secure provider-network capability:

- **F1 — login-shell PATH least privilege**: `sandbox.LoginShellPolicy` grants only the shell executable directory and the specific per-shell startup files (`/etc/profile`, enumerated `/etc/profile.d/*`, and the shell's own home startup files); it no longer grants the real home directory or a blanket `/etc`. `SanitizeLoginPATH` validates untrusted output (absolute-only, no cwd/relative/control-character entries, bounded, deduplicated). A malicious profile cannot read `~/.aws-credentials`, `~/.ssh/*`, `~/.git-credentials`, or an unrelated `/etc` file.
- **F2 — startup ordering**: harness discovery/probing now runs after `recovery.Reconcile` (stale process and probe reconciliation, terminal-state repair, checkpoint/publication recovery, cleanup), and the scheduler still starts last. `TestStartupOrderingRecoveryBeforeDiscovery` observes the durable step order.
- **F3 — probe process ownership**: discovery probes are registered in a durable `probe_owners` table (schema v5) before launch; the token is inherited by descendants; a probe completes only when no owned descendant remains. Startup reconciliation terminates daemonized (setsid) probe descendants by token before discovery runs again. `TestProcessBoundaryProbeDescendantReconciled` SIGKILLs a real server whose probe left a setsid grandchild and proves the next server reconciles it while an unrelated process survives.
- **Secure provider networking (Linux)**: a provider route requires permission **and** platform capability **and** harness transport compatibility **and** a destination policy, all independent. The harness runs in a per-attempt user+network namespace whose only reachable endpoint is a per-attempt HTTPS `CONNECT` broker on a private Unix socket; the broker authorizes `host:port`, resolves on the trusted side, and revalidates every resolved address (loopback/private/link-local/multicast/unspecified/broadcast/IPv4-mapped/6to4/Teredo denied) on every connection, closing DNS rebinding and SSRF. Proxy variables are only transport; the namespace is the boundary, so direct sockets to localhost/LAN/Internet, UDP, AF_UNIX, Docker, `AF_NETLINK` and `AF_PACKET` all fail. `TestProviderNetnsEnforcement` proves the full bypass matrix including child/grandchild and DNS. Tool callbacks and validation remain `NetworkNone`.

Still partial or unverified after this pass:

- Real installed ACP harness interoperability against the secure provider transport (deterministic fixture only; no harness is installed by Wayshard).
- macOS/Windows provider networking: unavailable and fail closed.
- macOS/Windows runtime sandbox enforcement and orphan-process reconciliation (code present; Linux-only and not executed natively on other platforms; a live run with an unreconcilable owner fails closed there).

## P1 Pass 1C-2 — real harness end-to-end

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
- `grok` and `claude` have executables but no Wayshard definition.
- A real-harness integration-conflict run was not performed (deterministic conflict coverage remains).
