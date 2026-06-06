package testsuite

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func TestEndToEndRootRouteOnlyThoughtRecipeExecution(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "review.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.code_review", "euclo:cap.capture", "euclo:cap.consume")
	thoughtrecipes := newThoughtRecipeRegistry(t, &surface.ThoughtRecipe{
		ID:       "euclo.thoughtrecipe.review",
		Name:     "review",
		Metadata: surface.ThoughtRecipeMetadata{Name: "review"},
	})
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnvWithModel(caps, stubLanguageModel{})),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithThoughtRecipeRegistry(thoughtrecipes),
	)

	env := contextdata.NewEnvelope("task-thoughtrecipe", "session-thoughtrecipe")
	seedTask(env, "review the auth package", "review.go")
	euclostate.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: "euclo.thoughtrecipe.review",
	})
	runPreIngestion(t, env, dir, []string{"review.go"})

	if err := graph.Execute(ctxWithTrigger(context.Background()), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "thoughtrecipe" {
		t.Fatalf("execution kind = %q, want thoughtrecipe", got)
	}
	if got := mustStringValue(t, env, "euclo.execution.thoughtrecipe_id"); got != "euclo.thoughtrecipe.review" {
		t.Fatalf("thoughtrecipe id = %q, want euclo.thoughtrecipe.review", got)
	}
	if got := mustStringValue(t, env, "euclo.fork.branch"); got != "thoughtrecipe_execution" {
		t.Fatalf("fork branch = %q, want thoughtrecipe_execution", got)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected thoughtrecipe execution to complete")
	}
	if got, ok := contextdata.GetTyped[any](env, euclostate.KeyExecutionCapabilityID); ok && got != "" {
		t.Fatalf("expected thoughtrecipe-only execution, got capability id %v", got)
	}
}

func TestEndToEndThoughtRecipeEmitsLifecycleEvents(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "review.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.code_review", "euclo:cap.capture", "euclo:cap.consume")
	thoughtrecipes := newThoughtRecipeRegistry(t, &surface.ThoughtRecipe{
		ID:       "euclo.thoughtrecipe.review",
		Name:     "review",
		Metadata: surface.ThoughtRecipeMetadata{Name: "review"},
	})

	// Wire a telemetry spy to capture emitted events.
	telemetrySpy := &captureTelemetry{}
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnvWithModel(caps, stubLanguageModel{})),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithThoughtRecipeRegistry(thoughtrecipes),
	)

	env := contextdata.NewEnvelope("task-lifecycle", "session-lifecycle")
	seedTask(env, "review the auth package", "review.go")
	euclostate.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: "euclo.thoughtrecipe.review",
	})
	runPreIngestion(t, env, dir, []string{"review.go"})

	// Execute with a context that has the telemetry spy.
	ctx := telemetry.WithTelemetry(context.Background(), telemetrySpy)
	if err := graph.Execute(ctx, env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	// Verify telemetry events were emitted.
	events := telemetrySpy.Events()
	if len(events) == 0 {
		t.Fatal("expected telemetry events, got none")
	}

	// Check for recipe.selected event.
	hasRecipeSelected := false
	hasStepStarted := false
	hasStepCompleted := false
	hasVerifyStarted := false
	hasExecutionComplete := false

	for _, ev := range events {
		switch ev.Type {
		case telemetry.EventType(reporting.EventTypeRecipeSelected):
			hasRecipeSelected = true
		case telemetry.EventType(reporting.EventTypeStepStartedEuclo):
			hasStepStarted = true
		case telemetry.EventType(reporting.EventTypeStepCompletedEuclo):
			hasStepCompleted = true
		case telemetry.EventType(reporting.EventTypeVerifyStarted):
			hasVerifyStarted = true
		case telemetry.EventType(reporting.EventTypeExecutionComplete):
			hasExecutionComplete = true
		}
	}

	if !hasRecipeSelected {
		t.Error("missing recipe.selected event")
	}
	if !hasStepStarted {
		t.Error("missing step.started event")
	}
	if !hasStepCompleted {
		t.Error("missing step.completed event")
	}
	if !hasVerifyStarted {
		t.Error("missing verify.started event")
	}
	if !hasExecutionComplete {
		t.Error("missing execution.complete event")
	}
}

// captureTelemetry is a telemetry spy that records emitted events.
type captureTelemetry struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (c *captureTelemetry) Emit(event telemetry.Event) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *captureTelemetry) Events() []telemetry.Event {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]telemetry.Event, len(c.events))
	copy(out, c.events)
	return out
}


