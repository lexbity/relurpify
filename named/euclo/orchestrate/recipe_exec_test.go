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
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
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
	env.SetWorkingValue("euclo.route.recipe_id", "fix-bug", contextdata.MemoryClassTask)
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
	env.SetWorkingValue("euclo.route.recipe_id", "fix-bug", contextdata.MemoryClassTask)
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

type stubRecipeModel struct{}

func (stubRecipeModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func (stubRecipeModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (stubRecipeModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "{}"}, nil
}

func (stubRecipeModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}
