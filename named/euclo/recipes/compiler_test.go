package recipe

import (
	"testing"
)

func TestRecipeCompilerCompilesToNodes(t *testing.T) {
	compiler := NewCompiler()

	recipe := &ThoughtRecipe{
		ID:   "test-recipe",
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID:   "step1",
				Type: "llm",
				Config: map[string]interface{}{
					"model": "gpt-4",
				},
			},
			{
				ID:   "step2",
				Type: "retrieve",
			},
		},
	}

	nodes, err := compiler.Compile(recipe)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(nodes))
	}

	if nodes[0].ID != "step1" {
		t.Errorf("Expected first node ID step1, got %s", nodes[0].ID)
	}

	if nodes[0].Type != "llm" {
		t.Errorf("Expected first node type llm, got %s", nodes[0].Type)
	}

	if nodes[1].ID != "step2" {
		t.Errorf("Expected second node ID step2, got %s", nodes[1].ID)
	}

	if nodes[1].Type != "retrieve" {
		t.Errorf("Expected second node type retrieve, got %s", nodes[1].Type)
	}
}

func TestRecipeCompilerStepDependencies(t *testing.T) {
	compiler := NewCompiler()

	recipe := &ThoughtRecipe{
		ID:   "test-recipe",
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID:   "step1",
				Type: "llm",
			},
			{
				ID:           "step2",
				Type:         "retrieve",
				Dependencies: []string{"step1"},
			},
			{
				ID:           "step3",
				Type:         "transform",
				Dependencies: []string{"step2"},
			},
		},
	}

	nodes, err := compiler.Compile(recipe)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(nodes))
	}

	// Check dependencies are preserved
	if len(nodes[1].Dependencies) != 1 {
		t.Errorf("Expected step2 to have 1 dependency, got %d", len(nodes[1].Dependencies))
	}

	if nodes[1].Dependencies[0] != "step1" {
		t.Errorf("Expected step2 to depend on step1, got %s", nodes[1].Dependencies[0])
	}

	if len(nodes[2].Dependencies) != 1 {
		t.Errorf("Expected step3 to have 1 dependency, got %d", len(nodes[2].Dependencies))
	}

	if nodes[2].Dependencies[0] != "step2" {
		t.Errorf("Expected step3 to depend on step2, got %s", nodes[2].Dependencies[0])
	}
}

func TestRecipeCompilerCaptureBinding(t *testing.T) {
	compiler := NewCompiler()

	recipe := &ThoughtRecipe{
		ID:   "test-recipe",
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID:   "step1",
				Type: "llm",
				Captures: map[string]string{
					"output": "",
				},
			},
		},
	}

	nodes, err := compiler.Compile(recipe)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(nodes))
	}

	if nodes[0].Captures == nil {
		t.Error("Expected captures to be resolved")
	}

	if nodes[0].Captures["output"] != "euclo.recipe.test-recipe.output" {
		t.Errorf("Expected capture key euclo.recipe.test-recipe.output, got %s", nodes[0].Captures["output"])
	}
}

func TestRecipeCompilerContextHintBinding(t *testing.T) {
	compiler := NewCompiler()

	recipe := &ThoughtRecipe{
		ID:   "test-recipe",
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID:   "step1",
				Type: "llm",
				Bindings: map[string]string{
					"instruction": "{{instruction}}",
				},
			},
		},
	}

	nodes, err := compiler.Compile(recipe)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(nodes))
	}

	if nodes[0].Bindings == nil {
		t.Error("Expected bindings to be resolved")
	}

	if nodes[0].Bindings["instruction"] != "euclo.task_envelope.instruction" {
		t.Errorf("Expected binding euclo.task_envelope.instruction, got %s", nodes[0].Bindings["instruction"])
	}
}

func TestRecipeCompilerFamilyHintBinding(t *testing.T) {
	compiler := NewCompiler()

	recipe := &ThoughtRecipe{
		ID:   "test-recipe",
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID:   "step1",
				Type: "llm",
				Bindings: map[string]string{
					"family": "{{family_hint}}",
				},
			},
		},
	}

	nodes, err := compiler.Compile(recipe)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(nodes))
	}

	if nodes[0].Bindings == nil {
		t.Error("Expected bindings to be resolved")
	}

	if nodes[0].Bindings["family"] != "euclo.task_envelope.family_hint" {
		t.Errorf("Expected binding euclo.task_envelope.family_hint, got %s", nodes[0].Bindings["family"])
	}
}

func TestRecipeCompilerNilRecipe(t *testing.T) {
	compiler := NewCompiler()

	_, err := compiler.Compile(nil)
	if err == nil {
		t.Error("Expected error for nil recipe")
	}
}

func TestRecipeCompilerInvalidRecipe(t *testing.T) {
	compiler := NewCompiler()

	// Invalid recipe (missing ID)
	recipe := &ThoughtRecipe{
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID:   "step1",
				Type: "llm",
			},
		},
	}

	_, err := compiler.Compile(recipe)
	if err == nil {
		t.Error("Expected error for invalid recipe")
	}
}

func TestRecipeCompilerConfigPreserved(t *testing.T) {
	compiler := NewCompiler()

	recipe := &ThoughtRecipe{
		ID:   "test-recipe",
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID:   "step1",
				Type: "llm",
				Config: map[string]interface{}{
					"model":       "gpt-4",
					"temperature": 0.7,
				},
			},
		},
	}

	nodes, err := compiler.Compile(recipe)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if nodes[0].Config == nil {
		t.Error("Expected config to be preserved")
	}

	if nodes[0].Config["model"] != "gpt-4" {
		t.Errorf("Expected config model gpt-4, got %v", nodes[0].Config["model"])
	}

	if nodes[0].Config["temperature"] != 0.7 {
		t.Errorf("Expected config temperature 0.7, got %v", nodes[0].Config["temperature"])
	}
}

