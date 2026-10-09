package orchestrate

import (
	"context"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/grounding"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// Envelope keys carrying dispatch-time re-grounding provenance.
const (
	regroundChunkIDsKey  = "euclo.execution.regrounded.chunk_ids"
	regroundSourceRunKey = "euclo.execution.regrounded.source_run_id"
	regroundEntriesKey   = "euclo.execution.regrounded.entries"
	// executionOutcomeKey mirrors the structured RecipeOutcome.
	executionOutcomeKey = "euclo.execution.outcome"
)

// restoreState consumes the Wave 1 IF-2 port at recipe dispatch: it queries the
// grounded capture corpus for the selected recipe and applies returned entries
// through the origin-preserving capture-application path before any step runs.
//
// It returns (result, true) when the recipe must not proceed (a typed capture
// failure on a restore entry); otherwise (nil, false) and the recipe runs. A
// query error is reported and the run continues cold — never a silent restore.
func (n *ThoughtRecipeExecutorNode) restoreState(ctx context.Context, env *contextdata.Envelope, recipeID string) (*execution.Result, bool) {
	if n == nil || n.stateReground == nil || env == nil {
		return nil, false
	}
	req := grounding.RegroundRequest{
		WorkspaceID: n.workspaceID(env),
		SessionID:   env.SessionIDSnapshot(),
		RecipeID:    strings.TrimSpace(recipeID),
	}
	restored, err := n.stateReground.Reground(ctx, req)
	if err != nil {
		n.emitGroundingEvent(ctx, env, telemetry.EventGroundingRegroundFailed, map[string]any{
			"recipe_id": recipeID,
			"err":       err.Error(),
		})
		return nil, false
	}
	if !restored.Grounded || len(restored.Entries) == 0 {
		return nil, false
	}

	chunkIDs := make([]string, 0, len(restored.Entries))
	entries := make([]map[string]any, 0, len(restored.Entries))
	for _, entry := range restored.Entries {
		if applyErr := thoughtrecipepkg.ApplyCaptureValue(env, entry.StateKey, entry.Value, originClass(entry.Origin)); applyErr != nil {
			// A restore that cannot re-enter the capture path is a typed capture
			// failure, not a silent coercion. No step is executing yet, so the
			// recipe-level policy is abort.
			failure := &euclotypes.StepFailure{Kind: euclotypes.FailureUnknown, Message: applyErr.Error(), Cause: applyErr}
			euclostate.SetStepFailure(env, failure)
			n.emitGroundingEvent(ctx, env, telemetry.EventStepOperationalFailure, map[string]any{
				"step_id":      recipeID,
				"kind":         string(euclotypes.FailureUnknown),
				"on_error":     "abort",
				"action_taken": "abort",
				"err":          applyErr.Error(),
			})
			return &execution.Result{
				NodeID:  n.id,
				Success: false,
				Data:    execution.NewErrorResultPayload(applyErr.Error()),
				Metadata: map[string]any{
					"failure_kind":      string(euclotypes.FailureUnknown),
					"on_error_resolved": "abort",
				},
			}, true
		}
		if id := strings.TrimSpace(entry.ChunkID); id != "" {
			chunkIDs = append(chunkIDs, id)
		}
		entries = append(entries, map[string]any{
			"state_key":  entry.StateKey,
			"chunk_id":   entry.ChunkID,
			"origin":     entry.Origin,
			"epistemics": entry.Epistemics,
		})
	}

	contextdata.SetTyped(env, regroundChunkIDsKey, chunkIDs)
	contextdata.SetTyped(env, regroundSourceRunKey, restored.SourceRunID)
	contextdata.SetTyped(env, regroundEntriesKey, entries)
	if len(chunkIDs) > 0 {
		refs := make([]contextdata.ChunkID, 0, len(chunkIDs))
		for _, id := range chunkIDs {
			refs = append(refs, contextdata.ChunkID(id))
		}
		env.AddRetrievalReference(contextdata.RetrievalReference{
			QueryID:     "reground:" + recipeID,
			QueryText:   "state re-ground",
			Scope:       "reground",
			ChunkIDs:    refs,
			TotalFound:  len(restored.Entries),
			RetrievedAt: time.Now().UTC(),
		})
	}
	n.emitGroundingEvent(ctx, env, telemetry.EventCaptureGrounded, map[string]any{
		"recipe_id": recipeID,
		"count":     len(restored.Entries),
	})
	return nil, false
}

// recordGraphLevelFailure classifies an error that aborts the whole recipe
// graph (most importantly a Wave 1 step-barrier grounding failure, which
// surfaces at the graph boundary rather than inside a node) and records it as
// a typed step failure plus step.operational_failure telemetry. It is
// idempotent in spirit: a failure already recorded at the node boundary is not
// recorded twice with a different class.
func (n *ThoughtRecipeExecutorNode) recordGraphLevelFailure(ctx context.Context, env *contextdata.Envelope, err error) {
	kind := thoughtrecipepkg.ClassifyFailure(err)
	if kind == "" {
		kind = euclotypes.FailureUnknown
	}
	failure := &euclotypes.StepFailure{Kind: kind, Message: err.Error(), Cause: err}
	if env != nil {
		euclostate.SetStepFailure(env, failure)
	}
	recipeID := ""
	if env != nil {
		recipeID = euclostate.GetExecutionThoughtRecipeID(env)
	}
	n.emitGroundingEvent(ctx, env, telemetry.EventStepOperationalFailure, map[string]any{
		"step_id":      recipeID,
		"kind":         string(kind),
		"on_error":     "abort",
		"action_taken": "abort",
		"err":          err.Error(),
	})
}

// workspaceID resolves the workspace scope for a re-ground query from the
// envelope, falling back to the executor's composed workspace.
func (n *ThoughtRecipeExecutorNode) workspaceID(env *contextdata.Envelope) string {
	if env != nil {
		if value, ok := contextdata.GetTyped[string](env, "input.workspace"); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if n == nil {
		return ""
	}
	return strings.TrimSpace(n.workspace)
}

func (n *ThoughtRecipeExecutorNode) emitGroundingEvent(ctx context.Context, env *contextdata.Envelope, eventType telemetry.EventType, metadata map[string]any) {
	sink := telemetry.TelemetryFromContext(ctx)
	if sink == nil && n != nil && n.deps != nil {
		sink = n.deps.Telemetry
	}
	if sink == nil {
		return
	}
	event := telemetry.Event{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
	if env != nil {
		event.TaskID = env.TaskIDSnapshot()
		event.SessionID = env.SessionIDSnapshot()
	}
	telemetry.StampCorrelation(ctx, &event)
	sink.Emit(event)
}

// originClass maps the Wave 1 origin vocabulary onto the envelope's OriginClass,
// defaulting to llm so an unrecorded origin never elevates.
func originClass(origin string) contextdata.OriginClass {
	class := contextdata.OriginClass(strings.TrimSpace(origin))
	if !class.Valid() {
		return contextdata.OriginLLM
	}
	return class
}

// attachRecipeOutcome aggregates the run's typed step failures into a
// structured RecipeOutcome, attached to the recipe result metadata and mirrored
// on the envelope. The turn ends with this structure, never a raw error.
func (n *ThoughtRecipeExecutorNode) attachRecipeOutcome(env *contextdata.Envelope, result *execution.Result, err error) {
	if result == nil {
		return
	}
	failures := recipeFailureMaps(euclostate.GetStepFailures(env))
	state := "completed"
	if err != nil || !result.Success {
		state = "failed"
	}
	outcome := map[string]any{
		"state":          state,
		"failures":       failures,
		"fallback_taken": euclostate.GetFallbackTaken(env),
	}
	if result.Metadata == nil {
		result.Metadata = map[string]any{}
	}
	result.Metadata["recipe_outcome"] = outcome
	if env != nil {
		contextdata.SetTyped(env, executionOutcomeKey, outcome)
	}
}

func recipeFailureMaps(failures []*euclotypes.StepFailure) []map[string]any {
	if len(failures) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(failures))
	for _, failure := range failures {
		if failure == nil {
			continue
		}
		out = append(out, map[string]any{
			"kind":    string(failure.Kind),
			"message": failure.Message,
		})
	}
	return out
}
