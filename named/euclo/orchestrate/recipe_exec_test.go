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
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
	"codeburg.org/lexbit/relurpify/named/euclo/recipetemplates"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type noopCompiler struct{}

func (noopCompiler) Compile(_ context.Context, _ compiler.CompilationRequest) (*compiler.CompilationResult, *compiler.CompilationRecord, error) {
	return &compiler.CompilationResult{}, &compiler.CompilationRecord{}, nil
}

func ctxWithTrigger(ctx context.Context) context.Context {
	return contextstream.WithTrigger(ctx, contextstream.NewTrigger(noopCompiler{}))
}

func TestRecipeExecutionNodeExecute(t *testing.T) {
	node := NewRecipeExecutorNode("recipe-exec1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind: "recipe",
		RecipeID:  "fix-bug",
	}, contextdata.MemoryClassTask)
	node.WithWorkspaceEnvironment(agentenv.WorkspaceEnvironment{
		Model:         stubRecipeModel{},
		Registry:      capability.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config: &core.Config{
			Name:  "recipe-exec-test",
			Model: "stub",
		},
	})
	registry := recipepkg.NewRecipeRegistry()
	if err := registry.Register(&recipepkg.ThoughtRecipe{
		ID:         "fix-bug",
		APIVersion: "euclo.v1",
		Kind:       "thought-recipe",
		Metadata: recipepkg.RecipeMetadata{
			Name: "fix-bug",
		},
		Sequence: recipepkg.RecipeSequence{
			Steps: []recipepkg.RecipeStep{
				{
					ID: "step-1",
					Parent: recipepkg.RecipeStepAgent{
						Paradigm: "react",
						Prompt:   "return a completion summary",
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	node.WithRecipeRegistry(registry)

	result, err := node.Execute(ctxWithTrigger(context.Background()), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if !result.Success {
		t.Fatalf("Expected successful recipe execution, got result: %+v", result)
	}
}

func TestRecipeExecutionNodeID(t *testing.T) {
	node := NewRecipeExecutorNode("recipe-exec1")

	if node.ID() != "recipe-exec1" {
		t.Errorf("Expected ID recipe-exec1, got %s", node.ID())
	}
}

func TestRecipeExecutionNodeType(t *testing.T) {
	node := NewRecipeExecutorNode("recipe-exec1")

	if node.Type() != agentgraph.NodeTypeSystem {
		t.Errorf("Expected Type system, got %s", node.Type())
	}
}

func TestRecipeExecutionNodeWritesToEnvelope(t *testing.T) {
	node := NewRecipeExecutorNode("recipe-exec1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind: "recipe",
		RecipeID:  "fix-bug",
	}, contextdata.MemoryClassTask)
	node.WithWorkspaceEnvironment(agentenv.WorkspaceEnvironment{
		Model:         stubRecipeModel{},
		Registry:      capability.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config: &core.Config{
			Name:  "recipe-exec-test",
			Model: "stub",
		},
	})
	registry := recipepkg.NewRecipeRegistry()
	if err := registry.Register(&recipepkg.ThoughtRecipe{
		ID:         "fix-bug",
		APIVersion: "euclo.v1",
		Kind:       "thought-recipe",
		Metadata: recipepkg.RecipeMetadata{
			Name: "fix-bug",
		},
		Sequence: recipepkg.RecipeSequence{
			Steps: []recipepkg.RecipeStep{
				{
					ID: "step-1",
					Parent: recipepkg.RecipeStepAgent{
						Paradigm: "react",
						Prompt:   "return a completion summary",
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	node.WithRecipeRegistry(registry)

	_, err := node.Execute(ctxWithTrigger(context.Background()), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	kind, ok := env.GetWorkingValue("euclo.execution.kind")
	if !ok {
		t.Error("Expected execution.kind in envelope")
	}

	if kind != "recipe" {
		t.Errorf("Expected execution.kind recipe, got %v", kind)
	}

	completed, ok := env.GetWorkingValue("euclo.execution.completed")
	if !ok {
		t.Error("Expected execution.completed in envelope")
	}

	if completed != true {
		t.Errorf("Expected execution.completed true, got %v", completed)
	}

	recipeKind, ok := env.GetWorkingValue("euclo.execution.kind")
	if !ok {
		t.Error("Expected execution.kind in envelope")
	}

	if recipeKind != "recipe" {
		t.Errorf("Expected execution.kind recipe, got %v", recipeKind)
	}

	recipeID, ok := env.GetWorkingValue("euclo.execution.recipe_id")
	if !ok {
		t.Error("Expected execution.recipe_id in envelope")
	}

	if recipeID != "fix-bug" {
		t.Errorf("Expected execution.recipe_id fix-bug, got %v", recipeID)
	}
}

func TestRecipeExecutionNodeFollowsClarificationHandoff(t *testing.T) {
	node := NewRecipeExecutorNode("recipe-exec-handoff")

	env := contextdata.NewEnvelope("task-handoff", "session-handoff")
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind: "recipe",
		RecipeID:  clarificationRecipeID,
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
		Model:         stubRecipeModel{},
		Registry:      capability.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config: &core.Config{
			Name:  "recipe-exec-handoff-test",
			Model: "stub",
		},
	})
	registry, err := recipetemplates.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	node.WithRecipeRegistry(registry)
	if err := node.registry.Register(&recipepkg.ThoughtRecipe{
		ID:         "euclo.recipe.code_review.custom",
		APIVersion: "euclo.recipe/v1",
		Kind:       "ThoughtRecipe",
		Metadata:   recipepkg.RecipeMetadata{Name: "code review custom"},
		Sequence: recipepkg.RecipeSequence{
			Steps: []recipepkg.RecipeStep{
				{
					ID:     "review",
					Type:   "react",
					Parent: recipepkg.RecipeStepAgent{Paradigm: "react", Prompt: "return a completion summary"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("Register custom recipe failed: %v", err)
	}
	env.SetWorkingValue("euclo.clarification.next_recipe_id", "euclo.recipe.code_review.custom", contextdata.MemoryClassTask)

	result, err := node.Execute(ctxWithTrigger(context.Background()), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful recipe execution, got %+v", result)
	}
	if got, ok := env.GetWorkingValue("euclo.execution.recipe_id"); !ok || got != "euclo.recipe.code_review.custom" {
		t.Fatalf("expected nested recipe execution to set recipe_id, got %#v (ok=%v)", got, ok)
	}
}

type stubRecipeModel struct{}

func (stubRecipeModel) Generate(context.Context, string, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func (stubRecipeModel) GenerateStream(context.Context, string, *contracts.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (stubRecipeModel) Chat(context.Context, []contracts.Message, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: "{}"}, nil
}

func (stubRecipeModel) ChatWithTools(context.Context, []contracts.Message, []contracts.LLMToolSpec, *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}
