package ctxengine

import (
	"context"
	"strings"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/knowledge"
)

func TestContextBudgetRespected(t *testing.T) {
	idx := &knowledge.Index{
		Revision: "rev1",
		Documents: []knowledge.Document{
			{
				Path: "AGENTS.md", Kind: knowledge.KindAgents, Family: knowledge.FamilyAgents,
				Scope: "/", AuthorityDomain: knowledge.DomainImplementation,
				Content: strings.Repeat("implementation rule must be followed. ", 80),
				Hash:    "h-agents", Status: knowledge.StatusOK,
			},
			{
				Path: "SPEC.md", Kind: knowledge.KindSpec, Family: knowledge.FamilyWayshard,
				Scope: "/", AuthorityDomain: knowledge.DomainProduct, Canonical: true,
				Content: strings.Repeat("product behavior belongs here. ", 80),
				Hash:    "h-spec", Status: knowledge.StatusOK,
			},
		},
	}
	eng := New()
	bundle, man, err := eng.Assemble(context.Background(), ContextRequest{
		Target:      TargetPlanner,
		Stage:       domain.StagePlan,
		TokenBudget: 40,
		Request:     "add a flag",
		Knowledge:   idx,
		Summary:     strings.Repeat("older chatter ", 40),
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Tokens > 40 {
		t.Fatalf("tokens %d exceed budget 40", bundle.Tokens)
	}
	if man.Tokens != bundle.Tokens || man.TokenBudget != 40 {
		t.Fatalf("manifest tokens=%d budget=%d", man.Tokens, man.TokenBudget)
	}
	if len(man.Omitted) == 0 {
		hasTrunc := false
		for _, it := range bundle.Items {
			if it.Delivery == DeliveryTruncated {
				hasTrunc = true
			}
		}
		if !hasTrunc {
			t.Fatalf("expected omitted or truncated items under tight budget, items=%d tokens=%d", len(bundle.Items), bundle.Tokens)
		}
	}
	for _, it := range man.Items {
		if it.Hash == "" || it.Provenance.Source == "" {
			t.Fatalf("manifest item missing provenance/hash: %+v", it)
		}
	}
}

func TestJevPrefersStructuredSignals(t *testing.T) {
	body := strings.Repeat("never dump this raw agents file into jev. ", 50)
	idx := &knowledge.Index{
		Revision: "rev-jev",
		Documents: []knowledge.Document{
			{
				Path: "AGENTS.md", Kind: knowledge.KindAgents, Family: knowledge.FamilyAgents,
				Scope: "/", AuthorityDomain: knowledge.DomainImplementation,
				Content: body, Hash: "h1", Status: knowledge.StatusOK,
			},
		},
		Sections: []knowledge.Section{
			{Heading: "Purpose", Hash: "s1"},
		},
		Conflicts: []knowledge.ConflictWarning{
			{Domain: knowledge.DomainImplementation, PathA: "AGENTS.md", PathB: "OTHER.md", Reason: "opposite directives"},
		},
	}
	eng := New()
	bundle, man, err := eng.Assemble(context.Background(), ContextRequest{
		Target:      TargetJev,
		Stage:       domain.StageAssess,
		TokenBudget: 4000,
		Request:     "how risky is this change?",
		Knowledge:   idx,
		GitFacts:    []GitFact{{Key: "branch", Value: "main"}},
		Artifacts: []ArtifactRef{{
			Kind: domain.ArtifactPlan, JSON: `{"objective":"x"}` + strings.Repeat(" padding", 40), Hash: "art1",
		}},
		Summary: "old summary must not be treated as spec",
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawSignal, sawConflict, sawRawDump, sawStructuredArtifact bool
	for _, it := range bundle.Items {
		switch it.Kind {
		case "knowledge_signal":
			sawSignal = true
			if it.Delivery != DeliveryStructured {
				t.Fatalf("knowledge signal delivery = %s", it.Delivery)
			}
			if !strings.Contains(it.Body, `"path":"AGENTS.md"`) && !strings.Contains(it.Body, `"path": "AGENTS.md"`) {
				t.Fatalf("signal missing path: %s", it.Body)
			}
			if strings.Contains(it.Body, body) {
				t.Fatal("Jev bundle dumped raw AGENTS.md body")
			}
		case "instruction":
			if it.Delivery == DeliveryInline && strings.Contains(it.Body, "never dump this raw") {
				sawRawDump = true
			}
		case "conflict":
			sawConflict = true
			if it.Delivery != DeliveryStructured {
				t.Fatalf("conflict delivery = %s", it.Delivery)
			}
		case "artifact":
			if it.Delivery == DeliveryStructured && !strings.Contains(it.Body, "padding") {
				sawStructuredArtifact = true
			}
			if strings.Contains(it.Body, "padding") && it.Delivery == DeliveryInline {
				t.Fatal("Jev dumped raw artifact JSON")
			}
		case "summary":
			if !strings.Contains(it.Reason, "not authority") && !strings.Contains(it.Reason, "compression") {
				t.Fatalf("summary reason should mark compression: %s", it.Reason)
			}
		}
		if it.Hash == "" {
			t.Fatalf("item %s missing hash", it.Kind)
		}
		if it.Provenance.Source == "" {
			t.Fatalf("item %s missing provenance", it.Kind)
		}
	}
	if !sawSignal {
		t.Fatal("Jev bundle missing structured knowledge signals")
	}
	if !sawConflict {
		t.Fatal("Jev bundle missing conflict signals")
	}
	if sawRawDump {
		t.Fatal("Jev bundle inlined raw instruction dump")
	}
	if !sawStructuredArtifact {
		t.Fatal("Jev bundle should pass artifacts as structured signals")
	}
	if man.KnowledgeRev != "rev-jev" {
		t.Fatalf("manifest rev = %s", man.KnowledgeRev)
	}
}

func TestPlannerGetsInstructionsExecutorGetsContract(t *testing.T) {
	idx := &knowledge.Index{
		Documents: []knowledge.Document{
			{
				Path: "AGENTS.md", Kind: knowledge.KindAgents, Family: knowledge.FamilyAgents,
				Scope: "/", AuthorityDomain: knowledge.DomainImplementation,
				Content: "write only in the run workspace", Hash: "a", Canonical: true,
			},
			{
				Path: "CLAUDE.md", Kind: knowledge.KindClaude, Family: knowledge.FamilyClaude,
				Scope: "/", AuthorityDomain: knowledge.DomainImplementation,
				Content: "claude-only voice", Hash: "c",
			},
		},
	}
	eng := New()
	plan, _, err := eng.Assemble(context.Background(), ContextRequest{
		Target: TargetPlanner, Stage: domain.StagePlan, TokenBudget: 2000,
		Request: "implement foo", Knowledge: idx,
		Artifacts: []ArtifactRef{{Kind: domain.ArtifactInvestigation, JSON: `{"findings":[]}`, Hash: "inv"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKindPath(plan, "instruction", "AGENTS.md") {
		t.Fatalf("planner missing AGENTS.md: %+v", kinds(plan))
	}
	if hasKindPath(plan, "instruction", "CLAUDE.md") {
		t.Fatal("planner must not inject CLAUDE.md without claude family")
	}

	exec, _, err := eng.Assemble(context.Background(), ContextRequest{
		Target: TargetExecutor, Stage: domain.StageExecute, TokenBudget: 2000,
		Request: "implement foo", Knowledge: idx,
		Artifacts: []ArtifactRef{
			{Kind: domain.ArtifactPlan, JSON: `{"objective":"foo"}`, Hash: "p"},
			{Kind: domain.ArtifactReview, JSON: `{"findings":[]}`, Hash: "r"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(exec, "artifact") {
		t.Fatal("executor missing plan artifact")
	}
	for _, it := range exec.Items {
		if it.Kind == "artifact" && it.Path == string(domain.ArtifactReview) {
			t.Fatal("executor should not receive review artifact")
		}
	}
}

func hasKind(b ContextBundle, kind string) bool {
	for _, it := range b.Items {
		if it.Kind == kind {
			return true
		}
	}
	return false
}

func hasKindPath(b ContextBundle, kind, path string) bool {
	for _, it := range b.Items {
		if it.Kind == kind && it.Path == path {
			return true
		}
	}
	return false
}

func kinds(b ContextBundle) []string {
	var out []string
	for _, it := range b.Items {
		out = append(out, it.Kind+":"+it.Path)
	}
	return out
}
