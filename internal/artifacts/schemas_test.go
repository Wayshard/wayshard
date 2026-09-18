package artifacts

import (
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
)

func TestPlanRequiresCriteriaAndValidation(t *testing.T) {
	_, err := ParseAndValidate(domain.ArtifactPlan, `{"kind":"plan","objective":"x"}`)
	if err == nil {
		t.Fatal("expected invalid plan")
	}
	p, err := ParseAndValidate(domain.ArtifactPlan, `{"kind":"plan","objective":"x","acceptanceCriteria":["done"],"validationPlan":["go test"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if p.(PlanArtifact).Objective != "x" {
		t.Fatal("objective")
	}
}

func TestExtractJSONFromProse(t *testing.T) {
	raw := "Here is the plan:\n```json\n{\"kind\":\"plan\",\"objective\":\"x\",\"acceptanceCriteria\":[\"a\"],\"validationPlan\":[\"t\"]}\n```\n"
	_, err := ParseAndValidate(domain.ArtifactPlan, raw)
	if err != nil {
		t.Fatal(err)
	}
}

func TestReviewerCannotOmitVerdict(t *testing.T) {
	_, err := ParseAndValidate(domain.ArtifactReview, `{"kind":"review","findings":[]}`)
	if err == nil {
		t.Fatal("expected missing verdict")
	}
}
