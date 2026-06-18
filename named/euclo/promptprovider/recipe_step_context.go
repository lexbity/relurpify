package promptprovider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

type thoughtrecipeStepContextProvider struct{}

// NewThoughtRecipeStepContextProvider constructs the Euclo thoughtrecipe step
// context provider.
func NewThoughtRecipeStepContextProvider() prompt.ContextProvider {
	return &thoughtrecipeStepContextProvider{}
}

func (p *thoughtrecipeStepContextProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	state := clarificationStateFromRuntime(ctx)
	if state == nil {
		return prompt.ContextChunk{Content: ""}
	}

	sv := surface.StateView{}
	if taskID := strings.TrimSpace(state.TaskID); taskID != "" {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Task ID: %s", taskID))
	}
	if sessionID := strings.TrimSpace(state.SessionID); sessionID != "" {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Session ID: %s", sessionID))
	}
	sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Clarification State Version: %d", state.StateVersion))
	if turnID := strings.TrimSpace(state.CurrentTurnID); turnID != "" {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Current Turn ID: %s", turnID))
	}
	if thoughtrecipeID := strings.TrimSpace(state.ActiveThoughtRecipeID); thoughtrecipeID != "" {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Active ThoughtRecipe ID: %s", thoughtrecipeID))
	}
	if checkpointID := strings.TrimSpace(state.LastCheckpointID); checkpointID != "" {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Last Checkpoint ID: %s", checkpointID))
	}
	if checkpointSeq := state.LastCheckpointSeq; checkpointSeq > 0 {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Last Checkpoint Seq: %d", checkpointSeq))
	}
	if question := firstNonEmpty(ctx.Variables["question"], ctx.Variables["instruction"]); question != "" {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Current Question: %s", question))
	}
	if promptID := strings.TrimSpace(ctx.Variables["prompt_id"]); promptID != "" {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Prompt ID: %s", promptID))
	}
	if query := runtimeString(ctx, "execution_step_context_stream_query"); query != "" {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Step Context Stream Query: %s", query))
	}
	if maxTokens, ok := runtimeInt(ctx, "execution_step_context_stream_max_tokens"); ok {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Step Context Stream Max Tokens: %d", maxTokens))
	}
	if mode := runtimeString(ctx, "execution_step_context_stream_mode"); mode != "" {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Step Context Stream Mode: %s", mode))
	}
	if values := runtimeStrings(ctx, "execution_step_context_inherit"); len(values) > 0 {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Step Context Inherit: "+strings.Join(values, ", "))
	}
	if values := runtimeStrings(ctx, "execution_step_context_capture"); len(values) > 0 {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Step Context Capture: "+strings.Join(values, ", "))
	}
	if evidence := intentEvidenceFromRuntime(ctx); evidence != nil {
		if action := strings.TrimSpace(evidence.ActionType); action != "" {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Evidence Action Type: %s", action))
		}
		if target := strings.TrimSpace(evidence.Target); target != "" {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Evidence Target: %s", target))
		}
		if scope := strings.TrimSpace(evidence.Scope); scope != "" {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Evidence Scope: %s", scope))
		}
		if risk := strings.TrimSpace(evidence.RiskLevel); risk != "" {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Evidence Risk Level: %s", risk))
		}
		if verb := strings.TrimSpace(evidence.ExpectedVerb); verb != "" {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Evidence Expected Verb: %s", verb))
		}
		if len(evidence.MissingFields) > 0 {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Evidence Missing Fields: "+strings.Join(evidence.MissingFields, ", "))
		}
		if len(evidence.ReasonCodes) > 0 {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Evidence Reason Codes: "+strings.Join(evidence.ReasonCodes, ", "))
		}
	}
	if interpretation := intentInterpretationFromRuntime(ctx); interpretation != nil {
		if action := strings.TrimSpace(interpretation.ActionType); action != "" {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Interpretation Action Type: %s", action))
		}
		if target := strings.TrimSpace(interpretation.Target); target != "" {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Interpretation Target: %s", target))
		}
		if scope := strings.TrimSpace(interpretation.Scope); scope != "" {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Interpretation Scope: %s", scope))
		}
		if risk := strings.TrimSpace(interpretation.RiskLevel); risk != "" {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Interpretation Risk Level: %s", risk))
		}
		if len(interpretation.MissingInfo) > 0 {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Interpretation Missing Info: "+strings.Join(interpretation.MissingInfo, ", "))
		}
		if rationale := strings.TrimSpace(interpretation.Rationale); rationale != "" {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Interpretation Rationale: %s", rationale))
		}
		if note := strings.TrimSpace(interpretation.ConfidenceNote); note != "" {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Interpretation Confidence Note: %s", note))
		}
		if len(interpretation.ReasonCodes) > 0 {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Interpretation Reason Codes: "+strings.Join(interpretation.ReasonCodes, ", "))
		}
	}
	if state.Ambiguity != nil {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Ambiguity Kind: %s", state.Ambiguity.Kind))
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Ambiguity Confidence: %.2f", state.Ambiguity.Confidence))
		if rationale := strings.TrimSpace(state.Ambiguity.Rationale); rationale != "" {
			sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, fmt.Sprintf("Ambiguity Rationale: %s", rationale))
		}
	}
	if anchors := anchorSummaries(state.GroundedAnchors); len(anchors) > 0 {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Grounded Anchors: "+strings.Join(anchors, ", "))
	}
	if entities := entitySummaries(state.ConfirmedEntities); len(entities) > 0 {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Confirmed Entities: "+strings.Join(entities, ", "))
	}
	if scopes := scopeSummaries(state.ConfirmedScopes); len(scopes) > 0 {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Confirmed Scopes: "+strings.Join(scopes, ", "))
	}
	if relationIntents := relationIntentSummaries(state.PendingRelationIntents); len(relationIntents) > 0 {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Pending Relation Intents: "+strings.Join(relationIntents, ", "))
	}
	if questions := questionSummaries(state.PendingQuestions); len(questions) > 0 {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Pending Questions: "+strings.Join(questions, ", "))
	}
	if projections := projectionSummaries(state.PendingProjection); len(projections) > 0 {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Pending Projection: "+strings.Join(projections, ", "))
	}
	if mutations := projectionRecordSummaries(state.AppliedMutations); len(mutations) > 0 {
		sv.ClarificationRuntimeLines = append(sv.ClarificationRuntimeLines, "Applied Mutations: "+strings.Join(mutations, ", "))
	}

	out := sv.RenderClarificationRuntime()
	if out == "" {
		return prompt.ContextChunk{Content: ""}
	}
	return prompt.ContextChunk{Content: out}
}

func (p *thoughtrecipeStepContextProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "euclo.thoughtrecipe_step_context",
		Description: "Provides clarification runtime context for thoughtrecipe step prompts",
		Paradigms:   []string{"euclo"},
		ReadsKeys: append(surface.PromptReadsKeys(),
			"execution_step_context_stream_query",
			"execution_step_context_stream_max_tokens",
			"execution_step_context_stream_mode",
			"execution_step_context_inherit",
			"execution_step_context_capture",
		),
	}
}

func intentEvidenceFromRuntime(ctx prompt.RuntimeContext) *intentcontext.IntentEvidence {
	if ctx.State != nil {
		if value, ok := ctx.State[intentcontext.IntentEvidenceKey]; ok && value != nil {
			if evidence, ok := value.(*intentcontext.IntentEvidence); ok {
				return evidence
			}
		}
	}
	if ctx.Envelope != nil {
		if evidence, ok := contextdata.GetTyped[*intentcontext.IntentEvidence](ctx.Envelope, intentcontext.IntentEvidenceKey); ok {
			return evidence
		}
	}
	return nil
}

func intentInterpretationFromRuntime(ctx prompt.RuntimeContext) *intentcontext.IntentInterpretation {
	if ctx.State != nil {
		if value, ok := ctx.State[intentcontext.IntentInterpretationKey]; ok && value != nil {
			if interpretation, ok := value.(*intentcontext.IntentInterpretation); ok {
				return interpretation
			}
		}
	}
	if ctx.Envelope != nil {
		if interpretation, ok := contextdata.GetTyped[*intentcontext.IntentInterpretation](ctx.Envelope, intentcontext.IntentInterpretationKey); ok {
			return interpretation
		}
	}
	return nil
}

