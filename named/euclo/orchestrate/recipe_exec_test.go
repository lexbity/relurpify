package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type noopCompiler struct{}

func (noopCompiler) Compile(_ context.Context, _ compiler.CompilationRequest) (*compiler.CompilationResult, *compiler.CompilationRecord, error) {
	return &compiler.CompilationResult{}, &compiler.CompilationRecord{}, nil
}

func ctxWithTrigger(ctx context.Context) context.Context {
	return contextstream.WithTrigger(ctx, contextstream.NewTrigger(noopCompiler{}))
}

func mustRegisterCompiledThoughtRecipe(t *testing.T, registry *thoughtrecipepkg.ThoughtRecipeRegistry, thoughtrecipe *thoughtrecipepkg.ThoughtRecipe) {
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
			Step: thoughtrecipepkg.ThoughtRecipeStep{
				ID:      stepID,
				Type:    "run",
				Prompt:  "Continue the thoughtrecipe.",
				Parent:  thoughtrecipepkg.ThoughtRecipeStepAgent{Paradigm: "goalcon"},
				Config:  map[string]any{},
				Context: thoughtrecipepkg.ThoughtRecipeStepContext{},
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
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: "fix-bug",
	}, contextdata.MemoryClassTask)
	node.WithWorkspaceEnvironment(agentenv.WorkspaceEnvironment{
		Model:         stubThoughtRecipeModel{},
		Registry:      capability.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config: &core.Config{
			Name:  "thoughtrecipe-exec-test",
			Model: "stub",
		},
	})
	registry := thoughtrecipepkg.NewThoughtRecipeRegistry()
	mustRegisterCompiledThoughtRecipe(t, registry, &thoughtrecipepkg.ThoughtRecipe{
		ID:   "fix-bug",
		Name: "fix-bug",
		Metadata: thoughtrecipepkg.ThoughtRecipeMetadata{
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
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: "fix-bug",
	}, contextdata.MemoryClassTask)
	node.WithWorkspaceEnvironment(agentenv.WorkspaceEnvironment{
		Model:         stubThoughtRecipeModel{},
		Registry:      capability.NewCapabilityRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config: &core.Config{
			Name:  "thoughtrecipe-exec-test",
			Model: "stub",
		},
	})
	registry := thoughtrecipepkg.NewThoughtRecipeRegistry()
	if err := registry.Register(&thoughtrecipepkg.ThoughtRecipe{
		ID:   "fix-bug",
		Name: "fix-bug",
		Metadata: thoughtrecipepkg.ThoughtRecipeMetadata{
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
	if result.Data["error"] != "compiled plan not found for thoughtrecipe: fix-bug" {
		t.Fatalf("unexpected error payload: %#v", result.Data["error"])
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
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: "fix-bug",
	}, contextdata.MemoryClassTask)
	node.WithWorkspaceEnvironment(agentenv.WorkspaceEnvironment{
		Model:         stubThoughtRecipeModel{},
		Registry:      capability.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config: &core.Config{
			Name:  "thoughtrecipe-exec-test",
			Model: "stub",
		},
	})
	registry := thoughtrecipepkg.NewThoughtRecipeRegistry()
	mustRegisterCompiledThoughtRecipe(t, registry, &thoughtrecipepkg.ThoughtRecipe{
		ID:   "fix-bug",
		Name: "fix-bug",
		Metadata: thoughtrecipepkg.ThoughtRecipeMetadata{
			Name: "fix-bug",
		},
	})
	node.WithThoughtRecipeRegistry(registry)

	_, err := node.Execute(ctxWithTrigger(context.Background()), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	kind, ok := env.GetWorkingValue("euclo.execution.kind")
	if !ok {
		t.Error("Expected execution.kind in envelope")
	}

	if kind != "thoughtrecipe" {
		t.Errorf("Expected execution.kind thoughtrecipe, got %v", kind)
	}

	completed, ok := env.GetWorkingValue("euclo.execution.completed")
	if !ok {
		t.Error("Expected execution.completed in envelope")
	}

	if completed != true {
		t.Errorf("Expected execution.completed true, got %v", completed)
	}

	thoughtrecipeKind, ok := env.GetWorkingValue("euclo.execution.kind")
	if !ok {
		t.Error("Expected execution.kind in envelope")
	}

	if thoughtrecipeKind != "thoughtrecipe" {
		t.Errorf("Expected execution.kind thoughtrecipe, got %v", thoughtrecipeKind)
	}

	thoughtrecipeID, ok := env.GetWorkingValue("euclo.execution.thoughtrecipe_id")
	if !ok {
		t.Error("Expected execution.thoughtrecipe_id in envelope")
	}

	if thoughtrecipeID != "fix-bug" {
		t.Errorf("Expected execution.thoughtrecipe_id fix-bug, got %v", thoughtrecipeID)
	}
}

func TestThoughtRecipeExecutionNodeFollowsClarificationHandoff(t *testing.T) {
	node := NewThoughtRecipeExecutorNode("thoughtrecipe-exec-handoff")

	env := contextdata.NewEnvelope("task-handoff", "session-handoff")
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: clarificationThoughtRecipeID,
	}, contextdata.MemoryClassTask)

	state := intentcontext.NewState("task-handoff", "session-handoff")
	state.Ambiguity = &intentcontext.AmbiguityCharacterization{
		Kind:              intentcontext.AmbiguityKindMultiMatch,
		Confidence:        0.2,
		Rationale:         "review target unclear",
		CandidateFamilies: []string{"review"},
	}
	if err := intentcontext.NewStateStore().Write(context.Background(), env, state); err != nil {
		t.Fatalf("write clarification state: %v", err)
	}

	node.WithWorkspaceEnvironment(agentenv.WorkspaceEnvironment{
		Model:         stubThoughtRecipeModel{},
		Registry:      capability.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config: &core.Config{
			Name:  "thoughtrecipe-exec-handoff-test",
			Model: "stub",
		},
	})
	registry := thoughtrecipepkg.NewThoughtRecipeRegistry()
	mustRegisterCompiledThoughtRecipe(t, registry, &thoughtrecipepkg.ThoughtRecipe{
		ID:       clarificationThoughtRecipeID,
		Name:     "intent clarification",
		Metadata: thoughtrecipepkg.ThoughtRecipeMetadata{Name: "intent clarification"},
	})
	node.WithThoughtRecipeRegistry(registry)
	mustRegisterCompiledThoughtRecipe(t, node.registry, &thoughtrecipepkg.ThoughtRecipe{
		ID:       "euclo.thoughtrecipe.code_review.custom",
		Name:     "code review custom",
		Metadata: thoughtrecipepkg.ThoughtRecipeMetadata{Name: "code review custom"},
	})
	env.SetWorkingValue("euclo.clarification.next_thoughtrecipe_id", "euclo.thoughtrecipe.code_review.custom", contextdata.MemoryClassTask)

	result, err := node.Execute(ctxWithTrigger(context.Background()), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful thoughtrecipe execution, got %+v", result)
	}
	if got, ok := env.GetWorkingValue("euclo.execution.thoughtrecipe_id"); !ok || got != "euclo.thoughtrecipe.code_review.custom" {
		t.Fatalf("expected nested thoughtrecipe execution to set thoughtrecipe_id, got %#v (ok=%v)", got, ok)
	}
	if got, ok := env.GetWorkingValue("euclo.route.continuation"); !ok {
		t.Fatal("expected continuation metadata in envelope")
	} else if meta, ok := got.(*RouteContinuation); !ok || meta == nil || !meta.SharedContext || meta.TargetRouteID != "euclo.thoughtrecipe.code_review.custom" {
		t.Fatalf("unexpected continuation metadata: %#v", got)
	}
	if got := mustEnvelopeString(t, env, intentcontext.ClarificationActiveThoughtRecipeKey); got != "euclo.thoughtrecipe.code_review.custom" {
		t.Fatalf("expected active thoughtrecipe id to follow handoff, got %q", got)
	}
}

type stubThoughtRecipeModel struct{}

func (stubThoughtRecipeModel) Generate(context.Context, string, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func (stubThoughtRecipeModel) GenerateStream(context.Context, string, *contracts.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (stubThoughtRecipeModel) Chat(context.Context, []contracts.Message, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: "{}"}, nil
}

func (stubThoughtRecipeModel) ChatWithTools(context.Context, []contracts.Message, []contracts.LLMToolSpec, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}
