// Package knowledge discovers and classifies repository-owned project knowledge.
package knowledge

import (
	"path"
	"strings"
)

// Kind is the semantic role of a knowledge document.
type Kind string

const (
	KindAgents       Kind = "agents"
	KindClaude       Kind = "claude"
	KindCursorRules  Kind = "cursor_rules"
	KindGemini       Kind = "gemini"
	KindCopilot      Kind = "copilot"
	KindSpec         Kind = "spec"
	KindDesign       Kind = "design"
	KindArchitecture Kind = "architecture"
	KindMemory       Kind = "memory"
	KindReadme       Kind = "readme"
	KindContributing Kind = "contributing"
	KindSecurity     Kind = "security"
	KindDocs         Kind = "docs"
	KindMarkdown     Kind = "markdown"
	KindCustom       Kind = "custom"
)

// Family is a native instruction ecosystem. Families keep their own scoping
// rules; there is no universal filename precedence across families.
type Family string

const (
	FamilyAgents     Family = "agents"
	FamilyClaude     Family = "claude"
	FamilyCursor     Family = "cursor"
	FamilyGemini     Family = "gemini"
	FamilyCopilot    Family = "copilot"
	FamilyWayshard   Family = "wayshard"
	FamilyGeneric    Family = "generic"
	FamilyDocs       Family = "docs"
	FamilyReferenced Family = "referenced"
	FamilyCustom     Family = "custom"
)

// AuthorityDomain is the concern a document owns. Conflicts are resolved by
// domain, not by filename ranking. Jev does not decide authority.
type AuthorityDomain string

const (
	DomainProduct        AuthorityDomain = "product"
	DomainArchitecture   AuthorityDomain = "architecture"
	DomainDesign         AuthorityDomain = "design"
	DomainImplementation AuthorityDomain = "implementation"
	DomainValidation     AuthorityDomain = "validation"
	DomainSecurity       AuthorityDomain = "security"
	DomainUsage          AuthorityDomain = "usage"
	DomainRationale      AuthorityDomain = "rationale"
	DomainUnknown        AuthorityDomain = "unknown"
)

const (
	SourceDiscovered = "discovered"
	SourceExplicit   = "explicit"
	SourceReferenced = "referenced"
)

const (
	StatusOK        = "ok"
	StatusTruncated = "truncated"
)

const (
	EdgeMarkdownLink = "markdown_link"
	EdgeEntryPoint   = "entry_point"
)

// SixCanonicalNames is the default root set for new Wayshard-designed projects.
// Existing repositories are never required to have these files.
var SixCanonicalNames = []string{
	"AGENTS.md",
	"DESIGN.md",
	"SPEC.md",
	"ARCHITECTURE.md",
	"MEMORY.md",
	"README.md",
}

// Document is a classified knowledge file. Repository files remain source of truth.
type Document struct {
	ID              string
	ProjectID       string
	Path            string
	Kind            Kind
	Family          Family
	Scope           string
	AuthorityDomain AuthorityDomain
	Source          string
	Hash            string
	Status          string
	Content         string
	Canonical       bool
	AlwaysApply     bool
	Globs           []string
	Explicit        bool
}

// Section is a heading-bounded region of a document.
type Section struct {
	ID         string
	DocumentID string
	Heading    string
	Ordinal    int
	Hash       string
	Body       string
}

// Edge is a reference from one document to a (possibly missing) path.
type Edge struct {
	ID             string
	FromDocumentID string
	FromPath       string
	ToPath         string
	Kind           string
	Broken         bool
	Cycle          bool
}

// Cycle is a closed walk over knowledge references.
type Cycle struct {
	Paths []string
}

func (k Kind) String() string { return string(k) }

func (f Family) String() string { return string(f) }

func (d AuthorityDomain) String() string { return string(d) }

// HarnessSpecific reports whether the family is a tool-owned instruction set
// that must not be auto-injected into other harnesses.
func (f Family) HarnessSpecific() bool {
	switch f {
	case FamilyClaude, FamilyCursor, FamilyGemini, FamilyCopilot:
		return true
	default:
		return false
	}
}

// IsCanonicalRootName reports whether base is one of the six default canonical filenames.
func IsCanonicalRootName(base string) bool {
	switch strings.ToLower(base) {
	case "agents.md", "design.md", "spec.md", "architecture.md", "memory.md", "readme.md":
		return true
	default:
		return false
	}
}

// IsMarkdown reports whether path looks like Markdown (including Cursor .mdc).
func IsMarkdown(rel string) bool {
	ext := strings.ToLower(path.Ext(rel))
	switch ext {
	case ".md", ".mdx", ".markdown", ".mdc":
		return true
	default:
		return false
	}
}

// IsSeedPath reports whether a repository-relative path is a known instruction
// or project-knowledge file that discovery should index without being linked.
func IsSeedPath(rel string) bool {
	rel = NormalizeRel(rel)
	if rel == "" || rel == "." {
		return false
	}
	base := strings.ToLower(path.Base(rel))
	dir := path.Dir(rel)
	atRoot := dir == "."

	switch base {
	case "agents.md", "claude.md", "gemini.md", "spec.md", "design.md", "architecture.md", "memory.md":
		return true
	case "readme.md", "contributing.md", "security.md":
		return atRoot
	case ".cursorrules":
		return true
	}
	if rel == ".github/copilot-instructions.md" {
		return true
	}
	if strings.HasPrefix(rel, ".cursor/rules/") && IsMarkdown(rel) {
		return true
	}
	if strings.HasPrefix(rel, ".github/instructions/") && IsMarkdown(rel) {
		return true
	}
	if strings.HasPrefix(rel, "docs/") && IsMarkdown(rel) {
		return true
	}
	return false
}

