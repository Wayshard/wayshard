// Package ctxengine assembles stage-specific context bundles.
package ctxengine

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Wayshard/wayshard/internal/crypto"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/knowledge"
)

// Target is a stage-specific context consumer.
type Target string

const (
	TargetJev      Target = "jev"
	TargetPlanner  Target = "planner"
	TargetExecutor Target = "executor"
	TargetReviewer Target = "reviewer"
	TargetRepair   Target = "repair"
	TargetExplore  Target = "explore"
)

// DeliveryMode records how an item was placed in the bundle.
type DeliveryMode string

const (
	DeliveryInline     DeliveryMode = "inline"
	DeliveryStructured DeliveryMode = "structured"
	DeliveryReference  DeliveryMode = "reference"
	DeliveryTruncated  DeliveryMode = "truncated"
	DeliveryOmitted    DeliveryMode = "omitted"
)

// Provenance is the inspectable origin of a context item.
type Provenance struct {
	Source   string `json:"source"`
	Path     string `json:"path,omitempty"`
	ID       string `json:"id,omitempty"`
	Revision string `json:"revision,omitempty"`
}

type WindowMessage struct {
	Role string
	Body string
}

type ArtifactRef struct {
	Kind  domain.ArtifactKind
	JSON  string
	Hash  string
	Stage domain.StageKind
}

type RepoHint struct {
	Path string `json:"path"`
	Note string `json:"note,omitempty"`
}

type GitFact struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type KnowledgeSignal struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Family   string `json:"family"`
	Domain   string `json:"authorityDomain"`
	Scope    string `json:"scope"`
	Hash     string `json:"hash"`
	Status   string `json:"status"`
	Explicit bool   `json:"explicit,omitempty"`
}

// ContextRequest is the input to Assemble.
type ContextRequest struct {
	ProjectID     string
	TaskID        string
	RunID         string
	Stage         domain.StageKind
	Target        Target
	TokenBudget   int
	Paths         []string
	HarnessFamily knowledge.Family

	Request      string
	Constraints  []string
	Summary      string
	RecentWindow []WindowMessage
	Artifacts    []ArtifactRef
	RepoHints    []RepoHint
	GitFacts     []GitFact
	Knowledge    *knowledge.Index
}

type ContextItem struct {
	Kind       string
	Path       string
	Body       string
	Provenance Provenance
	Reason     string
	Authority  knowledge.AuthorityDomain
	Hash       string
	Delivery   DeliveryMode
	Tokens     int
}

type ContextBundle struct {
	Target Target
	Items  []ContextItem
	Tokens int
}

type ManifestItem struct {
	Kind       string                    `json:"kind"`
	Path       string                    `json:"path,omitempty"`
	Hash       string                    `json:"hash"`
	Authority  knowledge.AuthorityDomain `json:"authority,omitempty"`
	Delivery   DeliveryMode              `json:"delivery"`
	Tokens     int                       `json:"tokens"`
	Reason     string                    `json:"reason"`
	Provenance Provenance                `json:"provenance"`
}

type OmittedItem struct {
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Hash   string `json:"hash,omitempty"`
	Reason string `json:"reason"`
	Tokens int    `json:"tokens"`
}

type ContextManifest struct {
	Target       Target           `json:"target"`
	Stage        domain.StageKind `json:"stage"`
	KnowledgeRev string           `json:"knowledgeRev,omitempty"`
	ItemCount    int              `json:"itemCount"`
	Tokens       int              `json:"tokens"`
	TokenBudget  int              `json:"tokenBudget"`
	Items        []ManifestItem   `json:"items"`
	Omitted      []OmittedItem    `json:"omitted,omitempty"`
}

// RepositoryRetriever is an optional supporting-material source. Deterministic
// authority resolution always runs first; retrieval is never required.
type RepositoryRetriever interface {
	Retrieve(ctx context.Context, req ContextRequest, budget int) ([]RepoHint, error)
}

type Engine struct {
	Retriever RepositoryRetriever
}

