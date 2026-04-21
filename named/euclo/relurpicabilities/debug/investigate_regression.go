package debug

import (
	"context"
	"fmt"
	"strings"

	agentblackboard "codeburg.org/lexbit/relurpify/agents/blackboard"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	euclobb "codeburg.org/lexbit/relurpify/named/euclo/execution/blackboard"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
)

type investigateRegressionCapability struct {
	env agentenv.AgentEnvironment
}

func NewInvestigateRegressionCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &investigateRegressionCapability{env: env}
}

func (c *investigateRegressionCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:debug.investigate_regression",
		Name:          "Investigate Regression",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "debugging", "regression"},
		Annotations:   map[string]any{"supported_profiles": []string{"reproduce_localize_patch"}},
	}
}

func (c *investigateRegressionCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindRegressionAnalysis,
			euclotypes.ArtifactKindReproduction,
			euclotypes.ArtifactKindRootCause,
			euclotypes.ArtifactKindAnalyze,
		},
	}
}

func (c *investigateRegressionCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !looksLikeRegressionRequest(artifacts) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "regression-specific capability requires regression-shaped intake"}
	}
	if !snapshot.HasExecuteTools && !snapshot.HasVerificationTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "execute or verification tools required for regression investigation"}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "regression-shaped intake with execution evidence available"}
}

func (c *investigateRegressionCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	if env.State == nil {
		env.State = core.NewContext()
	}
	env.State.Set(euclostate.KeyBlackboardSeedFacts, map[string]any{"regression:symptom": taskInstruction(env.Task)})
	bbResult, err := euclobb.Execute(ctx, env, euclotypes.ExecutorSemanticContext{}, regressionKnowledgeSources(), 6, func(bb *agentblackboard.Blackboard) bool {
		payload, ok := boardEntryValue(bb, "regression:reproduction")
		if !ok {
			return false
		}
		record, ok := payload.(map[string]any)
		if !ok {
			return false
		}
		reproduced, _ := record["reproduced"].(bool)
		return reproduced
	})
	if err != nil {
		return euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "regression investigation failed",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "regression_investigation_failed",
				Message:      err.Error(),
				Recoverable:  true,
				FailedPhase:  "investigate_regression",
				ParadigmUsed: "blackboard",
			},
		}
	}
	artifacts := regressionArtifactsFromBoard(bbResult.Board, "euclo:debug.investigate_regression")
	mergeArtifacts(env.State, artifacts)
	return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "regression investigation completed", Artifacts: artifacts}
}

func regressionKnowledgeSources() []agentblackboard.KnowledgeSource {
	return []agentblackboard.KnowledgeSource{
		euclobb.NewAnalysisKnowledgeSource("Diff Analyst", "not regression:suspect_changes exists", []string{"cli_git", "file_read"},
			`You are the diff analyst for a regression investigation.
Goal: {{goal}}
Context: {{entries}}
Return JSON with:
- facts: [{"key":"regression:suspect_changes","value":[{"commit":"...","file":"...","function":"...","relevance_score":0.0,"change_description":"..."}]}]
- summary: short string`),
		euclobb.NewAnalysisKnowledgeSource("Test-Change Correlator", "regression:suspect_changes exists", []string{"file_read", "file_glob", "cli_go", "cli_python"},
			`You correlate failing tests to recent changes.
Goal: {{goal}}
Context: {{entries}}
Return JSON with:
- facts: [{"key":"regression:correlations","value":[{"change":"...","test":"...","result":"...","correlation_strength":0.0}]}]
- summary: short string`),
		euclobb.NewSynthesisKnowledgeSource("Reproduction Specialist", "not regression:reproduction exists", []string{"regression:suspect_changes", "regression:correlations", "regression:symptom"},
			`You reproduce and confirm the regression.
Inputs: {{input_entries}}
Return JSON with:
- facts: [{"key":"regression:reproduction","value":{"reproduced":true,"method":"...","error":"...","root_change":{"commit":"...","file":"...","function":"...","change_description":"..."},"confidence":0.0}}]
- summary: short string`),
	}
}

