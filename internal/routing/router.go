// Package routing selects harness/model routes using deterministic filters then optional Jev fit.
package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/jev"
)

type Candidate struct {
	Harness      domain.HarnessInstallation
	ModelID      string
	Effort       string
	DynamicModel bool
}

type Config struct {
	Profile          domain.RoutingProfile
	AllowedHarnesses []string
	AllowedModels    []string
	ForbiddenModels  []string
	MaxCostMicros    int64
	ForceHarness     string
	ForceModel       string
	Pool             []string // automatic routing pool of model ids
}

type Decision struct {
	Candidate Candidate
	Fallbacks []Candidate
	Reason    string
	Degraded  bool
	Blocked   domain.BlockedReason
	Detail    string
}

type Router struct {
	Engine jev.DecisionEngine
}

func (r *Router) Route(ctx context.Context, stage domain.StageKind, cfg Config, cands []Candidate, assess *jev.Assessment) Decision {
	viable := hardFilter(stage, cfg, cands)
	if cfg.ForceHarness != "" || cfg.ForceModel != "" {
		forced := filterForced(viable, cfg)
		if len(forced) == 0 {
			return Decision{Blocked: domain.BlockedNoViableRoute, Detail: "forced harness/model is impossible or forbidden"}
		}
		viable = forced
	}
	if len(viable) == 0 {
		return Decision{Blocked: domain.BlockedNoViableRoute, Detail: "no harness/model satisfies stage requirements"}
	}
	ranked := prefer(cfg, viable)
	if r.Engine != nil && assess != nil && r.Engine.Available(ctx) && len(ranked) > 1 {
		ranked = r.semanticOrder(ctx, stage, ranked, assess)
	}
	primary := ranked[0]
	var fb []Candidate
	if len(ranked) > 1 {
		fb = ranked[1:]
		if len(fb) > 3 {
			fb = fb[:3]
		}
	}
	degraded := assess != nil && assess.Degraded
	reason := "hard-filter then policy"
	if degraded {
		reason = "deterministic fallback (jev unavailable)"
	}
	return Decision{Candidate: primary, Fallbacks: fb, Reason: reason, Degraded: degraded}
}

func hardFilter(stage domain.StageKind, cfg Config, cands []Candidate) []Candidate {
	var out []Candidate
	for _, c := range cands {
		if c.Harness.Health == domain.HarnessUnavailable || c.Harness.Health == domain.HarnessIncompatible {
			continue
		}
		if c.Harness.Compatibility == domain.CompatIncompatible {
			continue
		}
		if c.Harness.Health == domain.HarnessUnauth {
			continue
		}
		if len(cfg.AllowedHarnesses) > 0 && !contains(cfg.AllowedHarnesses, c.Harness.ID) && !contains(cfg.AllowedHarnesses, c.Harness.DisplayName) {
			continue
		}
		if c.ModelID != "" && contains(cfg.ForbiddenModels, c.ModelID) {
			continue
		}
		if len(cfg.Pool) > 0 && c.ModelID != "" && !contains(cfg.Pool, c.ModelID) && cfg.ForceModel == "" {
			continue
		}
		if stage.WritesWorkspace() && c.Harness.Health != domain.HarnessReady && c.Harness.Health != domain.HarnessDegraded {
			continue
		}
		out = append(out, c)
	}
	return out
}

func filterForced(cands []Candidate, cfg Config) []Candidate {
	var out []Candidate
	for _, c := range cands {
		if cfg.ForceHarness != "" && c.Harness.ID != cfg.ForceHarness && !strings.EqualFold(c.Harness.DisplayName, cfg.ForceHarness) && c.Harness.Executable != cfg.ForceHarness {
			continue
		}
		if cfg.ForceModel != "" && c.ModelID != cfg.ForceModel {
			continue
		}
		out = append(out, c)
	}
	return out
}

func prefer(cfg Config, cands []Candidate) []Candidate {
	score := func(c Candidate) int {
		n := 0
		if c.Harness.Health == domain.HarnessReady {
			n += 10
		}
		switch cfg.Profile {
		case domain.ProfileQuality:
			n += 2
		case domain.ProfileSpeed:
			if strings.Contains(strings.ToLower(c.ModelID), "fast") || strings.Contains(strings.ToLower(c.ModelID), "mini") {
				n += 3
			}
		case domain.ProfileEconomy:
			if strings.Contains(strings.ToLower(c.ModelID), "mini") || strings.Contains(strings.ToLower(c.ModelID), "haiku") || strings.Contains(strings.ToLower(c.ModelID), "flash") {
				n += 3
			}
		}
		return n
	}
	out := append([]Candidate(nil), cands...)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if score(out[j]) > score(out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func (r *Router) semanticOrder(ctx context.Context, stage domain.StageKind, cands []Candidate, assess *jev.Assessment) []Candidate {
	// Jev may only compare among already viable candidates.
	ids := map[string]string{}
	crit := map[string]any{}
	for i, c := range cands {
		key := fmt.Sprintf("c%d", i)
		ids[key] = c.Harness.ID + "/" + c.ModelID
		crit[key] = c.Harness.DisplayName + " " + c.ModelID
	}
	a, err := r.Engine.Assess(ctx, map[string]any{
		"stage":       stage,
		"candidates":  ids,
		"taskSignals": assess.Answers,
	}, map[string]jev.Question{
		"fit": {Type: "choice", Instructions: "Which already-viable route is the best semantic fit?", Criteria: crit},
	})
	if err != nil {
		return cands
	}
	choice := jev.Choice(a, "fit")
	if choice == "" {
		return cands
	}
	var picked Candidate
	found := false
	var rest []Candidate
	for i, c := range cands {
		if fmt.Sprintf("c%d", i) == choice || c.Harness.ID+"/"+c.ModelID == choice {
			picked = c
			found = true
			continue
		}
		rest = append(rest, c)
	}
	if !found {
		return cands
	}
	return append([]Candidate{picked}, rest...)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func FallbacksJSON(fb []Candidate) string {
	type row struct {
		Harness string `json:"harnessId"`
		Model   string `json:"modelId"`
	}
	var rows []row
	for _, c := range fb {
		rows = append(rows, row{c.Harness.ID, c.ModelID})
	}
	b, _ := json.Marshal(rows)
	return string(b)
}
