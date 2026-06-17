package thoughtrecipe

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

func (c *stepCore) resolveFromRegistry(ctx context.Context, env *contextdata.Envelope) (string, error) {
	if c.deps.PromptRegistry == nil {
		return "", fmt.Errorf("no prompt registry available")
	}
	rctx := c.buildRuntimeContext(ctx, env)
	return c.deps.PromptRegistry.Resolve(c.step.PromptID, rctx)
}

func (c *stepCore) buildRuntimeContext(ctx context.Context, env *contextdata.Envelope) prompt.RuntimeContext {
	data := thoughtrecipeTemplateData(env, c.step)
	scopedRegistry := c.scopedRegistry()
	runtime := prompt.NewRuntimeContext(env, c.step.Paradigm, "euclo").
		WithVariable("instruction", c.renderTemplate(c.step.Prompt, data)).
		WithVariable("question", func() string {
			if strings.TrimSpace(c.step.Question) != "" {
				return c.renderTemplate(c.step.Question, data)
			}
			return c.step.Prompt
		}()).
		WithVariable("prompt_id", c.step.PromptID).
		WithStateMap(clarificationRuntimeState(ctx, env)).
		WithStateMap(c.stepRuntimeState(data))

	runtime.Task = &execution.Task{
		ID:          c.id,
		Type:        c.step.Paradigm,
		Instruction: c.renderTemplate(c.step.Prompt, data),
		Context:     data,
	}
	if scopedRegistry != nil {
		runtime.Tools = scopedRegistry.ModelCallableTools(ctx)
		if snapshot := scopedRegistry.CaptureExecutionCatalogSnapshot(ctx); snapshot != nil {
			runtime.Capabilities = snapshot.InspectableCapabilities()
		}
	} else if c.deps.Registry != nil {
		runtime.Tools = c.deps.Registry.ModelCallableTools(ctx)
		runtime.Capabilities = c.deps.Registry.AllCapabilities()
	}
	runtime.AgentSpec = nil
	return runtime
}

func (c *stepCore) stepRuntimeState(data map[string]any) map[string]any {
	state := map[string]any{
		"execution_step_id":   c.step.ID,
		"execution_step_type": c.step.Kind.String(),
		"execution_paradigm":  c.step.Paradigm,
		"execution_goal":      c.step.Goal,
		"execution_question":  c.step.Question,
		"execution_prompt_id": c.step.PromptID,
		"execution_instruction": func() string {
			if strings.TrimSpace(c.step.Goal) != "" {
				return c.renderTemplate(c.step.Goal, data)
			}
			return c.renderTemplate(c.step.Prompt, data)
		}(),
	}
	if ctxState := stepContextRuntimeState(c.step); len(ctxState) > 0 {
		for k, v := range ctxState {
			state[k] = v
		}
	}
	if strings.TrimSpace(c.step.CapabilityID) != "" {
		state[executionCapabilityIDKey] = c.step.CapabilityID
	}

	return state
}

func stepContextRuntimeState(step ExecutionStep) map[string]any {
	out := make(map[string]any)
	if stream := step.Stream; stream != nil {
		if query := strings.TrimSpace(stream.QueryTemplate); query != "" {
			out["execution_step_context_stream_query"] = query
		}
		if maxTokens := stream.MaxTokens; maxTokens > 0 {
			out["execution_step_context_stream_max_tokens"] = maxTokens
		}
		if mode := strings.TrimSpace(stream.Mode); mode != "" {
			out["execution_step_context_stream_mode"] = mode
		}
	}
	if ingest := step.Ingest; ingest != nil {
		if mode := strings.TrimSpace(ingest.Mode); mode != "" {
			out["execution_step_context_ingest_mode"] = mode
		}
		if len(ingest.IncludeGlobs) > 0 {
			out["execution_step_context_ingest_include_globs"] = append([]string(nil), ingest.IncludeGlobs...)
		}
		if len(ingest.ExcludeGlobs) > 0 {
			out["execution_step_context_ingest_exclude_globs"] = append([]string(nil), ingest.ExcludeGlobs...)
		}
		if root := strings.TrimSpace(ingest.WorkspaceRoot); root != "" {
			out["execution_step_context_ingest_workspace_root"] = root
		}
	}
	if len(step.Inherit) > 0 {
		out["execution_step_context_inherit"] = append([]string(nil), step.Inherit...)
	}
	if len(step.Capture) > 0 {
		out["execution_step_context_capture"] = append([]string(nil), step.Capture...)
	}
	return out
}

