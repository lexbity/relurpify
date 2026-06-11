package thoughtrecipe

import (
	"context"
	"sort"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	regpkg "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/prompt/prompttest"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

type stubCapabilityHandler struct {
	id   string
	args map[string]any
}

func (h *stubCapabilityHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	_ = ctx
	_ = env
	return descriptor.CapabilityDescriptor{
		ID:            h.id,
		Name:          h.id,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
}

func (h *stubCapabilityHandler) Invoke(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error) {
	_ = ctx
	_ = env
	h.args = args
	return &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"answer": "ok",
		},
		Metadata: map[string]any{
			"source": "stub",
		},
	}, nil
}

func TestThoughtRecipeStepNodeExecuteCapability(t *testing.T) {
	reg := regpkg.NewRegistry()
	handler := &stubCapabilityHandler{id: "euclo:cap.ast_query"}
	if err := reg.RegisterInvocableCapability(handler); err != nil {
		t.Fatalf("register invocable capability: %v", err)
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValueWithClass("euclo.task_envelope.instruction", "find symbols", contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("euclo.task_envelope.family_hint", "query", contextdata.MemoryClassTask)

	step := ExecutionStep{
		ID:           "step1",
		CapabilityID: "euclo:cap.ast_query",
		Step: surface.ThoughtRecipeStep{
			ID:           "step1",
			CapabilityID: "euclo:cap.ast_query",
			Config: map[string]any{
				"query": "find symbols",
				"limit": 5,
			},
		},
	}

	node := NewThoughtRecipeStepNode("step1.execute", &paradigm.Deps{Registry: reg}, step)
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
	if got, _ := execution.ResultField(result.Data, "capability_id"); got != "euclo:cap.ast_query" {
		t.Fatalf("expected capability_id in result, got %v", got)
	}
	outputValue, _ := execution.ResultField(result.Data, "output")
	output, ok := outputValue.(map[string]any)
	if !ok {
		t.Fatalf("expected output map, got %T", outputValue)
	}
	if output["answer"] != "ok" {
		t.Fatalf("expected output answer ok, got %v", output["answer"])
	}
	if handler.args["query"] != "find symbols" {
		t.Fatalf("expected config to populate query, got %v", handler.args["query"])
	}
	if handler.args["limit"] != 5 {
		t.Fatalf("expected config to populate limit, got %v", handler.args["limit"])
	}
	if got, ok := env.GetWorkingValue("euclo.execution.step.step1.success"); !ok || got != true {
		t.Fatalf("expected success marker in envelope, got %v (ok=%v)", got, ok)
	}
	if got, ok := env.GetWorkingValue("euclo.execution.step.step1.result"); !ok {
		t.Fatal("expected step result in envelope")
	} else if data, ok := got.(map[string]any); !ok || data["capability_id"] != "euclo:cap.ast_query" {
		t.Fatalf("unexpected envelope step result: %#v", got)
	}
}

func TestThoughtRecipeStepNodeBuildRuntimeContextClarificationState(t *testing.T) {
	env := contextdata.NewEnvelope("task-clarify", "session-clarify")
	state := intentcontext.NewState("task-clarify", "session-clarify")
	state.StateVersion = 11
	state.CurrentTurnID = "turn-11"
	state.ActiveThoughtRecipeID = "thoughtrecipe.intent.clarify"
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
		Step: surface.ThoughtRecipeStep{
			ID:     "clarify.step",
			Prompt: "Which module should be updated?",
		},
	}
	node := NewThoughtRecipeStepNode("clarify.step", &paradigm.Deps{}, step)

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
	if got := rctx.State["euclo.intent.clarification.active_thoughtrecipe_id"]; got != "thoughtrecipe.intent.clarify" {
		t.Fatalf("expected active thoughtrecipe id in runtime state, got %#v", got)
	}
	if got := rctx.State["euclo.intent.clarification.last_checkpoint_id"]; got != "" {
		t.Fatalf("expected empty checkpoint id in runtime state, got %#v", got)
	}
	if got := rctx.State["euclo.intent.clarification.grounded_anchor_ids"]; got == nil {
		t.Fatal("expected grounded anchor ids in runtime state")
	}
	if got := rctx.State["euclo.intent.clarification.pending_relation_intents"]; got == nil {
		t.Fatal("expected pending relation intents in runtime state")
	}
	if got := rctx.State["euclo.intent.clarification.applied_mutations"]; got == nil {
		t.Fatal("expected applied mutations in runtime state")
	}
	if got := rctx.Variables["instruction"]; got == "" {
		t.Fatal("expected instruction variable to be populated")
	}
}

func TestThoughtRecipeStepNodeUsesRegistryPromptID(t *testing.T) {
	registry := prompttest.New().With("euclo.intent.clarify.question.v1", "resolved prompt text")
	env := contextdata.NewEnvelope("task-1", "session-1")
	step := ExecutionStep{
		ID:       "clarify.step",
		Paradigm: "euclo",
		PromptID: "euclo.intent.clarify.question.v1",
		Prompt:   "inline fallback should not be used",
		Step: surface.ThoughtRecipeStep{
			ID:       "clarify.step",
			PromptID: "euclo.intent.clarify.question.v1",
			Prompt:   "inline fallback should not be used",
		},
	}
	node := NewThoughtRecipeStepNode("clarify.step", &paradigm.Deps{PromptRegistry: registry}, step)

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
	if got := task.Metadata["execution_step_type"]; got != "" {
		t.Fatalf("expected step type metadata to be empty for ad hoc step, got %#v", got)
	}
	if got, ok := env.GetWorkingValue("euclo.execution.step.clarify.step.type"); !ok || got != "" {
		t.Fatalf("expected step type metadata in envelope, got %#v (ok=%v)", got, ok)
	}
	if got, ok := env.GetWorkingValue("euclo.execution.step.clarify.step.prompt_id"); !ok || got != "euclo.intent.clarify.question.v1" {
		t.Fatalf("expected prompt_id metadata in envelope, got %#v (ok=%v)", got, ok)
	}
}

func TestThoughtRecipeStepNodeScopesRuntimeToolsFromEffectiveToolNames(t *testing.T) {
	reg := regpkg.NewRegistry()
	if err := reg.RegisterLegacyTool(semanticTestTool{name: "file_read", available: true}); err != nil {
		t.Fatalf("register file_read: %v", err)
	}
	if err := reg.RegisterLegacyTool(semanticTestTool{name: "file_write", available: true}); err != nil {
		t.Fatalf("register file_write: %v", err)
	}

	deps := &paradigm.Deps{Registry: reg}
	baseEnv := contextdata.NewEnvelope("task-scope", "session-scope")
	scopedStep := ExecutionStep{
		ID:                 "scope.step",
		Paradigm:           "react",
		EffectiveToolNames: []string{"file_write"},
		Step: surface.ThoughtRecipeStep{
			ID: "scope.step",
		},
	}
	scopedNode := NewThoughtRecipeStepNode("scope.step", deps, scopedStep)
	rctx := scopedNode.buildRuntimeContext(baseEnv)

	if got, want := runtimeToolNames(rctx.Tools), []string{"file_write"}; !equalStringSlices(got, want) {
		t.Fatalf("scoped runtime tools = %#v, want %#v", got, want)
	}
	if got, want := runtimeCapabilityNames(rctx.Capabilities), []string{"file_write"}; !equalStringSlices(got, want) {
		t.Fatalf("scoped runtime capabilities = %#v, want %#v", got, want)
	}

	scopedRegistry := scopedNode.scopedRegistry()
	if scopedRegistry == nil {
		t.Fatal("expected scoped registry")
	}
	if got, want := runtimeToolNames(scopedRegistry.ModelCallableTools()), []string{"file_write"}; !equalStringSlices(got, want) {
		t.Fatalf("scoped registry tools = %#v, want %#v", got, want)
	}
	if _, err := scopedRegistry.InvokeCapability(context.Background(), baseEnv.State(), "file_write", map[string]any{}); err != nil {
		t.Fatalf("expected scoped file_write invocation to succeed: %v", err)
	}
	if _, err := scopedRegistry.InvokeCapability(context.Background(), baseEnv.State(), "file_read", map[string]any{}); err == nil {
		t.Fatal("expected scoped registry to reject file_read")
	}

	nextStep := ExecutionStep{
		ID:       "scope.next",
		Paradigm: "react",
		Step: surface.ThoughtRecipeStep{
			ID: "scope.next",
		},
	}
	nextNode := NewThoughtRecipeStepNode("scope.next", deps, nextStep)
	nextRctx := nextNode.buildRuntimeContext(baseEnv)
	if got, want := runtimeToolNames(nextRctx.Tools), []string{"file_read", "file_write"}; !equalStringSlices(got, want) {
		t.Fatalf("unscoped runtime tools = %#v, want %#v", got, want)
	}
	if scoped := nextNode.scopedRegistry(); scoped != nil {
		t.Fatalf("expected no scoped registry for unbounded step, got %#v", scoped)
	}
}

func TestThoughtRecipeStepNodePromptIDRequiresRegistry(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	step := ExecutionStep{
		ID:       "clarify.step",
		Paradigm: "euclo",
		PromptID: "euclo.intent.clarify.question.v1",
		Step: surface.ThoughtRecipeStep{
			ID:       "clarify.step",
			PromptID: "euclo.intent.clarify.question.v1",
		},
	}
	node := NewThoughtRecipeStepNode("clarify.step", &paradigm.Deps{}, step)

	if _, err := node.buildTask(env); err == nil {
		t.Fatal("expected buildTask to fail without a prompt registry")
	}
}

func runtimeToolNames(tools []ports.Tool) []string {
	if len(tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		names = append(names, tool.Name())
	}
	sort.Strings(names)
	return names
}

func runtimeCapabilityNames(caps []descriptor.CapabilityDescriptor) []string {
	if len(caps) == 0 {
		return nil
	}
	names := make([]string, 0, len(caps))
	for _, cap := range caps {
		name := cap.Name
		if name == "" {
			name = cap.ID
		}
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func TestThoughtRecipeStepNodeWritesClarificationMetadata(t *testing.T) {
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
		Step: surface.ThoughtRecipeStep{
			ID:   "extract.step",
			Type: string(ClarificationStepTypeExtract),
			Config: map[string]any{
				"output_schema_id": "clarification.answer.v1",
			},
		},
	}

	node := NewThoughtRecipeStepNode("extract.step", &paradigm.Deps{}, step)
	task, err := node.buildTask(env)
	if err != nil {
		t.Fatalf("buildTask failed: %v", err)
	}
	if got := task.Metadata["execution_step_type"]; got != string(ClarificationStepTypeExtract) {
		t.Fatalf("expected step type metadata, got %#v", got)
	}
	if got := task.Metadata["execution_clarification_type"]; got != string(ClarificationStepTypeExtract) {
		t.Fatalf("expected clarification type metadata, got %#v", got)
	}
	if got, ok := env.GetWorkingValue("euclo.execution.step.extract.step.clarification_schema_id"); !ok || got != "clarification.answer.v1" {
		t.Fatalf("expected clarification schema metadata in envelope, got %#v (ok=%v)", got, ok)
	}
	if got, ok := env.GetWorkingValue("euclo.execution.step.extract.step.clarification_config"); !ok || got == nil {
		t.Fatalf("expected clarification config in envelope, got %#v (ok=%v)", got, ok)
	}
	if got, ok := env.GetWorkingValue("euclo.execution.step.extract.step.clarification_allowed_statuses"); !ok || got == nil {
		t.Fatalf("expected allowed statuses in envelope, got %#v (ok=%v)", got, ok)
	}
	rctx := node.buildRuntimeContext(env)
	if got := rctx.State["execution_step_type"]; got != string(ClarificationStepTypeExtract) {
		t.Fatalf("expected runtime step type metadata, got %#v", got)
	}
	if got := rctx.State["execution_clarification_schema_id"]; got != "clarification.answer.v1" {
		t.Fatalf("expected runtime clarification schema id, got %#v", got)
	}
}

func TestThoughtRecipeStepNodeDelegationFiltersChildEnvelopeAndReturnsCaptures(t *testing.T) {
	parent := contextdata.NewEnvelope("task-parent", "session-parent")
	parent.SetWorkingValueWithClass("input.findings", "parent findings", contextdata.MemoryClassTask)
	parent.SetWorkingValueWithClass("state.unrelated", "keep me on parent", contextdata.MemoryClassTask)
	parent.SetWorkingValueWithClass("scratch.parent_only", "parent scratch", contextdata.MemoryClassTask)
	state := intentcontext.NewState("task-parent", "session-parent")
	state.StateVersion = 4
	state.CurrentTurnID = "turn-parent"
	state.ActiveThoughtRecipeID = "euclo.thoughtrecipe.intent.clarify"
	state.GroundedAnchors = []retrieval.AnchorRef{{AnchorID: "anchor-parent", ChunkID: "chunk-parent", Term: "finding", Class: "clarified_entity", Active: true}}
	if err := intentcontext.NewStateStore().Write(context.Background(), parent, state); err != nil {
		t.Fatalf("write clarification state: %v", err)
	}

	step := ExecutionStep{
		ID:       "delegate.step",
		Type:     "delegate",
		Paradigm: "react",
		Sources:  []string{"input.findings"},
		CaptureBindings: []CaptureBinding{
			{
				Source:      Identifier{positioned: positioned{Span: NewSpan("delegate.euclo", 4, 5, 4, 11)}, Value: "result"},
				Destination: PathExpr{positioned: positioned{Span: NewSpan("delegate.euclo", 4, 15, 4, 26)}, Raw: "state.plan", Parts: []Identifier{{Value: "state"}, {Value: "plan"}}},
			},
		},
		Step: surface.ThoughtRecipeStep{
			ID:   "delegate.step",
			Type: "delegate",
		},
	}
	node := NewThoughtRecipeStepNode("delegate.step.execute", &paradigm.Deps{}, step)

	child := node.buildDelegationEnvelope(parent)
	if child == nil {
		t.Fatal("expected child envelope")
	}
	if child.TaskID == parent.TaskID {
		t.Fatalf("expected delegated child task id to differ from parent, got %q", child.TaskID)
	}
	if got, ok := child.GetWorkingValue("input.findings"); !ok || got != "parent findings" {
		t.Fatalf("expected delegated child to inherit declared source, got %#v (ok=%v)", got, ok)
	}
	if got, ok := child.GetWorkingValue(intentcontext.ClarificationStateKey); !ok || got == nil {
		t.Fatalf("expected clarification state on child, got %#v (ok=%v)", got, ok)
	}
	if got, ok := child.GetWorkingValue("euclo.handoff.continuation"); !ok || got == nil {
		t.Fatalf("expected handoff continuation metadata on child, got %#v (ok=%v)", got, ok)
	}
	if got, ok := child.GetWorkingValue("state.unrelated"); ok || got != nil {
		t.Fatalf("expected unrelated parent state to be filtered out, got %#v (ok=%v)", got, ok)
	}
	if got, ok := child.GetWorkingValue("scratch.parent_only"); ok || got != nil {
		t.Fatalf("expected parent scratch to be isolated, got %#v (ok=%v)", got, ok)
	}

	task, err := node.buildTask(child)
	if err != nil {
		t.Fatalf("buildTask failed: %v", err)
	}
	if got := task.Context["euclo.delegate.parent_task_id"]; got != parent.TaskID {
		t.Fatalf("expected parent task id in delegate context, got %#v", got)
	}
	if got := task.Context["euclo.delegate.child_task_id"]; got != child.TaskID {
		t.Fatalf("expected child task id in delegate context, got %#v", got)
	}
	if got, ok := task.Context["euclo.delegate.source_keys"].([]string); !ok || len(got) != 1 || got[0] != "input.findings" {
		t.Fatalf("expected delegated source keys in task context, got %#v", task.Context["euclo.delegate.source_keys"])
	}

	result := &execution.Result{
		NodeID:  "delegate.step.execute",
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"result": "child summary",
		}),
	}
	if err := node.writeDelegationCaptures(parent, child, result); err != nil {
		t.Fatalf("writeDelegationCaptures failed: %v", err)
	}
	if got, ok := parent.GetWorkingValue("state.plan"); !ok || got != "child summary" {
		t.Fatalf("expected capture to return to parent state, got %#v (ok=%v)", got, ok)
	}
	if got, ok := parent.GetWorkingValue("euclo.execution.delegate.step.state.plan"); ok {
		t.Fatalf("unexpected legacy delegated capture key present: %#v", got)
	}
}

