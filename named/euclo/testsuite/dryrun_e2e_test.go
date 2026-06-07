package testsuite

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type countingCapabilityHandler struct {
	mu    sync.Mutex
	count int
	id    string
}

func (h *countingCapabilityHandler) Descriptor(context.Context, *contextdata.Envelope) capability.CapabilityDescriptor {
	return capability.CapabilityDescriptor{
		ID:            h.id,
		Name:          h.id,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  capability.AvailabilitySpec{Available: true},
	}
}

func (h *countingCapabilityHandler) Invoke(context.Context, *contextdata.Envelope, map[string]interface{}) (*ports.CapabilityExecutionResult, error) {
	h.mu.Lock()
	h.count++
	h.mu.Unlock()
	return &ports.CapabilityExecutionResult{Success: true, Data: map[string]any{"ok": true}}, nil
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
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
	)

	env := contextdata.NewEnvelope("task-dryrun", "session-dryrun")
	seedTask(env, "add a cache to the handler", "dryrun.go")
	euclostate.SetDryRunMode(env, true)
	runPreIngestion(t, env, dir, []string{"dryrun.go"})
	rec := &recordingTelemetry{}

	if err := graph.Execute(telemetry.WithTelemetry(context.Background(), rec), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	if got := mustStringValue(t, env, "euclo.route.outcome"); got != "dry_run" {
		t.Fatalf("route outcome = %q, want dry_run", got)
	}
	if got, _ := euclostate.GetOutcomeCategory(env); got != "success" {
		t.Fatalf("outcome category = %q, want success", got)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected dry-run execution to be marked complete")
	}
	if handler.Count() != 0 {
		t.Fatalf("expected no capability execution during dry run, got %d invocations", handler.Count())
	}
	assertEventOrder(t, rec.types(), []telemetry.EventType{
		telemetry.EventType("euclo.route.dry_run"),
		telemetry.EventType("euclo.execution.complete"),
	})
}

func TestDryRunEndToEndSimulatedDryRunThoughtRecipeRoute(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "review.go", "package demo\n")

	thoughtrecipeID := "euclo.thoughtrecipe.review"
	caps := capability.NewRegistry()
	thoughtrecipes := newThoughtRecipeRegistry(t, &surface.ThoughtRecipe{
		ID:       thoughtrecipeID,
		Name:     "review",
		Metadata: surface.ThoughtRecipeMetadata{Name: "review"},
	})
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithThoughtRecipeRegistry(thoughtrecipes),
	)

	env := contextdata.NewEnvelope("task-dryrun-thoughtrecipe", "session-dryrun-thoughtrecipe")
	seedTask(env, "review the auth package", "review.go")
	euclostate.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: thoughtrecipeID,
	})
	euclostate.SetDryRunMode(env, true)
	runPreIngestion(t, env, dir, []string{"review.go"})
	rec := &recordingTelemetry{}

	if err := graph.Execute(telemetry.WithTelemetry(context.Background(), rec), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	if got := mustStringValue(t, env, "euclo.route.outcome"); got != "dry_run" {
		t.Fatalf("route outcome = %q, want dry_run", got)
	}
	routeSelection, ok := euclostate.GetRouteSelection(env)
	if !ok || routeSelection == nil {
		t.Fatalf("expected *RouteSelection, got %#v", routeSelection)
	}
	if routeSelection.RouteKind != "thoughtrecipe" || routeSelection.ThoughtRecipeID != thoughtrecipeID {
		t.Fatalf("route selection = %+v, want thoughtrecipe %s", routeSelection, thoughtrecipeID)
	}
	if got := euclostate.GetExecutionKind(env); got != "" {
		t.Fatalf("expected no execution.kind during dry run, got %v", got)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected dry-run execution to be marked complete")
	}
	assertEventOrder(t, rec.types(), []telemetry.EventType{
		telemetry.EventType("euclo.route.dry_run"),
		telemetry.EventType("euclo.execution.complete"),
	})
}

func capabilityRegistryWithHandler(t *testing.T, handler *countingCapabilityHandler) *capability.CapabilityRegistry {
	t.Helper()
	reg := capability.NewRegistry()
	if err := reg.RegisterInvocableCapability(handler); err != nil {
		t.Fatalf("register handler: %v", err)
	}
	return reg
}
