package domain

import "time"

// Project is a server-owned identity independent of filesystem path.
type Project struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Path          string        `json:"path"`
	SourceKind    string        `json:"sourceKind"`
	RepoIdentity  string        `json:"repoIdentity"`
	GitRemote     string        `json:"gitRemote"`
	DefaultBranch string        `json:"defaultBranch"`
	Status        ProjectStatus `json:"status"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
	LastOpenedAt  time.Time     `json:"lastOpenedAt"`
	KnowledgeRev  string        `json:"knowledgeRev"`
}

type Conversation struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Pinned    bool      `json:"pinned"`
}

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
)

type Message struct {
	ID             string      `json:"id"`
	ConversationID string      `json:"conversationId"`
	Role           MessageRole `json:"role"`
	Body           string      `json:"body"`
	TaskID         string      `json:"taskId"`
	CreatedAt      time.Time   `json:"createdAt"`
}

type Task struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"projectId"`
	ConversationID string    `json:"conversationId"`
	MessageID      string    `json:"messageId"`
	Objective      string    `json:"objective"`
	ArtifactOnly   bool      `json:"artifactOnly"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Run struct {
	ID              string         `json:"id"`
	TaskID          string         `json:"taskId"`
	ProjectID       string         `json:"projectId"`
	ConversationID  string         `json:"conversationId"`
	Status          RunStatus      `json:"status"`
	BlockedReason   BlockedReason  `json:"blockedReason,omitempty"`
	BlockedDetail   string         `json:"blockedDetail,omitempty"`
	Profile         RoutingProfile `json:"profile"`
	DegradedRouting bool           `json:"degradedRouting"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	StartedAt       *time.Time     `json:"startedAt,omitempty"`
	FinishedAt      *time.Time     `json:"finishedAt,omitempty"`
	IdempotencyKey  string         `json:"idempotencyKey,omitempty"`
}

type Stage struct {
	ID        string             `json:"id"`
	RunID     string             `json:"runId"`
	Kind      StageKind          `json:"kind"`
	Ordinal   int                `json:"ordinal"`
	Status    StageAttemptStatus `json:"status"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

type StageAttempt struct {
	ID           string             `json:"id"`
	StageID      string             `json:"stageId"`
	RunID        string             `json:"runId"`
	Ordinal      int                `json:"ordinal"`
	Status       StageAttemptStatus `json:"status"`
	HarnessID    string             `json:"harnessId"`
	ModelID      string             `json:"modelId"`
	FailureClass FailureClass       `json:"failureClass,omitempty"`
	Error        string             `json:"error,omitempty"`
	StartedAt    *time.Time         `json:"startedAt,omitempty"`
	FinishedAt   *time.Time         `json:"finishedAt,omitempty"`
	CheckpointID string             `json:"checkpointId,omitempty"`
	CreatedAt    time.Time          `json:"createdAt"`
}

type TaskAssessment struct {
	ID             string    `json:"id"`
	RunID          string    `json:"runId"`
	JevModel       string    `json:"jevModel"`
	QuestionSet    string    `json:"questionSet"`
	PolicyVersion  string    `json:"policyVersion"`
	DimensionsJSON string    `json:"dimensions"`
	InputHash      string    `json:"inputHash"`
	UsageJSON      string    `json:"usage"`
	Degraded       bool      `json:"degraded"`
	CreatedAt      time.Time `json:"createdAt"`
}

type RouteDecision struct {
	ID            string         `json:"id"`
	RunID         string         `json:"runId"`
	StageID       string         `json:"stageId"`
	AssessmentID  string         `json:"assessmentId,omitempty"`
	HarnessID     string         `json:"harnessId"`
	ModelID       string         `json:"modelId"`
	Effort        string         `json:"effort,omitempty"`
	Profile       RoutingProfile `json:"profile"`
	Isolation     IsolationMode  `json:"isolation"`
	FallbacksJSON string         `json:"fallbacks"`
	PolicyVersion string         `json:"policyVersion"`
	Reason        string         `json:"reason"`
	Degraded      bool           `json:"degraded"`
	CreatedAt     time.Time      `json:"createdAt"`
}

type Artifact struct {
	ID         string       `json:"id"`
	RunID      string       `json:"runId"`
	StageID    string       `json:"stageId"`
	AttemptID  string       `json:"attemptId"`
	Kind       ArtifactKind `json:"kind"`
	SchemaVer  int          `json:"schemaVersion"`
	ObjectHash string       `json:"objectHash,omitempty"`
	JSON       string       `json:"json"`
	Valid      bool         `json:"valid"`
	CreatedAt  time.Time    `json:"createdAt"`
}

type Device struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Verifier   []byte     `json:"-"`
	Salt       []byte     `json:"-"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastSeenAt time.Time  `json:"lastSeenAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	PairingID  string     `json:"pairingId,omitempty"`
}

type PairingInvitation struct {
	ID            string     `json:"id"`
	CodeHash      []byte     `json:"-"`
	AdvertisedURL string     `json:"advertisedUrl"`
	ListenURL     string     `json:"listenUrl"`
	ServerFinger  string     `json:"fingerprint"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	UsedAt        *time.Time `json:"usedAt,omitempty"`
	CreatedBy     string     `json:"createdBy"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type AuthSession struct {
	ID        string    `json:"id"`
	DeviceID  string    `json:"deviceId"`
	TokenHash []byte    `json:"-"`
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

type Approval struct {
	ID         string     `json:"id"`
	RunID      string     `json:"runId"`
	StageID    string     `json:"stageId"`
	Kind       string     `json:"kind"`
	Resource   string     `json:"resource"`
	Reason     string     `json:"reason"`
	ScopesJSON string     `json:"scopes"`
	Status     string     `json:"status"`
	ResolvedBy string     `json:"resolvedBy,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
}

