package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wayshard/wayshard/internal/auth"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/events"
	"github.com/Wayshard/wayshard/internal/harness"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/pty"
	"github.com/Wayshard/wayshard/internal/scheduler"
	"github.com/Wayshard/wayshard/internal/secrets"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/version"
	"github.com/Wayshard/wayshard/internal/webembed"
	"github.com/coder/websocket"
)

type Server struct {
	Store     *storage.Store
	Auth      *auth.Service
	Hub       *events.Hub
	Sched     *scheduler.Scheduler
	Vault     *secrets.Vault
	PTY       *pty.Manager
	Log       *slog.Logger
	Listen    string
	Advertise string
	DataDir   string
	// ProviderNet is the runtime-probed provider networking capability,
	// resolved once at startup for diagnostics.
	ProviderNet domain.ProviderNetworkCapability
	// Catalog is the effective harness catalog for discovery diagnostics.
	Catalog *harness.Catalog
	http    *http.Server
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/meta", s.meta)
	mux.HandleFunc("GET /v1/server", s.requireAuth(s.getServer))
	mux.HandleFunc("POST /v1/pairing/invitations", s.pairingCreate)
	mux.HandleFunc("POST /v1/pairing/complete", s.pairingComplete)
	mux.HandleFunc("GET /v1/pairing/challenge", s.pairingChallenge)
	mux.HandleFunc("GET /v1/devices", s.requireAuth(s.listDevices))
	mux.HandleFunc("POST /v1/devices/{id}/revoke", s.requireAuth(s.revokeDevice))
	mux.HandleFunc("GET /v1/projects", s.requireAuth(s.listProjects))
	mux.HandleFunc("POST /v1/projects/open", s.requireAuth(s.openProject))
	mux.HandleFunc("POST /v1/projects/create", s.requireAuth(s.createProject))
	mux.HandleFunc("POST /v1/projects/clone", s.requireAuth(s.cloneProject))
	mux.HandleFunc("GET /v1/projects/{id}", s.requireAuth(s.getProject))
	mux.HandleFunc("POST /v1/projects/{id}/locate", s.requireAuth(s.locateProject))
	mux.HandleFunc("DELETE /v1/projects/{id}", s.requireAuth(s.removeProject))
	mux.HandleFunc("GET /v1/projects/{id}/conversations", s.requireAuth(s.listConversations))
	mux.HandleFunc("POST /v1/projects/{id}/conversations", s.requireAuth(s.createConversation))
	mux.HandleFunc("GET /v1/conversations/{id}", s.requireAuth(s.getConversation))
	mux.HandleFunc("GET /v1/conversations/{id}/messages", s.requireAuth(s.listMessages))
	mux.HandleFunc("POST /v1/conversations/{id}/messages", s.requireAuth(s.postMessage))
	mux.HandleFunc("GET /v1/runs/{id}", s.requireAuth(s.getRun))
	mux.HandleFunc("POST /v1/runs/{id}/cancel", s.requireAuth(s.cancelRun))
	mux.HandleFunc("POST /v1/runs/{id}/retry", s.requireAuth(s.retryRun))
	mux.HandleFunc("POST /v1/runs/{id}/integrate", s.requireAuth(s.integrateRun))
	mux.HandleFunc("GET /v1/runs/{id}/artifacts", s.requireAuth(s.listArtifacts))
	mux.HandleFunc("GET /v1/runs/{id}/usage", s.requireAuth(s.getUsage))
	mux.HandleFunc("GET /v1/runs/{id}/routes", s.requireAuth(s.getRoutes))
	mux.HandleFunc("GET /v1/runs/{id}/stages", s.requireAuth(s.getStages))
	mux.HandleFunc("GET /v1/harnesses", s.requireAuth(s.listHarnesses))
	mux.HandleFunc("GET /v1/harness-definitions", s.requireAuth(s.harnessDefinitions))
	mux.HandleFunc("POST /v1/harnesses/rescan", s.requireAuth(s.rescanHarnesses))
	mux.HandleFunc("GET /v1/notifications", s.requireAuth(s.listNotes))
	mux.HandleFunc("POST /v1/notifications/{id}/read", s.requireAuth(s.readNote))
	mux.HandleFunc("GET /v1/approvals", s.requireAuth(s.listApprovals))
	mux.HandleFunc("POST /v1/approvals/{id}/resolve", s.requireAuth(s.resolveApproval))
	mux.HandleFunc("GET /v1/settings", s.requireAuth(s.getSettings))
	mux.HandleFunc("PUT /v1/settings", s.requireAuth(s.putSettings))
	mux.HandleFunc("GET /v1/projects/{id}/files", s.requireAuth(s.listFiles))
	mux.HandleFunc("GET /v1/projects/{id}/file", s.requireAuth(s.readFile))
	mux.HandleFunc("PUT /v1/projects/{id}/file", s.requireAuth(s.writeFile))
	mux.HandleFunc("GET /v1/projects/{id}/changes", s.requireAuth(s.projectChanges))
	mux.HandleFunc("GET /v1/runs/{id}/changes", s.requireAuth(s.runChanges))
	mux.HandleFunc("GET /v1/runs/{id}/file", s.requireAuth(s.runFile))
	mux.HandleFunc("GET /v1/projects/{id}/knowledge", s.requireAuth(s.projectKnowledge))
	mux.HandleFunc("GET /v1/runs/{id}/context", s.requireAuth(s.runContext))
	mux.HandleFunc("GET /v1/runs/{id}/details", s.requireAuth(s.runDetails))
	mux.HandleFunc("GET /v1/storage", s.requireAuth(s.storageInfo))
	mux.HandleFunc("POST /v1/storage/gc", s.requireAuth(s.storageGC))
	mux.HandleFunc("POST /v1/backups", s.requireAuth(s.createBackup))
	mux.HandleFunc("GET /v1/sandbox", s.requireAuth(s.sandboxInfo))
	mux.HandleFunc("GET /v1/projects/{id}/terminals", s.requireAuth(s.listTerminals))
	mux.HandleFunc("POST /v1/projects/{id}/terminals", s.requireAuth(s.startTerminal))
	mux.HandleFunc("GET /v1/ws/pty", s.ptyWS)
	mux.HandleFunc("GET /v1/ws", s.ws)
	mux.Handle("/", webembed.Handler())
	return s.middleware(mux)
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := s.Listen
	if addr == "" {
		addr = "127.0.0.1:7420"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.http = &http.Server{Handler: s.Handler(), BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() {
		<-ctx.Done()
		shctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.http.Shutdown(shctx)
	}()
	s.Log.Info("wayshard server listening", "addr", ln.Addr().String())
	err = s.http.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Wayshard-API", strconv.Itoa(version.Compatibility))
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "product": version.Product})
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, version.Current())
}

