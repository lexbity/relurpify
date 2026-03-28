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

// UnitOfWorkContextPayload converts a UnitOfWork to a compact task-context map.
func UnitOfWorkContextPayload(uow UnitOfWork) map[string]any {
	return map[string]any{
		"id":                  uow.ID,
		"workflow_id":         uow.WorkflowID,
		"run_id":              uow.RunID,
		"execution_id":        uow.ExecutionID,
		"mode_id":             uow.ModeID,
		"objective_kind":      uow.ObjectiveKind,
		"behavior_family":     uow.BehaviorFamily,
		"context_strategy_id": uow.ContextStrategyID,
		"semantic_inputs": map[string]any{
			"pattern_refs":              append([]string{}, uow.SemanticInputs.PatternRefs...),
			"tension_refs":              append([]string{}, uow.SemanticInputs.TensionRefs...),
			"prospective_refs":          append([]string{}, uow.SemanticInputs.ProspectiveRefs...),
			"convergence_refs":          append([]string{}, uow.SemanticInputs.ConvergenceRefs...),
			"learning_interaction_refs": append([]string{}, uow.SemanticInputs.LearningInteractionRefs...),
		},
		"executor":           uow.ExecutorDescriptor,
		"resolved_policy":    uow.ResolvedPolicy,
		"result_class":       uow.ResultClass,
		"status":             uow.Status,
		"deferred_issue_ids": append([]string{}, uow.DeferredIssueIDs...),
	}
}
