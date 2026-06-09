package orchestrate

import (
	"context"
	"sort"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	regpkg "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/model"
	ecap "codeburg.org/lexbit/relurpify/named/euclo/capabilities"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

type noopCompiler struct{}

func (noopCompiler) Compile(_ context.Context, _ contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	return &contextports.CompilationResult{}, nil
}

func ctxWithTrigger(ctx context.Context) context.Context {
	return contextstream.WithTrigger(ctx, contextstream.NewTrigger(noopCompiler{}))
}

func mustRegisterCompiledThoughtRecipe(t *testing.T, registry *thoughtrecipepkg.ThoughtRecipeRegistry, thoughtrecipe *surface.ThoughtRecipe) {
	t.Helper()
	if thoughtrecipe == nil {
		t.Fatal("thoughtrecipe is nil")
	}
	stepID := thoughtrecipe.ID + ".step0"
	plan := &thoughtrecipepkg.ExecutionPlan{
		ThoughtRecipe: thoughtrecipe,
		Steps: []thoughtrecipepkg.ExecutionStep{{
			ID:       stepID,
			Type:     "run",
			Paradigm: "goalcon",
			Goal:     "Continue the thoughtrecipe.",
			Prompt:   "Continue the thoughtrecipe.",
			Step: surface.ThoughtRecipeStep{
				ID:      stepID,
				Type:    "run",
				Prompt:  "Continue the thoughtrecipe.",
				Parent:  surface.ThoughtRecipeStepAgent{Paradigm: "goalcon"},
				Config:  map[string]any{},
				Context: surface.ThoughtRecipeStepContext{},
			},
		}},
	}
	if err := registry.RegisterCompiled(thoughtrecipe, plan, "test"); err != nil {
		t.Fatalf("RegisterCompiled failed: %v", err)
	}
}

func TestThoughtRecipeExecutionNodeExecute(t *testing.T) {
	node := NewThoughtRecipeExecutorNode("thoughtrecipe-exec1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: "fix-bug",
	})
	node.WithParadigmDeps(&paradigm.Deps{
		Model:         stubThoughtRecipeModel{},
		Registry:      regpkg.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config: &execution.Config{
			Name:  "thoughtrecipe-exec-test",
			Model: "stub",
		},
	})
	registry := thoughtrecipepkg.NewThoughtRecipeRegistry()
	mustRegisterCompiledThoughtRecipe(t, registry, &surface.ThoughtRecipe{
		ID:   "fix-bug",
		Name: "fix-bug",
		Metadata: surface.ThoughtRecipeMetadata{
			Name: "fix-bug",
		},
	})
	node.WithThoughtRecipeRegistry(registry)

	result, err := node.Execute(ctxWithTrigger(context.Background()), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if !result.Success {
		t.Fatalf("Expected successful thoughtrecipe execution, got result: %+v", result)
	}
}

func TestThoughtRecipeExecutionNodeRejectsUncompiledThoughtRecipeEntries(t *testing.T) {
	node := NewThoughtRecipeExecutorNode("thoughtrecipe-exec-missing-plan")

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: "fix-bug",
	})
	node.WithParadigmDeps(&paradigm.Deps{
		Model:         stubThoughtRecipeModel{},
		Registry:      regpkg.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config: &execution.Config{
			Name:  "thoughtrecipe-exec-test",
			Model: "stub",
		},
	})
	registry := thoughtrecipepkg.NewThoughtRecipeRegistry()
	if err := registry.Register(&surface.ThoughtRecipe{
		ID:   "fix-bug",
		Name: "fix-bug",
		Metadata: surface.ThoughtRecipeMetadata{
			Name: "fix-bug",
		},
	}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	node.WithThoughtRecipeRegistry(registry)

	result, err := node.Execute(ctxWithTrigger(context.Background()), env)
	if err == nil {
		t.Fatal("expected execute to fail for uncompiled thoughtrecipe entry")
	}
	if result == nil || result.Success {
		t.Fatalf("expected failed result, got %+v", result)
	}
	if got, ok := execution.ResultField(result.Data, "error"); !ok || got != "compiled plan not found for thoughtrecipe: fix-bug" {
		t.Fatalf("unexpected error payload: %#v", got)
	}
}

