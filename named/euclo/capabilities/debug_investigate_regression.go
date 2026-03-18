package capabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/agents/blackboard"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type investigateRegressionCapability struct {
	env agentenv.AgentEnvironment
}

func (c *investigateRegressionCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:debug.investigate_regression",
		Name:          "Investigate Regression",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "debugging", "regression"},
		Annotations: map[string]any{
			"supported_profiles": []string{"reproduce_localize_patch"},
		},
	}
}

func (c *investigateRegressionCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindIntake, Required: true},
		},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindRegressionAnalysis,
			euclotypes.ArtifactKindReproduction,
			euclotypes.ArtifactKindRootCause,
			euclotypes.ArtifactKindExplore,
			euclotypes.ArtifactKindAnalyze,
			euclotypes.ArtifactKindEditIntent,
			euclotypes.ArtifactKindVerification,
		},
	}
}

func (c *investigateRegressionCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !looksLikeRegressionRequest(artifacts) {
		return euclotypes.EligibilityResult{
			Eligible: false,
			Reason:   "regression-specific capability requires regression-shaped intake",
		}
	}
	if !snapshot.HasExecuteTools && !snapshot.HasVerificationTools {
		return euclotypes.EligibilityResult{
			Eligible: false,
			Reason:   "execute or verification tools required for regression investigation",
		}
	}
	return euclotypes.EligibilityResult{
		Eligible: true,
		Reason:   "regression-shaped intake with execution evidence available",
	}
}

func (c *investigateRegressionCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	producerID := "euclo:debug.investigate_regression"
	if env.State == nil {
		env.State = core.NewContext()
	}

	instruction := capTaskInstruction(env.Task)
	env.State.Set("euclo.blackboard_seed_facts", map[string]any{
		"regression:symptom": instruction,
	})

	bbResult, err := ExecuteBlackboard(ctx, env, regressionKnowledgeSources(), 6, func(bb *blackboard.Blackboard) bool {
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
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:reproduce_localize_patch",
			},
		}
	}

	investigationArtifacts := regressionArtifactsFromBoard(bbResult.Board, producerID)
	mergeStateArtifactsToContext(env.State, investigationArtifacts)

	rlp := &reproduceLocalizePatchCapability{env: c.env}
	rlpResult := rlp.Execute(ctx, env)

	artifacts := append([]euclotypes.Artifact{}, investigationArtifacts...)
	artifacts = append(artifacts, rlpResult.Artifacts...)

	status := rlpResult.Status
	summary := "regression investigation completed"
	if rlpResult.Summary != "" {
		summary = "regression investigation completed; " + rlpResult.Summary
	}

	result := euclotypes.ExecutionResult{
		Status:    status,
		Summary:   summary,
		Artifacts: artifacts,
	}
	if rlpResult.FailureInfo != nil {
		result.FailureInfo = rlpResult.FailureInfo
	}
	if confidence := rootCauseConfidence(bbResult.Board); confidence >= 0.8 {
		result.RecoveryHint = &euclotypes.RecoveryHint{
			Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
			SuggestedCapability: "euclo:reproduce_localize_patch",
			Context: map[string]any{
				"root_cause_confidence":  confidence,
				"localization_prefilled": true,
			},
		}
	} else if rlpResult.RecoveryHint != nil {
		result.RecoveryHint = rlpResult.RecoveryHint
	}
	return result
}

func regressionKnowledgeSources() []blackboard.KnowledgeSource {
	return []blackboard.KnowledgeSource{
		NewAnalysisKnowledgeSource(
			"Diff Analyst",
			"not regression:suspect_changes exists",
			[]string{"cli_git", "file_read"},
			`You are the diff analyst for a regression investigation.
Goal: {{goal}}
Context: {{entries}}
Return JSON with:
- facts: [{"key":"regression:suspect_changes","value":[{"commit":"...","file":"...","function":"...","relevance_score":0.0,"change_description":"..."}]}]
- summary: short string`,
		),
		NewAnalysisKnowledgeSource(
			"Test-Change Correlator",
			"regression:suspect_changes exists",
			[]string{"file_read", "file_glob", "cli_go", "cli_python"},
			`You correlate failing tests to recent changes.
Goal: {{goal}}
Context: {{entries}}
Return JSON with:
- facts: [{"key":"regression:correlations","value":[{"change":"...","test":"...","result":"...","correlation_strength":0.0}]}]
- summary: short string`,
		),
		NewSynthesisKnowledgeSource(
			"Reproduction Specialist",
			"not regression:reproduction exists",
			[]string{"regression:suspect_changes", "regression:correlations", "regression:symptom"},
			`You reproduce and confirm the regression.
Inputs: {{input_entries}}
Return JSON with:
- facts: [{"key":"regression:reproduction","value":{"reproduced":true,"method":"...","error":"...","root_change":{"commit":"...","file":"...","function":"...","change_description":"..."},"confidence":0.0}}]
- summary: short string`,
		),
	}
}

