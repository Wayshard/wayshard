package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/Wayshard/wayshard/internal/auth"
	"github.com/Wayshard/wayshard/internal/backup"
	"github.com/Wayshard/wayshard/internal/ctxengine"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/harness"
	"github.com/Wayshard/wayshard/internal/knowledge"
	"github.com/Wayshard/wayshard/internal/sandbox"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/workspace"
	"github.com/coder/websocket"
)

func (s *Server) projectChanges(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	p, err := s.Store.GetProject(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	b, err := workspace.Open(p.Path)
	if err != nil {
		writeErr(w, err)
		return
	}
	st, err := b.CurrentState(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"kind": "workspace", "branch": st.Branch, "head": st.HEAD, "files": st.Files})
}

func (s *Server) runChanges(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	ws, err := s.Store.GetWorkspaceByRun(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, 200, map[string]any{"kind": "run", "files": []any{}})
		return
	}
	snapDir := filepath.Join(filepath.Dir(ws.RunPath), "snapshot")
	snap, err := workspace.LoadSnapshot(snapDir)
	if err != nil {
		writeJSON(w, 200, map[string]any{"kind": "run", "files": []any{}, "error": err.Error()})
		return
	}
	d, err := workspace.ComputeDelta(snap, ws.RunPath)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"kind": "run", "delta": d})
}

