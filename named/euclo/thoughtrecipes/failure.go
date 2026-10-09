package thoughtrecipe

import (
	"context"
	"errors"
	"strings"
	"time"

	rewooagent "codeburg.org/lexbit/relurpify/cognitionzoo/rewoo"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// operationalFailureKindMetadata is the result-metadata key that carries the
// typed failure class from a failed step node to the graph's fallback edge
// predicate. The predicate fires only when this is a known operational class.
const operationalFailureKindMetadata = "failure_kind"

// policyActionKind is the normalized step on_error action vocabulary (D6).
type policyActionKind string

const (
	policyAbort    policyActionKind = "abort"
	policyContinue policyActionKind = "continue"
	policyFallback policyActionKind = "fallback"
	policyAsk      policyActionKind = "ask"
)

// ClassifyFailure maps an execution error onto the §3.5 operational-failure
// taxonomy. The grounding class is identified by errors.Is against the Wave 1
// sentinel only — never by string matching (FR-23). Unknown errors take the
// conservative `unknown` class, whose default policy is abort.
func ClassifyFailure(err error) euclotypes.FailureKind {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, knowledge.ErrGroundingFailed):
		return euclotypes.FailureGroundingFailed
	case errors.Is(err, context.Canceled):
		return euclotypes.FailureCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return euclotypes.FailureModelUnavailable
	case errors.Is(err, rewooagent.ErrRewooPlanInvalid):
		return euclotypes.FailureModelInvalidOutput
	default:
		var denied *permissions.PermissionDeniedError
		if errors.As(err, &denied) {
			return euclotypes.FailureCapabilityDenied
		}
		return euclotypes.FailureUnknown
	}
}

// stepPolicyAction normalizes the authored on_error policy. The legacy `skip`
// spelling maps to `continue` (the vocabulary the capability path already
// carried). A missing policy defaults to abort.
func stepPolicyAction(step ExecutionStep) policyActionKind {
	if step.OnError == nil {
		return policyAbort
	}
	switch strings.ToLower(strings.TrimSpace(step.OnError.Action)) {
	case "continue", "skip":
		return policyContinue
	case "fallback":
		return policyFallback
	case "ask":
		return policyAsk
	default:
		return policyAbort
	}
}

// recordOperationalFailure classifies err, applies the step's on_error policy,
// records the typed failure on the envelope, emits step.operational_failure,
// and returns a structured result. It never returns a raw error: the turn ends
// with a structured degraded result, and the failure never substitutes a
// paradigm the recipe did not declare (FR-7).
//
// The interim `ask` branch is honest about the resolver not existing yet: it
// aborts and records ask_unavailable, and is deleted when Phase 8 lands the
// InteractionResolver. The resolver == nil seam is intentionally greppable.
func (c *stepCore) recordOperationalFailure(ctx context.Context, env *contextdata.Envelope, err error) *execution.Result {
	kind := ClassifyFailure(err)
	action := stepPolicyAction(c.step)
	actionTaken := action
	resolveUnavailable := false
	if kind == "" {
		kind = euclotypes.FailureUnknown
	}

	// Phase 4 interim: the interaction resolver does not exist yet. `ask`
	// degrades to abort with an explicit, greppable marker. Phase 8 deletes
	// this branch and routes `ask` through interaction.Resolver.
	if action == policyAsk {
		actionTaken = policyAbort
		resolveUnavailable = true
	}

	failure := &euclotypes.StepFailure{Kind: kind, Message: err.Error(), Cause: err}
	if env != nil {
		state.SetStepFailure(env, failure)
		c.writeStepFailureMetadata(env, kind, action, actionTaken)
	}

	success := action == policyContinue
	c.emitOperationalFailure(ctx, env, kind, action, actionTaken, resolveUnavailable)

	data := map[string]any{
		"failure_kind":    string(kind),
		"on_error_action": string(action),
	}
	if success {
		data["skipped"] = true
		data["skipped_reason"] = err.Error()
		data["on_error_resolved"] = string(policyContinue)
	} else {
		data["error"] = err.Error()
		data["on_error_resolved"] = string(actionTaken)
	}

	result := &execution.Result{
		NodeID:  c.id,
		Success: success,
		Data:    execution.NewToolResultPayload(data),
		Metadata: map[string]any{
			operationalFailureKindMetadata: string(kind),
			"on_error_action":              string(action),
			"on_error_resolved":            string(actionTaken),
		},
	}
	if resolveUnavailable {
		result.Metadata["ask_unavailable"] = true
	}
	if !success {
		result.Error = err.Error()
	}
	return result
}

