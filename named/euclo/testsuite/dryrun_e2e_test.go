package testsuite

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type countingCapabilityHandler struct {
	mu    sync.Mutex
	count int
	id    string
}

func (h *countingCapabilityHandler) Descriptor(context.Context, *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            h.id,
		Name:          h.id,
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
		Availability:  core.AvailabilitySpec{Available: true},
	}
}

func (h *countingCapabilityHandler) Invoke(context.Context, *contextdata.Envelope, map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	h.mu.Lock()
	h.count++
	h.mu.Unlock()
	return &contracts.CapabilityExecutionResult{Success: true, Data: map[string]any{"ok": true}}, nil
}

func (h *countingCapabilityHandler) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

func TestDryRunEndToEndSimulatedDryRun(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "dryrun.go", "package demo\n")

	handler := &countingCapabilityHandler{id: "euclo:cap.targeted_refactor"}
	caps := capabilityRegistryWithHandler(t, handler)
	classifier := &mockTier2Classifier{
		responses: map[string]tier2Response{
			"implementation": {Sequence: []string{handler.id}, Operator: "OR"},
		},
	}
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithCapabilityClassifier(classifier),
	)

	env := contextdata.NewEnvelope("task-dryrun", "session-dryrun")
	seedTask(env, "add a cache to the handler", "dryrun.go")
	env.SetWorkingValue("euclo.dry_run_mode", true, contextdata.MemoryClassTask)
	runPreIngestion(t, env, dir, []string{"dryrun.go"})
	telemetry := &recordingTelemetry{}

	if err := graph.Execute(core.WithTelemetry(context.Background(), telemetry), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	if got := mustStringValue(t, env, "euclo.route.outcome"); got != "dry_run" {
		t.Fatalf("route outcome = %q, want dry_run", got)
	}
	if got := mustStringValue(t, env, "euclo.outcome.category"); got != "success" {
		t.Fatalf("outcome category = %q, want success", got)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected dry-run execution to be marked complete")
	}
	if handler.Count() != 0 {
		t.Fatalf("expected no capability execution during dry run, got %d invocations", handler.Count())
	}
	assertEventOrder(t, telemetry.types(), []core.EventType{
		core.EventType("euclo.route.dry_run"),
		core.EventType("euclo.execution.complete"),
	})
}

func TestDryRunEndToEndSimulatedDryRunThoughtRecipeRoute(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "review.go", "package demo\n")

	thoughtrecipeID := "euclo.thoughtrecipe.review"
	caps := capability.NewCapabilityRegistry()
	thoughtrecipes := newThoughtRecipeRegistry(t, &thoughtrecipepkg.ThoughtRecipe{
		ID:       thoughtrecipeID,
		Name:     "review",
		Metadata: thoughtrecipepkg.ThoughtRecipeMetadata{Name: "review"},
	})
	classifier := &mockTier2Classifier{
		responses: map[string]tier2Response{
			"review": {Sequence: []string{"euclo:cap.code_review"}, Operator: "OR"},
		},
	}
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithThoughtRecipeRegistry(thoughtrecipes),
		orchestrate.WithCapabilityClassifier(classifier),
	)

	env := contextdata.NewEnvelope("task-dryrun-thoughtrecipe", "session-dryrun-thoughtrecipe")
	seedTask(env, "review the auth package", "review.go")
	env.SetWorkingValue("euclo.route_selection", &orchestrate.RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: thoughtrecipeID,
	}, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.dry_run_mode", true, contextdata.MemoryClassTask)
	runPreIngestion(t, env, dir, []string{"review.go"})
	telemetry := &recordingTelemetry{}

	if err := graph.Execute(core.WithTelemetry(context.Background(), telemetry), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	if got := mustStringValue(t, env, "euclo.route.outcome"); got != "dry_run" {
		t.Fatalf("route outcome = %q, want dry_run", got)
	}
	selection, ok := env.GetWorkingValue("euclo.route_selection")
	if !ok {
		t.Fatal("expected route_selection in envelope")
	}
	routeSelection, ok := selection.(*orchestrate.RouteSelection)
	if !ok || routeSelection == nil {
		t.Fatalf("expected *RouteSelection, got %T", selection)
	}
	if routeSelection.RouteKind != "thoughtrecipe" || routeSelection.ThoughtRecipeID != thoughtrecipeID {
		t.Fatalf("route selection = %+v, want thoughtrecipe %s", routeSelection, thoughtrecipeID)
	}
	if got, ok := env.GetWorkingValue("euclo.execution.kind"); ok {
		t.Fatalf("expected no execution.kind during dry run, got %v", got)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected dry-run execution to be marked complete")
	}
	assertEventOrder(t, telemetry.types(), []core.EventType{
		core.EventType("euclo.route.dry_run"),
		core.EventType("euclo.execution.complete"),
	})
}

func capabilityRegistryWithHandler(t *testing.T, handler *countingCapabilityHandler) *capability.CapabilityRegistry {
	t.Helper()
	reg := capability.NewCapabilityRegistry()
	if err := reg.RegisterInvocableCapability(handler); err != nil {
		t.Fatalf("register handler: %v", err)
	}
	return reg
}
