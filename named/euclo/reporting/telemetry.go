package reporting

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// EucloTelemetry wraps telemetry.Telemetry with typed Emit helpers.
type EucloTelemetry struct {
	sink telemetry.Telemetry
}

// NewEucloTelemetry creates a new telemetry wrapper.
func NewEucloTelemetry(sink telemetry.Telemetry) *EucloTelemetry {
	if sink == nil {
		sink = noopTelemetry{}
	}
	return &EucloTelemetry{sink: sink}
}

func (t *EucloTelemetry) emit(ctx context.Context, eventType EventType, payload any) {
	if t == nil || t.sink == nil {
		return
	}
	meta := eventPayloadMap(payload)
	taskID, _ := meta["task_id"].(string)
	t.sink.Emit(telemetry.Event{
		Type:      telemetry.EventType(eventType),
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
		Metadata:  meta,
	})
}

func (t *EucloTelemetry) EmitIntakeComplete(ctx context.Context, ev EventIntakeComplete) {
	t.emit(ctx, EventTypeIntakeComplete, ev)
}

func (t *EucloTelemetry) EmitFamilySelected(ctx context.Context, ev EventFamilySelected) {
	t.emit(ctx, EventTypeFamilySelected, ev)
}

func (t *EucloTelemetry) EmitIngestionComplete(ctx context.Context, ev EventIngestionComplete) {
	t.emit(ctx, EventTypeIngestionComplete, ev)
}

func (t *EucloTelemetry) EmitStreamRequested(ctx context.Context, ev EventStreamRequested) {
	t.emit(ctx, EventTypeStreamRequested, ev)
}

func (t *EucloTelemetry) EmitCapabilityClassified(ctx context.Context, ev EventCapabilityClassified) {
	t.emit(ctx, EventTypeCapabilityClassified, ev)
}

func (t *EucloTelemetry) EmitRouteSelected(ctx context.Context, ev EventRouteSelected) {
	t.emit(ctx, EventTypeRouteSelected, ev)
}

func (t *EucloTelemetry) EmitGateResult(ctx context.Context, ev EventGateResult) {
	t.emit(ctx, EventTypeGateResult, ev)
}

func (t *EucloTelemetry) EmitProjectionCompleted(ctx context.Context, ev EventProjectionCompleted) {
	t.emit(ctx, EventTypeProjectionCompleted, ev)
}

func (t *EucloTelemetry) EmitClarificationStarted(ctx context.Context, ev EventClarificationStarted) {
	t.emit(ctx, EventTypeClarificationStarted, ev)
}

func (t *EucloTelemetry) EmitClarificationAnswered(ctx context.Context, ev EventClarificationAnswered) {
	t.emit(ctx, EventTypeClarificationAnswered, ev)
}

func (t *EucloTelemetry) EmitClarificationGrounded(ctx context.Context, ev EventClarificationGrounded) {
	t.emit(ctx, EventTypeClarificationGrounded, ev)
}

func (t *EucloTelemetry) EmitClarificationProjected(ctx context.Context, ev EventClarificationProjected) {
	t.emit(ctx, EventTypeClarificationProjected, ev)
}

func (t *EucloTelemetry) EmitClarificationCompleted(ctx context.Context, ev EventClarificationCompleted) {
	t.emit(ctx, EventTypeClarificationCompleted, ev)
}

func (t *EucloTelemetry) EmitFrameEmitted(ctx context.Context, ev EventFrameEmitted) {
	t.emit(ctx, EventTypeFrameEmittedEuclo, ev)
}

func (t *EucloTelemetry) EmitFrameResolved(ctx context.Context, ev EventFrameResolved) {
	t.emit(ctx, EventTypeFrameResolvedEuclo, ev)
}

func (t *EucloTelemetry) EmitJobSubmitted(ctx context.Context, ev EventJobSubmitted) {
	t.emit(ctx, EventTypeJobSubmitted, ev)
}

func (t *EucloTelemetry) EmitJobCompleted(ctx context.Context, ev EventJobCompleted) {
	t.emit(ctx, EventTypeJobCompleted, ev)
}

func (t *EucloTelemetry) EmitBranchResolved(ctx context.Context, ev EventBranchResolved) {
	t.emit(ctx, EventTypeBranchResolved, ev)
}