func New() *Engine { return &Engine{} }

func (e *Engine) Assemble(ctx context.Context, req ContextRequest) (ContextBundle, ContextManifest, error) {
	if err := ctx.Err(); err != nil {
		return ContextBundle{}, ContextManifest{}, err
	}
	if req.Target == "" {
		req.Target = targetForStage(req.Stage)
	}
	p := newPacker(req.TokenBudget)

	p.add(ContextItem{
		Kind:       "request",
		Body:       req.Request,
		Reason:     "current user intent; outranks older conversation summaries",
		Provenance: Provenance{Source: "user_request", ID: req.TaskID, Revision: hashOf(req.Request)},
		Delivery:   DeliveryInline,
		Authority:  knowledge.DomainUnknown,
	})
	for _, c := range req.Constraints {
		p.add(ContextItem{
			Kind:       "constraint",
			Body:       c,
			Reason:     "hard task constraint",
			Provenance: Provenance{Source: "constraint", Revision: hashOf(c)},
			Delivery:   DeliveryInline,
		})
	}

	switch req.Target {
	case TargetJev:
		packJev(p, req)
	default:
		packStage(p, req)
	}

	if e != nil && e.Retriever != nil {
		remain := 0
		if req.TokenBudget > 0 {
			remain = req.TokenBudget - p.used
		}
		if remain > 0 || req.TokenBudget == 0 {
			hints, err := e.Retriever.Retrieve(ctx, req, remain)
			if err == nil {
				for _, h := range hints {
					p.add(hintItem(h))
				}
			}
		}
	}

	return p.result(req)
}

func targetForStage(stage domain.StageKind) Target {
	switch stage {
	case domain.StagePlan, domain.StageReplan:
		return TargetPlanner
	case domain.StageExecute:
		return TargetExecutor
	case domain.StageReview:
		return TargetReviewer
	case domain.StageRepair:
		return TargetRepair
	case domain.StageExplore:
		return TargetExplore
	case domain.StageAssess:
		return TargetJev
	default:
		return TargetPlanner
	}
}

func packJev(p *packer, req ContextRequest) {
	idx := req.Knowledge
	if idx != nil {
		signals := make([]KnowledgeSignal, 0, len(idx.Documents))
		headings := make([]map[string]string, 0)
		for _, d := range idx.Documents {
			signals = append(signals, KnowledgeSignal{
				Path: d.Path, Kind: string(d.Kind), Family: string(d.Family),
				Domain: string(d.AuthorityDomain), Scope: d.Scope, Hash: d.Hash,
				Status: d.Status, Explicit: d.Explicit,
			})
		}
		for _, s := range idx.Sections {
			if s.Heading == "" {
				continue
			}
			headings = append(headings, map[string]string{"heading": s.Heading, "hash": s.Hash})
			if len(headings) >= 80 {
				break
			}
		}
		p.add(ContextItem{
			Kind:       "knowledge_signal",
			Body:       mustJSON(map[string]any{"documents": signals, "headings": headings, "revision": idx.Revision}),
			Reason:     "structured project-knowledge signals; not a raw document dump",
			Provenance: Provenance{Source: "knowledge", Revision: idx.Revision},
			Delivery:   DeliveryStructured,
		})
		if len(idx.Conflicts) > 0 {
			p.add(ContextItem{
				Kind:       "conflict",
				Body:       mustJSON(idx.Conflicts),
				Reason:     "heuristic conflict warnings; Jev does not decide document authority",
				Provenance: Provenance{Source: "knowledge", Revision: idx.Revision},
				Delivery:   DeliveryStructured,
			})
		}
	}
	packGitAndRepo(p, req)
	packArtifacts(p, req, true)
	packSummary(p, req)
	packWindow(p, req, 4)
}