func clarificationRuntimeState(ctx context.Context, env *contextdata.Envelope) map[string]any {
	state := make(map[string]any)
	if current, err := intentcontext.NewStateStore().Read(ctx, env); err == nil && current != nil {
		state[intentcontext.ClarificationStateKey] = current.Clone()
		state["euclo.intent.clarification.state_version"] = current.StateVersion
		state["euclo.intent.clarification.current_turn_id"] = current.CurrentTurnID
		state["euclo.intent.clarification.active_thoughtrecipe_id"] = current.ActiveThoughtRecipeID
		state["euclo.intent.clarification.last_checkpoint_id"] = current.LastCheckpointID
		state["euclo.intent.clarification.last_checkpoint_seq"] = current.LastCheckpointSeq
		state["euclo.intent.clarification.confirmed_entity_ids"] = stableEntityIDs(current.ConfirmedEntities)
		state["euclo.intent.clarification.confirmed_scope_ids"] = stableScopeIDs(current.ConfirmedScopes)
		state["euclo.intent.clarification.pending_relation_intents"] = append([]intentcontext.RelationIntent(nil), current.PendingRelationIntents...)
		state["euclo.intent.clarification.pending_projection_ids"] = stableProjectionIDs(current.PendingProjection)
		state["euclo.intent.clarification.applied_mutations"] = append([]intentcontext.ProjectionRecord(nil), current.AppliedMutations...)
		state["euclo.intent.clarification.grounded_anchor_ids"] = anchorIDs(current.GroundedAnchors)
		if current.Ambiguity != nil {
			state["euclo.intent.clarification.ambiguity_kind"] = string(current.Ambiguity.Kind)
			state["euclo.intent.clarification.ambiguity_confidence"] = current.Ambiguity.Confidence
			state["euclo.intent.clarification.ambiguity_rationale"] = current.Ambiguity.Rationale
		}
		if len(current.PendingQuestions) > 0 {
			state["euclo.intent.clarification.pending_questions"] = append([]intentcontext.ClarificationQuestion(nil), current.PendingQuestions...)
		}
		if len(current.Turns) > 0 {
			state["euclo.intent.clarification.turn_ids"] = turnIDs(current.Turns)
		}
	}
	return state
}

func anchorIDs(anchors []retrieval.AnchorRef) []string {
	if len(anchors) == 0 {
		return nil
	}
	ids := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		if strings.TrimSpace(anchor.AnchorID) != "" {
			ids = append(ids, strings.TrimSpace(anchor.AnchorID))
		}
	}
	return ids
}

func stableEntityIDs(entities []intentcontext.ConfirmedEntity) []string {
	if len(entities) == 0 {
		return nil
	}
	ids := make([]string, 0, len(entities))
	for _, entity := range entities {
		if strings.TrimSpace(entity.StableID) != "" {
			ids = append(ids, strings.TrimSpace(entity.StableID))
		}
	}
	return ids
}

func stableScopeIDs(scopes []intentcontext.ConfirmedScope) []string {
	if len(scopes) == 0 {
		return nil
	}
	ids := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if strings.TrimSpace(scope.StableID) != "" {
			ids = append(ids, strings.TrimSpace(scope.StableID))
		}
	}
	return ids
}

func stableProjectionIDs(intents []intentcontext.ProjectionIntent) []string {
	if len(intents) == 0 {
		return nil
	}
	ids := make([]string, 0, len(intents))
	for _, intent := range intents {
		if strings.TrimSpace(intent.StableID) != "" {
			ids = append(ids, strings.TrimSpace(intent.StableID))
		}
	}
	return ids
}

func turnIDs(turns []intentcontext.ClarificationTurn) []string {
	if len(turns) == 0 {
		return nil
	}
	ids := make([]string, 0, len(turns))
	for _, turn := range turns {
		if strings.TrimSpace(turn.TurnID) != "" {
			ids = append(ids, strings.TrimSpace(turn.TurnID))
		}
	}
	return ids
}
