package reporting

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// TestRecordModelUsageAccumulates pins the D7 accumulator: one entry per
// model invocation (retries included), tokens summed across calls.
func TestRecordModelUsageAccumulates(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")

	contextdata.RecordModelUsage(env.State(), model.TokenUsage{PromptTokens: 100, CompletionTokens: 20})
	contextdata.RecordModelUsage(env.State(), model.TokenUsage{PromptTokens: 50, CompletionTokens: 30})

	usage := contextdata.ModelUsageFromEnv(env)
	if usage.Calls != 2 || usage.TokenUsage() != 200 {
		t.Fatalf("usage = %+v, want 2 calls / 200 tokens", usage)
	}
}

// TestTelemetryNodeReportsHonestCounters pins AC-7: the completion emitters
// read the run's accumulated model usage and the step aggregate — the
// hardcoded `llm_calls: 0, token_usage: 0` literals are gone.
func TestTelemetryNodeReportsHonestCounters(t *testing.T) {
	sink := &captureTelemetry{}
	node := NewTelemetryNode("telemetry-1")
	ctx := telemetry.WithTelemetry(context.Background(), sink)

	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetExecutionCompleted(env, true)
	contextdata.RecordModelUsage(env.State(), model.TokenUsage{PromptTokens: 120, CompletionTokens: 80})
	contextdata.RecordModelUsage(env.State(), model.TokenUsage{PromptTokens: 30, CompletionTokens: 10})
	contextdata.SetTyped(env, "euclo.execution.step_total", 2)

	if _, err := node.Execute(ctx, env); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var complete *telemetry.Event
	for i := range sink.events {
		if sink.events[i].Type == telemetry.EventType(EventTypeExecutionComplete) {
			complete = &sink.events[i]
		}
	}
	if complete == nil {
		t.Fatal("execution.complete not emitted")
	}
	// Event metadata round-trips through JSON encoding: numerics arrive as
	// float64.
	if got := asFloat(complete.Metadata["llm_calls"]); got != 2 {
		t.Fatalf("llm_calls = %v, want 2 (the fake model's invocation count)", got)
	}
	if got := asFloat(complete.Metadata["token_usage"]); got != 240 {
		t.Fatalf("token_usage = %v, want 240 (sum of prompt+completion)", got)
	}
	if got := asFloat(complete.Metadata["step_count"]); got != 2 {
		t.Fatalf("step_count = %v, want 2 (the step aggregate)", got)
	}
}

// TestRouteCompletedEmittedAtCompletionWithRealElapsed pins D9/FR-11: the
// completion node emits euclo.route.completed with the real elapsed since
// route selection and the actual outcome — the selection-time emit with
// elapsed_ms:0 is gone (grep-gated in dispatching.go).
func TestRouteCompletedEmittedAtCompletionWithRealElapsed(t *testing.T) {
	sink := &captureTelemetry{}
	node := NewTelemetryNode("telemetry-1")
	ctx := telemetry.WithTelemetry(context.Background(), sink)

	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetExecutionCompleted(env, true)
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:       euclotypes.RouteKindThoughtRecipe,
		ThoughtRecipeID: "euclo.thoughtrecipe.default",
	})
	state.SetRouteSelectedAt(env, time.Now().Add(-150*time.Millisecond))

	if _, err := node.Execute(ctx, env); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var completed *telemetry.Event
	for i := range sink.events {
		if sink.events[i].Type == telemetry.EventType(EventTypeRouteCompleted) {
			completed = &sink.events[i]
		}
	}
	if completed == nil {
		t.Fatal("route.completed not emitted at completion")
	}
	// Raw events carry int64; marshaled payloads arrive as float64.
	elapsedMs := asFloat(completed.Metadata["elapsed_ms"])
	if elapsedMs <= 0 {
		t.Fatalf("elapsed_ms = %v, want the real elapsed (>0)", completed.Metadata["elapsed_ms"])
	}
	if completed.Metadata["outcome"] != string(RouteOutcomeSuccess) {
		t.Fatalf("outcome = %v, want success", completed.Metadata["outcome"])
	}
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		return -1
	}
}
