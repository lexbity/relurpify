package reporting

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

type captureTelemetry struct {
	events []core.Event
}

func (c *captureTelemetry) Emit(event core.Event) {
	c.events = append(c.events, event)
}

func TestTelemetryNodeExecute(t *testing.T) {
	node := NewTelemetryNode("telemetry1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetExecutionCompleted(env, true)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if result["outcome_category"] != "success" {
		t.Errorf("Expected outcome_category success, got %v", result["outcome_category"])
	}
}

func TestTelemetryNodeID(t *testing.T) {
	node := NewTelemetryNode("telemetry1")

	if node.ID() != "telemetry1" {
		t.Errorf("Expected ID telemetry1, got %s", node.ID())
	}
}

func TestTelemetryNodeType(t *testing.T) {
	node := NewTelemetryNode("telemetry1")

	if node.Type() != "telemetry" {
		t.Errorf("Expected Type telemetry, got %s", node.Type())
	}
}

func TestTelemetryNodeWritesToEnvelope(t *testing.T) {
	node := NewTelemetryNode("telemetry1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetExecutionCompleted(env, true)

	_, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	category, ok := state.GetOutcomeCategory(env)
	if !ok {
		t.Error("Expected outcome.category in envelope")
	}

	if category != "success" {
		t.Errorf("Expected outcome.category success, got %v", category)
	}

	reason, ok := contextdata.GetTyped[string](env, "euclo.outcome.reason")
	if !ok {
		t.Error("Expected outcome.reason in envelope")
	}

	if reason != "execution completed successfully" {
		t.Errorf("Expected outcome.reason execution completed successfully, got %v", reason)
	}
}

func TestTelemetryNodeIncompleteExecution(t *testing.T) {
	node := NewTelemetryNode("telemetry1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetExecutionCompleted(env, false)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["outcome_category"] != "cancelled" {
		t.Errorf("Expected outcome_category cancelled, got %v", result["outcome_category"])
	}
}

func TestEmitRouteSelected_RecordsEventType(t *testing.T) {
	sink := &captureTelemetry{}
	ctx := core.WithTelemetry(context.Background(), sink)

	EmitRouteSelected(ctx, "task-1", "session-1", "query", "capability", "euclo:cap.ast_query", 3, false)

	if len(sink.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(sink.events))
	}
	event := sink.events[0]
	if event.Type != core.EventType(EventTypeRouteSelected) {
		t.Fatalf("expected event type %q, got %q", EventTypeRouteSelected, event.Type)
	}
	if event.Metadata["family"] != "query" {
		t.Fatalf("expected family metadata, got %#v", event.Metadata["family"])
	}
	if event.Metadata["route_id"] != "euclo:cap.ast_query" {
		t.Fatalf("expected route_id metadata, got %#v", event.Metadata["route_id"])
	}
	if event.Metadata["candidate_count"] != 3 {
		t.Fatalf("expected candidate_count metadata, got %#v", event.Metadata["candidate_count"])
	}
}

func TestEmitRouteCompleted_IncludesElapsed(t *testing.T) {
	sink := &captureTelemetry{}
	ctx := core.WithTelemetry(context.Background(), sink)

	EmitRouteCompleted(ctx, "task-1", "session-1", "thoughtrecipe", "euclo:thoughtrecipe.default", RouteOutcomeSuccess, []string{"artifact"}, 125*time.Millisecond)

	if len(sink.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(sink.events))
	}
	event := sink.events[0]
	if event.Type != core.EventType(EventTypeRouteCompleted) {
		t.Fatalf("expected completed event type, got %q", event.Type)
	}
	if got := event.Metadata["elapsed_ms"]; got != int64(125) && got != 125 {
		t.Fatalf("expected elapsed_ms metadata, got %#v", got)
	}
	if got := event.Metadata["outcome"]; got != string(RouteOutcomeSuccess) {
		t.Fatalf("expected outcome metadata, got %#v", got)
	}
}

func TestEmitRouteUnavailable_IncludesReason(t *testing.T) {
	sink := &captureTelemetry{}
	ctx := core.WithTelemetry(context.Background(), sink)

	EmitRouteUnavailable(ctx, "task-1", "session-1", "euclo:cap.targeted_refactor", "unavailable:tool_not_enabled", "tool dependency missing: file_write")

	if len(sink.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(sink.events))
	}
	event := sink.events[0]
	if event.Type != core.EventType(EventTypeRouteUnavailable) {
		t.Fatalf("expected unavailable event type, got %q", event.Type)
	}
	if event.Metadata["availability"] != "unavailable:tool_not_enabled" {
		t.Fatalf("expected availability metadata, got %#v", event.Metadata["availability"])
	}
	if event.Metadata["reason"] != "tool dependency missing: file_write" {
		t.Fatalf("expected reason metadata, got %#v", event.Metadata["reason"])
	}
}

