// Command wayshard-fake-acp is a deterministic ACP v1 agent for tests.
//
// It speaks JSON-RPC 2.0 NDJSON on stdin/stdout. stderr is diagnostic.
// Scenario is selected with WAYSHARD_FAKE_SCENARIO; stage artifact shape with
// WAYSHARD_FAKE_STAGE. It never calls paid APIs.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const jsonrpc = "2.0"

func main() {
	recordProcessLaunch()
	// Deterministic timeout fixture: never answer the version probe so a caller's
	// probe timeout is exercised. The caller bounds (and ultimately kills) it.
	if os.Getenv("WAYSHARD_FAKE_HANG_VERSION") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	for _, a := range os.Args[1:] {
		if a == "--version" || a == "-version" || a == "-v" {
			fmt.Println("wayshard-fake-acp 0.0.0-dev")
			return
		}
	}
	scenario := getenv("WAYSHARD_FAKE_SCENARIO", "success")
	stage := getenv("WAYSHARD_FAKE_STAGE", "plan")
	a := &agent{
		scenario: scenario,
		stage:    stage,
		pending:  make(map[string]chan response),
		cancel:   make(map[string]chan struct{}),
		sessions: make(map[string]struct{}),
	}
	if err := a.run(os.Stdin, os.Stdout); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "wayshard-fake-acp: %v\n", err)
		os.Exit(1)
	}
}

type idRaw struct{ raw json.RawMessage }

func (id idRaw) MarshalJSON() ([]byte, error) {
	if len(id.raw) == 0 {
		return []byte("null"), nil
	}
	return id.raw, nil
}

func (id *idRaw) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		id.raw = nil
		return nil
	}
	id.raw = append(id.raw[:0], b...)
	return nil
}

func (id idRaw) key() string { return string(id.raw) }

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *idRaw          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      idRaw           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type agent struct {
	scenario string
	stage    string

	mu       sync.Mutex
	w        io.Writer
	nextID   atomic.Int64
	pending  map[string]chan response
	cancel   map[string]chan struct{}
	sessions map[string]struct{}
	authed   bool
	initDone bool
}

func (a *agent) run(in io.Reader, out io.Writer) error {
	a.w = out
	br := bufio.NewReader(in)
	for {
		line, err := br.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			a.handleLine(line)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (a *agent) handleLine(line []byte) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		fmt.Fprintf(os.Stderr, "fake-acp: bad json: %v\n", err)
		return
	}
	if req.Method == "" && req.ID != nil {
		a.mu.Lock()
		ch, ok := a.pending[req.ID.key()]
		if ok {
			delete(a.pending, req.ID.key())
		}
		a.mu.Unlock()
		if ok {
			ch <- response{JSONRPC: jsonrpc, ID: *req.ID, Result: req.Result, Error: req.Error}
		}
		return
	}
	if req.Method == "session/cancel" {
		var p struct {
			SessionID string `json:"sessionId"`
		}
		_ = json.Unmarshal(req.Params, &p)
		a.mu.Lock()
		ch := a.cancel[p.SessionID]
		a.mu.Unlock()
		if ch != nil {
			select {
			case <-ch:
			default:
				close(ch)
			}
		}
		return
	}
	if req.ID == nil {
		return
	}
	go a.handleRequest(req)
}

func (a *agent) handleRequest(req request) {
	switch req.Method {
	case "initialize":
		a.onInitialize(req)
	case "authenticate":
		a.authed = true
		a.reply(req, map[string]any{})
	case "session/new":
		a.onNewSession(req)
	case "session/load":
		a.replyErr(req, -32601, "session/load not advertised")
	case "session/prompt":
		a.onPrompt(req)
	default:
		if strings.HasPrefix(req.Method, "_") {
			a.replyErr(req, -32601, "method not found: "+req.Method)
			return
		}
		a.replyErr(req, -32601, "method not found: "+req.Method)
	}
}

func (a *agent) onInitialize(req request) {
	if a.scenario == "malformed" {
		a.writeRaw([]byte("this is not json-rpc\n"))
		return
	}
	// Deterministic timeout fixture: never answer initialize so a caller's
	// handshake/probe timeout is exercised. The caller bounds (and kills) it.
	if os.Getenv("WAYSHARD_FAKE_HANG_INITIALIZE") == "1" {
		time.Sleep(24 * time.Hour)
		return
	}
	auth := []any{}
	if a.scenario == "auth_required" {
		auth = []any{map[string]any{"id": "fake", "name": "Fake auth"}}
	}
	a.initDone = true
	a.reply(req, map[string]any{
		"protocolVersion": 1,
		"agentCapabilities": map[string]any{
			"loadSession":        false,
			"promptCapabilities": map[string]any{"image": false, "audio": false, "embeddedContext": false},
			"mcpCapabilities":    map[string]any{"http": false, "sse": false},
		},
		"agentInfo": map[string]any{
			"name":    "wayshard-fake-acp",
			"title":   "Wayshard Fake ACP",
			"version": "0.0.0-dev",
		},
		"authMethods": auth,
	})
}