func TestRecipeCompilerPlanPropagatesCapabilityID(t *testing.T) {
	compiler := NewCompiler()

	recipe := &ThoughtRecipe{
		APIVersion: "euclo.v1",
		Kind:       "thought-recipe",
		Metadata: RecipeMetadata{
			Name: "Capability Recipe",
		},
		Sequence: RecipeSequence{
			Steps: []RecipeStep{
				{
					ID:           "step1",
					CapabilityID: "euclo:cap.ast_query",
					Bindings: map[string]string{
						"query": "euclo.task_envelope.instruction",
					},
				},
			},
		},
	}

	plan, err := compiler.CompilePlan(recipe, nil)
	if err != nil {
		t.Fatalf("CompilePlan failed: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].CapabilityID != "euclo:cap.ast_query" {
		t.Fatalf("expected capability ID to propagate, got %q", plan.Steps[0].CapabilityID)
	}
}

func TestRecipeCompilerPlanPreservesClarificationMetadata(t *testing.T) {
	compiler := NewCompiler()

	recipe := &ThoughtRecipe{
		APIVersion: "euclo.v1",
		Kind:       "thought-recipe",
		Metadata: RecipeMetadata{
			Name: "Clarification Recipe",
		},
		Sequence: RecipeSequence{
			Steps: []RecipeStep{
				{
					ID:   "extract",
					Type: string(ClarificationStepTypeExtract),
					Config: map[string]any{
						"output_schema_id": "clarification.answer.v1",
						"validation_mode":  "strict",
						"required_fields":  []string{"answer"},
					},
				},
			},
		},
	}

	plan, err := compiler.CompilePlan(recipe, nil)
	if err != nil {
		t.Fatalf("CompilePlan failed: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].Type != string(ClarificationStepTypeExtract) {
		t.Fatalf("expected clarification step type to propagate, got %q", plan.Steps[0].Type)
	}
	if plan.Steps[0].ClarificationConfig == nil {
		t.Fatal("expected clarification config to propagate")
	}
	if got := plan.Steps[0].ClarificationConfig.OutputSchemaID; got != "clarification.answer.v1" {
		t.Fatalf("unexpected output schema id: %q", got)
	}
	if got := plan.Steps[0].ClarificationConfig.ValidationMode; got != "strict" {
		t.Fatalf("unexpected validation mode: %q", got)
	}
}

func TestRecipeCompilerPlanCompilesGroups(t *testing.T) {
	compiler := NewCompiler()

	recipe := &ThoughtRecipe{
		APIVersion: "euclo.v1",
		Kind:       "thought-recipe",
		ID:         "grouped-recipe",
		Metadata: RecipeMetadata{
			Name: "Grouped Recipe",
		},
		Global: RecipeGlobal{
			Context: RecipeContext{
				Aliases: map[string]string{
					"shared": "euclo.recipe.shared.value",
				},
			},
		},
		Sequence: RecipeSequence{
			Steps: []RecipeStep{
				{
					ID:   "intro",
					Type: "llm",
				},
			},
			Parallel: []ParallelGroup{
				{
					ID:    "fanout",
					Merge: MergePolicyAll,
					Steps: []RecipeStep{
						{
							ID:   "left",
							Type: string(ClarificationStepTypeProject),
							Captures: map[string]string{
								"out": "",
							},
							Config: map[string]any{
								"output_schema_id": "clarification.project.v1",
							},
						},
						{
							ID:   "right",
							Type: "retrieve",
							Captures: map[string]string{
								"out": "",
							},
						},
					},
				},
			},
			Conditional: []ConditionalGroup{
				{
					ID:        "branch",
					Condition: "task.type == review",
					If: []RecipeStep{
						{
							ID:   "if_step",
							Type: "transform",
						},
					},
					Else: []RecipeStep{
						{
							ID:   "else_step",
							Type: "verify",
						},
					},
				},
			},
		},
	}

	plan, err := compiler.CompilePlan(recipe, nil)
	if err != nil {
		t.Fatalf("CompilePlan failed: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 top-level step, got %d", len(plan.Steps))
	}
	if len(plan.Parallel) != 1 {
		t.Fatalf("expected 1 parallel group, got %d", len(plan.Parallel))
	}
	if len(plan.Conditional) != 1 {
		t.Fatalf("expected 1 conditional group, got %d", len(plan.Conditional))
	}

	if got := plan.Parallel[0].Steps[0].Step.ID; got != "fanout.parallel.0.left" {
		t.Fatalf("unexpected parallel step ID: %s", got)
	}
	if got := plan.Parallel[0].Steps[0].Type; got != string(ClarificationStepTypeProject) {
		t.Fatalf("unexpected parallel step type: %s", got)
	}
	if plan.Parallel[0].Steps[0].ClarificationConfig == nil {
		t.Fatal("expected clarification config on parallel step")
	}
	if got := plan.Parallel[0].Steps[0].Captures["out"]; got != "euclo.recipe.grouped-recipe.fanout.parallel.0.out" {
		t.Fatalf("unexpected parallel capture key: %s", got)
	}
	if got := plan.Parallel[0].Steps[1].Step.ID; got != "fanout.parallel.1.right" {
		t.Fatalf("unexpected parallel step ID: %s", got)
	}
	if got := plan.Conditional[0].IfSteps[0].Step.ID; got != "branch.if.0.if_step" {
		t.Fatalf("unexpected conditional if-step ID: %s", got)
	}
	if got := plan.Conditional[0].ElseSteps[0].Step.ID; got != "branch.else.0.else_step" {
		t.Fatalf("unexpected conditional else-step ID: %s", got)
	}
}