// writeStepFailureMetadata records the typed failure class alongside the step
// metadata so downstream routing, captures, and reporting can distinguish a
// degraded step from a completed one.
func (c *stepCore) writeStepFailureMetadata(env *contextdata.Envelope, kind euclotypes.FailureKind, action, actionTaken policyActionKind) {
	if env == nil {
		return
	}
	base := "euclo.execution.step." + c.step.ID
	contextdata.SetTyped(env, base+".failure_kind", string(kind))
	contextdata.SetTyped(env, base+".on_error_action", string(action))
	contextdata.SetTyped(env, base+".on_error_resolved", string(actionTaken))
}

// emitOperationalFailure emits the step.operational_failure telemetry event
// with the correlation identifiers already flowing through the node context.
func (c *stepCore) emitOperationalFailure(ctx context.Context, env *contextdata.Envelope, kind euclotypes.FailureKind, action, actionTaken policyActionKind, askUnavailable bool) {
	sink := telemetry.TelemetryFromContext(ctx)
	if sink == nil && c.deps != nil {
		sink = c.deps.Telemetry
	}
	if sink == nil {
		return
	}
	metadata := map[string]any{
		"step_id":      c.step.ID,
		"paradigm":     c.step.Paradigm,
		"kind":         string(kind),
		"on_error":     string(action),
		"action_taken": string(actionTaken),
	}
	if askUnavailable {
		metadata["ask_unavailable"] = true
	}
	if env != nil {
		metadata["task_id"] = env.TaskIDSnapshot()
	}
	event := telemetry.Event{
		Type:      telemetry.EventStepOperationalFailure,
		Message:   "step operational failure",
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(ctx, &event)
	sink.Emit(event)
}

// markFallbackActivated records that an authored fallback step took over and
// emits step.fallback_activated. It is called by the fallback node itself so
// the activation is observable on the step that actually ran.
func (c *stepCore) markFallbackActivated(ctx context.Context, env *contextdata.Envelope, result *execution.Result) {
	if c.step.FallbackFor == "" {
		return
	}
	if env != nil {
		state.SetFallbackTaken(env, true)
	}
	if result != nil {
		if result.Metadata == nil {
			result.Metadata = map[string]any{}
		}
		result.Metadata["fallback_taken"] = true
		result.Metadata["fallback_for"] = c.step.FallbackFor
	}
	sink := telemetry.TelemetryFromContext(ctx)
	if sink == nil && c.deps != nil {
		sink = c.deps.Telemetry
	}
	if sink == nil {
		return
	}
	event := telemetry.Event{
		Type:      telemetry.EventStepFallbackActivated,
		Message:   "authored fallback activated",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"step_id":      c.step.ID,
			"fallback_for": c.step.FallbackFor,
			"paradigm":     c.step.Paradigm,
		},
	}
	telemetry.StampCorrelation(ctx, &event)
	sink.Emit(event)
}

// operationalFailureClass reports the typed operational failure class recorded
// on a step result, or "" when the result is a success or carries no class.
// The graph fallback edge uses this so only classified operational failures
// trigger an authored fallback (D7).
func operationalFailureClass(result *execution.Result) string {
	if result == nil || result.Success || result.Metadata == nil {
		return ""
	}
	kind, _ := result.Metadata[operationalFailureKindMetadata].(string)
	return strings.TrimSpace(kind)
}