func (a *agent) onNewSession(req request) {
	if a.scenario == "auth_required" && !a.authed {
		a.replyErr(req, -32000, "auth_required")
		return
	}
	sid := "sess_fake_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	a.mu.Lock()
	a.sessions[sid] = struct{}{}
	a.cancel[sid] = make(chan struct{})
	a.mu.Unlock()
	a.reply(req, map[string]any{"sessionId": sid, "configOptions": []any{}})
}

func (a *agent) onPrompt(req request) {
	var p struct {
		SessionID string `json:"sessionId"`
		Prompt    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"prompt"`
	}
	_ = json.Unmarshal(req.Params, &p)
	if p.SessionID == "" {
		a.replyErr(req, -32602, "sessionId required")
		return
	}

	switch a.scenario {
	case "crash":
		fmt.Fprintln(os.Stderr, "fake-acp: crashing as requested")
		os.Exit(2)
	case "timeout":
		time.Sleep(24 * time.Hour)
		return
	case "missing_model":
		a.replyErr(req, -32603, "model not available")
		return
	case "provider_fail":
		a.notifyUpdate(p.SessionID, "agent_message_chunk", "provider error")
		a.replyErr(req, -32603, "provider failure")
		return
	}

	a.mu.Lock()
	cancelCh := a.cancel[p.SessionID]
	a.mu.Unlock()

	if a.maybeHangAfterWrite(req, p.SessionID, cancelCh) {
		return
	}

	if a.scenario == "permission" && a.effectiveStage() == getenv("WAYSHARD_FAKE_PERMISSION_STAGE", "execute") {
		outcome, kind, err := a.requestPermission(p.SessionID)
		if err != nil {
			a.replyErr(req, -32603, err.Error())
			return
		}
		// A real ACP harness inspects the selected option's kind. A denial is
		// reported as a selected reject option, not as an approval.
		if outcome != "selected" || !strings.Contains(kind, "allow") {
			a.notifyUpdate(p.SessionID, "agent_message_chunk", "permission denied; protected action not executed")
			// Return a coherent stage artifact without performing the protected
			// operation.
			a.streamFinal(p.SessionID, artifactJSON(a.effectiveStage(), true))
			a.reply(req, map[string]any{"stopReason": "end_turn"})
			return
		}
		// Approved: perform the protected operation exactly once.
		if canary := os.Getenv("WAYSHARD_FAKE_PERMISSION_CANARY"); canary != "" {
			_ = os.WriteFile(canary, []byte("executed\n"), 0o644)
		}
	}

	if a.scenario == "ignore_cancel" {
		select {
		case <-cancelCh:
			time.Sleep(50 * time.Millisecond)
		case <-time.After(2 * time.Second):
		}
		body := artifactJSON(a.effectiveStage(), true)
		a.streamFinal(p.SessionID, body)
		a.reply(req, map[string]any{"stopReason": "end_turn"})
		return
	}

	select {
	case <-cancelCh:
		a.reply(req, map[string]any{"stopReason": "cancelled"})
		return
	default:
	}

	if a.scenario == "invalid_output" {
		a.streamFinal(p.SessionID, "I am done. (not a structured artifact)")
		a.reply(req, map[string]any{"stopReason": "end_turn"})
		return
	}

	ok := a.scenario != "review_reject"
	body := artifactJSON(a.effectiveStage(), ok)
	a.notifyUpdate(p.SessionID, "agent_thought_chunk", "considering the task")
	if a.effectiveStage() == "execute" || a.effectiveStage() == "repair" {
		a.notifyTool(p.SessionID, "call_1", "apply patch", "edit", "completed")
		if rel := os.Getenv("WAYSHARD_FAKE_WRITE_FILE"); rel != "" {
			cwd, _ := os.Getwd()
			_ = os.WriteFile(filepath.Join(cwd, rel), []byte("package agent\n// wayshard fake ACP change\n"), 0o644)
		}
	}
	if path := os.Getenv("WAYSHARD_FAKE_READ_FILE"); path != "" {
		_, _ = a.callClient("fs/read_text_file", map[string]any{
			"sessionId": p.SessionID,
			"path":      path,
		})
	}
	select {
	case <-cancelCh:
		a.reply(req, map[string]any{"stopReason": "cancelled"})
		return
	default:
	}
	a.streamFinal(p.SessionID, body)
	a.reply(req, map[string]any{"stopReason": "end_turn"})
}

// maybeHangAfterWrite implements the deterministic crash fixture: for the
// configured hang stage it makes real workspace mutations (direct file write
// plus an ACP terminal/tool callback), signals the controller, then hangs until
// the server process is killed. Once the success marker exists it behaves
// normally, so a recovered retry can complete.
func (a *agent) maybeHangAfterWrite(req request, sessionID string, cancelCh chan struct{}) bool {
	if a.scenario != "hang_after_write" {
		return false
	}
	stage := a.effectiveStage()
	if markerExists() {
		return false
	}
	if stage == "review" && getenv("WAYSHARD_FAKE_REVIEW_REJECT", "") == "1" {
		a.streamFinal(sessionID, artifactJSON("review", false))
		a.reply(req, map[string]any{"stopReason": "end_turn"})
		return true
	}
	if stage != getenv("WAYSHARD_FAKE_HANG_STAGE", "execute") {
		return false
	}
	cwd, _ := os.Getwd()
	tracked := getenv("WAYSHARD_FAKE_PARTIAL_TRACKED", "tracked.txt")
	_ = os.WriteFile(filepath.Join(cwd, tracked), []byte("partial-"+stage+"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(cwd, getenv("WAYSHARD_FAKE_PARTIAL_FILE", "partial.txt")), []byte("partial\n"), 0o644)
	if toolFile := os.Getenv("WAYSHARD_FAKE_TOOL_WRITE_FILE"); toolFile != "" {
		a.runToolWrite(sessionID, toolFile)
	}
	if toolCmd := os.Getenv("WAYSHARD_FAKE_TOOL_CMD"); toolCmd != "" {
		a.runToolRaw(sessionID, toolCmd)
	}
	if sig := os.Getenv("WAYSHARD_FAKE_SIGNAL_FILE"); sig != "" {
		_ = os.WriteFile(sig, []byte(stage+"\n"), 0o644)
		// Record our PID so a test controller can reap this hung process after
		// the server is SIGKILLed (SIGKILL does not propagate to children).
		_ = os.WriteFile(sig+".pid", []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
	}
	select {
	case <-cancelCh:
	case <-time.After(24 * time.Hour):
	}
	a.reply(req, map[string]any{"stopReason": "cancelled"})
	return true
}

// runToolWrite asks Wayshard to run a command through the ACP terminal/tool
// callback, so the mutation happens through the server's tool callback rather
// than the harness process itself.
func (a *agent) runToolWrite(sessionID, rel string) {
	raw, err := a.callClient("terminal/create", map[string]any{
		"sessionId": sessionID,
		"command":   "/bin/sh",
		"args":      []string{"-c", "printf tool-partial > " + rel},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake-acp: terminal/create: %v\n", err)
		return
	}
	var res struct {
		TerminalID string `json:"terminalId"`
	}
	_ = json.Unmarshal(raw, &res)
	if res.TerminalID == "" {
		return
	}
	_, _ = a.callClient("terminal/wait_for_exit", map[string]any{"sessionId": sessionID, "terminalId": res.TerminalID})
	_, _ = a.callClient("terminal/release", map[string]any{"sessionId": sessionID, "terminalId": res.TerminalID})
}

// runToolRaw asks Wayshard to run a long-lived command through the ACP
// terminal/tool callback without waiting for it to exit. Used by the crash
// fixture to leave a backgrounded descendant that outlives the server.
func (a *agent) runToolRaw(sessionID, script string) {
	raw, err := a.callClient("terminal/create", map[string]any{
		"sessionId": sessionID,
		"command":   "/bin/sh",
		"args":      []string{"-c", script},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake-acp: terminal/create raw: %v\n", err)
		return
	}
	var res struct {
		TerminalID string `json:"terminalId"`
	}
	_ = json.Unmarshal(raw, &res)
	// Give the command a moment to background its writer and record its pid.
	time.Sleep(1200 * time.Millisecond)
}

func markerExists() bool {
	p := os.Getenv("WAYSHARD_FAKE_SUCCESS_MARKER")
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func (a *agent) effectiveStage() string {
	switch a.scenario {
	case "repair":
		return "repair"
	case "explore":
		return "explore"
	case "review_reject":
		return "review"
	}
	return a.stage
}

func (a *agent) streamFinal(sessionID, text string) {
	a.notifyUpdate(sessionID, "agent_message_chunk", text)
}

func (a *agent) notifyUpdate(sessionID, kind, text string) {
	a.writeJSON(map[string]any{
		"jsonrpc": jsonrpc,
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": sessionID,
			"update": map[string]any{
				"sessionUpdate": kind,
				"content":       map[string]any{"type": "text", "text": text},
			},
		},
	})
}

func (a *agent) notifyTool(sessionID, id, title, kind, status string) {
	a.writeJSON(map[string]any{
		"jsonrpc": jsonrpc,
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": sessionID,
			"update": map[string]any{
				"sessionUpdate": "tool_call",
				"toolCallId":    id,
				"title":         title,
				"kind":          kind,
				"status":        status,
			},
		},
	})
}

// permissionOptions are the allow/reject choices the harness offers. The kind
// is what a real ACP harness must inspect: Wayshard reports a denial by
// selecting the reject option, not by cancelling.
func permissionOptions() []map[string]any {
	return []map[string]any{
		{"optionId": "allow-once", "name": "Allow once", "kind": "allow_once"},
		{"optionId": "reject-once", "name": "Reject", "kind": "reject_once"},
	}
}

func (a *agent) requestPermission(sessionID string) (outcome, kind string, err error) {
	a.notifyTool(sessionID, "call_perm", "write file", "edit", "pending")
	opts := permissionOptions()
	raw, err := a.callClient("session/request_permission", map[string]any{
		"sessionId": sessionID,
		"toolCall": map[string]any{
			"toolCallId": "call_perm",
			"title":      "write file",
			"kind":       "edit",
			"status":     "pending",
		},
		"options": opts,
	})
	if err != nil {
		return "", "", err
	}
	var parsed struct {
		Outcome struct {
			Outcome  string `json:"outcome"`
			OptionID string `json:"optionId"`
		} `json:"outcome"`
	}
	_ = json.Unmarshal(raw, &parsed)
	if parsed.Outcome.Outcome == "" {
		return "cancelled", "", nil
	}
	for _, o := range opts {
		if o["optionId"] == parsed.Outcome.OptionID {
			k, _ := o["kind"].(string)
			return parsed.Outcome.Outcome, k, nil
		}
	}
	return parsed.Outcome.Outcome, "", nil
}

func (a *agent) callClient(method string, params any) (json.RawMessage, error) {
	id := idRaw{raw: json.RawMessage(strconv.FormatInt(a.nextID.Add(1)+1000, 10))}
	ch := make(chan response, 1)
	a.mu.Lock()
	a.pending[id.key()] = ch
	a.mu.Unlock()
	a.writeJSON(map[string]any{
		"jsonrpc": jsonrpc,
		"id":      json.RawMessage(id.raw),
		"method":  method,
		"params":  params,
	})
	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("client %s: %d %s", method, resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-time.After(15 * time.Second):
		return nil, fmt.Errorf("client %s timed out", method)
	}
}

func (a *agent) reply(req request, result any) {
	a.writeJSON(response{JSONRPC: jsonrpc, ID: *req.ID, Result: marshal(result)})
}

func (a *agent) replyErr(req request, code int, msg string) {
	a.writeJSON(response{JSONRPC: jsonrpc, ID: *req.ID, Error: &rpcError{Code: code, Message: msg}})
}

func (a *agent) writeJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake-acp marshal: %v\n", err)
		return
	}
	a.writeRaw(append(b, '\n'))
}

func (a *agent) writeRaw(b []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, _ = a.w.Write(b)
}

func marshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// recordProcessLaunch is a test fixture: when WAYSHARD_FAKE_RECORD names a file,
// the fake appends one JSON line describing its working directory, argv and the
// environment values named by WAYSHARD_FAKE_RECORD_KEYS. Native tests use it to
// verify exactly what Wayshard passed to a launched process — working directory,
// argument quoting, and inherited environment — on every platform.
func recordProcessLaunch() {
	path := os.Getenv("WAYSHARD_FAKE_RECORD")
	if path == "" {
		return
	}
	cwd, _ := os.Getwd()
	payload := map[string]any{"cwd": cwd, "args": os.Args[1:]}
	if keys := os.Getenv("WAYSHARD_FAKE_RECORD_KEYS"); keys != "" {
		env := map[string]string{}
		for _, k := range strings.Split(keys, ",") {
			if k != "" {
				env[k] = os.Getenv(k)
			}
		}
		payload["env"] = env
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

func artifactJSON(stage string, ok bool) string {
	switch stage {
	case "execute", "repair":
		return `{"kind":"implementation_report","summary":"applied the requested change","filesChanged":["agent.go"],"deviations":[],"expectedValidation":["true"]}`
	case "review":
		if !ok {
			return `{"kind":"review","verdict":"fail","criteria":[{"id":"c1","status":"fail","evidence":"acceptance criterion not met"}],"findings":[{"severity":"blocking","path":"main.go","explanation":"behavior mismatch","requiredFix":"implement the missing branch"}]}`
		}
		return `{"kind":"review","verdict":"pass","criteria":[{"id":"c1","status":"pass","evidence":"matches the plan"}],"findings":[]}`
	case "explore":
		return `{"kind":"investigation","question":"how does this work","findings":["deterministic fake finding"],"openQuestions":[]}`
	default:
		return `{"kind":"plan","objective":"implement the requested change","constraints":[],"acceptanceCriteria":["behavior matches the request"],"expectedPaths":["agent.go"],"validationPlan":["true"],"risks":[]}`
	}
}
