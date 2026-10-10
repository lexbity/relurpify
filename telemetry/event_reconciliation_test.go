package telemetry

import (
	"testing"
	"time"
)

// The platform/observability Event vocabulary was absorbed into telemetry
// (S4). This file is the reconciliation table, as compile-time assertions:
//
//   - Field parity: telemetry.Event carries every field observability.Event
//     had. The observability Actor struct {Kind, ID, Label} flattened into
//     Event.Actor (= Actor.ID, string) plus Event.ActorKind (= Actor.Kind).
//     (Actor.Label had no reader outside the framework event log, which
//     keeps its own labeled actor type.)
//   - Const parity: the 30 observability EventType consts that collided
//     with existing telemetry consts were dropped — the telemetry spelling
//     won. The three without a counterpart moved: EventBudgetSnapshot and
//     EventSessionResetRequired already existed here as untyped consts;
//     EventTapeRecordFailed is newly declared in telemetry_types.go.
//
// A regression that deletes a merged field or an absorbed const fails the
// build here.
func TestEventReconciliation_FieldParity(t *testing.T) {
	// Every merged field must be assignable in one struct literal.
	ev := Event{
		Type:      EventLLMPrompt,
		SessionID: "s",
		RunID:     "r",
		TraceID:   "t",
		AgentID:   "a",
		NodeID:    "n",
		SpanID:    "sp",
		TaskID:    "task",
		Message:   "m",
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{"k": "v"},
		Seq:       1,
		Partition: "p",
		Payload:   []byte{1},
		Actor:     "actor-id",
		ActorKind: "agent",
	}
	if ev.Actor != "actor-id" || ev.ActorKind != "agent" {
		t.Fatalf("actor flattening broken: %+v", ev)
	}
}

func TestEventReconciliation_AbsorbedConsts(t *testing.T) {
	// Consts absorbed from platform/observability with no telemetry
	// counterpart. The collided ones (EventGraphStart, EventLLMPrompt, …)
	// keep the telemetry spelling and are exercised elsewhere.
	for name, val := range map[string]EventType{
		"EventTapeRecordFailed": EventTapeRecordFailed,
	} {
		if val == "" {
			t.Errorf("absorbed const %s must not be empty", name)
		}
	}
	if EventBudgetSnapshot != "budget.snapshot" || EventSessionResetRequired != "session.reset_required" {
		t.Errorf("pre-existing budget/reset consts must keep their spelling: %q %q", EventBudgetSnapshot, EventSessionResetRequired)
	}
}