// ClassifyFilename infers kind/family/domain from a repository-relative path.
// Explicit declarations in the file body outrank this inference.
func ClassifyFilename(rel string) (kind Kind, family Family, domain AuthorityDomain, ok bool) {
	rel = NormalizeRel(rel)
	base := strings.ToLower(path.Base(rel))
	switch base {
	case "agents.md":
		return KindAgents, FamilyAgents, DomainImplementation, true
	case "claude.md":
		return KindClaude, FamilyClaude, DomainImplementation, true
	case "gemini.md":
		return KindGemini, FamilyGemini, DomainImplementation, true
	case "spec.md":
		return KindSpec, FamilyWayshard, DomainProduct, true
	case "design.md":
		return KindDesign, FamilyWayshard, DomainDesign, true
	case "architecture.md":
		return KindArchitecture, FamilyWayshard, DomainArchitecture, true
	case "memory.md":
		return KindMemory, FamilyWayshard, DomainRationale, true
	case "readme.md":
		return KindReadme, FamilyGeneric, DomainUsage, true
	case "contributing.md":
		return KindContributing, FamilyGeneric, DomainImplementation, true
	case "security.md":
		return KindSecurity, FamilyGeneric, DomainSecurity, true
	case ".cursorrules":
		return KindCursorRules, FamilyCursor, DomainImplementation, true
	}
	if rel == ".github/copilot-instructions.md" {
		return KindCopilot, FamilyCopilot, DomainImplementation, true
	}
	if strings.HasPrefix(rel, ".cursor/rules/") && IsMarkdown(rel) {
		return KindCursorRules, FamilyCursor, DomainImplementation, true
	}
	if strings.HasPrefix(rel, ".github/instructions/") && IsMarkdown(rel) {
		return KindCopilot, FamilyCopilot, DomainImplementation, true
	}
	if strings.HasPrefix(rel, "docs/") && IsMarkdown(rel) {
		return KindDocs, FamilyDocs, DomainUsage, true
	}
	if IsMarkdown(rel) {
		return KindMarkdown, FamilyReferenced, DomainUnknown, true
	}
	return "", "", "", false
}

func ParseKind(s string) Kind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "agents", "agents.md":
		return KindAgents
	case "claude", "claude.md":
		return KindClaude
	case "cursor", "cursor_rules", "cursorrules":
		return KindCursorRules
	case "gemini", "gemini.md":
		return KindGemini
	case "copilot":
		return KindCopilot
	case "spec", "specification":
		return KindSpec
	case "design":
		return KindDesign
	case "architecture", "arch":
		return KindArchitecture
	case "memory", "rationale":
		return KindMemory
	case "readme":
		return KindReadme
	case "contributing":
		return KindContributing
	case "security":
		return KindSecurity
	case "docs", "doc":
		return KindDocs
	case "custom":
		return KindCustom
	case "markdown", "md":
		return KindMarkdown
	default:
		return Kind("")
	}
}

func ParseFamily(s string) Family {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "agents":
		return FamilyAgents
	case "claude":
		return FamilyClaude
	case "cursor":
		return FamilyCursor
	case "gemini":
		return FamilyGemini
	case "copilot":
		return FamilyCopilot
	case "wayshard", "canonical":
		return FamilyWayshard
	case "generic":
		return FamilyGeneric
	case "docs":
		return FamilyDocs
	case "referenced":
		return FamilyReferenced
	case "custom":
		return FamilyCustom
	default:
		return Family("")
	}
}

func ParseDomain(s string) AuthorityDomain {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "product", "spec", "behavior":
		return DomainProduct
	case "architecture", "arch":
		return DomainArchitecture
	case "design", "ux":
		return DomainDesign
	case "implementation", "impl", "agents":
		return DomainImplementation
	case "validation", "test":
		return DomainValidation
	case "security":
		return DomainSecurity
	case "usage", "readme":
		return DomainUsage
	case "rationale", "memory":
		return DomainRationale
	default:
		return DomainUnknown
	}
}

// NormalizeRel returns a clean repository-relative slash path.
func NormalizeRel(rel string) string {
	rel = strings.TrimSpace(rel)
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = path.Clean(rel)
	rel = strings.TrimPrefix(rel, "./")
	if rel == "." || rel == "/" {
		return ""
	}
	return strings.TrimPrefix(rel, "/")
}

func scopeFor(rel string) string {
	rel = NormalizeRel(rel)
	dir := path.Dir(rel)
	if dir == "." || dir == "" {
		return "/"
	}
	return dir
}

func inScope(scope, filePath string) bool {
	filePath = NormalizeRel(filePath)
	scope = NormalizeRel(scope)
	if scope == "" || scope == "/" {
		return true
	}
	if filePath == scope {
		return true
	}
	return strings.HasPrefix(filePath, scope+"/")
}
