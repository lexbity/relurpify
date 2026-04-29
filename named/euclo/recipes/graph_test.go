package recipe

import (
	"testing"
)

func TestGraphBuilderBuildsGraph(t *testing.T) {
	builder := NewGraphBuilder()

	nodes := []CompiledNode{
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
	}

	graph, err := builder.Build(nodes)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(graph.Nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(graph.Nodes))
	}

	// Check edges
	if len(graph.Edges["step1"]) != 1 {
		t.Errorf("Expected step1 to have 1 edge, got %d", len(graph.Edges["step1"]))
	}

	if graph.Edges["step1"][0] != "step2" {
		t.Errorf("Expected step1 -> step2 edge, got step1 -> %s", graph.Edges["step1"][0])
	}
}

func TestGraphBuilderTopologicalOrder(t *testing.T) {
	builder := NewGraphBuilder()

	nodes := []CompiledNode{
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
	}

	graph, err := builder.Build(nodes)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	order, err := builder.TopologicalOrder(graph)
	if err != nil {
		t.Fatalf("TopologicalOrder failed: %v", err)
	}

	if len(order) != 3 {
		t.Errorf("Expected 3 nodes in order, got %d", len(order))
	}

	// Check that step1 comes before step2
	step1Idx := -1
	step2Idx := -1
	for i, nodeID := range order {
		if nodeID == "step1" {
			step1Idx = i
		}
		if nodeID == "step2" {
			step2Idx = i
		}
	}

	if step1Idx == -1 || step2Idx == -1 {
		t.Error("Expected step1 and step2 in order")
	}

	if step1Idx >= step2Idx {
		t.Error("Expected step1 to come before step2")
	}
}

func TestGraphBuilderHandlesCycles(t *testing.T) {
	builder := NewGraphBuilder()

	nodes := []CompiledNode{
		{
			ID:           "step1",
			Type:         "llm",
			Dependencies: []string{"step2"},
		},
		{
			ID:           "step2",
			Type:         "retrieve",
			Dependencies: []string{"step1"},
		},
	}

	_, err := builder.Build(nodes)
	if err == nil {
		t.Error("Expected error for cyclic graph")
	}
}

func TestGraphBuilderEmptyNodes(t *testing.T) {
	builder := NewGraphBuilder()

	_, err := builder.Build([]CompiledNode{})
	if err == nil {
		t.Error("Expected error for empty nodes")
	}
}

func TestGraphBuilderMissingDependency(t *testing.T) {
	builder := NewGraphBuilder()

	nodes := []CompiledNode{
		{
			ID:   "step1",
			Type: "llm",
		},
		{
			ID:           "step2",
			Type:         "retrieve",
			Dependencies: []string{"nonexistent"},
		},
	}

	_, err := builder.Build(nodes)
	if err == nil {
		t.Error("Expected error for missing dependency")
	}
}

func TestGraphBuilderSingleNode(t *testing.T) {
	builder := NewGraphBuilder()

	nodes := []CompiledNode{
		{
			ID:   "step1",
			Type: "llm",
		},
	}

	graph, err := builder.Build(nodes)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(graph.Nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(graph.Nodes))
	}

	order, err := builder.TopologicalOrder(graph)
	if err != nil {
		t.Fatalf("TopologicalOrder failed: %v", err)
	}

	if len(order) != 1 {
		t.Errorf("Expected 1 node in order, got %d", len(order))
	}

	if order[0] != "step1" {
		t.Errorf("Expected step1 in order, got %s", order[0])
	}
}

func TestGraphBuilderMultipleDependencies(t *testing.T) {
	builder := NewGraphBuilder()

	nodes := []CompiledNode{
		{
			ID:   "step1",
			Type: "llm",
		},
		{
			ID:   "step2",
			Type: "retrieve",
		},
		{
			ID:           "step3",
			Type:         "transform",
			Dependencies: []string{"step1", "step2"},
		},
	}

	graph, err := builder.Build(nodes)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(graph.Edges["step1"]) != 1 {
		t.Errorf("Expected step1 to have 1 edge, got %d", len(graph.Edges["step1"]))
	}

	if len(graph.Edges["step2"]) != 1 {
		t.Errorf("Expected step2 to have 1 edge, got %d", len(graph.Edges["step2"]))
	}
}

func TestGraphBuilderDAG(t *testing.T) {
	builder := NewGraphBuilder()

	nodes := []CompiledNode{
		{
			ID:   "a",
			Type: "llm",
		},
		{
			ID:   "b",
			Type: "retrieve",
		},
		{
			ID:           "c",
			Type:         "transform",
			Dependencies: []string{"a"},
		},
		{
			ID:           "d",
			Type:         "emit",
			Dependencies: []string{"b"},
		},
		{
			ID:           "e",
			Type:         "gate",
			Dependencies: []string{"c", "d"},
		},
	}

	graph, err := builder.Build(nodes)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	order, err := builder.TopologicalOrder(graph)
	if err != nil {
		t.Fatalf("TopologicalOrder failed: %v", err)
	}

	// Verify topological ordering constraints
	aIdx := indexOf(order, "a")
	cIdx := indexOf(order, "c")
	if aIdx >= cIdx {
		t.Error("Expected a to come before c")
	}

	bIdx := indexOf(order, "b")
	dIdx := indexOf(order, "d")
	if bIdx >= dIdx {
		t.Error("Expected b to come before d")
	}

	eIdx := indexOf(order, "e")
	if cIdx >= eIdx {
		t.Error("Expected c to come before e")
	}
	if dIdx >= eIdx {
		t.Error("Expected d to come before e")
	}
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}
