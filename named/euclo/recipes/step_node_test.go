package recipe

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/prompt/prompttest"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
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
	env.SetWorkingValue("euclo.task_envelope.instruction", "find symbols", contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.task_envelope.family_hint", "query", contextdata.MemoryClassTask)

	step := ExecutionStep{
		ID:           "step1",
		CapabilityID: "euclo:cap.ast_query",
		Bindings: map[string]string{
			"query": "euclo.task_envelope.instruction",
		},
		Step: RecipeStep{
			ID:           "step1",
			CapabilityID: "euclo:cap.ast_query",
			Config: map[string]any{
				"limit": 5,
			},
			Bindings: map[string]string{
				"query": "euclo.task_envelope.instruction",
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

func TestRecipeStepNodeBuildRuntimeContextClarificationState(t *testing.T) {
	env := contextdata.NewEnvelope("task-clarify", "session-clarify")
	state := intentcontext.NewState("task-clarify", "session-clarify")
	state.StateVersion = 11
	state.CurrentTurnID = "turn-11"
	state.ActiveRecipeID = "recipe.intent.clarify"
	state.GroundedAnchors = []retrieval.AnchorRef{
		{AnchorID: "anchor-11", ChunkID: "chunk-11", Term: "Envelope", Definition: "type anchor", Class: "clarified_entity", Active: true},
	}
	if err := intentcontext.NewStateStore().Write(context.Background(), env, state); err != nil {
		t.Fatalf("write clarification state: %v", err)
	}

	step := ExecutionStep{
		ID:       "clarify.step",
		Paradigm: "euclo",
		Prompt:   "Which module should be updated?",
		Step: RecipeStep{
			ID:     "clarify.step",
			Prompt: "Which module should be updated?",
		},
	}
	node := NewRecipeStepNode("clarify.step", agentenv.WorkspaceEnvironment{}, step)

	rctx := node.buildRuntimeContext(env)
	clarState, ok := rctx.State[intentcontext.ClarificationStateKey].(*intentcontext.ClarificationState)
	if !ok {
		t.Fatalf("expected clarification state in prompt runtime, got %#v", rctx.State[intentcontext.ClarificationStateKey])
	}
	if clarState.StateVersion != 11 {
		t.Fatalf("expected state version 11, got %d", clarState.StateVersion)
	}
	if got := rctx.State["euclo.intent.clarification.current_turn_id"]; got != "turn-11" {
		t.Fatalf("expected turn id in runtime state, got %#v", got)
	}
	if got := rctx.State["euclo.intent.clarification.grounded_anchor_ids"]; got == nil {
		t.Fatal("expected grounded anchor ids in runtime state")
	}
	if got := rctx.Variables["instruction"]; got == "" {
		t.Fatal("expected instruction variable to be populated")
	}
}

func TestRecipeStepNodeUsesRegistryPromptID(t *testing.T) {
	registry := prompttest.New().With("euclo.intent.clarify.question.v1", "resolved prompt text")
	env := contextdata.NewEnvelope("task-1", "session-1")
	step := ExecutionStep{
		ID:       "clarify.step",
		Paradigm: "euclo",
		PromptID: "euclo.intent.clarify.question.v1",
		Prompt:   "inline fallback should not be used",
		Step: RecipeStep{
			ID:       "clarify.step",
			PromptID: "euclo.intent.clarify.question.v1",
			Prompt:   "inline fallback should not be used",
		},
	}
	node := NewRecipeStepNode("clarify.step", agentenv.WorkspaceEnvironment{PromptRegistry: registry}, step)

	task, err := node.buildTask(env)
	if err != nil {
		t.Fatalf("buildTask: %v", err)
	}
	if task.Instruction != "resolved prompt text" {
		t.Fatalf("expected registry-backed prompt content, got %q", task.Instruction)
	}
	if got := task.Context["prompt_id"]; got != "euclo.intent.clarify.question.v1" {
		t.Fatalf("expected prompt_id in task context, got %#v", got)
	}
	if got := task.Metadata["recipe_step_type"]; got != "" {
		t.Fatalf("expected step type metadata to be empty for ad hoc step, got %#v", got)
	}
	if got, ok := env.GetWorkingValue("euclo.recipe.step.clarify.step.type"); !ok || got != "" {
		t.Fatalf("expected step type metadata in envelope, got %#v (ok=%v)", got, ok)
	}
	if got, ok := env.GetWorkingValue("euclo.recipe.step.clarify.step.prompt_id"); !ok || got != "euclo.intent.clarify.question.v1" {
		t.Fatalf("expected prompt_id metadata in envelope, got %#v (ok=%v)", got, ok)
	}
}

func TestRecipeStepNodeWritesClarificationMetadata(t *testing.T) {
	env := contextdata.NewEnvelope("task-meta", "session-meta")
	step := ExecutionStep{
		ID:   "extract.step",
		Type: string(ClarificationStepTypeExtract),
		ClarificationConfig: &ClarificationStepConfig{
			OutputSchemaID:   "clarification.answer.v1",
			ValidationMode:   "strict",
			RequiredFields:   []string{"answer"},
			AllowedStatuses:  []intentcontext.ClarificationStepStatus{intentcontext.ClarificationStepStatusSucceeded},
			StateWriteKeys:   []string{"euclo.intent.clarification.confirmed_entities"},
			ProjectionPolicy: "apply",
		},
		Step: RecipeStep{
			ID:   "extract.step",
			Type: string(ClarificationStepTypeExtract),
			Config: map[string]any{
				"output_schema_id": "clarification.answer.v1",
			},
		},
	}

	node := NewRecipeStepNode("extract.step", agentenv.WorkspaceEnvironment{}, step)
	task, err := node.buildTask(env)
	if err != nil {
		t.Fatalf("buildTask failed: %v", err)
	}
	if got := task.Metadata["recipe_step_type"]; got != string(ClarificationStepTypeExtract) {
		t.Fatalf("expected step type metadata, got %#v", got)
	}
	if got := task.Metadata["recipe_clarification_type"]; got != string(ClarificationStepTypeExtract) {
		t.Fatalf("expected clarification type metadata, got %#v", got)
	}
	if got, ok := env.GetWorkingValue("euclo.recipe.step.extract.step.clarification_schema_id"); !ok || got != "clarification.answer.v1" {
		t.Fatalf("expected clarification schema metadata in envelope, got %#v (ok=%v)", got, ok)
	}
	if got, ok := env.GetWorkingValue("euclo.recipe.step.extract.step.clarification_config"); !ok || got == nil {
		t.Fatalf("expected clarification config in envelope, got %#v (ok=%v)", got, ok)
	}
	if got, ok := env.GetWorkingValue("euclo.recipe.step.extract.step.clarification_allowed_statuses"); !ok || got == nil {
		t.Fatalf("expected allowed statuses in envelope, got %#v (ok=%v)", got, ok)
	}
	rctx := node.buildRuntimeContext(env)
	if got := rctx.State["recipe_step_type"]; got != string(ClarificationStepTypeExtract) {
		t.Fatalf("expected runtime step type metadata, got %#v", got)
	}
	if got := rctx.State["recipe_clarification_schema_id"]; got != "clarification.answer.v1" {
		t.Fatalf("expected runtime clarification schema id, got %#v", got)
	}
}