func TestEmitRouteDryRun_FullReport(t *testing.T) {
	sink := &captureTelemetry{}
	ctx := core.WithTelemetry(context.Background(), sink)
	report := map[string]any{"selected_route": "euclo:cap.ast_query", "candidate_count": 2}

	EmitRouteDryRun(ctx, "task-1", "session-1", report)

	if len(sink.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(sink.events))
	}
	event := sink.events[0]
	if event.Type != core.EventType(EventTypeRouteDryRun) {
		t.Fatalf("expected dry-run event type, got %q", event.Type)
	}
	if event.Metadata["report"] == nil {
		t.Fatal("expected report metadata")
	}
}

func TestEmitRouteFallback_BothIDs(t *testing.T) {
	sink := &captureTelemetry{}
	ctx := core.WithTelemetry(context.Background(), sink)

	EmitRouteFallback(ctx, "task-1", "session-1", "euclo:cap.primary", "euclo:cap.fallback", "primary unavailable")

	if len(sink.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(sink.events))
	}
	event := sink.events[0]
	if event.Type != core.EventType(EventTypeRouteFallback) {
		t.Fatalf("expected fallback event type, got %q", event.Type)
	}
	if event.Metadata["primary_id"] != "euclo:cap.primary" {
		t.Fatalf("expected primary_id metadata, got %#v", event.Metadata["primary_id"])
	}
	if event.Metadata["fallback_id"] != "euclo:cap.fallback" {
		t.Fatalf("expected fallback_id metadata, got %#v", event.Metadata["fallback_id"])
	}
}

func TestEmitRouteSelected_NilTelemetry_NoOp(t *testing.T) {
	EmitRouteSelected(context.Background(), "task-1", "session-1", "query", "capability", "euclo:cap.ast_query", 1, false)
}

func TestEucloTelemetry_EmitsTypedEvents(t *testing.T) {
	sink := &captureTelemetry{}
	telemetry := NewEucloTelemetry(sink)
	ctx := context.Background()

	telemetry.EmitIntakeComplete(ctx, EventIntakeComplete{
		EventHeader:   EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 1},
		WinningFamily: "analysis",
		Confidence:    0.9,
		Ambiguous:     false,
	})
	telemetry.EmitFamilySelected(ctx, EventFamilySelected{
		EventHeader: EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 2},
		FamilyID:    "analysis",
		Confidence:  0.8,
		Keywords:    []string{"explain"},
	})
	telemetry.EmitIngestionComplete(ctx, EventIngestionComplete{
		EventHeader: EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 3},
		Mode:        "files_only",
		FileCount:   2,
		ChunkCount:  5,
	})
	telemetry.EmitStreamRequested(ctx, EventStreamRequested{
		EventHeader: EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 4},
		Query:       "summarize",
		MaxTokens:   1024,
		Mode:        "analysis",
	})
	telemetry.EmitCapabilityClassified(ctx, EventCapabilityClassified{
		EventHeader:  EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 5},
		FamilyID:     "analysis",
		Capabilities: []string{"query"},
		Operator:     "llm",
		LLMCalls:     1,
	})
	telemetry.EmitRouteSelected(ctx, EventRouteSelected{
		EventHeader:    EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 6},
		FamilyID:       "analysis",
		RouteKind:      "capability",
		RouteID:        "euclo:cap.ast_query",
		CandidateCount: 2,
		FallbackTaken:  false,
	})
	telemetry.EmitGateResult(ctx, EventGateResult{
		EventHeader: EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 7},
		GateID:      "gate-1",
		Passed:      true,
		Decision:    "allow",
	})
	telemetry.EmitFrameEmitted(ctx, EventFrameEmitted{
		EventHeader: EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 8},
		FrameID:     "frame-1",
		FrameType:   "hitl_approval",
		SlotCount:   2,
	})
	telemetry.EmitFrameResolved(ctx, EventFrameResolved{
		EventHeader: EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 9},
		FrameID:     "frame-1",
		ChosenSlot:  "approve",
		RespondedBy: "user-1",
	})
	telemetry.EmitJobSubmitted(ctx, EventJobSubmitted{
		EventHeader:   EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 10},
		JobID:         "job-1",
		RouteID:       "euclo:cap.ast_query",
		ExecutionMode: "background",
	})
	telemetry.EmitJobCompleted(ctx, EventJobCompleted{
		EventHeader: EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 11},
		JobID:       "job-1",
		Status:      "completed",
		DurationMs:  42,
	})
	telemetry.EmitStepCompleted(ctx, EventStepCompleted{
		EventHeader:     EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 12},
		StepID:          "step-1",
		ThoughtRecipeID: "thoughtrecipe-1",
		Paradigm:        "thoughtrecipe",
		Success:         true,
		DurationMs:      17,
	})
	telemetry.EmitExecutionComplete(ctx, EventExecutionComplete{
		EventHeader: EventHeader{TaskID: "task-1", SessionID: "session-1", Seq: 13},
		Outcome:     "success",
		OutcomeKind: "execution completed successfully",
		StepCount:   3,
		LLMCalls:    1,
		TokenUsage:  128,
	})

	if got := len(sink.events); got != 13 {
		t.Fatalf("expected 13 events, got %d", got)
	}
	if sink.events[0].Type != core.EventType(EventTypeIntakeComplete) {
		t.Fatalf("unexpected first event type %q", sink.events[0].Type)
	}
	if sink.events[12].Metadata["outcome"] != "success" {
		t.Fatalf("expected final outcome metadata, got %#v", sink.events[12].Metadata["outcome"])
	}
}

