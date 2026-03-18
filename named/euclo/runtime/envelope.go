package runtime

import (
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// BuildExecutionEnvelope constructs an ExecutionEnvelope from agent runtime state.
func BuildExecutionEnvelope(
	task *core.Task,
	state *core.Context,
	mode euclotypes.ModeResolution,
	profile euclotypes.ExecutionProfileSelection,
	env agentenv.AgentEnvironment,
	workflowStore euclotypes.WorkflowArtifactWriter,
	workflowID, runID string,
	telemetry core.Telemetry,
) euclotypes.ExecutionEnvelope {
	return euclotypes.ExecutionEnvelope{
		Task:          task,
		Mode:          mode,
		Profile:       profile,
		Registry:      env.Registry,
		State:         state,
		Memory:        env.Memory,
		Environment:   env,
		WorkflowStore: workflowStore,
		WorkflowID:    workflowID,
		RunID:         runID,
		Telemetry:     telemetry,
	}
}

// ClassificationContextPayload converts a TaskClassification to a map for task context.
func ClassificationContextPayload(classification TaskClassification) map[string]any {
	return map[string]any{
		"intent_families":                   append([]string{}, classification.IntentFamilies...),
		"recommended_mode":                  classification.RecommendedMode,
		"mixed_intent":                      classification.MixedIntent,
		"edit_permitted":                    classification.EditPermitted,
		"requires_evidence_before_mutation": classification.RequiresEvidenceBeforeMutation,
		"requires_deterministic_stages":     classification.RequiresDeterministicStages,
		"scope":                             classification.Scope,
		"risk_level":                        classification.RiskLevel,
		"reason_codes":                      append([]string{}, classification.ReasonCodes...),
	}
}