func (s *Server) projectKnowledge(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	p, err := s.Store.GetProject(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	idx, err := knowledge.Discover(r.Context(), p.Path)
	if err != nil {
		writeErr(w, err)
		return
	}
	_ = s.Store.ReplaceProjectKnowledge(r.Context(), p.ID, idx)
	writeJSON(w, 200, idx)
}

func (s *Server) runContext(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	run, err := s.Store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	// Prefer the durable manifest of the bundle actually delivered to a stage.
	if arts, err := s.Store.ListArtifacts(r.Context(), run.ID); err == nil {
		for i := len(arts) - 1; i >= 0; i-- {
			if arts[i].Kind == domain.ArtifactContextManifest {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				w.Write([]byte(arts[i].JSON))
				return
			}
		}
	}
	task, _ := s.Store.GetTask(r.Context(), run.TaskID)
	proj, _ := s.Store.GetProject(r.Context(), run.ProjectID)
	idx, _ := knowledge.Discover(r.Context(), proj.Path)
	eng := ctxengine.New()
	bundle, man, err := eng.Assemble(r.Context(), ctxengine.ContextRequest{
		ProjectID: proj.ID, TaskID: task.ID, RunID: run.ID, Target: ctxengine.TargetPlanner, TokenBudget: 8000, Knowledge: idx, Request: task.Objective,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"bundle": bundle, "manifest": man})
}

func (s *Server) runDetails(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	run, err := s.Store.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	stages, _ := s.Store.ListStages(r.Context(), run.ID)
	arts, _ := s.Store.ListArtifacts(r.Context(), run.ID)
	routes, _ := s.Store.ListRouteDecisions(r.Context(), run.ID)
	assess, _ := s.Store.ListAssessments(r.Context(), run.ID)
	usage, _ := s.Store.ListUsage(r.Context(), run.ID)
	writeJSON(w, 200, map[string]any{
		"run": run, "stages": stages, "artifacts": arts, "routes": routes, "assessments": assess, "usage": usage,
	})
}

func (s *Server) storageInfo(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	acc := s.Store.StorageAccounting(r.Context())
	disk, _ := storage.Disk(s.Store.Root)
	ok, _ := s.Store.WriteHeavyAllowed()
	writeJSON(w, 200, map[string]any{"accounting": acc, "disk": disk, "writeHeavyAllowed": ok, "reposIncludedInBackup": false, "note": "Source repositories are not included in backups by default."})
}

func (s *Server) storageGC(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	n, err := s.Store.GCObjects(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	wsc, _ := s.Store.CleanupWorkspaces(r.Context(), 0)
	cps, _ := s.Store.ReclaimCheckpoints(r.Context(), 0)
	writeJSON(w, 200, map[string]any{"objectsRemoved": n, "workspacesRemoved": wsc, "checkpointsReclaimed": cps})
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	var body struct {
		Dest           string `json:"dest"`
		IncludeSecrets bool   `json:"includeSecrets"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Dest == "" {
		body.Dest = filepath.Join(s.DataDir, "backups", "manual")
	}
	if err := backup.Create(r.Context(), s.Store, body.Dest, body.IncludeSecrets); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "dest": body.Dest, "includesRepos": false, "includesSecrets": body.IncludeSecrets})
}

func (s *Server) sandboxInfo(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	_ = r
	report := sandbox.Probe()
	writeJSON(w, 200, map[string]any{
		"backend":         report.Backend,
		"available":       report.Available,
		"mode":            report.Mode,
		"features":        report.Features,
		"missing":         report.Missing,
		"detail":          report.Detail,
		"providerNetwork": s.ProviderNet,
	})
}

// harnessDefinitions reports the effective harness catalog (shipped + user)
// separately from discovered installations, so diagnostics can distinguish a
// disabled/overridden definition from a missing installation.
func (s *Server) harnessDefinitions(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	_ = r
	type row struct {
		ID                      string   `json:"id"`
		DisplayName             string   `json:"displayName"`
		Source                  string   `json:"source"`
		Enabled                 bool     `json:"enabled"`
		ACP                     string   `json:"acp"`
		Executables             []string `json:"executables,omitempty"`
		Bridges                 []string `json:"bridges,omitempty"`
		Platforms               []string `json:"platforms,omitempty"`
		RequiresProviderNetwork bool     `json:"requiresProviderNetwork"`
		DeclaredTransport       string   `json:"declaredTransport"`
	}
	defs := []row{}
	var diags []harness.CatalogDiagnostic
	if s.Catalog != nil {
		diags = s.Catalog.Diagnostics
		for _, d := range s.Catalog.Definitions {
			defs = append(defs, row{
				ID: d.ID, DisplayName: d.DisplayName, Source: string(d.Source), Enabled: d.Enabled,
				ACP: d.ACP, Executables: d.Executables, Bridges: d.Bridges, Platforms: d.Platforms,
				RequiresProviderNetwork: d.RequiresProviderNetwork, DeclaredTransport: d.DeclaredTransport,
			})
		}
	}
	writeJSON(w, 200, map[string]any{"definitions": defs, "diagnostics": diags})
}

func (s *Server) listTerminals(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	if s.PTY == nil {
		writeJSON(w, 200, []any{})
		return
	}
	list := s.PTY.List(r.PathValue("id"))
	type row struct {
		ID, ProjectID string
		Alive         bool
	}
	var out []row
	for _, t := range list {
		out = append(out, row{ID: t.ID, ProjectID: t.ProjectID, Alive: true})
	}
	writeJSON(w, 200, out)
}

func (s *Server) startTerminal(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	if s.PTY == nil {
		http.Error(w, `{"error":"pty unavailable"}`, 500)
		return
	}
	proj, err := s.Store.GetProject(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	sess, err := s.PTY.Start(context.Background(), proj.ID, proj.Path, p.DeviceID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": sess.ID, "projectId": sess.ProjectID, "alive": true})
}

func (s *Server) ptyWS(w http.ResponseWriter, r *http.Request) {
	if _, err := s.principal(r); err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	id := r.URL.Query().Get("id")
	if s.PTY == nil {
		http.Error(w, "pty unavailable", 500)
		return
	}
	sess, ok := s.PTY.Attach(id)
	if !ok {
		writeJSON(w, 410, map[string]any{"error": "terminal lost", "reason": "server restart or process exit"})
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "bye")
	ctx := r.Context()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := sess.File.Read(buf)
			if n > 0 {
				_ = c.Write(ctx, websocket.MessageBinary, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			// detach; do not kill PTY
			return
		}
		_, _ = sess.File.Write(data)
	}
}
