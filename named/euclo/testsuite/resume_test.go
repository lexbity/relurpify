package testsuite

import (
	"context"
	"reflect"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	eucloagent "codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

func TestDryRunEndToEndSessionResumePreservesRoute(t *testing.T) {
	caps := capability.NewCapabilityRegistry()
	env := agentenv.WorkspaceEnvironment{
		Registry: caps,
	}
	agent := eucloagent.New(env)
	if err := agent.Initialize(nil); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := caps.RegisterInvocableCapability(&testCapabilityHandler{
		descriptor: core.CapabilityDescriptor{
			ID:            "euclo:cap.resume_route",
			Name:          "euclo:cap.resume_route",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		invoke: func(context.Context, *contextdata.Envelope, map[string]any) (*contracts.CapabilityExecutionResult, error) {
			return &contracts.CapabilityExecutionResult{
				Success: true,
				Data: map[string]any{
					"capability_id": "euclo:cap.resume_route",
					"result":        "resume:ok",
				},
			}, nil
		},
	}); err != nil {
		t.Fatalf("register resume capability: %v", err)
	}

	task := &core.Task{
		ID:          "task-resume",
		Type:        "euclo",
		Instruction: "resume execution without reclassification",
		Data:        map[string]any{},
		Context:     map[string]any{},
		Metadata:    map[string]any{},
	}
	envelope := contextdata.NewEnvelope("task-resume", "session-resume")
	seedTask(envelope, task.Instruction)
	euclostate.SetIntentClassification(envelope, &intake.IntentClassification{
		WinningFamily: "implementation",
		Confidence:    1.0,
	})
	euclostate.SetRouteSelection(envelope, &orchestrate.RouteSelection{
		RouteKind:    "capability",
		CapabilityID: "euclo:cap.resume_route",
	})

	result, err := agent.Execute(context.Background(), task, envelope)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful execution, got %#v", result)
	}
	if got := mustStringValue(t, envelope, "euclo.execution.kind"); got != "capability" {
		t.Fatalf("execution kind = %q, want capability", got)
	}
	selection, ok := envelope.GetWorkingValue("euclo.route_selection")
	if !ok {
		t.Fatal("expected route_selection in envelope")
	}
	routeSelection, ok := selection.(*orchestrate.RouteSelection)
	if !ok || routeSelection == nil {
		t.Fatalf("expected *RouteSelection, got %T", selection)
	}
	if routeSelection.CapabilityID != "euclo:cap.resume_route" {
		t.Fatalf("route capability = %q, want euclo:cap.resume_route", routeSelection.CapabilityID)
	}
	if !mustBoolValue(t, envelope, "euclo.execution.completed") {
		t.Fatal("expected execution to complete")
	}
	if hasResumeState(agent) {
		t.Fatal("expected agent resume state to be cleared after Execute")
	}
}

func hasResumeState(agent *eucloagent.Agent) bool {
	if agent == nil {
		return false
	}
	value := reflect.ValueOf(agent).Elem()
	classification := value.FieldByName("resumeClassification")
	route := value.FieldByName("resumeRouteSelection")
	return (!classification.IsNil()) || (!route.IsNil())
}
