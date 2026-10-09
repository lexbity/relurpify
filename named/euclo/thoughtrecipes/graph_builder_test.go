package thoughtrecipe

import (
	"fmt"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

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
	graph, err := BuildThoughtRecipeGraph(plan, &paradigm.Deps{}, nil)
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
	graph, err := BuildThoughtRecipeGraph(plan, &paradigm.Deps{}, nil)
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

func TestFallbackStepInheritsParentScope(t *testing.T) {
	parent := ExecutionStep{
		ID:    "parent.step",
		Kind:  StepKindRun,
		Scope: AllowTools([]string{"file_write"}),
	}
	agent := &surface.ThoughtRecipeStepAgent{
		Paradigm: "react",
		Prompt:   "fallback",
	}
	fallback := buildFallbackStep(fallbackExecutionStep(parent, agent))

	if !fallback.Scope.IsResolved() {
		t.Fatal("fallback scope should be resolved")
	}
	got := fallback.Scope.AllowedToolNames()
	if !equalStringSlices(got, []string{"file_write"}) {
		t.Fatalf("fallback allowed tools = %#v, want [file_write]", got)
	}
}

// fallbackExecutionStep constructs a primary step carrying an authored fallback
// agent, the shape surface code builds (D7).
func fallbackExecutionStep(parent ExecutionStep, agent *surface.ThoughtRecipeStepAgent) ExecutionStep {
	parent.Fallback = agent
	return parent
}

// TestBuildThoughtRecipeGraphWiresAuthoredFallback pins the D7 wiring: the
// primary exec node carries a success edge and an operational-class-only
// fallback edge to the fallback node.
func TestBuildThoughtRecipeGraphWiresAuthoredFallback(t *testing.T) {
	primary := ExecutionStep{
		ID:       "primary.step",
		Kind:     StepKindRun,
		Paradigm: "react",
		Goal:     "do it",
		Scope:    AllowAll(),
		Fallback: &surface.ThoughtRecipeStepAgent{Paradigm: "planner", Prompt: "fallback"},
	}
	plan := &ExecutionPlan{
		ThoughtRecipe: &surface.ThoughtRecipe{ID: "fallback.graph", Name: "fallback graph"},
		Steps:         []ExecutionStep{primary},
	}
	graph, err := BuildThoughtRecipeGraph(plan, &paradigm.Deps{}, nil)
	if err != nil {
		t.Fatalf("BuildThoughtRecipeGraph: %v", err)
	}
	execID := "primary.step.execute"
	if !graph.HasNode(execID) {
		t.Fatalf("missing primary exec node %s", execID)
	}
	if !graph.HasNode("primary.step.fallback") {
		t.Fatal("missing fallback node primary.step.fallback")
	}

	var fallbackEdge *agentgraph.Edge
	var successEdge *agentgraph.Edge
	for i := range graph.OutgoingEdges(execID) {
		edge := graph.OutgoingEdges(execID)[i]
		switch edge.To {
		case "primary.step.fallback":
			fallbackEdge = &edge
		case "euclo.execution.done":
			successEdge = &edge
		}
	}
	if fallbackEdge == nil {
		t.Fatal("expected an edge from primary to the fallback node")
	}
	if successEdge == nil {
		t.Fatal("expected the success edge from primary to continue the recipe")
	}

	// Classified operational failure fires the fallback.
	if !fallbackEdge.Condition(&execution.Result{Success: false, Metadata: map[string]any{
		operationalFailureKindMetadata: string(euclotypes.FailureModelUnavailable),
	}}, nil) {
		t.Fatal("operational failure must fire the fallback edge")
	}
	// A structured success must not fire it.
	if fallbackEdge.Condition(&execution.Result{Success: true}, nil) {
		t.Fatal("success must not fire the fallback edge")
	}
	// An unclassified failure must not fire it either (D7 tightening).
	if fallbackEdge.Condition(&execution.Result{Success: false, Metadata: map[string]any{}}, nil) {
		t.Fatal("unclassified failure must not fire the fallback edge")
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