func (t *EucloTelemetry) EmitParallelFanout(ctx context.Context, ev EventParallelFanout) {
	t.emit(ctx, EventTypeParallelFanout, ev)
}

func (t *EucloTelemetry) EmitRecipeSelected(ctx context.Context, ev EventRecipeSelected) {
	t.emit(ctx, EventTypeRecipeSelected, ev)
}

func (t *EucloTelemetry) EmitStepStarted(ctx context.Context, ev EventStepStarted) {
	t.emit(ctx, EventTypeStepStartedEuclo, ev)
}

func (t *EucloTelemetry) EmitStepCompleted(ctx context.Context, ev EventStepCompleted) {
	t.emit(ctx, EventTypeStepCompletedEuclo, ev)
}

func (t *EucloTelemetry) EmitVerifyStarted(ctx context.Context, ev EventVerifyStarted) {
	t.emit(ctx, EventTypeVerifyStarted, ev)
}

func (t *EucloTelemetry) EmitVerifyComplete(ctx context.Context, ev EventVerifyComplete) {
	t.emit(ctx, EventTypeVerifyComplete, ev)
}

func (t *EucloTelemetry) EmitExecutionComplete(ctx context.Context, ev EventExecutionComplete) {
	t.emit(ctx, EventTypeExecutionComplete, ev)
}

type noopTelemetry struct{}

func (noopTelemetry) Emit(telemetry.Event) {}

// TelemetryNode reports execution metrics and outcomes.
type TelemetryNode struct {
	id        string
	telemetry *EucloTelemetry
}

// NewTelemetryNode creates a new telemetry node.
func NewTelemetryNode(id string) *TelemetryNode {
	return &TelemetryNode{id: id}
}

// ID returns the node ID.
func (n *TelemetryNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *TelemetryNode) Type() string {
	return "telemetry"
}

