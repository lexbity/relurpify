package interaction

import (
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// Euclo interaction event types.
const (
	EventInteractionFrameEmit   core.EventType = "euclo_interaction_frame"
	EventInteractionResponse    core.EventType = "euclo_interaction_response"
	EventInteractionPhaseSkip   core.EventType = "euclo_interaction_phase_skip"
	EventInteractionTransition  core.EventType = "euclo_interaction_transition"
	EventInteractionBudgetLimit core.EventType = "euclo_interaction_budget_limit"
)

// InteractionTelemetry wraps core.Telemetry with typed interaction event helpers.
type InteractionTelemetry struct {
	inner core.Telemetry
}

// NewInteractionTelemetry creates an interaction telemetry wrapper.
// Returns a no-op wrapper if inner is nil.
func NewInteractionTelemetry(inner core.Telemetry) *InteractionTelemetry {
	return &InteractionTelemetry{inner: inner}
}

// EmitFrame records a frame emission event.
func (t *InteractionTelemetry) EmitFrame(frame InteractionFrame) {
	if t == nil || t.inner == nil {
		return
	}
	t.inner.Emit(core.Event{
		Type:      EventInteractionFrameEmit,
		Timestamp: time.Now(),
		Message:   "interaction frame emitted",
		Metadata: map[string]interface{}{
			"frame_kind": string(frame.Kind),
			"mode":       frame.Mode,
			"phase":      frame.Phase,
		},
	})
}

// EmitResponse records a user response event.
func (t *InteractionTelemetry) EmitResponse(resp UserResponse, phase, mode string, responseTime time.Duration) {
	if t == nil || t.inner == nil {
		return
	}
	t.inner.Emit(core.Event{
		Type:      EventInteractionResponse,
		Timestamp: time.Now(),
		Message:   "interaction response received",
		Metadata: map[string]interface{}{
			"action_id":     resp.ActionID,
			"phase":         phase,
			"mode":          mode,
			"response_ms":   responseTime.Milliseconds(),
			"has_text":      resp.Text != "",
			"has_selection": len(resp.Selections) > 0,
		},
	})
}

// EmitPhaseSkip records a phase skip event.
func (t *InteractionTelemetry) EmitPhaseSkip(phase, mode, reason string) {
	if t == nil || t.inner == nil {
		return
	}
	t.inner.Emit(core.Event{
		Type:      EventInteractionPhaseSkip,
		Timestamp: time.Now(),
		Message:   "phase skipped",
		Metadata: map[string]interface{}{
			"phase":  phase,
			"mode":   mode,
			"reason": reason,
		},
	})
}

// EmitTransition records a mode transition event.
func (t *InteractionTelemetry) EmitTransition(fromMode, toMode, trigger string, accepted bool) {
	if t == nil || t.inner == nil {
		return
	}
	t.inner.Emit(core.Event{
		Type:      EventInteractionTransition,
		Timestamp: time.Now(),
		Message:   "mode transition",
		Metadata: map[string]interface{}{
			"from_mode": fromMode,
			"to_mode":   toMode,
			"trigger":   trigger,
			"accepted":  accepted,
		},
	})
}

// EmitBudgetLimit records a budget exhaustion event.
func (t *InteractionTelemetry) EmitBudgetLimit(reason, phase, mode string) {
	if t == nil || t.inner == nil {
		return
	}
	t.inner.Emit(core.Event{
		Type:      EventInteractionBudgetLimit,
		Timestamp: time.Now(),
		Message:   "interaction budget limit reached",
		Metadata: map[string]interface{}{
			"reason": reason,
			"phase":  phase,
			"mode":   mode,
		},
	})
}
