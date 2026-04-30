package reporting

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
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
	env.SetWorkingValue("euclo.execution.completed", true, contextdata.MemoryClassTask)

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
	env.SetWorkingValue("euclo.execution.completed", true, contextdata.MemoryClassTask)

	_, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	category, ok := env.GetWorkingValue("euclo.outcome.category")
	if !ok {
		t.Error("Expected outcome.category in envelope")
	}

	if category != "success" {
		t.Errorf("Expected outcome.category success, got %v", category)
	}

	reason, ok := env.GetWorkingValue("euclo.outcome.reason")
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
	env.SetWorkingValue("euclo.execution.completed", false, contextdata.MemoryClassTask)

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

	EmitRouteCompleted(ctx, "task-1", "session-1", "recipe", "euclo:recipe.default", RouteOutcomeSuccess, []string{"artifact"}, 125*time.Millisecond)

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
