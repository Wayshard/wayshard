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
| Local recovery pairing when no devices | `allowLocalAdmin` | API tests use loopback first-run | done |
| SecretVault envelope encryption, no replace on unlock fail | `internal/secrets` | `internal/secrets/vault_test.go` | done |
| APIs never return secret plaintext | `Vault.Status` | `TestAPIsDoNotReturnPlaintext` | done |

## Projects, knowledge, context

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Open/clone/create/locate/remove; open passive | `internal/api/server.go` | `TestOpenProjectIsPassive`, `TestRemoveProjectDoesNotDeleteSource` | done |
| Knowledge discovery, cycles, six-file recognition, no `.wayshard` | `internal/knowledge` | `internal/knowledge/discover_test.go` | done |
| Stage-specific context + manifest | `internal/ctxengine` | `internal/ctxengine/engine_test.go` | done |

## Orchestration

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Assess → plan → execute (fake ACP writes run workspace) → validate → review → READY_TO_INTEGRATE → three-way integrate → COMPLETE | `internal/orchestrator/machine.go`, `internal/harness/exec.go`, `cmd/wayshard-fake-acp` | `TestSourceChangingOrchestrationThroughIntegrate` (`internal/app/source_e2e_test.go`); asserts source unchanged until integrate, dirty `user.txt` stays user-owned, RunDelta is agent-only, journal present, complete only after publish | done |
| Integration conflict through orchestrator (not integration package alone) | `conflictBefore` wrapping `IntegrateAdapter` | `TestOrchestratorIntegrationConflictBlocks` | done |
| Append-only attempts | `AppendAttempt` | `TestFailedAttemptNotRewritten` | done |
| NO_VIABLE_ROUTE | `internal/routing` | `TestHardFilterImpossible`, `TestNoViableRouteWithoutHarness` | done |
| Jev DecisionEngine + deterministic fallback | `internal/jev` | routing tests with DeterministicEngine | done |
| Fake ACP scenarios | `cmd/wayshard-fake-acp`, `internal/harness/exec.go` | `TestACPExecPlanViaFakeHarness`, `TestACPExecAuthRequired`, `internal/acp/driver_test.go` | done |
| Invalid stage output bounded retry | `runStage` correction attempt | orchestrator machine | done |
| Validation server-owned, passive discovery | `internal/validation` | `TestPassiveDiscoveryDoesNotExecute` | done |
| CompletionPolicy Go-owned | `internal/orchestrator/completion.go` | `completion_test.go` | done |
| Artifact-only complete without integrate | CompletionPolicy + e2e | `TestArtifactOnlyCompletesWithoutIntegration` | done |

## Workspace / Git / files / PTY

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Isolated run workspace, dirty baseline | `internal/workspace` | `TestDirtyBaselineNotAttributedToAgent` | done |
| Three-way integrate, branch block, journal, no auto-stage | `internal/integration` | `integrate_test.go` | done |
| File save CAS / stale hash | `PUT /v1/projects/{id}/file` | `TestFileSaveConflict` | done |
| Server-owned PTY, disconnect does not kill, restart reports loss | `internal/pty`, `GET /v1/ws/pty` | PTY start uses `exec.Command` not request ctx; 410 on missing | done |

## Security / storage / backup

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Linux sandbox: user/net/mount/pid/uts namespaces + uid/gid map + process group + pdeathsig; constrain-or-fail if namespaces missing | `internal/sandbox/linux.go` | `TestNativeCompileAndConstrain`, `TestProcessTreeKill`, `TestCompileRequiresWritableRoots` (linux CI) | done |
| macOS sandbox: `sandbox-exec` seatbelt profile compiled from SandboxPolicy (FS roots, network deny/allow); process group; fail if `sandbox-exec` missing when Required | `internal/sandbox/seatbelt.go`, `darwin.go`, `darwin_stub.go` | `TestSeatbeltProfileCompilation` (all OS); `TestNativeCompileAndConstrain` / `TestProcessTreeKill` on darwin CI; `TestForeignBackendsAreUnavailableHere` | done |
| Windows sandbox: Job Objects (kill-on-close, active-process, job memory) + new process group; AppContainer **not claimed**; fail closed if CreateJobObject/assign fails when Required | `internal/sandbox/windows.go`, `windows_stub.go` | compiled `GOOS=windows go build ./...`; `TestNativeCompileAndConstrain` / `TestProcessTreeKill` on windows CI; `TestForeignBackendsAreUnavailableHere` | done |
| Required isolation never silently unrestricted; unsupported backends error even if Required=false | `Manager.Start`, `AsConstrainer`, `UnsupportedBackend` | `TestRequiredIsolationNeverSilent`, `TestReducedSecurityStillDoesNotSilentlyUnrestrict`, `TestUnsupportedConstrainFailsClosed`, `TestProbeNeverClaimsUnrestricted` | done |
| Harness launch applies compiled policy (`SetupCmd`/`AfterStart`) instead of policy types only | `internal/acp.Spec`, `internal/harness/exec.go` | fake-ACP e2e under constrained launch | done |
| Object GC, workspace retention, disk pressure | `internal/storage/gc.go` | `gc_test.go` | done |
| Backup excludes repos; optional secrets | `internal/backup` | `backup_test.go` | done |
| Startup reconciliation | `internal/recovery` | used in `app.Open` | done |

## Clients

| Requirement | Code | Tests | Status |
|---|---|---|---|
| Web full surfaces (session/changes/files/terminal + overlays) | `clients/web/src/wayshard` | `app.test.ts`, vite build | done |
| CLI/TUI full client same API | `cmd/wayshard`, `clients/tui/src/index.ts` | tui test, go build | done |
| Desktop Tauri 2 + local server provision independent of window | `clients/desktop/src-tauri` | Rust `provision_local_server` | done |
| Android is Tauri 2 of the same web client, not a custom WebView | `clients/desktop` identifier `dev.wayshard.app`, `ANDROID.md`, release `tauri android build` | CI release android job | done |
| Shared SDK | `clients/sdk` | sdk unit test | done |

## CI/CD

| Requirement | Code | Tests | Status |
|---|---|---|---|
| PR CI no secrets/paid models/harness installs | `.github/workflows/ci.yml` | workflow | done |
| Linux/macOS/Windows server+CLI | ci.go job + `make build-cross` | workflow | done |
| Desktop Linux/macOS/Windows | release desktop matrix | workflow | done |
| Android Tauri APK | release android job | workflow | done |
| Checksums, notices, Go SBOM | release job | workflow | done |
| Web embedded in server | `internal/webembed` | embed dist | done |

## External / manual only (not implementation failures)

| Item | Why it cannot be closed in-repo |
|---|---|
| GitHub org/repo administration, Actions enablement | operator |
| `TYPESAFE_API_KEY` live Jev | credential |
| User-installed OpenCode/Codex/etc. | product boundary: never install harnesses |
| Android signing keystore | secret |
| Tailscale Serve / tunnel | user networking |
| Real-harness compatibility jobs | provisioned environments only |
