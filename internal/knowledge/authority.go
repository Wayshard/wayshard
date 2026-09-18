package knowledge

import (
	"sort"
	"strings"

	"github.com/Wayshard/wayshard/internal/domain"
)

// ResolveQuery selects applicable documents for a path/stage/harness family.
type ResolveQuery struct {
	Paths         []string
	Stage         domain.StageKind
	HarnessFamily Family
}

// ResolvedDocument is a document applicable to a query, with a native-family reason.
type ResolvedDocument struct {
	Document    Document
	Reason      string
	Specificity int
	Weight      int
}

// Resolve returns documents that apply to the query using each family's native
// scoping. Explicit declarations outrank filename inference. There is no
// universal filename precedence across ecosystems. Jev does not decide
// authority; MEMORY/rationale never overrides the domain owner.
func Resolve(idx *Index, q ResolveQuery) []ResolvedDocument {
	if idx == nil {
		return nil
	}
	paths := make([]string, 0, len(q.Paths))
	for _, p := range q.Paths {
		p = NormalizeRel(p)
		if p != "" {
			paths = append(paths, p)
		}
	}
	projectWide := len(paths) == 0
	var out []ResolvedDocument
	for _, doc := range idx.Documents {
		if !includeFamily(q.HarnessFamily, doc.Family) {
			continue
		}
		ok, spec, reason := applies(doc, paths, projectWide)
		if !ok {
			continue
		}
		out = append(out, ResolvedDocument{
			Document:    doc,
			Reason:      reason,
			Specificity: spec,
			Weight:      domainWeight(q.Stage, doc.AuthorityDomain),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Document.Explicit != b.Document.Explicit {
			return a.Document.Explicit
		}
		if a.Specificity != b.Specificity {
			return a.Specificity > b.Specificity
		}
		if a.Weight != b.Weight {
			return a.Weight > b.Weight
		}
		if a.Document.Family != b.Document.Family {
			return a.Document.Family < b.Document.Family
		}
		return a.Document.Path < b.Document.Path
	})
	return out
}

func includeFamily(harness, doc Family) bool {
	if !doc.HarnessSpecific() {
		return true
	}
	return harness != "" && harness == doc
}

func applies(doc Document, paths []string, projectWide bool) (bool, int, string) {
	spec := specificity(doc)
	if len(doc.Globs) > 0 {
		if projectWide {
			if doc.AlwaysApply {
				return true, spec, "cursor/copilot alwaysApply glob rule"
			}
			return false, 0, ""
		}
		for _, p := range paths {
			if matchAnyGlob(doc.Globs, p) {
				return true, spec + globBonus(doc.Globs, p), "native glob scope"
			}
		}
		return false, 0, ""
	}
	if doc.AlwaysApply {
		return true, spec, "always applies in native family"
	}

	switch doc.Family {
	case FamilyAgents, FamilyClaude, FamilyGemini:
		if projectWide {
			if doc.Scope == "/" || doc.Scope == "" {
				return true, spec, "root " + string(doc.Family) + " instruction"
			}
			return false, 0, ""
		}
		for _, p := range paths {
			if inScope(doc.Scope, p) {
				return true, spec, "hierarchical " + string(doc.Family) + " scope " + doc.Scope
			}
		}
		return false, 0, ""
	case FamilyCursor, FamilyCopilot:
		if projectWide && (doc.Scope == "/" || doc.AlwaysApply) {
			return true, spec, "root " + string(doc.Family) + " instruction"
		}
		if !projectWide {
			for _, p := range paths {
				if inScope(doc.Scope, p) {
					return true, spec, string(doc.Family) + " directory scope"
				}
			}
		}
		return false, 0, ""
	default:
		if projectWide {
			if doc.Scope == "/" || doc.Scope == "" || doc.Canonical {
				reason := "project-wide " + string(doc.AuthorityDomain)
				if doc.Explicit {
					reason = "explicit " + reason
				}
				return true, spec, reason
			}
			return false, 0, ""
		}
		for _, p := range paths {
			if inScope(doc.Scope, p) {
				reason := "path in scope " + doc.Scope
				if doc.Explicit {
					reason = "explicit declaration; " + reason
				}
				return true, spec, reason
			}
		}
		return false, 0, ""
	}
}

func specificity(doc Document) int {
	n := 0
	if doc.Explicit {
		n += 1000
	}
	scope := NormalizeRel(doc.Scope)
	if scope != "" && scope != "/" {
		n += strings.Count(scope, "/") + 1
		n += len(scope)
	}
	if len(doc.Globs) > 0 {
		n += 10
	}
	return n
}

func globBonus(globs []string, filePath string) int {
	best := 0
	for _, g := range globs {
		if matchAnyGlob([]string{g}, filePath) {
			s := len(g)
			if s > best {
				best = s
			}
		}
	}
	return best
}

func domainWeight(stage domain.StageKind, d AuthorityDomain) int {
	preferred := map[domain.StageKind][]AuthorityDomain{
		domain.StagePlan:     {DomainProduct, DomainArchitecture, DomainDesign, DomainImplementation, DomainValidation},
		domain.StageReplan:   {DomainProduct, DomainArchitecture, DomainDesign, DomainImplementation},
		domain.StageExplore:  {DomainUsage, DomainArchitecture, DomainProduct, DomainDesign},
		domain.StageExecute:  {DomainImplementation, DomainArchitecture, DomainValidation, DomainSecurity},
		domain.StageRepair:   {DomainImplementation, DomainValidation, DomainArchitecture},
		domain.StageReview:   {DomainProduct, DomainDesign, DomainArchitecture, DomainValidation, DomainSecurity, DomainImplementation},
		domain.StageValidate: {DomainValidation, DomainImplementation, DomainProduct},
		domain.StageAssess:   {DomainProduct, DomainArchitecture, DomainImplementation, DomainUsage},
	}
	list := preferred[stage]
	if len(list) == 0 {
		list = []AuthorityDomain{DomainProduct, DomainArchitecture, DomainDesign, DomainImplementation}
	}
	for i, x := range list {
		if x == d {
			return 100 - i
		}
	}
	if d == DomainRationale {
		return 1
	}
	return 5
}

// DomainOwner returns the document that owns a concern among candidates.
// Filename ranking is not used; domain membership is. Rationale never wins.
func DomainOwner(docs []ResolvedDocument, domain AuthorityDomain) *ResolvedDocument {
	var best *ResolvedDocument
	for i := range docs {
		d := &docs[i]
		if d.Document.AuthorityDomain != domain {
			continue
		}
		if domain != DomainRationale && d.Document.AuthorityDomain == DomainRationale {
			continue
		}
		if best == nil {
			best = d
			continue
		}
		if d.Document.Explicit && !best.Document.Explicit {
			best = d
			continue
		}
		if d.Specificity > best.Specificity {
			best = d
		}
	}
	return best
}