func TestTelemetryNodeEmitsProjectionMutationEvent(t *testing.T) {
	sink := &captureTelemetry{}
	node := NewTelemetryNode("telemetry1")
	node.telemetry = NewEucloTelemetry(sink)

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetExecutionCompleted(env, true)
	contextdata.SetTyped(env, "euclo.projection.plan_id", "plan-1")
	contextdata.SetTyped(env, "euclo.projection.mutation_result", &graphdb.MutationResult{
		StableID:     "mutation-1",
		Scope:        graphdb.MutationScopeProjection,
		Status:       graphdb.MutationStatusCreated,
		Reason:       "projection pass completed",
		CreatedIDs:   []string{"node-1"},
		RecordIDs:    []string{"node-1"},
		StateVersion: 3,
	})

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}
	if got := len(sink.events); got != 2 {
		t.Fatalf("expected 2 events, got %d", got)
	}
	projection, ok := result["projection"].(map[string]any)
	if !ok {
		t.Fatalf("expected projection summary in result, got %#v", result["projection"])
	}
	if projection["plan_id"] != "plan-1" {
		t.Fatalf("unexpected projection plan id: %#v", projection["plan_id"])
	}
	if sink.events[0].Type != core.EventType(EventTypeProjectionCompleted) {
		t.Fatalf("expected projection event first, got %q", sink.events[0].Type)
	}
	if sink.events[0].Metadata["mutation_stable_id"] != "mutation-1" {
		t.Fatalf("unexpected mutation stable id: %#v", sink.events[0].Metadata["mutation_stable_id"])
	}
	if sink.events[0].Metadata["plan_id"] != "plan-1" {
		t.Fatalf("unexpected plan id: %#v", sink.events[0].Metadata["plan_id"])
	}
	if sink.events[1].Type != core.EventType(EventTypeExecutionComplete) {
		t.Fatalf("expected execution event second, got %q", sink.events[1].Type)
	}
}

func TestTelemetryNodeEmitsClarificationCompletionEvent(t *testing.T) {
	sink := &captureTelemetry{}
	node := NewTelemetryNode("telemetry1")
	node.telemetry = NewEucloTelemetry(sink)

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetExecutionCompleted(env, true)
	contextdata.SetTyped(env, intentcontext.ClarificationActiveThoughtRecipeKey, "euclo.thoughtrecipe.intent.clarify")

	if _, err := node.Execute(context.Background(), env); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if got := len(sink.events); got != 2 {
		t.Fatalf("expected 2 events, got %d", got)
	}
	if sink.events[1].Type != core.EventType(EventTypeClarificationCompleted) {
		t.Fatalf("expected clarification completion event, got %q", sink.events[1].Type)
	}
	if sink.events[1].Metadata["thoughtrecipe_id"] != "euclo.thoughtrecipe.intent.clarify" {
		t.Fatalf("unexpected thoughtrecipe id: %#v", sink.events[1].Metadata["thoughtrecipe_id"])
	}
}

func TestEventStructs_JSONMarshal(t *testing.T) {
	ev := EventExecutionComplete{
		EventHeader: EventHeader{
			TaskID:     "task-1",
			SessionID:  "session-1",
			Seq:        99,
			OccurredAt: time.Unix(10, 0).UTC(),
		},
		Outcome:     "partial_success",
		OutcomeKind: "partial",
		StepCount:   4,
		LLMCalls:    2,
		TokenUsage:  512,
	}

	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatal("expected valid json")
	}
}
