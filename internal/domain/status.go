package domain

// RunStatus is the semantic lifecycle of a Run.
type RunStatus string

const (
	RunQueued             RunStatus = "queued"
	RunAssessing          RunStatus = "assessing"
	RunPlanning           RunStatus = "planning"
	RunExploring          RunStatus = "exploring"
	RunExecuting          RunStatus = "executing"
	RunValidating         RunStatus = "validating"
	RunReviewing          RunStatus = "reviewing"
	RunRepairing          RunStatus = "repairing"
	RunReplanning         RunStatus = "replanning"
	RunReadyToIntegrate   RunStatus = "ready_to_integrate"
	RunIntegrating        RunStatus = "integrating"
	RunComplete           RunStatus = "complete"
	RunBlocked            RunStatus = "blocked"
	RunFailed             RunStatus = "failed"
	RunCancelled          RunStatus = "cancelled"
	RunIntegrationBlocked RunStatus = "integration_blocked"
	RunSafeMode           RunStatus = "safe_mode"
)

func (s RunStatus) Terminal() bool {
	switch s {
	case RunComplete, RunFailed, RunCancelled:
		return true
	default:
		return false
	}
}

// StageKind is a semantic workflow step.
type StageKind string

const (
	StageAssess    StageKind = "assess"
	StagePlan      StageKind = "plan"
	StageExplore   StageKind = "explore"
	StageExecute   StageKind = "execute"
	StageValidate  StageKind = "validate"
	StageReview    StageKind = "review"
	StageRepair    StageKind = "repair"
	StageReplan    StageKind = "replan"
	StageIntegrate StageKind = "integrate"
)

func (k StageKind) ReadOnly() bool {
	switch k {
	case StageAssess, StagePlan, StageExplore, StageReview, StageReplan:
		return true
	default:
		return false
	}
}

func (k StageKind) WritesWorkspace() bool {
	switch k {
	case StageExecute, StageRepair:
		return true
	default:
		return false
	}
}

// StageAttemptStatus is infrastructure-level attempt outcome. Attempts are append-only.
type StageAttemptStatus string

const (
	AttemptPending     StageAttemptStatus = "pending"
	AttemptRunning     StageAttemptStatus = "running"
	AttemptSucceeded   StageAttemptStatus = "succeeded"
	AttemptFailed      StageAttemptStatus = "failed"
	AttemptInterrupted StageAttemptStatus = "interrupted"
	AttemptCancelled   StageAttemptStatus = "cancelled"
	AttemptInvalid     StageAttemptStatus = "invalid_output"
)

// CheckStatus is a validation check result.
type CheckStatus string

const (
	CheckPass        CheckStatus = "pass"
	CheckFail        CheckStatus = "fail"
	CheckWarn        CheckStatus = "warn"
	CheckSkipped     CheckStatus = "skipped"
	CheckBlocked     CheckStatus = "blocked"
	CheckNotVerified CheckStatus = "not_verified"
)

// IsolationMode is the effective tool-execution isolation of a harness route.
type IsolationMode string

const (
	IsolationNative        IsolationMode = "native"
	IsolationAdapterBridge IsolationMode = "adapter_bridge"
	IsolationOuterOnly     IsolationMode = "outer_only"
)

// HarnessHealth is the discovered installation state.
type HarnessHealth string

const (
	HarnessReady        HarnessHealth = "ready"
	HarnessDegraded     HarnessHealth = "degraded"
	HarnessUnauth       HarnessHealth = "unauthenticated"
	HarnessIncompatible HarnessHealth = "incompatible"
	HarnessUnavailable  HarnessHealth = "unavailable"
)

// Compatibility class based on probed ACP abilities, not marketing labels.
type CompatibilityClass string

const (
	CompatIncompatible CompatibilityClass = "incompatible"
	CompatCore         CompatibilityClass = "core"
	CompatRoutable     CompatibilityClass = "routable"
	CompatEnhanced     CompatibilityClass = "enhanced"
)

// ProjectStatus is independent of filesystem path.
type ProjectStatus string

const (
	ProjectAvailable   ProjectStatus = "available"
	ProjectUnavailable ProjectStatus = "unavailable"
	ProjectMoved       ProjectStatus = "moved"
)

// BlockedReason is a structured run block.
type BlockedReason string

