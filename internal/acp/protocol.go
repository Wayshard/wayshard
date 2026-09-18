// Package acp implements the Agent Client Protocol v1 driver.
//
// Protocol mechanics live here. Harness-specific launch, isolation, and
// resume policy belong in internal/harness adapters. Capability negotiation
// is authoritative; optional ACP features are never inferred from a name.
package acp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	JSONRPCVersion    = "2.0"
	ProtocolVersionV1 = 1
)

// JSON-RPC method names. Client→agent unless noted.
const (
	MethodInitialize               = "initialize"
	MethodAuthenticate             = "authenticate"
	MethodLogout                   = "logout"
	MethodSessionNew               = "session/new"
	MethodSessionLoad              = "session/load"
	MethodSessionPrompt            = "session/prompt"
	MethodSessionCancel            = "session/cancel"             // notification
	MethodSessionRequestPermission = "session/request_permission" // agent→client
	MethodSessionUpdate            = "session/update"             // agent→client notification
	MethodFSReadTextFile           = "fs/read_text_file"          // agent→client
	MethodFSWriteTextFile          = "fs/write_text_file"         // agent→client
	MethodTerminalCreate           = "terminal/create"            // agent→client
	MethodTerminalOutput           = "terminal/output"
	MethodTerminalRelease          = "terminal/release"
	MethodTerminalWaitForExit      = "terminal/wait_for_exit"
	MethodTerminalKill             = "terminal/kill"
)

// JSON-RPC / ACP error codes.
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
	ErrCodeAuthRequired   = -32000
	ErrCodeNotFound       = -32002
	ErrCodeTooManyPending = -32003
)

// Stop reasons from session/prompt.
const (
	StopEndTurn         = "end_turn"
	StopCancelled       = "cancelled"
	StopMaxTokens       = "max_tokens"
	StopMaxTurnRequests = "max_turn_requests"
	StopRefusal         = "refusal"
)

const (
	UpdateUserMessageChunk        = "user_message_chunk"
	UpdateAgentMessageChunk       = "agent_message_chunk"
	UpdateAgentThoughtChunk       = "agent_thought_chunk"
	UpdateToolCall                = "tool_call"
	UpdateToolCallUpdate          = "tool_call_update"
	UpdatePlan                    = "plan"
	UpdateAvailableCommandsUpdate = "available_commands_update"
	UpdateCurrentModeUpdate       = "current_mode_update"
	UpdateConfigOptionUpdate      = "config_option_update"
	UpdateSessionInfoUpdate       = "session_info_update"
	UpdateUsageUpdate             = "usage_update"
)

const (
	PermissionAllowOnce    = "allow_once"
	PermissionAllowAlways  = "allow_always"
	PermissionRejectOnce   = "reject_once"
	PermissionRejectAlways = "reject_always"
	OutcomeSelected        = "selected"
	OutcomeCancelled       = "cancelled"
)

// ID is a JSON-RPC 2.0 identifier (string, number, or empty/null).
type ID struct {
	raw json.RawMessage
}

func NewNumberID(n int64) ID {
	b, _ := json.Marshal(n)
	return ID{raw: b}
}

func NewStringID(s string) ID {
	b, _ := json.Marshal(s)
	return ID{raw: b}
}

func (id ID) MarshalJSON() ([]byte, error) {
	if len(id.raw) == 0 {
		return []byte("null"), nil
	}
	return id.raw, nil
}

func (id *ID) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		id.raw = nil
		return nil
	}
	id.raw = append(id.raw[:0], b...)
	return nil
}

func (id ID) String() string {
	if len(id.raw) == 0 {
		return "null"
	}
	return string(id.raw)
}

func (id ID) IsZero() bool { return len(id.raw) == 0 }

func (id ID) Equal(other ID) bool { return string(id.raw) == string(other.raw) }

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      ID              `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      ID              `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Notification is a JSON-RPC 2.0 notification (no id).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Error is a JSON-RPC 2.0 error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return "acp: <nil error>"
	}
	return fmt.Sprintf("acp rpc %d: %s", e.Code, e.Message)
}

func (e *Error) AuthRequired() bool {
	return e != nil && e.Code == ErrCodeAuthRequired
}