func packStage(p *packer, req ContextRequest) {
	idx := req.Knowledge
	if idx != nil {
		resolved := knowledge.Resolve(idx, knowledge.ResolveQuery{
			Paths:         req.Paths,
			Stage:         req.Stage,
			HarnessFamily: req.HarnessFamily,
		})
		for _, r := range resolved {
			d := r.Document
			p.add(ContextItem{
				Kind:       "instruction",
				Path:       d.Path,
				Body:       d.Content,
				Reason:     r.Reason,
				Authority:  d.AuthorityDomain,
				Provenance: Provenance{Source: "knowledge", Path: d.Path, ID: d.ID, Revision: d.Hash},
				Delivery:   DeliveryInline,
			})
		}
		if len(idx.Conflicts) > 0 {
			p.add(ContextItem{
				Kind:       "conflict",
				Body:       mustJSON(idx.Conflicts),
				Reason:     "advisory conflicts; domain owner still decides, Jev does not",
				Provenance: Provenance{Source: "knowledge", Revision: idx.Revision},
				Delivery:   DeliveryStructured,
			})
		}
	}
	packArtifacts(p, req, false)
	packSummary(p, req)
	packWindow(p, req, 8)
	packGitAndRepo(p, req)
}

func packSummary(p *packer, req ContextRequest) {
	if strings.TrimSpace(req.Summary) == "" {
		return
	}
	p.add(ContextItem{
		Kind:   "summary",
		Body:   req.Summary,
		Reason: "conversation summary is compression, not authority",
		Provenance: Provenance{
			Source:   "conversation_summary",
			ID:       req.TaskID,
			Revision: hashOf(req.Summary),
		},
		Delivery: DeliveryInline,
	})
}

func packWindow(p *packer, req ContextRequest, n int) {
	msgs := req.RecentWindow
	if n > 0 && len(msgs) > n {
		msgs = msgs[len(msgs)-n:]
	}
	for i, m := range msgs {
		p.add(ContextItem{
			Kind:   "window",
			Body:   m.Role + ": " + m.Body,
			Reason: "recent conversation window",
			Provenance: Provenance{
				Source:   "conversation_window",
				ID:       req.TaskID,
				Revision: hashOf(m.Body),
			},
			Delivery: DeliveryInline,
			Path:     strconv.Itoa(i),
		})
	}
}

func packGitAndRepo(p *packer, req ContextRequest) {
	if len(req.GitFacts) > 0 {
		p.add(ContextItem{
			Kind:       "git_fact",
			Body:       mustJSON(req.GitFacts),
			Reason:     "fresh workspace/Git facts",
			Provenance: Provenance{Source: "git", Revision: hashOf(mustJSON(req.GitFacts))},
			Delivery:   DeliveryStructured,
		})
	}
	if len(req.RepoHints) > 0 {
		p.add(ContextItem{
			Kind:       "repo_hint",
			Body:       mustJSON(req.RepoHints),
			Reason:     "structural repository orientation, not a substitute for exploration",
			Provenance: Provenance{Source: "repo_structure", Revision: hashOf(mustJSON(req.RepoHints))},
			Delivery:   DeliveryStructured,
		})
	}
}

func packArtifacts(p *packer, req ContextRequest, structuredOnly bool) {
	for _, a := range req.Artifacts {
		if !artifactRelevant(req.Target, a.Kind) {
			continue
		}
		body := a.JSON
		delivery := DeliveryInline
		reason := "durable run artifact " + string(a.Kind)
		if structuredOnly {
			delivery = DeliveryStructured
			body = mustJSON(map[string]any{
				"kind": a.Kind, "hash": a.Hash, "stage": a.Stage, "bytes": len(a.JSON),
			})
			reason = "structured artifact signal for Jev; body not dumped"
		}
		p.add(ContextItem{
			Kind:       "artifact",
			Path:       string(a.Kind),
			Body:       body,
			Reason:     reason,
			Provenance: Provenance{Source: "artifact", ID: a.Hash, Revision: a.Hash},
			Delivery:   delivery,
		})
	}
}

