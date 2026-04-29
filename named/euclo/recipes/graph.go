package recipe

import (
	"fmt"
)

// Graph represents an execution graph with nodes and edges.
type Graph struct {
	Nodes map[string]*GraphNode
	Edges map[string][]string // node ID -> dependent node IDs
}

// GraphNode represents a node in the execution graph.
type GraphNode struct {
	ID           string
	Type         string
	Description  string
	Config       map[string]interface{}
	Captures     map[string]string
	Bindings     map[string]string
}

// GraphBuilder constructs execution graphs from compiled recipe nodes.
type GraphBuilder struct{}

// NewGraphBuilder creates a new graph builder.
func NewGraphBuilder() *GraphBuilder {
	return &GraphBuilder{}
}

// Build constructs a graph from compiled nodes with topological ordering.
func (b *GraphBuilder) Build(nodes []CompiledNode) (*Graph, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes to build graph")
	}

	graph := &Graph{
		Nodes: make(map[string]*GraphNode),
		Edges: make(map[string][]string),
	}

	// Add all nodes to the graph
	for _, node := range nodes {
		graphNode := &GraphNode{
			ID:           node.ID,
			Type:         node.Type,
			Description:  node.Description,
			Config:       node.Config,
			Captures:     node.Captures,
			Bindings:     node.Bindings,
		}
		graph.Nodes[node.ID] = graphNode
		graph.Edges[node.ID] = []string{}
	}

	// Add edges based on dependencies
	for _, node := range nodes {
		for _, dep := range node.Dependencies {
			if _, ok := graph.Nodes[dep]; !ok {
				return nil, fmt.Errorf("dependency %s not found for node %s", dep, node.ID)
			}
			graph.Edges[dep] = append(graph.Edges[dep], node.ID)
		}
	}

	// Check for cycles
	if err := b.detectCycles(graph); err != nil {
		return nil, fmt.Errorf("cycle detected in graph: %w", err)
	}

	return graph, nil
}

// detectCycles detects cycles in the graph using DFS.
func (b *GraphBuilder) detectCycles(graph *Graph) error {
	visited := make(map[string]bool)
	recursionStack := make(map[string]bool)

	for nodeID := range graph.Nodes {
		if !visited[nodeID] {
			if err := b.dfsVisit(nodeID, graph, visited, recursionStack); err != nil {
				return err
			}
		}
	}

	return nil
}

// dfsVisit performs DFS visit for cycle detection.
func (b *GraphBuilder) dfsVisit(nodeID string, graph *Graph, visited, recursionStack map[string]bool) error {
	visited[nodeID] = true
	recursionStack[nodeID] = true

	for _, neighbor := range graph.Edges[nodeID] {
		if !visited[neighbor] {
			if err := b.dfsVisit(neighbor, graph, visited, recursionStack); err != nil {
				return err
			}
		} else if recursionStack[neighbor] {
			return fmt.Errorf("cycle detected involving node %s", neighbor)
		}
	}

	recursionStack[nodeID] = false
	return nil
}

// TopologicalOrder returns nodes in topological order.
func (b *GraphBuilder) TopologicalOrder(graph *Graph) ([]string, error) {
	// Kahn's algorithm for topological sorting
	inDegree := make(map[string]int)
	for nodeID := range graph.Nodes {
		inDegree[nodeID] = 0
	}

	for _, deps := range graph.Edges {
		for _, dep := range deps {
			inDegree[dep]++
		}
	}

	queue := []string{}
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	result := []string{}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		for _, neighbor := range graph.Edges[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(result) != len(graph.Nodes) {
		return nil, fmt.Errorf("graph has a cycle")
	}

	return result, nil
}
