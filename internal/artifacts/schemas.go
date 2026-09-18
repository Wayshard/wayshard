// Package artifacts defines durable stage output contracts and server-side validation.
package artifacts

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wayshard/wayshard/internal/domain"
)

const SchemaVersion = 1

type PlanArtifact struct {
	Kind               string   `json:"kind"`
	Objective          string   `json:"objective"`
	Constraints        []string `json:"constraints"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	ExpectedPaths      []string `json:"expectedPaths"`
	ValidationPlan     []string `json:"validationPlan"`
	Risks              []string `json:"risks"`
	Assumptions        []string `json:"assumptions"`
	ArtifactOnly       bool     `json:"artifactOnly"`
}

type TaskContract = PlanArtifact

type ImplementationReport struct {
	Kind               string   `json:"kind"`
	Summary            string   `json:"summary"`
	FilesChanged       []string `json:"filesChanged"`
	Deviations         []string `json:"deviations"`
	ExpectedValidation []string `json:"expectedValidation"`
}

type CriterionEvidence struct {
	ID       string `json:"id"`
	Status   string `json:"status"` // pass|fail|not_verified
	Evidence string `json:"evidence"`
}

type ReviewFinding struct {
	Severity    string `json:"severity"` // blocking|major|minor|info
	Path        string `json:"path"`
	Explanation string `json:"explanation"`
	RequiredFix string `json:"requiredFix"`
}

type ReviewArtifact struct {
	Kind     string              `json:"kind"`
	Verdict  string              `json:"verdict"` // pass|fail|insufficient
	Criteria []CriterionEvidence `json:"criteria"`
	Findings []ReviewFinding     `json:"findings"`
}

type ValidationCheck struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Command     string `json:"command"`
	Dir         string `json:"dir"`
	Required    bool   `json:"required"`
	Status      string `json:"status"`
	Baseline    string `json:"baseline,omitempty"`
	ExitCode    int    `json:"exitCode"`
	DurationMS  int64  `json:"durationMs"`
	Summary     string `json:"summary"`
	LogHash     string `json:"logHash"`
	Fingerprint string `json:"fingerprint"`
}

type ValidationArtifact struct {
	Kind             string            `json:"kind"`
	Checks           []ValidationCheck `json:"checks"`
	BlockingFailures []string          `json:"blockingFailures"`
	Warnings         []string          `json:"warnings"`
	Unverified       []string          `json:"unverified"`
	BaselineCompared bool              `json:"baselineCompared"`
}

type InvestigationArtifact struct {
	Kind     string   `json:"kind"`
	Question string   `json:"question"`
	Findings []string `json:"findings"`
	OpenQs   []string `json:"openQuestions"`
}

type ResearchArtifact struct {
	Kind    string   `json:"kind"`
	Topic   string   `json:"topic"`
	Notes   []string `json:"notes"`
	Sources []string `json:"sources"`
}

type FailureArtifact struct {
	Kind    string `json:"kind"`
	Class   string `json:"class"`
	Message string `json:"message"`
	Retry   bool   `json:"retry"`
}

type DiffSummary struct {
	Kind        string   `json:"kind"`
	AgentFiles  []string `json:"agentFiles"`
	Preexisting []string `json:"preexisting"`
	Conflicted  []string `json:"conflicted"`
}

type RecoveryRecord struct {
	Kind       string `json:"kind"`
	AttemptID  string `json:"attemptId"`
	Checkpoint string `json:"checkpoint"`
	Reason     string `json:"reason"`
}

func ParseAndValidate(kind domain.ArtifactKind, raw string) (any, error) {
	raw = extractJSON(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty artifact")
	}
	switch kind {
	case domain.ArtifactPlan, domain.ArtifactTaskContract:
		var p PlanArtifact
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return nil, err
		}
		p.Kind = "plan"
		if strings.TrimSpace(p.Objective) == "" {
			return nil, fmt.Errorf("plan missing objective")
		}
		if len(p.AcceptanceCriteria) == 0 {
			return nil, fmt.Errorf("plan missing acceptance criteria")
		}
		if len(p.ValidationPlan) == 0 {
			return nil, fmt.Errorf("plan missing validation plan")
		}
		return p, nil
	case domain.ArtifactImplementation:
		var r ImplementationReport
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		if strings.TrimSpace(r.Summary) == "" {
			return nil, fmt.Errorf("implementation report missing summary")
		}
		return r, nil
	case domain.ArtifactReview:
		var r ReviewArtifact
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		if r.Verdict == "" {
			return nil, fmt.Errorf("review missing verdict")
		}
		return r, nil
	case domain.ArtifactValidation:
		var v ValidationArtifact
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		return v, nil
	case domain.ArtifactInvestigation:
		var v InvestigationArtifact
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		return v, nil
	case domain.ArtifactResearch:
		var v ResearchArtifact
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		return v, nil
	case domain.ArtifactFailure:
		var v FailureArtifact
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		return v, nil
	case domain.ArtifactDiffSummary:
		var v DiffSummary
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		return v, nil
	case domain.ArtifactRecovery:
		var v RecoveryRecord
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		return v, nil
	default:
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, err
		}
		return m, nil
	}
}

func Marshal(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "{") {
		return s
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

func KindForStage(k domain.StageKind) domain.ArtifactKind {
	switch k {
	case domain.StagePlan, domain.StageReplan:
		return domain.ArtifactPlan
	case domain.StageExplore:
		return domain.ArtifactInvestigation
	case domain.StageExecute, domain.StageRepair:
		return domain.ArtifactImplementation
	case domain.StageValidate:
		return domain.ArtifactValidation
	case domain.StageReview:
		return domain.ArtifactReview
	default:
		return domain.ArtifactFailure
	}
}