func TestThoughtRecipeStepNodeAskPausesAndResumesWithCapture(t *testing.T) {
	env := contextdata.NewEnvelope("task-ask", "session-ask")
	step := ExecutionStep{
		ID:       "ask.step",
		Type:     "ask",
		Paradigm: "euclo",
		Question: "Choose a mode.",
		Choices:  []string{"review", "refactor"},
		CaptureBindings: []CaptureBinding{
			{
				Source:      Identifier{positioned: positioned{Span: NewSpan("ask.euclo", 4, 5, 4, 11)}, Value: "answer"},
				Destination: PathExpr{positioned: positioned{Span: NewSpan("ask.euclo", 4, 15, 4, 26)}, Raw: "state.intent", Parts: []Identifier{{Value: "state"}, {Value: "intent"}}},
			},
		},
		Step: surface.ThoughtRecipeStep{
			ID:     "ask.step",
			Type:   "ask",
			Prompt: "Choose a mode.",
		},
	}
	node := NewThoughtRecipeStepNode("ask.step.execute", &paradigm.Deps{}, step)

	first, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("first execute failed: %v", err)
	}
	if first == nil || func() bool { v, _ := execution.ResultField(first.Data, "paused"); return v != true }() {
		t.Fatalf("expected paused ask result, got %+v", first)
	}
	frameValue, ok := env.GetWorkingValue(askFrameKey(step.ID))
	if !ok {
		t.Fatal("expected ask frame in envelope")
	}
	frame, ok := frameValue.(*interaction.InteractionFrame)
	if !ok || frame == nil {
		t.Fatalf("expected interaction frame, got %#v", frameValue)
	}
	now := time.Now().UTC()
	frame.Response = &interaction.FrameResult{
		ChosenSlot:  "refactor",
		ExtraData:   map[string]any{"answer": "refactor"},
		RespondedBy: "testsuite",
		RespondedAt: now,
	}
	frame.RespondedAt = &now

	second, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("second execute failed: %v", err)
	}
	if second == nil || func() bool { v, _ := execution.ResultField(second.Data, "answer"); return v != "refactor" }() {
		t.Fatalf("expected answered ask result, got %+v", second)
	}
	if got, ok := env.GetWorkingValue("state.intent"); !ok || got != "refactor" {
		t.Fatalf("expected captured state.intent refactor, got %#v (ok=%v)", got, ok)
	}
	if got := state.GetInteractionResumeNodeID(env); got != "" {
		t.Fatalf("expected resume node cleared, got %#v", got)
	}
}