type Notification struct {
	ID        string           `json:"id"`
	Kind      NotificationKind `json:"kind"`
	ProjectID string           `json:"projectId"`
	RunID     string           `json:"runId"`
	Title     string           `json:"title"`
	Body      string           `json:"body"`
	Attention bool             `json:"attention"`
	ReadAt    *time.Time       `json:"readAt,omitempty"`
	CreatedAt time.Time        `json:"createdAt"`
}

type Event struct {
	Seq       int64     `json:"seq"`
	Type      string    `json:"type"`
	ProjectID string    `json:"projectId"`
	RunID     string    `json:"runId"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"createdAt"`
}

type Setting struct {
	Scope     SettingScope `json:"scope"`
	ScopeID   string       `json:"scopeId"`
	Key       string       `json:"key"`
	ValueJSON string       `json:"value"`
	Revision  int64        `json:"revision"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

type HarnessInstallation struct {
	ID               string             `json:"id"`
	DefinitionID     string             `json:"definitionId"`
	DisplayName      string             `json:"displayName"`
	Executable       string             `json:"executable"`
	Version          string             `json:"version"`
	Adapter          string             `json:"adapter"`
	Health           HarnessHealth      `json:"health"`
	Compatibility    CompatibilityClass `json:"compatibility"`
	Isolation        IsolationMode      `json:"isolation"`
	AuthStatus       string             `json:"authStatus"`
	CapabilitiesJSON string             `json:"capabilities"`
	ModelsJSON       string             `json:"models"`
	LastProbedAt     time.Time          `json:"lastProbedAt"`
	Notes            string             `json:"notes"`

	// Catalog discovery diagnostics.
	DefinitionSource        string            `json:"definitionSource,omitempty"`
	DefinitionFingerprint   string            `json:"definitionFingerprint,omitempty"`
	BridgeExecutable        string            `json:"bridgeExecutable,omitempty"`
	BridgePresent           bool              `json:"bridgePresent"`
	ACPStatus               string            `json:"acpStatus,omitempty"`
	BlockingReason          string            `json:"blockingReason,omitempty"`
	ProviderTransport       ProviderTransport `json:"providerTransport,omitempty"`
	ModelSelection          string            `json:"modelSelection,omitempty"`
	RequiresProviderNetwork bool              `json:"requiresProviderNetwork"`
}

type WorkspaceRecord struct {
	ID         string    `json:"id"`
	RunID      string    `json:"runId"`
	ProjectID  string    `json:"projectId"`
	Kind       string    `json:"kind"`
	SourcePath string    `json:"sourcePath"`
	RunPath    string    `json:"runPath"`
	SnapshotID string    `json:"snapshotId"`
	Branch     string    `json:"branch"`
	HEAD       string    `json:"head"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Snapshot struct {
	ID           string    `json:"id"`
	WorkspaceID  string    `json:"workspaceId"`
	Branch       string    `json:"branch"`
	HEAD         string    `json:"head"`
	DirtyJSON    string    `json:"dirty"`
	KnowledgeRev string    `json:"knowledgeRev"`
	ObjectHash   string    `json:"objectHash"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Integration struct {
	ID            string    `json:"id"`
	RunID         string    `json:"runId"`
	ProjectID     string    `json:"projectId"`
	Status        string    `json:"status"`
	BaseSnapshot  string    `json:"baseSnapshot"`
	TargetBranch  string    `json:"targetBranch"`
	CurrentBranch string    `json:"currentBranch"`
	JournalHash   string    `json:"journalHash"`
	Error         string    `json:"error,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Usage struct {
	ID           string    `json:"id"`
	RunID        string    `json:"runId"`
	StageID      string    `json:"stageId"`
	AttemptID    string    `json:"attemptId"`
	HarnessID    string    `json:"harnessId"`
	ModelID      string    `json:"modelId"`
	Provider     string    `json:"provider"`
	InputTokens  int64     `json:"inputTokens"`
	OutputTokens int64     `json:"outputTokens"`
	CacheTokens  int64     `json:"cacheTokens"`
	CostMicros   int64     `json:"costMicros"`
	Estimated    bool      `json:"estimated"`
	PricingJSON  string    `json:"pricing"`
	CreatedAt    time.Time `json:"createdAt"`
}

type ServerIdentity struct {
	ServerID    string    `json:"serverId"`
	PublicKey   []byte    `json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
	DisplayName string    `json:"displayName"`
}