// Execute collects and reports telemetry data.
func (n *TelemetryNode) Execute(ctx context.Context, env *contextdata.Envelope) (map[string]any, error) {
	completed, _ := contextdata.GetTyped[bool](env, "euclo.execution.completed")
	blocked, _ := contextdata.GetTyped[bool](env, "euclo.policy.blocked")
	errorCount := 0
	if v, ok := contextdata.GetTyped[any](env, "euclo.execution.error_count"); ok {
		switch value := v.(type) {
		case int:
			errorCount = value
		case int64:
			errorCount = int(value)
		case float64:
			errorCount = int(value)
		}
	}

	outcome := ClassifyOutcome(completed, errorCount, blocked)
	state.SetOutcomeCategory(env, string(outcome.Category))
	contextdata.SetTyped(env, "euclo.outcome.reason", outcome.Reason)
	contextdata.SetTyped(env, "euclo.outcome.completed", outcome.Completed)
	state.SetOutcomeTelemetry(env, map[string]any{
		"category":    string(outcome.Category),
		"reason":      outcome.Reason,
		"completed":   outcome.Completed,
		"error_count": outcome.ErrorCount,
		"blocked":     blocked,
	})

	if mutation, ok := contextdata.GetTyped[*graphdb.MutationResult](env, "euclo.projection.mutation_result"); ok && mutation != nil {
		contextdata.SetTyped(env, "euclo.outcome_telemetry.projection", map[string]any{
			"stable_id":      mutation.StableID,
			"scope":          string(mutation.Scope),
			"status":         string(mutation.Status),
			"reason":         mutation.Reason,
			"created_ids":    append([]string(nil), mutation.CreatedIDs...),
			"updated_ids":    append([]string(nil), mutation.UpdatedIDs...),
			"annotated_ids":  append([]string(nil), mutation.AnnotatedIDs...),
			"superseded_ids": append([]string(nil), mutation.SupersededIDs...),
			"matched_ids":    append([]string(nil), mutation.MatchedIDs...),
			"rejected_ids":   append([]string(nil), mutation.RejectedIDs...),
			"conflict_ids":   append([]string(nil), mutation.ConflictIDs...),
			"record_ids":     append([]string(nil), mutation.RecordIDs...),
			"details":        mutation.Details,
		})
	}

	tel := n.telemetry
	if tel == nil {
		tel = NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
	}
	planIDStr, _ := contextdata.GetTyped[string](env, "euclo.projection.plan_id")
	if tel != nil {
		if mutation, ok := contextdata.GetTyped[*graphdb.MutationResult](env, "euclo.projection.mutation_result"); ok && mutation != nil {
			tel.EmitProjectionCompleted(ctx, EventProjectionCompleted{
				EventHeader: EventHeader{
					TaskID:     env.TaskID,
					SessionID:  env.SessionID,
					Seq:        0,
					OccurredAt: time.Now().UTC(),
				},
				PlanID:           planIDStr,
				MutationStableID: mutation.StableID,
				MutationScope:    string(mutation.Scope),
				MutationStatus:   string(mutation.Status),
				Reason:           mutation.Reason,
				CreatedIDs:       append([]string(nil), mutation.CreatedIDs...),
				UpdatedIDs:       append([]string(nil), mutation.UpdatedIDs...),
				AnnotatedIDs:     append([]string(nil), mutation.AnnotatedIDs...),
				SupersededIDs:    append([]string(nil), mutation.SupersededIDs...),
				MatchedIDs:       append([]string(nil), mutation.MatchedIDs...),
				RejectedIDs:      append([]string(nil), mutation.RejectedIDs...),
				ConflictIDs:      append([]string(nil), mutation.ConflictIDs...),
				RecordIDs:        append([]string(nil), mutation.RecordIDs...),
				Details:          mutation.Details,
			})
		}
		tel.EmitVerifyStarted(ctx, EventVerifyStarted{
			EventHeader: EventHeader{
				TaskID:     env.TaskID,
				SessionID:  env.SessionID,
				Seq:        0,
				OccurredAt: time.Now().UTC(),
			},
			StepCount: 0,
		})
		tel.EmitVerifyComplete(ctx, EventVerifyComplete{
			EventHeader: EventHeader{
				TaskID:     env.TaskID,
				SessionID:  env.SessionID,
				Seq:        0,
				OccurredAt: time.Now().UTC(),
			},
			Outcome: string(outcome.Category),
			Summary: outcome.Reason,
		})
		tel.EmitExecutionComplete(ctx, EventExecutionComplete{
			EventHeader: EventHeader{
				TaskID:     env.TaskID,
				SessionID:  env.SessionID,
				Seq:        0,
				OccurredAt: time.Now().UTC(),
			},
			Outcome:     string(outcome.Category),
			OutcomeKind: outcome.Reason,
			StepCount:   0,
			LLMCalls:    0,
			TokenUsage:  0,
		})
		if shouldEmitClarificationCompletion(env) {
			tel.EmitClarificationCompleted(ctx, EventClarificationCompleted{
				EventHeader: EventHeader{
					TaskID:     env.TaskID,
					SessionID:  env.SessionID,
					Seq:        0,
					OccurredAt: time.Now().UTC(),
				},
				ThoughtRecipeID: clarificationThoughtRecipeIDFromEnv(env),
				StateVersion:    clarificationStateVersionFromEnv(env),
				PlanID:          planIDStr,
				Completion:      string(outcome.Category),
			})
		}
	}

	report := map[string]any{
		"outcome_category": string(outcome.Category),
		"outcome_reason":   outcome.Reason,
		"completed":        outcome.Completed,
	}
	if mutation, ok := contextdata.GetTyped[*graphdb.MutationResult](env, "euclo.projection.mutation_result"); ok && mutation != nil {
		report["projection"] = map[string]any{
			"plan_id":         planIDStr,
			"stable_id":       mutation.StableID,
			"scope":           string(mutation.Scope),
			"status":          string(mutation.Status),
			"reason":          mutation.Reason,
			"created_ids":     append([]string(nil), mutation.CreatedIDs...),
			"updated_ids":     append([]string(nil), mutation.UpdatedIDs...),
			"annotated_ids":   append([]string(nil), mutation.AnnotatedIDs...),
			"superseded_ids":  append([]string(nil), mutation.SupersededIDs...),
			"matched_ids":     append([]string(nil), mutation.MatchedIDs...),
			"rejected_ids":    append([]string(nil), mutation.RejectedIDs...),
			"conflict_ids":    append([]string(nil), mutation.ConflictIDs...),
			"record_ids":      append([]string(nil), mutation.RecordIDs...),
			"state_version":   mutation.StateVersion,
			"applied_at":      mutation.AppliedAt,
			"idempotency_key": mutation.IdempotencyKey,
			"details":         mutation.Details,
		}
	}

	return map[string]any{
		"outcome_category": report["outcome_category"],
		"outcome_reason":   report["outcome_reason"],
		"completed":        report["completed"],
		"projection":       report["projection"],
	}, nil
}