func artifactRelevant(target Target, kind domain.ArtifactKind) bool {
	switch target {
	case TargetJev:
		return true
	case TargetPlanner, TargetExplore:
		switch kind {
		case domain.ArtifactInvestigation, domain.ArtifactResearch, domain.ArtifactPlan, domain.ArtifactTaskContract:
			return true
		}
	case TargetExecutor:
		switch kind {
		case domain.ArtifactPlan, domain.ArtifactTaskContract:
			return true
		}
	case TargetReviewer:
		switch kind {
		case domain.ArtifactPlan, domain.ArtifactTaskContract, domain.ArtifactImplementation,
			domain.ArtifactValidation, domain.ArtifactDiffSummary, domain.ArtifactTestReport:
			return true
		}
	case TargetRepair:
		switch kind {
		case domain.ArtifactPlan, domain.ArtifactTaskContract, domain.ArtifactImplementation,
			domain.ArtifactValidation, domain.ArtifactReview, domain.ArtifactFailure, domain.ArtifactTestReport:
			return true
		}
	}
	return false
}

func hintItem(h RepoHint) ContextItem {
	return ContextItem{
		Kind:       "repo_hint",
		Path:       h.Path,
		Body:       mustJSON(h),
		Reason:     "retrieved supporting material",
		Provenance: Provenance{Source: "repo_structure", Path: h.Path, Revision: hashOf(h.Path + h.Note)},
		Delivery:   DeliveryStructured,
	}
}

type packer struct {
	budget  int
	used    int
	items   []ContextItem
	omitted []OmittedItem
}

func newPacker(budget int) *packer { return &packer{budget: budget} }

func (p *packer) add(item ContextItem) {
	if strings.TrimSpace(item.Body) == "" && item.Kind != "request" {
		return
	}
	if item.Hash == "" {
		item.Hash = hashOf(item.Kind + "\x00" + item.Path + "\x00" + item.Body)
	}
	item.Tokens = estimateTokens(item.Body)
	if p.budget > 0 && p.used+item.Tokens > p.budget {
		remain := p.budget - p.used
		if remain >= 24 && (item.Delivery == DeliveryInline || item.Delivery == DeliveryStructured) {
			item.Body = truncateToTokens(item.Body, remain)
			item.Delivery = DeliveryTruncated
			item.Tokens = estimateTokens(item.Body)
			item.Hash = hashOf(item.Kind + "\x00" + item.Path + "\x00" + item.Body)
			if p.used+item.Tokens <= p.budget {
				p.used += item.Tokens
				p.items = append(p.items, item)
				return
			}
		}
		p.omitted = append(p.omitted, OmittedItem{
			Kind: item.Kind, Path: item.Path, Hash: item.Hash,
			Reason: "token budget", Tokens: item.Tokens,
		})
		return
	}
	p.used += item.Tokens
	p.items = append(p.items, item)
}

func (p *packer) result(req ContextRequest) (ContextBundle, ContextManifest, error) {
	rev := ""
	if req.Knowledge != nil {
		rev = req.Knowledge.Revision
	}
	man := ContextManifest{
		Target:       req.Target,
		Stage:        req.Stage,
		KnowledgeRev: rev,
		ItemCount:    len(p.items),
		Tokens:       p.used,
		TokenBudget:  req.TokenBudget,
		Omitted:      p.omitted,
	}
	for _, it := range p.items {
		man.Items = append(man.Items, ManifestItem{
			Kind: it.Kind, Path: it.Path, Hash: it.Hash, Authority: it.Authority,
			Delivery: it.Delivery, Tokens: it.Tokens, Reason: it.Reason, Provenance: it.Provenance,
		})
	}
	return ContextBundle{Target: req.Target, Items: p.items, Tokens: p.used}, man, nil
}

func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	n := (len(s) + 3) / 4
	if n < 1 {
		return 1
	}
	return n
}

func truncateToTokens(s string, tokens int) string {
	n := tokens * 4
	if n <= 0 || len(s) <= n {
		return s
	}
	if n < 4 {
		n = 4
	}
	return s[:n]
}

func hashOf(s string) string { return crypto.HashSHA256([]byte(s)) }

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