func (s *Server) requireAuth(h func(http.ResponseWriter, *http.Request, *auth.Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := s.principal(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		h(w, r, p)
	}
}

func (s *Server) principal(r *http.Request) (*auth.Principal, error) {
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	cookie, _ := r.Cookie("wayshard_session")
	ck := ""
	if cookie != nil {
		ck = cookie.Value
	}
	return s.Auth.Authenticate(r.Context(), strings.TrimSpace(bearer), ck)
}

func (s *Server) allowLocalAdmin(r *http.Request) bool {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host == "" {
		host = r.RemoteAddr
	}
	if !auth.LocalAdminBypass(host) {
		return false
	}
	// loopback-only recovery pairing / first-run when no trusted device exists
	devs, err := s.Store.ListDevices(r.Context())
	if err != nil {
		return false
	}
	for _, d := range devs {
		if d.RevokedAt == nil {
			return false
		}
	}
	return true
}

func (s *Server) pairingCreate(w http.ResponseWriter, r *http.Request) {
	if _, err := s.principal(r); err != nil && !s.allowLocalAdmin(r) {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	var body struct {
		AdvertisedURL string `json:"advertisedUrl"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)
	if body.AdvertisedURL == "" {
		body.AdvertisedURL = s.Advertise
	}
	if body.AdvertisedURL == "" {
		body.AdvertisedURL = "http://" + s.Listen
	}
	res, err := s.Auth.CreateInvitation(r.Context(), body.AdvertisedURL, "api")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) pairingComplete(w http.ResponseWriter, r *http.Request) {
	var req auth.CompletePairingRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.Auth.CompletePairing(r.Context(), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	if res.Session != "" {
		http.SetCookie(w, &http.Cookie{Name: "wayshard_session", Value: res.Session, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	}
	writeJSON(w, 200, res)
}

func (s *Server) pairingChallenge(w http.ResponseWriter, r *http.Request) {
	nonce, _ := hex.DecodeString(r.URL.Query().Get("nonce"))
	if len(nonce) == 0 {
		nonce = []byte("wayshard-pairing")
	}
	id, fp, sig, err := s.Auth.Challenge(r.Context(), nonce)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"serverId": id, "fingerprint": fp, "signature": hex.EncodeToString(sig)})
}

func (s *Server) getServer(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ident, err := s.Store.GetServerIdentity(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"serverId": ident.ServerID, "displayName": ident.DisplayName, "listen": s.Listen, "advertise": s.Advertise,
		"vaultLocked": s.Vault != nil && s.Vault.Locked(), "device": p,
		"compatibility": version.Current(),
	})
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	devs, err := s.Store.ListDevices(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	type view struct {
		ID, Name, Kind string
		CreatedAt      time.Time
		Revoked        bool
	}
	var out []view
	for _, d := range devs {
		out = append(out, view{ID: d.ID, Name: d.Name, Kind: d.Kind, CreatedAt: d.CreatedAt, Revoked: d.RevokedAt != nil})
	}
	writeJSON(w, 200, out)
}

func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	if err := s.Store.RevokeDevice(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	list, err := s.Store.ListProjects(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) openProject(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	path, err := filepath.Abs(body.Path)
	if err != nil {
		writeErr(w, err)
		return
	}
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		http.Error(w, `{"error":"path not a directory"}`, 400)
		return
	}
	if existing, err := s.Store.GetProjectByPath(r.Context(), path); err == nil {
		_ = s.Store.TouchProject(r.Context(), existing.ID)
		writeJSON(w, 200, existing)
		return
	}
	name := body.Name
	if name == "" {
		name = filepath.Base(path)
	}
	kind := "filesystem"
	repoID := path
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		kind = "git"
	}
	p := &domain.Project{Name: name, Path: path, SourceKind: kind, RepoIdentity: repoID, Status: domain.ProjectAvailable}
	if err := s.Store.InsertProject(r.Context(), p); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 201, p)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
		Git  bool   `json:"git"`
		Desc string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	path, _ := filepath.Abs(body.Path)
	if err := os.MkdirAll(path, 0o755); err != nil {
		writeErr(w, err)
		return
	}
	// Do not create canonical documents.
	if body.Git {
		_ = execSilent("git", path, "init")
	}
	s.openProject(w, rWithJSON(r, map[string]string{"path": path, "name": body.Name}), p)
}

func (s *Server) cloneProject(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var body struct {
		URL  string `json:"url"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	path, _ := filepath.Abs(body.Path)
	if err := execSilent("git", filepath.Dir(path), "clone", body.URL, path); err != nil {
		writeErr(w, err)
		return
	}
	s.openProject(w, rWithJSON(r, map[string]string{"path": path}), p)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	p, err := s.Store.GetProject(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := os.Stat(p.Path); err != nil {
		p.Status = domain.ProjectUnavailable
	}
	writeJSON(w, 200, p)
}

func (s *Server) locateProject(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	var body struct {
		Path string `json:"path"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.Store.UpdateProjectPath(r.Context(), r.PathValue("id"), body.Path, body.Path); err != nil {
		writeErr(w, err)
		return
	}
	s.getProject(w, r, nil)
}

func (s *Server) removeProject(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	// Never deletes source directory.
	if err := s.Store.RemoveProject(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "sourceDeleted": false})
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	list, err := s.Store.ListConversations(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	var body struct {
		Title string `json:"title"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	c := &domain.Conversation{ProjectID: r.PathValue("id"), Title: body.Title}
	if err := s.Store.InsertConversation(r.Context(), c); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 201, c)
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	c, err := s.Store.GetConversation(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, c)
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	list, err := s.Store.ListMessages(r.Context(), r.PathValue("id"), 500)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) postMessage(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	key := r.Header.Get("Idempotency-Key")
	var body struct {
		Text         string `json:"text"`
		ArtifactOnly bool   `json:"artifactOnly"`
		Profile      string `json:"profile"`
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err := json.Unmarshal(raw, &body); err != nil {
		writeErr(w, err)
		return
	}
	if key != "" {
		sum := sha256.Sum256(raw)
		if resp, ok, err := s.Store.GetIdempotency(r.Context(), key, p.DeviceID, hex.EncodeToString(sum[:])); err == nil && ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(resp))
			return
		} else if errors.Is(err, storage.ErrConflict) {
			http.Error(w, `{"error":"idempotency key reused with a different request"}`, http.StatusConflict)
			return
		}
	}
	conv, err := s.Store.GetConversation(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	msg := &domain.Message{ConversationID: conv.ID, Role: domain.RoleUser, Body: body.Text}
	task := &domain.Task{ProjectID: conv.ProjectID, ConversationID: conv.ID, Objective: body.Text, ArtifactOnly: body.ArtifactOnly}
	run := &domain.Run{ProjectID: conv.ProjectID, ConversationID: conv.ID, Profile: domain.RoutingProfile(body.Profile), IdempotencyKey: key}
	if run.Profile == "" {
		run.Profile = domain.ProfileAuto
	}
	if err := s.Store.CreateTaskRun(r.Context(), msg, task, run); err != nil {
		writeErr(w, err)
		return
	}
	if s.Sched != nil {
		s.Sched.Enqueue(run.ID)
	}
	out := map[string]any{"message": msg, "task": task, "run": run}
	b, _ := json.Marshal(out)
	if key != "" {
		sum := sha256.Sum256(raw)
		_ = s.Store.PutIdempotency(r.Context(), key, p.DeviceID, hex.EncodeToString(sum[:]), string(b))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	w.Write(b)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	run, err := s.Store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, run)
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	id := r.PathValue("id")
	// Interrupt active execution first; this cancels the stage context, which
	// tears down the ACP process tree.
	if s.Sched != nil {
		s.Sched.Cancel(id)
	}
	changed, err := s.Store.CancelRun(r.Context(), id, "cancelled by client")
	if err != nil {
		writeErr(w, err)
		return
	}
	if !changed {
		if run, err := s.Store.GetRun(r.Context(), id); err == nil {
			writeJSON(w, 200, map[string]any{"ok": true, "status": run.Status})
			return
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "status": domain.RunCancelled})
}

func (s *Server) retryRun(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	old, err := s.Store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	task, err := s.Store.GetTask(r.Context(), old.TaskID)
	if err != nil {
		writeErr(w, err)
		return
	}
	run := &domain.Run{TaskID: old.TaskID, ProjectID: old.ProjectID, ConversationID: old.ConversationID, Profile: old.Profile, Status: domain.RunQueued}
	msg := &domain.Message{ConversationID: old.ConversationID, Role: domain.RoleUser, Body: task.Objective}
	if err := s.Store.CreateTaskRun(r.Context(), msg, task, run); err != nil {
		writeErr(w, err)
		return
	}
	if s.Sched != nil {
		s.Sched.Enqueue(run.ID)
	}
	writeJSON(w, 202, run)
}

func (s *Server) integrateRun(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	if err := s.Store.UpdateRunStatus(r.Context(), r.PathValue("id"), domain.RunReadyToIntegrate, "", "manual integrate"); err != nil {
		writeErr(w, err)
		return
	}
	if s.Sched != nil {
		s.Sched.Enqueue(r.PathValue("id"))
	}
	writeJSON(w, 202, map[string]any{"ok": true})
}

func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	list, err := s.Store.ListArtifacts(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) getUsage(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	list, err := s.Store.ListUsage(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) getRoutes(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	list, err := s.Store.ListRouteDecisions(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) getStages(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	list, err := s.Store.ListStages(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) listHarnesses(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	list, err := s.Store.ListHarnessInstallations(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) rescanHarnesses(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	writeJSON(w, 202, map[string]any{"ok": true, "note": "scan queued"})
}

func (s *Server) listNotes(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	list, err := s.Store.ListNotifications(r.Context(), r.URL.Query().Get("unread") == "1", 100)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) readNote(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	_ = s.Store.MarkNotificationRead(r.Context(), r.PathValue("id"))
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) listApprovals(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	list, err := s.Store.ListPendingApprovals(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) resolveApproval(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var body struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.Store.ResolveApproval(r.Context(), r.PathValue("id"), body.Status, p.DeviceID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	scope := domain.SettingScope(r.URL.Query().Get("scope"))
	if scope == "" {
		scope = domain.ScopeServer
	}
	list, err := s.Store.ListSettings(r.Context(), scope, r.URL.Query().Get("scopeId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	var set domain.Setting
	if err := json.NewDecoder(r.Body).Decode(&set); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Store.UpsertSetting(r.Context(), set); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, set)
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	p, err := s.Store.GetProject(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	rel := r.URL.Query().Get("path")
	root, err := secureJoin(p.Path, rel, false)
	if err != nil {
		http.Error(w, "path", 400)
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		writeErr(w, err)
		return
	}
	type ent struct {
		Name string `json:"name"`
		Dir  bool   `json:"dir"`
		Size int64  `json:"size"`
	}
	var out []ent
	for _, e := range entries {
		info, _ := e.Info()
		sz := int64(0)
		if info != nil {
			sz = info.Size()
		}
		out = append(out, ent{Name: e.Name(), Dir: e.IsDir(), Size: sz})
	}
	writeJSON(w, 200, out)
}

func (s *Server) readFile(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	p, err := s.Store.GetProject(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	rel := r.URL.Query().Get("path")
	full, err := safeJoin(p.Path, rel)
	if err != nil {
		http.Error(w, `{"error":"path escapes project"}`, http.StatusBadRequest)
		return
	}
	b, err := os.ReadFile(full)
	if err != nil {
		writeErr(w, err)
		return
	}
	sum := sha256.Sum256(b)
	writeJSON(w, 200, map[string]any{"path": rel, "content": string(b), "hash": hex.EncodeToString(sum[:]), "binary": !isText(b)})
}

func (s *Server) writeFile(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	p, err := s.Store.GetProject(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Path         string `json:"path"`
		Content      string `json:"content"`
		ExpectedHash string `json:"expectedHash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	full, err := secureJoin(p.Path, body.Path, true)
	if err != nil {
		http.Error(w, `{"error":"path escapes project"}`, http.StatusBadRequest)
		return
	}
	cur, err := os.ReadFile(full)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		writeErr(w, err)
		return
	}
	if err == nil {
		sum := sha256.Sum256(cur)
		if body.ExpectedHash != "" && hex.EncodeToString(sum[:]) != body.ExpectedHash {
			http.Error(w, `{"error":"conflict","code":"stale_hash"}`, http.StatusConflict)
			return
		}
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		writeErr(w, err)
		return
	}
	if err := os.WriteFile(full, []byte(body.Content), 0o644); err != nil {
		writeErr(w, err)
		return
	}
	sum := sha256.Sum256([]byte(body.Content))
	writeJSON(w, 200, map[string]any{"ok": true, "hash": hex.EncodeToString(sum[:])})
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	if _, err := s.principal(r); err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "bye")
	last, _ := strconv.ParseInt(r.URL.Query().Get("lastEventSeq"), 10, 64)
	projectID := r.URL.Query().Get("projectId")
	runID := r.URL.Query().Get("runId")
	ch, cancel := s.Hub.Subscribe(last, projectID, runID)
	defer cancel()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case fr, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(fr)
			if err := c.Write(ctx, websocket.MessageText, b); err != nil {
				return
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	code := 500
	if errors.Is(err, storage.ErrNotFound) {
		code = 404
	} else if errors.Is(err, storage.ErrConflict) || errors.Is(err, auth.ErrUsed) {
		code = 409
	} else if errors.Is(err, auth.ErrUnauthorized) || errors.Is(err, auth.ErrRevoked) || errors.Is(err, auth.ErrIdentity) {
		code = 401
	} else if errors.Is(err, auth.ErrExpired) {
		code = 410
	}
	http.Error(w, `{"error":`+strconv.Quote(err.Error())+`}`, code)
}

func safeJoin(root, rel string) (string, error) {
	return secureJoin(root, rel, false)
}

// secureJoin resolves the target and requires it to remain inside root, even
// when intermediate path components are symlinks. forWrite additionally
// resolves the deepest existing ancestor.
func secureJoin(root, rel string, forWrite bool) (string, error) {
	rootClean := filepath.Clean(root)
	if resolved, err := filepath.EvalSymlinks(rootClean); err == nil {
		rootClean = resolved
	}
	full := filepath.Join(rootClean, filepath.Clean("/"+rel))
	check := full
	if forWrite {
		// Resolve the nearest existing ancestor so a new file cannot be
		// written through a symlinked directory that escapes the root.
		check = full
		for {
			parent := filepath.Dir(check)
			if parent == check {
				break
			}
			if _, err := os.Lstat(check); err == nil {
				break
			}
			check = parent
		}
	}
	resolved, err := filepath.EvalSymlinks(check)
	if err != nil {
		resolved = check
	}
	if resolved != rootClean && !strings.HasPrefix(resolved, rootClean+string(os.PathSeparator)) {
		return "", errors.New("path escapes project")
	}
	if full != rootClean && !strings.HasPrefix(full, rootClean+string(os.PathSeparator)) {
		return "", errors.New("path escapes project")
	}
	return full, nil
}

func isText(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return false
		}
	}
	return true
}

func execSilent(name, dir string, args ...string) error {
	// used only for git clone/init which are user-requested, not discovery
	c := exec.Command(name, args...)
	c.Dir = dir
	return c.Run()
}

func rWithJSON(r *http.Request, v any) *http.Request {
	b, _ := json.Marshal(v)
	nr := r.Clone(r.Context())
	nr.Body = io.NopCloser(strings.NewReader(string(b)))
	return nr
}

// Ensure orchestrator import used for docs / future wiring.
var _ = orchestrator.NewID
