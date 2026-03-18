package capabilities

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type designAlternativesCapability struct {
	env agentenv.AgentEnvironment
}

func (c *designAlternativesCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:design.alternatives",
		Name:          "Design Alternatives",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "planning", "design"},
		Annotations: map[string]any{
			"supported_profiles": []string{"plan_stage_execute", "edit_verify_repair"},
		},
	}
}

func (c *designAlternativesCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindIntake, Required: true},
		},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindPlanCandidates,
			euclotypes.ArtifactKindPlan,
		},
	}
}

func (c *designAlternativesCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasReadTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "read tools required for design alternatives"}
	}
	if !looksLikeAlternativesRequest(artifacts) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "alternatives capability requires alternatives or comparison intent"}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "planning intent requests alternatives"}
}

func (c *designAlternativesCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	producerID := "euclo:design.alternatives"
	plannerCap := &plannerPlanCapability{env: c.env}

	var candidates []map[string]any
	for idx, prompt := range alternativePrompts() {
		result, err := plannerCap.runPlanner(ctx, env, prompt)
		if err != nil || result == nil || !result.Success {
			return euclotypes.ExecutionResult{
				Status:  euclotypes.ExecutionStatusFailed,
				Summary: "failed generating plan alternatives",
				FailureInfo: &euclotypes.CapabilityFailure{
					Code:         "plan_alternatives_failed",
					Message:      errMsg(err, result),
					Recoverable:  true,
					FailedPhase:  "generate_alternatives",
					ParadigmUsed: "planner",
				},
				RecoveryHint: &euclotypes.RecoveryHint{
					Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
					SuggestedCapability: "euclo:planner.plan",
				},
			}
		}
		candidate := buildPlanCandidate(idx+1, prompt, result)
		candidates = append(candidates, candidate)
	}

	comparison := comparePlanCandidates(candidates)
	selectedID, selectionReason := selectBestPlanCandidate(candidates)
	selectedPlan := selectedCandidatePlan(candidates, selectedID)

	planCandidatesPayload := map[string]any{
		"candidates":       candidates,
		"comparison":       comparison,
		"selected_id":      selectedID,
		"selection_reason": selectionReason,
	}

	return euclotypes.ExecutionResult{
		Status:  euclotypes.ExecutionStatusCompleted,
		Summary: "generated and compared plan alternatives",
		Artifacts: []euclotypes.Artifact{
			{
				ID:         "plan_candidates",
				Kind:       euclotypes.ArtifactKindPlanCandidates,
				Summary:    summarizePayload(planCandidatesPayload),
				Payload:    planCandidatesPayload,
				ProducerID: producerID,
				Status:     "produced",
			},
			{
				ID:         "selected_plan",
				Kind:       euclotypes.ArtifactKindPlan,
				Summary:    firstNonEmpty(stringValue(selectedPlan["summary"]), "selected plan"),
				Payload:    selectedPlan["plan"],
				ProducerID: producerID,
				Status:     "produced",
				Metadata: map[string]any{
					"selected_candidate_id": selectedID,
					"selection_reason":      selectionReason,
				},
			},
		},
	}
}

func alternativePrompts() []string {
	return []string{
		"Minimize files changed. Prefer extending existing patterns.",
		"Prioritize correctness and testability over minimal diff.",
		"Find the simplest possible approach, even if unconventional.",
	}
}

func buildPlanCandidate(index int, prompt string, result *core.Result) map[string]any {
	planPayload := map[string]any{}
	if result != nil && result.Data != nil {
		for key, value := range result.Data {
			planPayload[key] = value
		}
	}
	properties := derivePlanProperties(planPayload)
	return map[string]any{
		"id":         fmt.Sprintf("candidate_%d", index),
		"summary":    firstNonEmpty(resultSummary(result), fmt.Sprintf("candidate %d", index)),
		"prompt":     prompt,
		"plan":       planPayload,
		"properties": properties,
	}
}

func derivePlanProperties(plan map[string]any) map[string]any {
	steps := collectionCount(plan["steps"])
	files := uniqueStringsFromAny(plan["files"])
	if len(files) == 0 {
		files = filesFromSteps(plan["steps"])
	}
	riskLevel := "low"
	switch {
	case len(files) > 5 || steps > 5:
		riskLevel = "high"
	case len(files) > 2 || steps > 3:
		riskLevel = "medium"
	}
	return map[string]any{
		"files_touched": len(files),
		"step_count":    steps,
		"risk_level":    riskLevel,
		"testability":   max(1, 5-len(files)),
		"reversibility": max(1, 5-steps),
		"complexity":    complexityLabel(steps, len(files)),
	}
}

func comparePlanCandidates(candidates []map[string]any) map[string]any {
	dimensions := []string{"files_touched", "step_count", "risk_level", "testability", "reversibility", "complexity"}
	rows := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		properties, _ := candidate["properties"].(map[string]any)
		row := map[string]any{"id": candidate["id"]}
		for _, dim := range dimensions {
			row[dim] = properties[dim]
		}
		rows = append(rows, row)
	}
	return map[string]any{
		"dimensions": dimensions,
		"rows":       rows,
	}
}

func selectBestPlanCandidate(candidates []map[string]any) (string, string) {
	bestID := ""
	bestScore := 1 << 30
	bestReason := ""
	for _, candidate := range candidates {
		properties, _ := candidate["properties"].(map[string]any)
		filesTouched := intValue(properties["files_touched"])
		stepCount := intValue(properties["step_count"])
		testability := intValue(properties["testability"])
		score := filesTouched*3 + stepCount*2 - testability
		id := stringValue(candidate["id"])
		reason := fmt.Sprintf("selected %s for low scope (%d files) and manageable complexity (%d steps)", id, filesTouched, stepCount)
		if score < bestScore || (score == bestScore && id < bestID) {
			bestID = id
			bestScore = score
			bestReason = reason
		}
	}
	return bestID, bestReason
}

func selectedCandidatePlan(candidates []map[string]any, selectedID string) map[string]any {
	for _, candidate := range candidates {
		if stringValue(candidate["id"]) == selectedID {
			return candidate
		}
	}
	return map[string]any{}
}

func looksLikeAlternativesRequest(artifacts euclotypes.ArtifactState) bool {
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	for _, token := range []string{"alternatives", "compare approaches", "more options", "which approach", "options"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func collectionCount(raw any) int {
	switch typed := raw.(type) {
	case []any:
		return len(typed)
	case []map[string]any:
		return len(typed)
	default:
		return 0
	}
}

func filesFromSteps(raw any) []string {
	steps, ok := raw.([]any)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	var files []string
	for _, step := range steps {
		record, ok := step.(map[string]any)
		if !ok {
			continue
		}
		for _, file := range uniqueStringsFromAny(record["files"]) {
			if _, exists := seen[file]; exists {
				continue
			}
			seen[file] = struct{}{}
			files = append(files, file)
		}
	}
	sort.Strings(files)
	return files
}

func uniqueStringsFromAny(raw any) []string {
	seen := map[string]struct{}{}
	var out []string
	switch typed := raw.(type) {
	case []string:
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, exists := seen[item]; exists {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	case []any:
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func complexityLabel(stepCount, fileCount int) string {
	switch {
	case stepCount >= 6 || fileCount >= 6:
		return "high"
	case stepCount >= 3 || fileCount >= 3:
		return "medium"
	default:
		return "low"
	}
}

func intValue(raw any) int {
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}