func TestThoughtRecipeExecutionNodeID(t *testing.T) {
	node := NewThoughtRecipeExecutorNode("thoughtrecipe-exec1")

	if node.ID() != "thoughtrecipe-exec1" {
		t.Errorf("Expected ID thoughtrecipe-exec1, got %s", node.ID())
	}
}

func TestThoughtRecipeExecutionNodeType(t *testing.T) {
	node := NewThoughtRecipeExecutorNode("thoughtrecipe-exec1")

	if node.Type() != agentgraph.NodeTypeSystem {
		t.Errorf("Expected Type system, got %s", node.Type())
	}
}

func TestThoughtRecipeExecutionNodeWritesToEnvelope(t *testing.T) {
	node := NewThoughtRecipeExecutorNode("thoughtrecipe-exec1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: "fix-bug",
	})
	node.WithParadigmDeps(&paradigm.Deps{
		Model:         stubThoughtRecipeModel{},
		Registry:      regpkg.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config: &execution.Config{
			Name:  "thoughtrecipe-exec-test",
			Model: "stub",
		},
	})
	registry := thoughtrecipepkg.NewThoughtRecipeRegistry()
	mustRegisterCompiledThoughtRecipe(t, registry, &surface.ThoughtRecipe{
		ID:   "fix-bug",
		Name: "fix-bug",
		Metadata: surface.ThoughtRecipeMetadata{
			Name: "fix-bug",
		},
	})
	node.WithThoughtRecipeRegistry(registry)

	_, err := node.Execute(ctxWithTrigger(context.Background()), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	kind := state.GetExecutionKind(env)
	if kind == "" {
		t.Error("Expected execution.kind in envelope")
	}

	if kind != "thoughtrecipe" {
		t.Errorf("Expected execution.kind thoughtrecipe, got %v", kind)
	}

	completed := state.GetExecutionCompleted(env)
	if !completed {
		t.Error("Expected execution.completed in envelope")
	}

	if completed != true {
		t.Errorf("Expected execution.completed true, got %v", completed)
	}

	thoughtrecipeKind := state.GetExecutionKind(env)
	if thoughtrecipeKind == "" {
		t.Error("Expected execution.kind in envelope")
	}

	if thoughtrecipeKind != "thoughtrecipe" {
		t.Errorf("Expected execution.kind thoughtrecipe, got %v", thoughtrecipeKind)
	}

	thoughtrecipeID := state.GetExecutionThoughtRecipeID(env)
	if thoughtrecipeID == "" {
		t.Error("Expected execution.thoughtrecipe_id in envelope")
	}

	if thoughtrecipeID != "fix-bug" {
		t.Errorf("Expected execution.thoughtrecipe_id fix-bug, got %v", thoughtrecipeID)
	}
}