func regressionArtifactsFromBoard(board *agentblackboard.Blackboard, producerID string) []euclotypes.Artifact {
	if board == nil {
		return nil
	}
	var artifacts []euclotypes.Artifact
	suspects, _ := boardEntryValue(board, "regression:suspect_changes")
	correlations, _ := boardEntryValue(board, "regression:correlations")
	reproduction, _ := boardEntryValue(board, "regression:reproduction")
	regressionAnalysis := map[string]any{
		"suspect_changes":   defaultValue(suspects, []any{}),
		"correlations":      defaultValue(correlations, []any{}),
		"narrowing_summary": regressionNarrowingSummary(suspects, correlations, reproduction),
		"time_range":        regressionTimeRange(suspects),
	}
	artifacts = append(artifacts, euclotypes.Artifact{ID: "regression_analysis", Kind: euclotypes.ArtifactKindRegressionAnalysis, Summary: summarize(regressionAnalysis), Payload: regressionAnalysis, ProducerID: producerID, Status: "produced"})
	if reproduction != nil {
		artifacts = append(artifacts, euclotypes.Artifact{ID: "regression_reproduction", Kind: euclotypes.ArtifactKindReproduction, Summary: summarize(reproduction), Payload: reproduction, ProducerID: producerID, Status: "produced"})
	}
	if rootCause := extractRegressionRootCause(reproduction); rootCause != nil {
		artifacts = append(artifacts,
			euclotypes.Artifact{ID: "regression_root_cause", Kind: euclotypes.ArtifactKindRootCause, Summary: summarize(rootCause), Payload: rootCause, ProducerID: producerID, Status: "produced"},
			euclotypes.Artifact{ID: "regression_localization", Kind: euclotypes.ArtifactKindAnalyze, Summary: summarize(rootCause), Payload: map[string]any{"root_cause": rootCause, "regression_analysis": regressionAnalysis}, ProducerID: producerID, Status: "produced"},
		)
	}
	return artifacts
}

func looksLikeRegressionRequest(artifacts euclotypes.ArtifactState) bool {
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	if text == "" {
		return false
	}
	for _, pattern := range []string{"investigate regression", "regression", "what changed", "used to work", "recent changes", "stopped working", "no longer"} {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func extractRegressionRootCause(payload any) map[string]any {
	record, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	if rootChange, ok := record["root_change"].(map[string]any); ok {
		return rootChange
	}
	return nil
}

func regressionNarrowingSummary(suspects, correlations, reproduction any) string {
	suspectCount := len(asSlice(suspects))
	correlationCount := len(asSlice(correlations))
	record, _ := reproduction.(map[string]any)
	method, _ := record["method"].(string)
	return fmt.Sprintf("narrowed from %d suspect changes to %d correlations via %s", suspectCount, correlationCount, strings.TrimSpace(method))
}

func regressionTimeRange(suspects any) map[string]any {
	items := asSlice(suspects)
	if len(items) == 0 {
		return map[string]any{}
	}
	first, _ := items[0].(map[string]any)
	last, _ := items[len(items)-1].(map[string]any)
	return map[string]any{"from_commit": stringField(first, "commit"), "to_commit": stringField(last, "commit")}
}

func boardEntryValue(bb *agentblackboard.Blackboard, entry string) (any, bool) {
	if bb == nil {
		return nil, false
	}
	entry = strings.TrimSpace(entry)
	for i := len(bb.Facts) - 1; i >= 0; i-- {
		if bb.Facts[i].Key == entry {
			return decodeMaybeJSON(bb.Facts[i].Value), true
		}
	}
	for i := len(bb.Artifacts) - 1; i >= 0; i-- {
		if bb.Artifacts[i].Kind == entry {
			return decodeMaybeJSON(bb.Artifacts[i].Content), true
		}
	}
	return nil, false
}

func asSlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func stringField(record map[string]any, field string) string {
	if record == nil {
		return ""
	}
	value, _ := record[field].(string)
	return strings.TrimSpace(value)
}
