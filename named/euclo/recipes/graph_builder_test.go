package recipe

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
)

func TestBuildRecipeGraphWiresLinearParallelAndConditionalSections(t *testing.T) {
	compiler := NewCompiler()

	recipe := &ThoughtRecipe{
		APIVersion: "euclo.v1",
		Kind:       "thought-recipe",
		ID:         "graph-recipe",
		Metadata: RecipeMetadata{
			Name: "Graph Recipe",
		},
		Sequence: RecipeSequence{
			Steps: []RecipeStep{
				{ID: "intro", Type: "llm"},
			},
			Parallel: []ParallelGroup{
				{
					ID:    "fanout",
					Merge: MergePolicyAll,
					Steps: []RecipeStep{
						{ID: "left", Type: "llm"},
						{ID: "right", Type: "retrieve"},
					},
				},
			},
			Conditional: []ConditionalGroup{
				{
					ID:        "branch",
					Condition: "task.type == review",
					If: []RecipeStep{
						{ID: "if_step", Type: "transform"},
					},
					Else: []RecipeStep{
						{ID: "else_step", Type: "verify"},
					},
				},
			},
		},
	}

	plan, err := compiler.CompilePlan(recipe, nil)
	if err != nil {
		t.Fatalf("CompilePlan failed: %v", err)
	}

	graph, err := BuildRecipeGraph(plan, agentenv.WorkspaceEnvironment{}, nil)
	if err != nil {
		t.Fatalf("BuildRecipeGraph failed: %v", err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("graph validation failed: %v", err)
	}

	if got := graph.StartNodeID(); got != "intro.execute" {
		t.Fatalf("unexpected start node: %s", got)
	}
	if !graph.HasNode("euclo.recipe.group.fanout.parallel") {
		t.Fatal("expected parallel group node to exist")
	}
	if !graph.HasNode("euclo.recipe.group.branch.conditional") {
		t.Fatal("expected conditional group node to exist")
	}
	if !graph.HasNode("euclo.recipe.group.branch.join") {
		t.Fatal("expected conditional join node to exist")
	}

	if got := edgeTargets(graph.OutgoingEdges("intro.execute")); len(got) != 1 || got[0] != "euclo.recipe.group.fanout.parallel" {
		t.Fatalf("unexpected intro outgoing edges: %v", got)
	}
	if got := edgeTargets(graph.OutgoingEdges("euclo.recipe.group.fanout.parallel")); !containsAll(got, []string{"fanout.parallel.0.left.execute", "fanout.parallel.1.right.execute", "euclo.recipe.group.branch.conditional"}) {
		t.Fatalf("unexpected parallel targets: %v", got)
	}
	if got := edgeTargets(graph.OutgoingEdges("euclo.recipe.group.branch.conditional")); !containsAll(got, []string{"branch.if.0.if_step.execute", "branch.else.0.else_step.execute"}) {
		t.Fatalf("unexpected conditional targets: %v", got)
	}
}

func edgeTargets(edges []agentgraph.Edge) []string {
	targets := make([]string, 0, len(edges))
	for _, edge := range edges {
		targets = append(targets, edge.To)
	}
	return targets
}

func containsAll(values []string, want []string) bool {
	if len(values) != len(want) {
		return false
	}
	seen := make(map[string]int, len(values))
	for _, v := range values {
		seen[v]++
	}
	for _, w := range want {
		if seen[w] == 0 {
			return false
		}
		seen[w]--
	}
	return true
}
