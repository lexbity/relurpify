package recipe

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type stubCapabilityHandler struct {
	id   string
	args map[string]interface{}
}

func (h *stubCapabilityHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	_ = ctx
	_ = env
	return core.CapabilityDescriptor{
		ID:            h.id,
		Name:          h.id,
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Availability:  core.AvailabilitySpec{Available: true},
	}
}

func (h *stubCapabilityHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	_ = ctx
	_ = env
	h.args = args
	return &contracts.CapabilityExecutionResult{
		Success: true,
		Data: map[string]interface{}{
			"answer": "ok",
		},
		Metadata: map[string]interface{}{
			"source": "stub",
		},
	}, nil
}

func TestRecipeStepNodeExecuteCapability(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	handler := &stubCapabilityHandler{id: "euclo:cap.ast_query"}
	if err := reg.RegisterInvocableCapability(handler); err != nil {
		t.Fatalf("register invocable capability: %v", err)
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("euclo.task.envelope.instruction", "find symbols", contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.task.envelope.family_hint", "query", contextdata.MemoryClassTask)

	step := ExecutionStep{
		ID:           "step1",
		CapabilityID: "euclo:cap.ast_query",
		Bindings: map[string]string{
			"query": "euclo.task.envelope.instruction",
		},
		Step: RecipeStep{
			ID:           "step1",
			CapabilityID: "euclo:cap.ast_query",
			Config: map[string]any{
				"limit": 5,
			},
			Bindings: map[string]string{
				"query": "euclo.task.envelope.instruction",
			},
		},
	}

	node := NewRecipeStepNode("step1.execute", agentenv.WorkspaceEnvironment{Registry: reg}, step)
	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if got := result.Data["capability_id"]; got != "euclo:cap.ast_query" {
		t.Fatalf("expected capability_id in result, got %v", got)
	}
	output, ok := result.Data["output"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected output map, got %T", result.Data["output"])
	}
	if output["answer"] != "ok" {
		t.Fatalf("expected output answer ok, got %v", output["answer"])
	}
	if handler.args["query"] != "find symbols" {
		t.Fatalf("expected binding to populate query, got %v", handler.args["query"])
	}
	if handler.args["limit"] != 5 {
		t.Fatalf("expected config to populate limit, got %v", handler.args["limit"])
	}
	if got, ok := env.GetWorkingValue("euclo.recipe.step.step1.success"); !ok || got != true {
		t.Fatalf("expected success marker in envelope, got %v (ok=%v)", got, ok)
	}
	if got, ok := env.GetWorkingValue("euclo.recipe.step.step1.result"); !ok {
		t.Fatal("expected step result in envelope")
	} else if data, ok := got.(map[string]any); !ok || data["capability_id"] != "euclo:cap.ast_query" {
		t.Fatalf("unexpected envelope step result: %#v", got)
	}
}
