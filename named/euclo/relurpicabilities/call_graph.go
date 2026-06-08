package relurpicabilities

import (
	"context"
	"fmt"

	capability "codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/taxonomy"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
)

// CallGraphHandler implements the call graph traversal capability.
type CallGraphHandler struct {
	env agentenv.WorkspaceEnvironment
}

// NewCallGraphHandler creates a new call graph handler.
func NewCallGraphHandler(env agentenv.WorkspaceEnvironment) *CallGraphHandler {
	return &CallGraphHandler{env: env}
}

// Descriptor returns the capability descriptor for the call graph handler.
func (h *CallGraphHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) capability.CapabilityDescriptor {
	return capability.CapabilityDescriptor{
		ID:            "euclo:cap.call_graph",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          "Call Graph",
		Version:       "1.0.0",
		Description:   "Traverses call relationships to build a structured graph of nodes and edges",
		Category:      "code_analysis",
		Tags:          []string{"callgraph", "graph", "read-only"},
		Source: capability.CapabilitySource{
			Scope: taxonomy.CapabilityScopeBuiltin,
		},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		RiskClasses:   []taxonomy.RiskClass{taxonomy.RiskClassReadOnly},
		EffectClasses: []taxonomy.EffectClass{},
		InputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"entry_point": {
					Type:        "string",
					Description: "Symbol name to use as entry point for graph traversal",
				},
				"depth": {
					Type:        "integer",
					Description: "Maximum traversal depth (default: 3)",
				},
				"include_dependencies": {
					Type:        "boolean",
					Description: "Include dependency graph edges (default: false)",
				},
			},
			Required: []string{"entry_point"},
		},
		OutputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"success": {
					Type:        "boolean",
					Description: "True if graph built successfully",
				},
				"entry_point": {
					Type:        "string",
					Description: "The entry point symbol",
				},
				"nodes": {
					Type:        "array",
					Description: "Graph nodes",
					Items: &schemacoerce.Schema{
						Type: "object",
					},
				},
				"edges": {
					Type:        "array",
					Description: "Graph edges",
					Items: &schemacoerce.Schema{
						Type: "object",
					},
				},
			},
		},
	}
}

// Invoke executes the call graph traversal and returns structured nodes and edges.
func (h *CallGraphHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*ports.ToolResult, error) {
	// Extract arguments
	entryPoint, ok := stringArg(args, "entry_point")
	if !ok || entryPoint == "" {
		return failResult("entry_point argument is required and must be non-empty"), nil
	}

	depth, _ := intArg(args, "depth", 3)
	includeDeps, _ := args["include_dependencies"].(bool)

	// Check for IndexManager
	if h.env.IndexManager == nil {
		return failResult("IndexManager not available in environment"), nil
	}

	// Get call graph for entry point
	callGraph, err := h.env.IndexManager.GetCallGraph(entryPoint)
	if err != nil {
		return failResult(fmt.Sprintf("call graph lookup failed: %v", err)), nil
	}

	// Collect all nodes from the call graph
	nodeSet := make(map[string]*ast.Node)
	nodeSet[callGraph.Root.ID] = callGraph.Root

	// Add callees
	if rootCallees, ok := callGraph.Callees[callGraph.Root.ID]; ok {
		for _, callee := range rootCallees {
			nodeSet[callee.ID] = callee
		}
	}

	// Add callers
	if rootCallers, ok := callGraph.Callers[callGraph.Root.ID]; ok {
		for _, caller := range rootCallers {
			nodeSet[caller.ID] = caller
		}
	}

	// Optionally include dependency graph
	if includeDeps {
		depGraph, err := h.env.IndexManager.GetDependencyGraph(entryPoint)
		if err == nil {
			nodeSet[depGraph.Root.ID] = depGraph.Root
			for _, dep := range depGraph.Dependencies {
				nodeSet[dep.ID] = dep
			}
			for _, dependent := range depGraph.Dependents {
				nodeSet[dependent.ID] = dependent
			}
		}
	}

	// Convert nodes and edges to structured output
	nodes, edges := callGraphToNodesEdges(callGraph, nodeSet, depth)

	// Write retrieval references for all nodes
	var allNodes []*ast.Node
	for _, node := range nodeSet {
		allNodes = append(allNodes, node)
	}
	writeRetrievalReferences(env, "call_graph_"+entryPoint, allNodes)

	return &ports.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"success":     true,
			"entry_point": entryPoint,
			"nodes":       nodes,
			"edges":       edges,
		},
	}, nil
}