func TestThoughtRecipeExecutionNodeFollowsClarificationHandoff(t *testing.T) {
	node := NewThoughtRecipeExecutorNode("thoughtrecipe-exec-handoff")

	env := contextdata.NewEnvelope("task-handoff", "session-handoff")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: clarificationThoughtRecipeID,
	})

	clarificationState := intentcontext.NewState("task-handoff", "session-handoff")
	clarificationState.Ambiguity = &intentcontext.AmbiguityCharacterization{
		Kind:              intentcontext.AmbiguityKindMultiMatch,
		Confidence:        0.2,
		Rationale:         "review target unclear",
		CandidateFamilies: []string{"review"},
	}
	if err := intentcontext.NewStateStore().Write(context.Background(), env, clarificationState); err != nil {
		t.Fatalf("write clarification state: %v", err)
	}

	node.WithParadigmDeps(&paradigm.Deps{
		Model:         stubThoughtRecipeModel{},
		Registry:      regpkg.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config: &execution.Config{
			Name:  "thoughtrecipe-exec-handoff-test",
			Model: "stub",
		},
	})
	registry := thoughtrecipepkg.NewThoughtRecipeRegistry()
	mustRegisterCompiledThoughtRecipe(t, registry, &surface.ThoughtRecipe{
		ID:       clarificationThoughtRecipeID,
		Name:     "intent clarification",
		Metadata: surface.ThoughtRecipeMetadata{Name: "intent clarification"},
	})
	node.WithThoughtRecipeRegistry(registry)
	mustRegisterCompiledThoughtRecipe(t, node.registry, &surface.ThoughtRecipe{
		ID:       "euclo.thoughtrecipe.code_review.custom",
		Name:     "code review custom",
		Metadata: surface.ThoughtRecipeMetadata{Name: "code review custom"},
	})
	state.SetClarificationNextThoughtRecipeID(env, "euclo.thoughtrecipe.code_review.custom")

	result, err := node.Execute(ctxWithTrigger(context.Background()), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful thoughtrecipe execution, got %+v", result)
	}
	if got := state.GetExecutionThoughtRecipeID(env); got != "euclo.thoughtrecipe.code_review.custom" {
		t.Fatalf("expected nested thoughtrecipe execution to set thoughtrecipe_id, got %#v", got)
	}
	if meta, ok := state.GetRouteContinuation(env); !ok || meta == nil || !meta.SharedContext || meta.TargetRouteID != "euclo.thoughtrecipe.code_review.custom" {
		t.Fatalf("unexpected continuation metadata: %#v", meta)
	}
	if got := mustEnvelopeString(t, env, intentcontext.ClarificationActiveThoughtRecipeKey); got != "euclo.thoughtrecipe.code_review.custom" {
		t.Fatalf("expected active thoughtrecipe id to follow handoff, got %q", got)
	}
}

type stubThoughtRecipeModel struct{}

func (stubThoughtRecipeModel) Generate(context.Context, string, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func (stubThoughtRecipeModel) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (stubThoughtRecipeModel) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "{}"}, nil
}

func (stubThoughtRecipeModel) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

type recordingThoughtRecipeModel struct {
	nativeToolCalling bool
	generatePrompts   []string
	chatMessages      [][]model.Message
	chatToolSpecs     [][]model.LLMToolSpec
}

func (m *recordingThoughtRecipeModel) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	_ = ctx
	_ = options
	m.generatePrompts = append(m.generatePrompts, prompt)
	return &model.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func (m *recordingThoughtRecipeModel) GenerateStream(ctx context.Context, prompt string, options *model.LLMOptions) (<-chan string, error) {
	_ = ctx
	_ = prompt
	_ = options
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *recordingThoughtRecipeModel) Chat(ctx context.Context, messages []model.Message, options *model.LLMOptions) (*model.LLMResponse, error) {
	_ = ctx
	_ = messages
	_ = options
	return &model.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func (m *recordingThoughtRecipeModel) ChatWithTools(ctx context.Context, messages []model.Message, tools []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	_ = ctx
	_ = options
	m.chatMessages = append(m.chatMessages, append([]model.Message(nil), messages...))
	m.chatToolSpecs = append(m.chatToolSpecs, append([]model.LLMToolSpec(nil), tools...))
	return &model.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func (m *recordingThoughtRecipeModel) ToolRepairStrategy() string {
	return "heuristic-only"
}

func (m *recordingThoughtRecipeModel) MaxToolsPerCall() int {
	return 0
}

func (m *recordingThoughtRecipeModel) UsesNativeToolCalling() bool {
	return m.nativeToolCalling
}

type recordingThoughtRecipeTool struct {
	name string
}

func (t recordingThoughtRecipeTool) Name() string                      { return t.name }
func (t recordingThoughtRecipeTool) Description() string               { return t.name }
func (t recordingThoughtRecipeTool) Category() string                  { return "test" }
func (t recordingThoughtRecipeTool) Parameters() []ports.ToolParameter { return nil }
func (t recordingThoughtRecipeTool) Execute(context.Context, map[string]interface{}) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true, Data: map[string]any{"name": t.name}}, nil
}
func (t recordingThoughtRecipeTool) IsAvailable(context.Context) bool { return true }
func (t recordingThoughtRecipeTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{}
}
func (t recordingThoughtRecipeTool) Tags() []string { return nil }

