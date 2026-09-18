package acp

import (
	"encoding/json"
	"regexp"
	"unicode/utf8"
)

// EventKind is a Wayshard-normalized ACP session event.
type EventKind string

const (
	EventMessageDelta EventKind = "message_delta"
	EventThoughtDelta EventKind = "thought_delta"
	EventToolCall     EventKind = "tool_call"
	EventToolUpdate   EventKind = "tool_update"
	EventPermission   EventKind = "permission"
	EventPlan         EventKind = "plan"
	EventDone         EventKind = "done"
	EventError        EventKind = "error"
)

const (
	maxEventText       = 64 * 1024
	maxDiagnosticBytes = 2048
	maxDiagnostics     = 64
)

// Event is a protocol-driver event after ACP session/update normalization.
type Event struct {
	Kind       EventKind
	SessionID  string
	Text       string
	ToolCallID string
	ToolTitle  string
	ToolKind   string
	ToolStatus string
	Plan       []PlanEntry
	StopReason string
	Error      string
	RawKind    string
	Diagnostic *Diagnostic
}

// Diagnostic is a bounded, redacted protocol/stderr record. It is kept
// separate from workflow events so raw ACP chatter does not become state.
type Diagnostic struct {
	Source    string // stderr | protocol | stdout_pollution | extension
	Text      string
	Truncated bool
}

var (
	secretKV = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|passwd|authorization|bearer|credential)\s*[:=]\s*\S+`)
	bearer   = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9\-._~+/]+=*`)
	skKey    = regexp.MustCompile(`\bsk-[A-Za-z0-9]{8,}\b`)
)

// Redact secrets-looking substrings and bound length.
func Redact(s string) string {
	s = bearer.ReplaceAllString(s, "bearer [redacted]")
	s = skKey.ReplaceAllString(s, "sk-[redacted]")
	s = secretKV.ReplaceAllString(s, "${1}=[redacted]")
	return Bound(s, maxDiagnosticBytes)
}

// Bound truncates s to max bytes on a rune boundary.
func Bound(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max] + "…truncated"
}

// NormalizeUpdate maps an ACP session/update notification into Wayshard events.
// Unknown extension update kinds are tolerated as diagnostics, not errors.
func NormalizeUpdate(n SessionNotification) []Event {
	u := n.Update
	base := Event{SessionID: n.SessionID, RawKind: u.SessionUpdate}
	switch u.SessionUpdate {
	case UpdateAgentMessageChunk, UpdateUserMessageChunk:
		base.Kind = EventMessageDelta
		base.Text = Bound(contentText(u.Content), maxEventText)
		return []Event{base}
	case UpdateAgentThoughtChunk:
		base.Kind = EventThoughtDelta
		base.Text = Bound(contentText(u.Content), maxEventText)
		return []Event{base}
	case UpdateToolCall:
		base.Kind = EventToolCall
		base.ToolCallID = u.ToolCallID
		base.ToolTitle = u.Title
		base.ToolKind = u.Kind
		base.ToolStatus = u.Status
		if base.ToolStatus == "" {
			base.ToolStatus = "pending"
		}
		base.Text = Bound(contentText(u.Content), maxEventText)
		return []Event{base}
	case UpdateToolCallUpdate:
		base.Kind = EventToolUpdate
		base.ToolCallID = u.ToolCallID
		base.ToolTitle = u.Title
		base.ToolKind = u.Kind
		base.ToolStatus = u.Status
		base.Text = Bound(contentText(u.Content), maxEventText)
		return []Event{base}
	case UpdatePlan:
		base.Kind = EventPlan
		base.Plan = u.Entries
		return []Event{base}
	default:
		raw, _ := json.Marshal(u)
		return []Event{{
			Kind:      EventError, // not a workflow error; see Diagnostic
			SessionID: n.SessionID,
			RawKind:   u.SessionUpdate,
			Diagnostic: &Diagnostic{
				Source: "extension",
				Text:   Redact("unknown session/update " + u.SessionUpdate + " " + string(raw)),
			},
		}}
	}
}

// NormalizePermission maps session/request_permission into a permission event.
func NormalizePermission(p RequestPermissionParams) Event {
	return Event{
		Kind:       EventPermission,
		SessionID:  p.SessionID,
		ToolCallID: p.ToolCall.ToolCallID,
		ToolTitle:  p.ToolCall.Title,
		ToolKind:   p.ToolCall.Kind,
		ToolStatus: p.ToolCall.Status,
		Text:       p.ToolCall.Title,
	}
}

// NormalizeDone maps a session/prompt result.
func NormalizeDone(sessionID string, resp PromptResponse) Event {
	return Event{Kind: EventDone, SessionID: sessionID, StopReason: resp.StopReason}
}

// NormalizeError maps a protocol/runtime failure.
func NormalizeError(sessionID, msg string) Event {
	return Event{Kind: EventError, SessionID: sessionID, Error: msg, Diagnostic: &Diagnostic{Source: "protocol", Text: Redact(msg)}}
}

func contentText(c *ContentBlock) string {
	if c == nil {
		return ""
	}
	if c.Text != "" {
		return c.Text
	}
	return c.Type
}
