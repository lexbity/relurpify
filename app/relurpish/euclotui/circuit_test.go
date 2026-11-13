package euclotui

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	"codeburg.org/lexbit/relurpify/telemetry"
)

func TestV2CircuitUsesRealTelemetryAndDetachedGraphs(t *testing.T) {
	s := NewSurface().(*EucloSurface)
	projection := surface.RecipeProjection{RecipeID: "repair", Steps: []surface.ProjectedStep{
		{StepID: "read", Paradigm: "react", ToolScopes: []string{"read"}},
		{StepID: "edit", DependsOn: []string{"read"}},
	}, Groups: []surface.ProjectedGroup{{GroupID: "g", MemberStepIDs: []string{"read"}}}}
	s.router.ApplyExecutionEvent(ExecutionEvent{Type: reporting.EventTypeRecipeSelected, Payload: map[string]any{"recipe": projection}})
	projection.Steps[1].DependsOn[0] = "input-mutated"
	projection.Groups[0].MemberStepIDs[0] = "input-mutated"
	s.ApplyExecEvent(stepEvent("euclo.step.started", "read", 1, false))
	s.ApplyExecEvent(stepEvent("euclo.step.completed", "read", 2, true))
	got := s.ExecutionSnapshot()
	if got.Steps[0].Status != "done" || got.Steps[1].Status != "" || got.Steps[1].Dependencies[0] != "read" {
		t.Fatalf("real events/projection did not produce honest facts: %+v", got)
	}
	got.Steps[1].Dependencies[0] = "output-mutated"
	got.Steps[0].ToolScopes[0] = "output-mutated"
	snapshot := s.router.Snapshot()
	snapshot.Recipe.Groups[0].MemberStepIDs[0] = "output-mutated"
	again := s.ExecutionSnapshot()
	if again.Steps[1].Dependencies[0] != "read" || again.Steps[0].ToolScopes[0] != "read" || s.router.Snapshot().Recipe.Groups[0].MemberStepIDs[0] != "read" {
		t.Fatal("execution reader or router leaked mutable nested graphs")
	}
	s.router.ApplyExecutionEvent(ExecutionEvent{Type: reporting.EventTypeRecipeSelected, Payload: map[string]any{"recipe": projection}})
	if s.ExecutionSnapshot().Steps[0].Status != "" {
		t.Fatal("new recipe selection inherited prior-run success")
	}
}

func TestV2CircuitConsumesActualEucloEmitter(t *testing.T) {
	sink := telemetry.NewBroadcastSink()
	channel, unsubscribe := sink.Subscribe(8)
	defer unsubscribe()
	emitter := reporting.NewEucloTelemetry(sink)
	emitter.EmitRecipeSelected(context.Background(), reporting.EventRecipeSelected{
		EventHeader: reporting.EventHeader{TaskID: "real-shape"},
		RecipeID:    "repair",
		Recipe:      surface.RecipeProjection{RecipeID: "repair", Name: "Repair", Steps: []surface.ProjectedStep{{StepID: "read", Paradigm: "react"}}},
	})
	s := NewSurface().(*EucloSurface)
	// Includes the real producer's JSON metadata conversion and sink sequencing.
	s.ApplyExecEvent(<-channel)
	s.ApplyExecEvent(stepEvent("euclo.step.started", "read", 2, false))
	got := s.ExecutionSnapshot()
	if got.RecipeID != "repair" || len(got.Steps) != 1 || got.Steps[0].Status != "active" {
		t.Fatalf("actual telemetry contract did not reach the execution canvas: %+v", got)
	}
}

func TestV2CircuitInvalidTelemetryRemainsVisible(t *testing.T) {
	s := NewSurface().(*EucloSurface)
	s.ApplyExecEvent(telemetry.Event{Type: "euclo.recipe.selected", Metadata: map[string]any{
		"recipe": map[string]any{"recipe_id": "bad", "steps": []any{map[string]any{"step_id": "x"}, map[string]any{"step_id": "x"}}},
	}})
	got := s.ExecutionSnapshot()
	if got.RecipeID != "" || !strings.Contains(got.Notice, "unique") {
		t.Fatalf("invalid graph silently appeared valid or vanished: %+v", got)
	}
}

func TestV2SequenceGuardIsScopedToTask(t *testing.T) {
	s := NewSurface().(*EucloSurface)
	first := stepEvent("euclo.step.started", "same-step", 100, false)
	s.ApplyExecEvent(first)
	second := stepEvent("euclo.step.completed", "same-step", 101, true)
	s.ApplyExecEvent(second)
	first.TaskID, first.Seq = "second-task", 1
	s.ApplyExecEvent(first)
	if s.router.Snapshot().StepRuntime["same-step"].Status != surface.StepActive {
		t.Fatal("new task's sequence restart was dropped as a stale event")
	}
}