type recordingCapabilityHandler struct {
	called bool
	args   map[string]any
}

func (h *recordingCapabilityHandler) Descriptor(context.Context, ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.code_review",
		Name:          "code_review",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
}

func (h *recordingCapabilityHandler) Invoke(ctx context.Context, env ports.State, args map[string]interface{}) (*ports.ToolResult, error) {
	_ = ctx
	_ = env
	h.called = true
	h.args = make(map[string]any, len(args))
	for key, value := range args {
		h.args[key] = value
	}
	return &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"name": "code_review",
			"args": args,
		},
	}, nil
}

func compileThoughtRecipeFromSource(t *testing.T, source string, toolReg *regpkg.CapabilityRegistry, capReg thoughtrecipepkg.CapabilityRegistryLookup) (*thoughtrecipepkg.ThoughtRecipeRegistry, *thoughtrecipepkg.ExecutionPlan) {
	t.Helper()
	doc, err := thoughtrecipepkg.ParseSource("scope_demo.euclo", source)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}
	symbols := thoughtrecipepkg.NewSymbolTable(doc)
	if toolReg != nil {
		symbols.WithToolRegistry(toolReg)
	}
	if capReg != nil {
		symbols.WithCapabilityRegistry(capReg)
	}
	if err := symbols.Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	plan, err := thoughtrecipepkg.LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	registry := thoughtrecipepkg.NewThoughtRecipeRegistry()
	if err := registry.RegisterCompiled(plan.ThoughtRecipe, plan, "scope_demo.euclo"); err != nil {
		t.Fatalf("RegisterCompiled failed: %v", err)
	}
	return registry, plan
}

func executeThoughtRecipeFromSource(t *testing.T, source string, runtimeReg, toolReg *regpkg.CapabilityRegistry, capReg thoughtrecipepkg.CapabilityRegistryLookup, nativeToolCalling bool) (*recordingThoughtRecipeModel, *contextdata.Envelope, *thoughtrecipepkg.ExecutionPlan) {
	t.Helper()
	recipeRegistry, plan := compileThoughtRecipeFromSource(t, source, toolReg, capReg)
	model := &recordingThoughtRecipeModel{nativeToolCalling: nativeToolCalling}
	deps := &paradigm.Deps{
		Model:    model,
		Registry: runtimeReg,
		Config:   &execution.Config{Model: "test-model", NativeToolCalling: nativeToolCalling},
	}

	taskEnv := contextdata.NewEnvelope("task-scope", "session-scope")
	state.SetRouteSelection(taskEnv, &euclotypes.RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: plan.ThoughtRecipe.ID,
	})

	node := NewThoughtRecipeExecutorNode("thoughtrecipe-exec").
		WithParadigmDeps(deps).
		WithThoughtRecipeRegistry(recipeRegistry)

	if _, err := node.Execute(context.Background(), taskEnv); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	return model, taskEnv, plan
}

func toolSpecNames(specs []model.LLMToolSpec) []string {
	if len(specs) == 0 {
		return nil
	}
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		if strings.TrimSpace(spec.Name) != "" {
			names = append(names, spec.Name)
		}
	}
	sort.Strings(names)
	return names
}