func shouldEmitClarificationCompletion(env *contextdata.Envelope) bool {
	if _, ok := contextdata.GetTyped[any](env, "euclo.clarification.request"); ok {
		return true
	}
	if _, ok := contextdata.GetTyped[any](env, "euclo.clarification.projection"); ok {
		return true
	}
	if _, ok := contextdata.GetTyped[any](env, intentcontext.ClarificationActiveThoughtRecipeKey); ok {
		return true
	}
	return false
}

func clarificationThoughtRecipeIDFromEnv(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, intentcontext.ClarificationActiveThoughtRecipeKey)
	return v
}

func clarificationStateVersionFromEnv(env *contextdata.Envelope) uint64 {
	if state, ok := contextdata.GetTyped[*intentcontext.ClarificationState](env, "euclo.intent.clarification.state"); ok && state != nil {
		return state.StateVersion
	}
	if v, ok := contextdata.GetTyped[any](env, "euclo.clarification.state_version"); ok {
		switch n := v.(type) {
		case uint64:
			return n
		case int:
			if n > 0 {
				return uint64(n)
			}
		}
	}
	return 0
}

// EmitRouteSelected reports the selected route and candidate metadata.
func EmitRouteSelected(ctx context.Context, taskID, sessionID, family, routeKind, routeID string, candidateCount int, fallbackTaken bool) {
	emitRouteEvent(ctx, EventTypeRouteSelected, taskID, sessionID, map[string]any{
		"session_id":      sessionID,
		"family":          family,
		"route_kind":      routeKind,
		"route_id":        routeID,
		"candidate_count": candidateCount,
		"fallback_taken":  fallbackTaken,
	})
}

// EmitRouteCompleted reports route completion metadata.
func EmitRouteCompleted(ctx context.Context, taskID, sessionID, routeKind, routeID string, outcome RouteOutcome, artifactKinds []string, elapsed time.Duration) {
	emitRouteEvent(ctx, EventTypeRouteCompleted, taskID, sessionID, map[string]any{
		"session_id":     sessionID,
		"route_kind":     routeKind,
		"route_id":       routeID,
		"outcome":        string(outcome),
		"artifact_kinds": append([]string(nil), artifactKinds...),
		"elapsed_ms":     elapsed.Milliseconds(),
	})
}

// EmitRouteUnavailable reports an unavailable route and the reason.
func EmitRouteUnavailable(ctx context.Context, taskID, sessionID, routeID, availability, reason string) {
	emitRouteEvent(ctx, EventTypeRouteUnavailable, taskID, sessionID, map[string]any{
		"session_id":   sessionID,
		"route_id":     routeID,
		"availability": availability,
		"reason":       reason,
	})
}

// EmitRouteDryRun reports a dry-run payload.
func EmitRouteDryRun(ctx context.Context, taskID, sessionID string, report any) {
	emitRouteEvent(ctx, EventTypeRouteDryRun, taskID, sessionID, map[string]any{
		"session_id": sessionID,
		"report":     report,
	})
}

// EmitRouteFallback reports primary and fallback route IDs.
func EmitRouteFallback(ctx context.Context, taskID, sessionID, primaryID, fallbackID, reason string) {
	emitRouteEvent(ctx, EventTypeRouteFallback, taskID, sessionID, map[string]any{
		"session_id":  sessionID,
		"primary_id":  primaryID,
		"fallback_id": fallbackID,
		"reason":      reason,
	})
}

func emitRouteEvent(ctx context.Context, eventType EventType, taskID, sessionID string, data map[string]any) {
	sink := telemetry.TelemetryFromContext(ctx)
	if sink == nil {
		return
	}
	sink.Emit(telemetry.Event{
		Type:      telemetry.EventType(string(eventType)),
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
		Metadata:  data,
	})
}