func NewError(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Wire is a union used when decoding an NDJSON frame.
type Wire struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *ID             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

func (w Wire) IsRequest() bool      { return w.Method != "" && w.ID != nil }
func (w Wire) IsNotification() bool { return w.Method != "" && w.ID == nil }
func (w Wire) IsResponse() bool     { return w.Method == "" && w.ID != nil }

func (w Wire) Request() Request {
	id := ID{}
	if w.ID != nil {
		id = *w.ID
	}
	return Request{JSONRPC: w.JSONRPC, ID: id, Method: w.Method, Params: w.Params}
}

func (w Wire) Response() Response {
	id := ID{}
	if w.ID != nil {
		id = *w.ID
	}
	return Response{JSONRPC: w.JSONRPC, ID: id, Result: w.Result, Error: w.Error}
}

func (w Wire) Notification() Notification {
	return Notification{JSONRPC: w.JSONRPC, Method: w.Method, Params: w.Params}
}

// ProtocolVersion is ACP's negotiated version. Wire form is an integer; a
// dotted string is accepted for robustness.
type ProtocolVersion int

func (v ProtocolVersion) MarshalJSON() ([]byte, error) {
	return json.Marshal(int(v))
}

func (v *ProtocolVersion) UnmarshalJSON(b []byte) error {
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		*v = ProtocolVersion(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("protocolVersion: %w", err)
	}
	s = strings.TrimSpace(s)
	if i, err := strconv.Atoi(s); err == nil {
		*v = ProtocolVersion(i)
		return nil
	}
	if i, err := strconv.Atoi(strings.Split(s, ".")[0]); err == nil {
		*v = ProtocolVersion(i)
		return nil
	}
	return fmt.Errorf("protocolVersion %q", s)
}

type Implementation struct {
	Name    string `json:"name,omitempty"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

type FileSystemCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

type ClientCapabilities struct {
	FS       FileSystemCapabilities `json:"fs"`
	Terminal bool                   `json:"terminal"`
	Auth     json.RawMessage        `json:"auth,omitempty"`
}

type PromptCapabilities struct {
	Image           bool `json:"image"`
	Audio           bool `json:"audio"`
	EmbeddedContext bool `json:"embeddedContext"`
}

type MCPCapabilities struct {
	HTTP bool `json:"http"`
	SSE  bool `json:"sse"`
}

type AgentCapabilities struct {
	LoadSession         bool               `json:"loadSession"`
	PromptCapabilities  PromptCapabilities `json:"promptCapabilities"`
	MCPCapabilities     MCPCapabilities    `json:"mcpCapabilities"`
	SessionCapabilities json.RawMessage    `json:"sessionCapabilities,omitempty"`
	Auth                json.RawMessage    `json:"auth,omitempty"`
}

func (c AgentCapabilities) HasNativeResume() bool {
	if c.LoadSession {
		return true
	}
	if len(c.SessionCapabilities) == 0 {
		return false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(c.SessionCapabilities, &m); err != nil {
		return false
	}
	if _, ok := m["resume"]; ok {
		return true
	}
	if _, ok := m["load"]; ok {
		return true
	}
	return false
}

func (c AgentCapabilities) NativeSandboxAdvertised() bool {
	if len(c.Auth) == 0 && len(c.SessionCapabilities) == 0 {
		return false
	}
	blob := string(c.SessionCapabilities) + string(c.Auth)
	return strings.Contains(strings.ToLower(blob), "sandbox")
}

type AuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

type InitializeRequest struct {
	ProtocolVersion    ProtocolVersion    `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         Implementation     `json:"clientInfo,omitempty"`
	Meta               json.RawMessage    `json:"_meta,omitempty"`
}

type InitializeResponse struct {
	ProtocolVersion   ProtocolVersion   `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
	AgentInfo         Implementation    `json:"agentInfo,omitempty"`
	AuthMethods       []AuthMethod      `json:"authMethods"`
	Meta              json.RawMessage   `json:"_meta,omitempty"`
}

type AuthenticateRequest struct {
	MethodID string          `json:"methodId"`
	Meta     json.RawMessage `json:"_meta,omitempty"`
}

type AuthenticateResponse struct {
	Meta json.RawMessage `json:"_meta,omitempty"`
}

type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     []EnvVariable     `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type EnvVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type NewSessionRequest struct {
	CWD                   string          `json:"cwd"`
	MCPServers            []MCPServer     `json:"mcpServers"`
	AdditionalDirectories []string        `json:"additionalDirectories,omitempty"`
	Meta                  json.RawMessage `json:"_meta,omitempty"`
}

type NewSessionResponse struct {
	SessionID     string          `json:"sessionId"`
	ConfigOptions json.RawMessage `json:"configOptions,omitempty"`
	Modes         json.RawMessage `json:"modes,omitempty"`
	Meta          json.RawMessage `json:"_meta,omitempty"`
}

type LoadSessionRequest struct {
	SessionID             string          `json:"sessionId"`
	CWD                   string          `json:"cwd"`
	MCPServers            []MCPServer     `json:"mcpServers"`
	AdditionalDirectories []string        `json:"additionalDirectories,omitempty"`
	Meta                  json.RawMessage `json:"_meta,omitempty"`
}

type LoadSessionResponse struct {
	ConfigOptions json.RawMessage `json:"configOptions,omitempty"`
	Modes         json.RawMessage `json:"modes,omitempty"`
	Meta          json.RawMessage `json:"_meta,omitempty"`
}

type ContentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Data     string          `json:"data,omitempty"`
	MimeType string          `json:"mimeType,omitempty"`
	URI      string          `json:"uri,omitempty"`
	Name     string          `json:"name,omitempty"`
	Meta     json.RawMessage `json:"_meta,omitempty"`
}

func TextBlock(s string) ContentBlock {
	return ContentBlock{Type: "text", Text: s}
}

type PromptRequest struct {
	SessionID string          `json:"sessionId"`
	Prompt    []ContentBlock  `json:"prompt"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type PromptResponse struct {
	StopReason string          `json:"stopReason"`
	Meta       json.RawMessage `json:"_meta,omitempty"`
}

type CancelNotification struct {
	SessionID string          `json:"sessionId"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type ToolCallUpdate struct {
	ToolCallID string          `json:"toolCallId,omitempty"`
	Title      string          `json:"title,omitempty"`
	Kind       string          `json:"kind,omitempty"`
	Status     string          `json:"status,omitempty"`
	Content    json.RawMessage `json:"content,omitempty"`
	Locations  json.RawMessage `json:"locations,omitempty"`
	RawInput   json.RawMessage `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage `json:"rawOutput,omitempty"`
}

type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

type RequestPermissionParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCallUpdate     `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
	Meta      json.RawMessage    `json:"_meta,omitempty"`
}

type PermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

type RequestPermissionResult struct {
	Outcome PermissionOutcome `json:"outcome"`
	Meta    json.RawMessage   `json:"_meta,omitempty"`
}

func SelectedPermission(optionID string) RequestPermissionResult {
	return RequestPermissionResult{Outcome: PermissionOutcome{Outcome: OutcomeSelected, OptionID: optionID}}
}

func CancelledPermission() RequestPermissionResult {
	return RequestPermissionResult{Outcome: PermissionOutcome{Outcome: OutcomeCancelled}}
}

type ReadTextFileParams struct {
	SessionID string          `json:"sessionId"`
	Path      string          `json:"path"`
	Line      *int            `json:"line,omitempty"`
	Limit     *int            `json:"limit,omitempty"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type ReadTextFileResult struct {
	Content string          `json:"content"`
	Meta    json.RawMessage `json:"_meta,omitempty"`
}

type WriteTextFileParams struct {
	SessionID string          `json:"sessionId"`
	Path      string          `json:"path"`
	Content   string          `json:"content"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type WriteTextFileResult struct {
	Meta json.RawMessage `json:"_meta,omitempty"`
}

type CreateTerminalParams struct {
	SessionID       string          `json:"sessionId"`
	Command         string          `json:"command"`
	Args            []string        `json:"args,omitempty"`
	CWD             string          `json:"cwd,omitempty"`
	Env             []EnvVariable   `json:"env,omitempty"`
	OutputByteLimit *int            `json:"outputByteLimit,omitempty"`
	Meta            json.RawMessage `json:"_meta,omitempty"`
}

type CreateTerminalResult struct {
	TerminalID string          `json:"terminalId"`
	Meta       json.RawMessage `json:"_meta,omitempty"`
}

type TerminalIDParams struct {
	SessionID  string          `json:"sessionId"`
	TerminalID string          `json:"terminalId"`
	Meta       json.RawMessage `json:"_meta,omitempty"`
}

type TerminalOutputResult struct {
	Output     string          `json:"output"`
	Truncated  bool            `json:"truncated"`
	ExitStatus json.RawMessage `json:"exitStatus,omitempty"`
	Meta       json.RawMessage `json:"_meta,omitempty"`
}

type WaitForTerminalExitResult struct {
	ExitCode *int            `json:"exitCode"`
	Signal   *string         `json:"signal,omitempty"`
	Meta     json.RawMessage `json:"_meta,omitempty"`
}

type PlanEntry struct {
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority uint   `json:"priority,omitempty"`
}

type SessionUpdate struct {
	SessionUpdate string          `json:"sessionUpdate"`
	Content       *ContentBlock   `json:"content,omitempty"`
	MessageID     string          `json:"messageId,omitempty"`
	ToolCallID    string          `json:"toolCallId,omitempty"`
	Title         string          `json:"title,omitempty"`
	Kind          string          `json:"kind,omitempty"`
	Status        string          `json:"status,omitempty"`
	RawInput      json.RawMessage `json:"rawInput,omitempty"`
	RawOutput     json.RawMessage `json:"rawOutput,omitempty"`
	Locations     json.RawMessage `json:"locations,omitempty"`
	Entries       []PlanEntry     `json:"entries,omitempty"`
	Commands      json.RawMessage `json:"availableCommands,omitempty"`
	CurrentModeID string          `json:"currentModeId,omitempty"`
	ConfigOptions json.RawMessage `json:"configOptions,omitempty"`
	Usage         json.RawMessage `json:"usage,omitempty"`
}

type SessionNotification struct {
	SessionID string          `json:"sessionId"`
	Update    SessionUpdate   `json:"update"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

func PromptText(blocks []ContentBlock) string {
	var b strings.Builder
	for _, c := range blocks {
		if c.Type == "text" || c.Type == "" {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}