func clarificationStateFromRuntime(ctx prompt.RuntimeContext) *intentcontext.ClarificationState {
	if ctx.State != nil {
		if value, ok := ctx.State[intentcontext.ClarificationStateKey]; ok && value != nil {
			if state, ok := value.(*intentcontext.ClarificationState); ok {
				return state
			}
		}
		if value, ok := ctx.State["clarification_state"]; ok && value != nil {
			if state, ok := value.(*intentcontext.ClarificationState); ok {
				return state
			}
		}
	}
	if ctx.Envelope != nil {
		if state, err := intentcontext.NewStateStore().Read(context.Background(), ctx.Envelope); err == nil {
			return state
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func runtimeString(ctx prompt.RuntimeContext, key string) string {
	if ctx.State != nil {
		if value, ok := ctx.State[key]; ok && value != nil {
			switch v := value.(type) {
			case string:
				return strings.TrimSpace(v)
			case fmt.Stringer:
				return strings.TrimSpace(v.String())
			default:
				return strings.TrimSpace(fmt.Sprint(v))
			}
		}
	}
	return ""
}

func runtimeInt(ctx prompt.RuntimeContext, key string) (int, bool) {
	if ctx.State == nil {
		return 0, false
	}
	value, ok := ctx.State[key]
	if !ok || value == nil {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

func runtimeStrings(ctx prompt.RuntimeContext, key string) []string {
	if ctx.State == nil {
		return nil
	}
	value, ok := ctx.State[key]
	if !ok || value == nil {
		return nil
	}
	switch v := value.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if trimmed := strings.TrimSpace(fmt.Sprint(item)); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	default:
		trimmed := strings.TrimSpace(fmt.Sprint(value))
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	}
}

func anchorSummaries(anchors []retrieval.AnchorRef) []string {
	if len(anchors) == 0 {
		return nil
	}
	out := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		label := strings.TrimSpace(anchor.Term)
		if label == "" {
			label = strings.TrimSpace(anchor.AnchorID)
		}
		if label == "" {
			continue
		}
		if anchor.ChunkID != "" {
			out = append(out, fmt.Sprintf("%s (%s)", label, strings.TrimSpace(anchor.ChunkID)))
		} else {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func entitySummaries(entities []intentcontext.ConfirmedEntity) []string {
	if len(entities) == 0 {
		return nil
	}
	out := make([]string, 0, len(entities))
	for _, entity := range entities {
		label := strings.TrimSpace(entity.Name)
		if label == "" {
			label = strings.TrimSpace(entity.StableID)
		}
		if label == "" {
			continue
		}
		if kind := strings.TrimSpace(string(entity.Kind)); kind != "" {
			out = append(out, fmt.Sprintf("%s [%s]", label, kind))
		} else {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func scopeSummaries(scopes []intentcontext.ConfirmedScope) []string {
	if len(scopes) == 0 {
		return nil
	}
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		label := strings.TrimSpace(scope.Name)
		if label == "" {
			label = strings.TrimSpace(scope.StableID)
		}
		if label != "" {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func questionSummaries(questions []intentcontext.ClarificationQuestion) []string {
	if len(questions) == 0 {
		return nil
	}
	out := make([]string, 0, len(questions))
	for _, question := range questions {
		label := strings.TrimSpace(question.Text)
		if label == "" {
			label = strings.TrimSpace(question.StableID)
		}
		if label != "" {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func projectionSummaries(intents []intentcontext.ProjectionIntent) []string {
	if len(intents) == 0 {
		return nil
	}
	out := make([]string, 0, len(intents))
	for _, intent := range intents {
		label := strings.TrimSpace(intent.StableID)
		if label != "" {
			parts := []string{label}
			if kind := strings.TrimSpace(intent.MutationKind); kind != "" {
				parts = append(parts, kind)
			}
			if root := strings.TrimSpace(intent.RevisionRootID); root != "" {
				parts = append(parts, "root="+root)
			}
			if key := strings.TrimSpace(intent.IdempotencyKey); key != "" {
				parts = append(parts, "key="+key)
			}
			if len(intent.SubjectIDs) > 0 {
				parts = append(parts, "subjects="+strings.Join(intent.SubjectIDs, "/"))
			}
			if len(intent.ObjectIDs) > 0 {
				parts = append(parts, "objects="+strings.Join(intent.ObjectIDs, "/"))
			}
			out = append(out, strings.Join(parts, " "))
			continue
		}
		if key := strings.TrimSpace(intent.IdempotencyKey); key != "" {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func relationIntentSummaries(intents []intentcontext.RelationIntent) []string {
	if len(intents) == 0 {
		return nil
	}
	out := make([]string, 0, len(intents))
	for _, intent := range intents {
		label := strings.TrimSpace(intent.StableID)
		if label == "" {
			continue
		}
		parts := []string{label}
		if source := strings.TrimSpace(intent.SourceEntityID); source != "" {
			parts = append(parts, "source="+source)
		}
		if target := strings.TrimSpace(intent.TargetEntityID); target != "" {
			parts = append(parts, "target="+target)
		}
		if relation := strings.TrimSpace(intent.RelationKind); relation != "" {
			parts = append(parts, "kind="+relation)
		}
		if direction := strings.TrimSpace(intent.Direction); direction != "" {
			parts = append(parts, "direction="+direction)
		}
		out = append(out, strings.Join(parts, " "))
	}
	sort.Strings(out)
	return out
}

func projectionRecordSummaries(records []intentcontext.ProjectionRecord) []string {
	if len(records) == 0 {
		return nil
	}
	out := make([]string, 0, len(records))
	for _, record := range records {
		label := strings.TrimSpace(record.StableID)
		if label == "" {
			continue
		}
		parts := []string{label}
		if root := strings.TrimSpace(record.RevisionRootID); root != "" {
			parts = append(parts, "root="+root)
		}
		if key := strings.TrimSpace(record.IdempotencyKey); key != "" {
			parts = append(parts, "key="+key)
		}
		if result := strings.TrimSpace(string(record.Result)); result != "" {
			parts = append(parts, "result="+result)
		}
		if by := strings.TrimSpace(record.AppliedBy); by != "" {
			parts = append(parts, "by="+by)
		}
		out = append(out, strings.Join(parts, " "))
	}
	sort.Strings(out)
	return out
}
