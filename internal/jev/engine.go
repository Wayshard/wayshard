// Package jev implements the DecisionEngine abstraction over TypeSafe Jev/System One.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Wayshard/wayshard/internal/crypto"
)

const QuestionSetVersion = "wayshard-assess-v1"
const PolicyVersion = "route-policy-v1"

// DecisionEngine isolates Jev from the orchestrator.
type DecisionEngine interface {
	Assess(ctx context.Context, state any, questions map[string]Question) (*Assessment, error)
	Available(ctx context.Context) bool
}

type Question struct {
	Type         string         `json:"type"` // noul | choice | score
	Instructions string         `json:"instructions"`
	Criteria     map[string]any `json:"criteria,omitempty"`
}

type Assessment struct {
	Model       string                     `json:"model"`
	Answers     map[string]json.RawMessage `json:"answers"`
	Usage       Usage                      `json:"usage"`
	InputHash   string                     `json:"inputHash"`
	QuestionSet string                     `json:"questionSet"`
	Degraded    bool                       `json:"degraded"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type HTTPEngine struct {
	BaseURL    string
	APIKey     string
	Model      string
	Client     *http.Client
	MaxRetries int
}

func NewHTTP(baseURL, apiKey, model string) *HTTPEngine {
	if baseURL == "" {
		baseURL = "https://api.typesafe.ai"
	}
	if model == "" {
		model = "jev-latest"
	}
	return &HTTPEngine{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		Model:      model,
		Client:     &http.Client{Timeout: 15 * time.Second},
		MaxRetries: 3,
	}
}

func (e *HTTPEngine) Available(ctx context.Context) bool {
	return e.APIKey != ""
}

type requestBody struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

func (e *HTTPEngine) Assess(ctx context.Context, state any, questions map[string]Question) (*Assessment, error) {
	if e.APIKey == "" {
		return nil, fmt.Errorf("jev api key not configured")
	}
	body, err := json.Marshal(requestBody{State: state, Model: e.Model, Questions: questions})
	if err != nil {
		return nil, err
	}
	var last error
	backoff := 200 * time.Millisecond
	for attempt := 0; attempt <= e.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/v1/systemone", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := e.Client.Do(req)
		if err != nil {
			last = err
		} else {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if resp.StatusCode == 429 || resp.StatusCode == 529 {
				last = fmt.Errorf("jev overloaded: %d", resp.StatusCode)
			} else if resp.StatusCode >= 400 {
				return nil, fmt.Errorf("jev http %d: %s", resp.StatusCode, b)
			} else {
				var out Assessment
				if err := json.Unmarshal(b, &out); err != nil {
					return nil, err
				}
				out.InputHash = crypto.HashSHA256(body)
				out.QuestionSet = QuestionSetVersion
				return &out, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
		}
	}
	return nil, last
}

// DeterministicEngine is used when Jev is unavailable or in tests.
type DeterministicEngine struct{}

func (DeterministicEngine) Available(context.Context) bool { return false }

func (DeterministicEngine) Assess(ctx context.Context, state any, questions map[string]Question) (*Assessment, error) {
	_ = ctx
	answers := map[string]json.RawMessage{}
	for id, q := range questions {
		switch q.Type {
		case "noul":
			answers[id] = json.RawMessage(`{"type":"noul","noul":0.55}`)
		case "choice":
			// pick first criterion key if any
			choice := "unknown"
			for k := range q.Criteria {
				choice = k
				break
			}
			b, _ := json.Marshal(map[string]any{"type": "choice", "choice": choice, "probabilities": map[string]float64{choice: 1}, "confidence": 0.3})
			answers[id] = b
		case "score":
			answers[id] = json.RawMessage(`{"type":"score","score":1,"legend":{"0":"low","1":"med","2":"high"},"probabilities":{"1":1},"confidence":0.3}`)
		}
	}
	st, _ := json.Marshal(state)
	return &Assessment{
		Model:       "deterministic-fallback",
		Answers:     answers,
		InputHash:   crypto.HashSHA256(st),
		QuestionSet: QuestionSetVersion,
		Degraded:    true,
	}, nil
}

func DefaultQuestions() map[string]Question {
	return map[string]Question{
		"task_type": {
			Type:         "choice",
			Instructions: "What kind of coding task is this?",
			Criteria: map[string]any{
				"implement": "source-changing implementation",
				"fix":       "bug fix",
				"refactor":  "refactor without behavior change",
				"research":  "research or brainstorming, no source change required",
				"docs":      "documentation only",
				"explore":   "needs investigation before a plan",
			},
		},
		"scope": {
			Type:         "score",
			Instructions: "How large is the likely change?",
			Criteria:     map[string]any{"levels": []string{"tiny", "local", "multi-file", "cross-cutting"}},
		},
		"risk": {
			Type:         "score",
			Instructions: "How risky is this change if wrong?",
			Criteria:     map[string]any{"levels": []string{"low", "moderate", "high", "critical"}},
		},
		"ambiguity": {
			Type:         "noul",
			Instructions: "Is the request too ambiguous to plan without exploration?",
			Criteria:     map[string]any{"true": "needs explore", "false": "planable now"},
		},
		"needs_plan": {
			Type:         "noul",
			Instructions: "Should a planning stage run before execution?",
		},
		"needs_review": {
			Type:         "noul",
			Instructions: "Should a semantic review stage run after validation?",
		},
		"artifact_only": {
			Type:         "noul",
			Instructions: "Can this complete without integrating source changes?",
		},
	}
}

func Noul(a *Assessment, key string) float64 {
	raw, ok := a.Answers[key]
	if !ok {
		return 0.5
	}
	var m struct {
		Noul float64 `json:"noul"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.Noul
}

func Choice(a *Assessment, key string) string {
	raw, ok := a.Answers[key]
	if !ok {
		return ""
	}
	var m struct {
		Choice string `json:"choice"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.Choice
}

func Score(a *Assessment, key string) float64 {
	raw, ok := a.Answers[key]
	if !ok {
		return 0
	}
	var m struct {
		Score float64 `json:"score"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.Score
}
