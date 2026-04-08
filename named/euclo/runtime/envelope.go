package runtime

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// BuildExecutionEnvelope constructs an ExecutionEnvelope from agent runtime state.
func BuildExecutionEnvelope(
	task *core.Task,
	state *core.Context,
	mode euclotypes.ModeResolution,
	profile euclotypes.ExecutionProfileSelection,
	env agentenv.AgentEnvironment,
	planStore frameworkplan.PlanStore,
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
		PlanStore:     planStore,
		PlanID:        firstNonEmpty(envelopeStateString(state, "euclo.plan_id"), envelopeStateStructString(state, "euclo.active_plan_version", "PlanID"), envelopeStateStructString(state, "euclo.active_plan_version", "ID")),
		PlanVersion:   firstNonZero(envelopeStateInt(state, "euclo.plan_version"), envelopeStateStructInt(state, "euclo.active_plan_version", "Version")),
		RootChunkIDs:  envelopeStateStringSlice(state, "euclo.bkc.root_chunk_ids"),
		ChunkStateRef: envelopeStateString(state, "euclo.bkc.checkpoint_ref"),
		WorkflowStore: workflowStore,
		WorkflowID:    workflowID,
		RunID:         runID,
		Telemetry:     telemetry,
	}
}

func envelopeStateString(state *core.Context, key string) string {
	if state == nil {
		return ""
	}
	return state.GetString(key)
}

func envelopeStateInt(state *core.Context, key string) int {
	if state == nil {
		return 0
	}
	raw, ok := state.Get(key)
	if !ok || raw == nil {
		return 0
	}
	switch typed := raw.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func envelopeStateStringSlice(state *core.Context, key string) []string {
	if state == nil {
		return nil
	}
	raw, ok := state.Get(key)
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func envelopeStateStructString(state *core.Context, key, field string) string {
	raw, ok := envelopeStateValue(state, key)
	if !ok {
		return ""
	}
	value := reflect.ValueOf(raw)
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return ""
	}
	f := value.FieldByName(field)
	if !f.IsValid() || !f.CanInterface() {
		return ""
	}
	if text, ok := f.Interface().(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func envelopeStateStructInt(state *core.Context, key, field string) int {
	raw, ok := envelopeStateValue(state, key)
	if !ok {
		return 0
	}
	value := reflect.ValueOf(raw)
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return 0
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0
	}
	f := value.FieldByName(field)
	if !f.IsValid() || !f.CanInterface() {
		return 0
	}
	switch typed := f.Interface().(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func envelopeStateValue(state *core.Context, key string) (any, bool) {
	if state == nil {
		return nil, false
	}
	raw, ok := state.Get(key)
	return raw, ok && raw != nil
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
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
		"id":                                 uow.ID,
		"workflow_id":                        uow.WorkflowID,
		"run_id":                             uow.RunID,
		"execution_id":                       uow.ExecutionID,
		"root_unit_of_work_id":               uow.RootID,
		"mode_id":                            uow.ModeID,
		"objective_kind":                     uow.ObjectiveKind,
		"behavior_family":                    uow.BehaviorFamily,
		"context_strategy_id":                uow.ContextStrategyID,
		"primary_relurpic_capability_id":     uow.PrimaryRelurpicCapabilityID,
		"supporting_relurpic_capability_ids": append([]string{}, uow.SupportingRelurpicCapabilityIDs...),
		"predecessor_unit_of_work_id":        uow.PredecessorUnitOfWorkID,
		"transition_reason":                  uow.TransitionReason,
		"transition_state":                   uow.TransitionState,
		"semantic_inputs": map[string]any{
			"pattern_refs":              append([]string{}, uow.SemanticInputs.PatternRefs...),
			"tension_refs":              append([]string{}, uow.SemanticInputs.TensionRefs...),
			"prospective_refs":          append([]string{}, uow.SemanticInputs.ProspectiveRefs...),
			"convergence_refs":          append([]string{}, uow.SemanticInputs.ConvergenceRefs...),
			"learning_interaction_refs": append([]string{}, uow.SemanticInputs.LearningInteractionRefs...),
			"pattern_proposals":         patternProposalPayload(uow.SemanticInputs.PatternProposals),
			"tension_clusters":          tensionClusterPayload(uow.SemanticInputs.TensionClusters),
			"coherence_suggestions":     coherenceSuggestionPayload(uow.SemanticInputs.CoherenceSuggestions),
			"prospective_pairings":      prospectivePairingPayload(uow.SemanticInputs.ProspectivePairings),
		},
		"executor":           uow.ExecutorDescriptor,
		"resolved_policy":    uow.ResolvedPolicy,
		"result_class":       uow.ResultClass,
		"status":             uow.Status,
		"deferred_issue_ids": append([]string{}, uow.DeferredIssueIDs...),
	}
}

func patternProposalPayload(input []PatternProposalSummary) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(input))
	for _, item := range input {
		out = append(out, map[string]any{
			"id":                   item.ProposalID,
			"title":                item.Title,
			"pattern_refs":         append([]string{}, item.PatternRefs...),
			"related_tension_refs": append([]string{}, item.RelatedTensionRefs...),
		})
	}
	return out
}

func tensionClusterPayload(input []TensionClusterSummary) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(input))
	for _, item := range input {
		out = append(out, map[string]any{
			"id":           item.ClusterID,
			"title":        item.Title,
			"severity":     item.Severity,
			"tension_refs": append([]string{}, item.TensionRefs...),
			"pattern_refs": append([]string{}, item.PatternRefs...),
		})
	}
	return out
}

func coherenceSuggestionPayload(input []CoherenceSuggestion) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(input))
	for _, item := range input {
		out = append(out, map[string]any{
			"id":               item.SuggestionID,
			"title":            item.Title,
			"suggested_action": item.SuggestedAction,
			"touched_symbols":  append([]string{}, item.TouchedSymbols...),
			"pattern_refs":     append([]string{}, item.PatternRefs...),
			"tension_refs":     append([]string{}, item.TensionRefs...),
		})
	}
	return out
}

func prospectivePairingPayload(input []ProspectivePairingSummary) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(input))
	for _, item := range input {
		out = append(out, map[string]any{
			"id":               item.PairingID,
			"title":            item.Title,
			"prospective_ref":  item.ProspectiveRef,
			"pattern_refs":     append([]string{}, item.PatternRefs...),
			"convergence_refs": append([]string{}, item.ConvergenceRefs...),
		})
	}
	return out
}