const (
	BlockedNoViableRoute BlockedReason = "NO_VIABLE_ROUTE"
	BlockedPermission    BlockedReason = "PERMISSION"
	BlockedBudget        BlockedReason = "BUDGET"
	BlockedSandbox       BlockedReason = "SANDBOX"
	BlockedHarnessAuth   BlockedReason = "HARNESS_AUTH"
	BlockedVaultLocked   BlockedReason = "VAULT_LOCKED"
	BlockedStorage       BlockedReason = "STORAGE"
	BlockedIntegration   BlockedReason = "INTEGRATION"
	BlockedUser          BlockedReason = "USER"
	BlockedPolicy        BlockedReason = "POLICY"
	BlockedRecovery      BlockedReason = "RECOVERY"
)

// FailureClass distinguishes retry policy.
type FailureClass string

const (
	FailInfrastructure FailureClass = "infrastructure"
	FailTask           FailureClass = "task"
	FailPolicy         FailureClass = "policy"
	FailUser           FailureClass = "user"
	FailData           FailureClass = "data"
)

// CompletionOutcome is decided by Go CompletionPolicy, never by model prose.
type CompletionOutcome string

const (
	OutcomeComplete  CompletionOutcome = "complete"
	OutcomeRepair    CompletionOutcome = "repair"
	OutcomeReplan    CompletionOutcome = "replan"
	OutcomeBlocked   CompletionOutcome = "blocked"
	OutcomeFailed    CompletionOutcome = "failed"
	OutcomeExplore   CompletionOutcome = "explore"
	OutcomeIntegrate CompletionOutcome = "ready_to_integrate"
)

// SessionResumeCapability is adapter-reported ACP session recovery.
type SessionResumeCapability string

const (
	ResumeNone        SessionResumeCapability = "none"
	ResumeReconstruct SessionResumeCapability = "reconstruct"
	ResumeNative      SessionResumeCapability = "native_resume"
)

// SettingScope matches SPEC configuration scopes.
type SettingScope string

const (
	ScopeServer  SettingScope = "server"
	ScopeUser    SettingScope = "user"
	ScopeProject SettingScope = "project"
	ScopeDevice  SettingScope = "device"
	ScopeRun     SettingScope = "run"
)

// ArtifactKind is a durable structured inter-stage output.
type ArtifactKind string

const (
	ArtifactPlan            ArtifactKind = "plan"
	ArtifactTaskContract    ArtifactKind = "task_contract"
	ArtifactInvestigation   ArtifactKind = "investigation"
	ArtifactImplementation  ArtifactKind = "implementation_report"
	ArtifactValidation      ArtifactKind = "validation"
	ArtifactReview          ArtifactKind = "review"
	ArtifactTestReport      ArtifactKind = "test_report"
	ArtifactResearch        ArtifactKind = "research"
	ArtifactFailure         ArtifactKind = "failure"
	ArtifactDiffSummary     ArtifactKind = "diff_summary"
	ArtifactRecovery        ArtifactKind = "recovery"
	ArtifactContextManifest ArtifactKind = "context_manifest"
)

// NetworkCapability is the network access a harness route requires.
type NetworkCapability string

const (
	// NetworkNone means the harness needs no external network.
	NetworkNone NetworkCapability = "none"
	// NetworkProvider means the harness needs model/provider network access.
	NetworkProvider NetworkCapability = "provider"
)

// NotificationKind is derived from durable domain events.
type NotificationKind string

const (
	NoteApprovalRequired    NotificationKind = "approval_required"
	NoteManualValidation    NotificationKind = "manual_validation_required"
	NoteRunComplete         NotificationKind = "run_complete"
	NoteRunBlocked          NotificationKind = "run_blocked"
	NoteRunFailed           NotificationKind = "run_failed"
	NoteIntegrationConflict NotificationKind = "integration_conflict"
	NoteServerAttention     NotificationKind = "server_attention"
	NoteHarnessAttention    NotificationKind = "harness_attention"
)

// RoutingProfile is a user-facing route preference.
type RoutingProfile string

const (
	ProfileAuto     RoutingProfile = "auto"
	ProfileBalanced RoutingProfile = "balanced"
	ProfileQuality  RoutingProfile = "quality"
	ProfileEconomy  RoutingProfile = "economy"
	ProfileSpeed    RoutingProfile = "speed"
)
