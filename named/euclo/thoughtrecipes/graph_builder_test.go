package thoughtrecipe

import (
	"fmt"
	"testing"

	"codeburg.org/lexbit/relurpify/execution/agentenv"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

func TestBuildThoughtRecipeGraphWiresLinearParallelAndConditionalSections(t *testing.T) {
	thoughtrecipe := &surface.ThoughtRecipe{
		ID:   "graph-thoughtrecipe",
		Name: "Graph ThoughtRecipe",
		Metadata: surface.ThoughtRecipeMetadata{
			Name: "Graph ThoughtRecipe",
		},
	}

	plan := &ExecutionPlan{
		ThoughtRecipe: thoughtrecipe,
		Steps: []ExecutionStep{{
			ID:       "intro",
			Type:     "run",
			Paradigm: "goalcon",
			Goal:     "Introduce the thoughtrecipe and continue.",
			Prompt:   "Introduce the thoughtrecipe and continue.",
			Step: surface.ThoughtRecipeStep{
				ID:      "intro",
				Type:    "run",
				Prompt:  "Introduce the thoughtrecipe and continue.",
				Parent:  surface.ThoughtRecipeStepAgent{Paradigm: "goalcon"},
				Config:  map[string]any{},
				Context: surface.ThoughtRecipeStepContext{},
			},
		}},
		Parallel: []CompiledParallelGroup{{
			Group: &surface.ParallelGroup{ID: "fanout", Merge: surface.MergePolicyAll},
			Steps: []CompiledStep{
				{
					Step: &surface.ThoughtRecipeStep{ID: "fanout.parallel.0.left", Type: "run", Prompt: "Process the left branch.", Parent: surface.ThoughtRecipeStepAgent{Paradigm: "goalcon"}, Context: surface.ThoughtRecipeStepContext{}},
					Type: "run",
				},
				{
					Step: &surface.ThoughtRecipeStep{ID: "fanout.parallel.1.right", Type: "run", Prompt: "Process the right branch.", Parent: surface.ThoughtRecipeStepAgent{Paradigm: "goalcon"}, Context: surface.ThoughtRecipeStepContext{}},
					Type: "run",
				},
			},
			Merge: string(surface.MergePolicyAll),
		}},
		Conditional: []CompiledConditionalGroup{{
			Group:     &surface.ConditionalGroup{ID: "branch", Condition: "thoughtrecipe.branch"},
			Condition: "thoughtrecipe.branch",
			IfSteps: []CompiledStep{{
				Step: &surface.ThoughtRecipeStep{ID: "branch.if.0.if_step", Type: "run", Prompt: "Handle the primary branch.", Parent: surface.ThoughtRecipeStepAgent{Paradigm: "goalcon"}, Context: surface.ThoughtRecipeStepContext{}},
				Type: "run",
			}},
			ElseSteps: []CompiledStep{{
				Step: &surface.ThoughtRecipeStep{ID: "branch.else.0.else_step", Type: "run", Prompt: "Handle the fallback branch.", Parent: surface.ThoughtRecipeStepAgent{Paradigm: "goalcon"}, Context: surface.ThoughtRecipeStepContext{}},
				Type: "run",
			}},
		}},
	}

	graph, err := BuildThoughtRecipeGraph(plan, agentenv.AgentContext{}, nil)
	if err != nil {
		t.Fatalf("BuildThoughtRecipeGraph failed: %v", err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("graph validation failed: %v", err)
	}

	if got := graph.StartNodeID(); got != "intro.execute" {
		t.Fatalf("unexpected start node: %s", got)
	}
	if !graph.HasNode("euclo.execution.group.fanout.parallel") {
		t.Fatal("expected parallel group node to exist")
	}
	if !graph.HasNode("euclo.execution.group.branch.conditional") {
		t.Fatal("expected conditional group node to exist")
	}
	if !graph.HasNode("euclo.execution.group.branch.join") {
		t.Fatal("expected conditional join node to exist")
	}

	if got := edgeTargets(graph.OutgoingEdges("intro.execute")); len(got) != 1 || got[0] != "euclo.execution.group.fanout.parallel" {
		t.Fatalf("unexpected intro outgoing edges: %v", got)
	}
	if got := edgeTargets(graph.OutgoingEdges("euclo.execution.group.fanout.parallel")); !containsAll(got, []string{"fanout.parallel.0.left.execute", "fanout.parallel.1.right.execute", "euclo.execution.group.branch.conditional"}) {
		t.Fatalf("unexpected parallel targets: %v", got)
	}
	if got := edgeTargets(graph.OutgoingEdges("euclo.execution.group.branch.conditional")); !containsAll(got, []string{"branch.if.0.if_step.execute", "branch.else.0.else_step.execute"}) {
		t.Fatalf("unexpected conditional targets: %v", got)
	}
}

func TestBuildThoughtRecipeGraphWiresRouteBranchesWithFirstMatchFallback(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe route_graph
"Route graph."

trigger as capability:
  may read workspace

agent reviewer uses react

route:
  when state.intent is review:
    run reviewer:
      goal "Review the prompt."
  when state.intent confidence below 70%:
    run reviewer:
      goal "Resolve the intent."
  otherwise:
    run reviewer:
      goal "Fallback."
`)

	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	graph, err := BuildThoughtRecipeGraph(plan, agentenv.AgentContext{}, nil)
	if err != nil {
		t.Fatalf("BuildThoughtRecipeGraph failed: %v", err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("graph validation failed: %v", err)
	}
	if len(plan.Routes) != 1 {
		t.Fatalf("expected one route group, got %d", len(plan.Routes))
	}

	groupID := plan.Routes[0].Group.ID
	routeID := fmt.Sprintf("euclo.execution.group.%s.route", groupID)
	branch0 := fmt.Sprintf("euclo.execution.group.%s.branch.0", groupID)
	branch1 := fmt.Sprintf("euclo.execution.group.%s.branch.1", groupID)
	branch0Step := plan.Routes[0].Branches[0].Steps[0].ID + ".execute"
	branch1Step := plan.Routes[0].Branches[1].Steps[0].ID + ".execute"
	branch2Step := plan.Routes[0].Branches[2].Steps[0].ID + ".execute"

	if got := graph.StartNodeID(); got != routeID {
		t.Fatalf("unexpected start node: %s", got)
	}
	if !graph.HasNode(branch0) || !graph.HasNode(branch1) {
		t.Fatalf("expected route predicate nodes %q and %q", branch0, branch1)
	}
	if got := edgeTargets(graph.OutgoingEdges(routeID)); len(got) != 1 || got[0] != branch0 {
		t.Fatalf("unexpected route root outgoing edges: %v", got)
	}
	if got := edgeTargets(graph.OutgoingEdges(branch0)); !containsAll(got, []string{branch0Step, branch1}) {
		t.Fatalf("unexpected branch 0 outgoing edges: %v", got)
	}
	if got := edgeTargets(graph.OutgoingEdges(branch1)); !containsAll(got, []string{branch1Step, branch2Step}) {
		t.Fatalf("unexpected branch 1 outgoing edges: %v", got)
	}
}

func TestBuildThoughtRecipeGraphWiresPipelineStagesInOrder(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe pipeline_graph
"Pipeline graph."

trigger as capability:
  may read workspace

agent explorer uses react
agent reviewer uses planner

pipeline:
  stage explore:
    run explorer:
      goal "Explore the workspace."
  stage summarize:
    run reviewer:
      from state.findings
      goal "Summarize the findings."
`)

	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	graph, err := BuildThoughtRecipeGraph(plan, agentenv.AgentContext{}, nil)
	if err != nil {
		t.Fatalf("BuildThoughtRecipeGraph failed: %v", err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("graph validation failed: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected one lowered pipeline step, got %d", len(plan.Steps))
	}
	step := plan.Steps[0]
	pipelineID := step.ID + ".pipeline"
	stage0ID := step.ID + ".stage.0.explore"
	stage1ID := step.ID + ".stage.1.summarize"
	stage0Body := step.PipelineStages[0].Steps[0].ID + ".execute"
	stage1Body := step.PipelineStages[1].Steps[0].ID + ".execute"

	if got := graph.StartNodeID(); got != pipelineID {
		t.Fatalf("unexpected start node: %s", got)
	}
	if !graph.HasNode(pipelineID) || !graph.HasNode(stage0ID) || !graph.HasNode(stage1ID) {
		t.Fatalf("expected pipeline and stage nodes to exist")
	}
	if got := edgeTargets(graph.OutgoingEdges(pipelineID)); len(got) != 1 || got[0] != stage0ID {
		t.Fatalf("unexpected pipeline outgoing edges: %v", got)
	}
	if got := edgeTargets(graph.OutgoingEdges(stage0ID)); len(got) != 1 || got[0] != stage0Body {
		t.Fatalf("unexpected stage 0 outgoing edges: %v", got)
	}
	if got := edgeTargets(graph.OutgoingEdges(stage0Body)); len(got) != 1 || got[0] != stage1ID {
		t.Fatalf("unexpected stage 0 body outgoing edges: %v", got)
	}
	if got := edgeTargets(graph.OutgoingEdges(stage1ID)); len(got) != 1 || got[0] != stage1Body {
		t.Fatalf("unexpected stage 1 outgoing edges: %v", got)
	}
	if got := edgeTargets(graph.OutgoingEdges(stage1Body)); len(got) != 1 || got[0] != step.ID+".join" {
		t.Fatalf("unexpected stage 1 body outgoing edges: %v", got)
	}
}

func TestExecutionStepFromAgentInheritsParentToolScope(t *testing.T) {
	parent := ExecutionStep{
		ID:                 "parent.step",
		ToolScopes:         []ToolScopeFrame{{ScopeKind: "run", ToolNames: []string{"file_write"}}},
		EffectiveToolNames: []string{"file_write"},
		Step: surface.ThoughtRecipeStep{
			ID: "parent.step",
			Config: map[string]any{
				"tool_scopes":          []map[string]any{{"scope_kind": "run", "tool_names": []string{"file_write"}}},
				"effective_tool_names": []string{"file_write"},
			},
		},
	}
	step := executionStepFromAgent("fallback.step", &surface.ThoughtRecipeStepAgent{
		Paradigm: "react",
		Prompt:   "fallback",
	}, parent)

	if got, want := len(step.ToolScopes), 1; got != want {
		t.Fatalf("fallback tool scope count = %d, want %d", got, want)
	}
	if got, want := step.EffectiveToolNames, []string{"file_write"}; !equalStringSlices(got, want) {
		t.Fatalf("fallback effective tools = %#v, want %#v", got, want)
	}
	if got, ok := step.Step.Config["effective_tool_names"].([]string); !ok || !equalStringSlices(got, []string{"file_write"}) {
		t.Fatalf("fallback step config effective_tool_names = %#v", step.Step.Config["effective_tool_names"])
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