func regressionArtifactsFromBoard(board *blackboard.Blackboard, producerID string) []euclotypes.Artifact {
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
	artifacts = append(artifacts, euclotypes.Artifact{
		ID:         "regression_analysis",
		Kind:       euclotypes.ArtifactKindRegressionAnalysis,
		Summary:    summarizePayload(regressionAnalysis),
		Payload:    regressionAnalysis,
		ProducerID: producerID,
		Status:     "produced",
	})

	if reproduction != nil {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "regression_reproduction",
			Kind:       euclotypes.ArtifactKindReproduction,
			Summary:    summarizePayload(reproduction),
			Payload:    reproduction,
			ProducerID: producerID,
			Status:     "produced",
		})
	}

	if rootCause := extractRegressionRootCause(reproduction); rootCause != nil {
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "regression_root_cause",
			Kind:       euclotypes.ArtifactKindRootCause,
			Summary:    summarizePayload(rootCause),
			Payload:    rootCause,
			ProducerID: producerID,
			Status:     "produced",
		})
		// Pre-fill analyze state so the follow-on debug profile starts localized.
		artifacts = append(artifacts, euclotypes.Artifact{
			ID:         "regression_localization",
			Kind:       euclotypes.ArtifactKindAnalyze,
			Summary:    summarizePayload(rootCause),
			Payload:    map[string]any{"root_cause": rootCause, "regression_analysis": regressionAnalysis},
			ProducerID: producerID,
			Status:     "produced",
		})
	}

	return artifacts
}

func mergeStateArtifactsToContext(state *core.Context, artifacts []euclotypes.Artifact) {
	if state == nil || len(artifacts) == 0 {
		return
	}
	existing := euclotypes.ArtifactStateFromContext(state).All()
	merged := append(existing, artifacts...)
	state.Set("euclo.artifacts", merged)
	for _, artifact := range artifacts {
		if key := euclotypes.StateKeyForArtifactKind(artifact.Kind); key != "" && artifact.Payload != nil {
			state.Set(key, artifact.Payload)
		}
	}
}

func looksLikeRegressionRequest(artifacts euclotypes.ArtifactState) bool {
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	if text == "" {
		return false
	}
	for _, pattern := range []string{
		"investigate regression",
		"regression",
		"what changed",
		"used to work",
		"recent changes",
		"stopped working",
		"no longer",
	} {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func instructionFromArtifacts(artifacts euclotypes.ArtifactState) string {
	for _, artifact := range artifacts.OfKind(euclotypes.ArtifactKindIntake) {
		if instruction := extractInstruction(artifact.Payload); instruction != "" {
			return instruction
		}
	}
	return ""
}

func extractInstruction(payload any) string {
	switch typed := payload.(type) {
	case map[string]any:
		if value, ok := typed["instruction"].(string); ok {
			return strings.TrimSpace(value)
		}
	case string:
		return strings.TrimSpace(typed)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return strings.TrimSpace(fmt.Sprint(payload))
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err == nil {
		if value, ok := decoded["instruction"].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return strings.TrimSpace(string(data))
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

func rootCauseConfidence(board *blackboard.Blackboard) float64 {
	payload, ok := boardEntryValue(board, "regression:reproduction")
	if !ok {
		return 0
	}
	record, ok := payload.(map[string]any)
	if !ok {
		return 0
	}
	confidence, _ := record["confidence"].(float64)
	return confidence
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
	return map[string]any{
		"from_commit": stringField(first, "commit"),
		"to_commit":   stringField(last, "commit"),
	}
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

func defaultValue(value any, fallback any) any {
	if value == nil {
		return fallback
	}
	return value
}