func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func containsAllStrings(haystack string, want ...string) bool {
	for _, needle := range want {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}

func TestThoughtRecipeExecutorNodeUsesScopedToolsFromSource(t *testing.T) {
	toolReg := regpkg.NewRegistry()
	for _, name := range []string{"scope_read", "scope_write"} {
		if err := toolReg.RegisterLegacyTool(recordingThoughtRecipeTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	source := `thoughtrecipe scope_demo
"Scoped demo."

trigger as capability:
  may invoke ["scope_read"]

agent reviewer uses react

run reviewer:
  goal "Inspect the workspace."
`

	model, _, plan := executeThoughtRecipeFromSource(t, source, toolReg, toolReg, nil, true)

	if got, want := len(model.chatToolSpecs), 1; got != want {
		t.Fatalf("native tool call count = %d, want %d", got, want)
	}
	if got, want := toolSpecNames(model.chatToolSpecs[0]), []string{"scope_read"}; !equalStringSlices(got, want) {
		t.Fatalf("native tool names = %#v, want %#v", got, want)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("plan step count = %d, want 1", len(plan.Steps))
	}
	if got, want := plan.Steps[0].EffectiveToolNames, []string{"scope_read"}; !equalStringSlices(got, want) {
		t.Fatalf("effective tool names = %#v, want %#v", got, want)
	}
}

func TestThoughtRecipeExecutorNodeAppliesRunLocalOverlay(t *testing.T) {
	toolReg := regpkg.NewRegistry()
	for _, name := range []string{"scope_read", "scope_write"} {
		if err := toolReg.RegisterLegacyTool(recordingThoughtRecipeTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	source := `thoughtrecipe scope_demo
"Scoped overlay demo."

trigger as capability:
  may invoke ["scope_read"]

agent reviewer uses react

run reviewer:
  may invoke ["scope_write"]
  goal "Inspect the workspace."
`

	model, _, plan := executeThoughtRecipeFromSource(t, source, toolReg, toolReg, nil, true)

	if got, want := len(model.chatToolSpecs), 1; got != want {
		t.Fatalf("native tool call count = %d, want %d", got, want)
	}
	if got, want := toolSpecNames(model.chatToolSpecs[0]), []string{"scope_read", "scope_write"}; !equalStringSlices(got, want) {
		t.Fatalf("run tool names = %#v, want %#v", got, want)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("plan step count = %d, want 1", len(plan.Steps))
	}
	if got, want := plan.Steps[0].EffectiveToolNames, []string{"scope_read", "scope_write"}; !equalStringSlices(got, want) {
		t.Fatalf("effective tool names = %#v, want %#v", got, want)
	}
}

func TestThoughtRecipeExecutorNodeSupportsFallbackPromptModeWithScopedTools(t *testing.T) {
	toolReg := regpkg.NewRegistry()
	for _, name := range []string{"scope_read", "scope_write"} {
		if err := toolReg.RegisterLegacyTool(recordingThoughtRecipeTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	source := `thoughtrecipe scope_demo
"Fallback demo."

trigger as capability:
  may invoke ["scope_read"]

agent reviewer uses react

run reviewer:
  goal "Inspect the workspace."
`

	model, _, _ := executeThoughtRecipeFromSource(t, source, toolReg, toolReg, nil, false)

	if got, want := len(model.generatePrompts), 1; got != want {
		t.Fatalf("generate prompt count = %d, want %d", got, want)
	}
	if prompt := model.generatePrompts[0]; !containsAllStrings(prompt, "scope_read") || strings.Contains(prompt, "scope_write") {
		t.Fatalf("fallback prompt = %q", prompt)
	}
}

func TestThoughtRecipeExecutorNodeKeepsUnscopedRecipesAtFullToolSurface(t *testing.T) {
	toolReg := regpkg.NewRegistry()
	for _, name := range []string{"scope_read", "scope_write"} {
		if err := toolReg.RegisterLegacyTool(recordingThoughtRecipeTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	source := `thoughtrecipe scope_demo
"Unscoped demo."

trigger as capability:
  may read workspace

agent reviewer uses react

run reviewer:
  goal "Inspect the workspace."
`

	model, _, _ := executeThoughtRecipeFromSource(t, source, toolReg, toolReg, nil, true)

	if got, want := len(model.chatToolSpecs), 1; got != want {
		t.Fatalf("native tool call count = %d, want %d", got, want)
	}
	if got, want := toolSpecNames(model.chatToolSpecs[0]), []string{"scope_read", "scope_write"}; !equalStringSlices(got, want) {
		t.Fatalf("unscoped tool names = %#v, want %#v", got, want)
	}
}

func TestThoughtRecipeExecutorNodeStillInvokesDirectCapabilities(t *testing.T) {
	capHandler := &recordingCapabilityHandler{}
	capReg := regpkg.NewRegistry()
	if err := capReg.RegisterInvocableCapability(capHandler); err != nil {
		t.Fatalf("register invocable capability: %v", err)
	}
	semanticCaps := ecap.NewRegistry()
	source := `thoughtrecipe scope_demo
"Capability demo."

trigger as capability:
  may read workspace

agent reviewer uses react

run reviewer:
  do relurpic:code_review
`

	model, env, _ := executeThoughtRecipeFromSource(t, source, capReg, nil, semanticCaps, true)

	_ = model
	if !capHandler.called {
		t.Fatal("expected direct capability invocation")
	}
	if got := state.GetExecutionCapabilityID(env); got != "euclo:cap.code_review" {
		t.Fatalf("capability id = %#v, want euclo:cap.code_review", got)
	}
	if got := state.GetExecutionKind(env); got != "thoughtrecipe" {
		t.Fatalf("execution kind = %#v, want thoughtrecipe", got)
	}
}

func TestThoughtRecipeExecutorNodeCombinesScopedToolsAndNestedCapabilityInvocation(t *testing.T) {
	toolReg := regpkg.NewRegistry()
	for _, name := range []string{"scope_read", "scope_write"} {
		if err := toolReg.RegisterLegacyTool(recordingThoughtRecipeTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}

	scopedSource := `thoughtrecipe scope_demo
"Scoped demo."

trigger as capability:
  may read workspace
  may invoke ["scope_read"]

agent reviewer uses react

run reviewer:
  may invoke ["scope_write"]
  goal "Inspect the workspace."
`
	model, _, _ := executeThoughtRecipeFromSource(t, scopedSource, toolReg, toolReg, nil, true)
	if len(model.chatToolSpecs) != 1 {
		t.Fatalf("native tool call count = %d, want 1", len(model.chatToolSpecs))
	}
	if got, want := toolSpecNames(model.chatToolSpecs[0]), []string{"scope_read", "scope_write"}; !equalStringSlices(got, want) {
		t.Fatalf("scoped tool names = %#v, want %#v", got, want)
	}

	capHandler := &recordingCapabilityHandler{}
	runtimeReg := regpkg.NewRegistry()
	if err := runtimeReg.RegisterInvocableCapability(capHandler); err != nil {
		t.Fatalf("register invocable capability: %v", err)
	}
	semanticCaps := ecap.NewRegistry()
	capSource := `thoughtrecipe capability_demo
"Capability demo."

trigger as capability:
  may read workspace

input workspace: "**/*"
agent reviewer uses react

run reviewer:
  do relurpic:code_review on input.workspace
`
	_, env, _ := executeThoughtRecipeFromSource(t, capSource, runtimeReg, nil, semanticCaps, true)
	if !capHandler.called {
		t.Fatal("expected direct capability invocation")
	}
	if got := state.GetExecutionCapabilityID(env); got != "euclo:cap.code_review" {
		t.Fatalf("capability id = %#v, want euclo:cap.code_review", got)
	}
}
