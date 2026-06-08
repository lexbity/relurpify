package relurpicabilities

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	"codeburg.org/lexbit/relurpify/governance/taxonomy"
)

// SymbolTraceHandler implements the symbol trace capability for call graph analysis.
type SymbolTraceHandler struct {
	env agentenv.AgentContext
}

// NewSymbolTraceHandler creates a new symbol trace handler.
func NewSymbolTraceHandler(env agentenv.AgentContext) *SymbolTraceHandler {
	return &SymbolTraceHandler{env: env}
}

// Descriptor returns the capability descriptor for the symbol trace handler.
func (h *SymbolTraceHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.symbol_trace",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          "Symbol Trace",
		Version:       "1.0.0",
		Description:   "Traces call relationships for a symbol to find callers and callees",
		Category:      "code_analysis",
		Tags:          []string{"callgraph", "trace", "read-only"},
		Source: descriptor.CapabilitySource{
			Scope: taxonomy.CapabilityScopeBuiltin,
		},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		RiskClasses:   []taxonomy.RiskClass{taxonomy.RiskClassReadOnly},
		EffectClasses: []taxonomy.EffectClass{},
		InputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"symbol": {
					Type:        "string",
					Description: "Symbol name to trace",
				},
			},
			Required: []string{"symbol"},
		},
		OutputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"success": {
					Type:        "boolean",
					Description: "True if trace executed successfully",
				},
				"symbol": {
					Type:        "string",
					Description: "The traced symbol name",
				},
				"root": {
					Type:        "object",
					Description: "Root symbol node",
				},
				"callees": {
					Type:        "array",
					Description: "Functions called by this symbol",
					Items: &schemacoerce.Schema{
						Type: "object",
					},
				},
				"callers": {
					Type:        "array",
					Description: "Functions that call this symbol",
					Items: &schemacoerce.Schema{
						Type: "object",
					},
				},
			},
		},
	}
}

// Invoke executes the symbol trace and returns call graph information.
func (h *SymbolTraceHandler) Invoke(ctx context.Context, env ports.State, args map[string]interface{}) (*ports.ToolResult, error) {
	// Extract arguments
	symbol, ok := stringArg(args, "symbol")
	if !ok || symbol == "" {
		return failResult("symbol argument is required and must be non-empty"), nil
	}

	// Check for IndexManager
	if h.env.IndexManager == nil {
		return failResult("IndexManager not available in environment"), nil
	}

	// Get call graph
	callGraph, err := h.env.IndexManager.GetCallGraph(symbol)
	if err != nil {
		return failResult(fmt.Sprintf("trace failed: %v", err)), nil
	}

	// Convert root node to map
	rootMap := map[string]interface{}{
		"id":          callGraph.Root.ID,
		"name":        callGraph.Root.Name,
		"type":        string(callGraph.Root.Type),
		"category":    string(callGraph.Root.Category),
		"language":    callGraph.Root.Language,
		"start_line":  callGraph.Root.StartLine,
		"end_line":    callGraph.Root.EndLine,
		"file_id":     callGraph.Root.FileID,
		"signature":   callGraph.Root.Signature,
		"is_exported": callGraph.Root.IsExported,
	}

	// Get callees and callers from the call graph
	var callees []*ast.Node
	var callers []*ast.Node
	if rootCallees, ok := callGraph.Callees[callGraph.Root.ID]; ok {
		callees = rootCallees
	}
	if rootCallers, ok := callGraph.Callers[callGraph.Root.ID]; ok {
		callers = rootCallers
	}

	// Convert to trace entries
	entries := traceEntries(callees, callers)

	return &ports.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"success": true,
			"symbol":  symbol,
			"root":    rootMap,
			"callees": traceEntries(callees, []*ast.Node{}),
			"callers": traceEntries([]*ast.Node{}, callers),
			"entries": entries,
		},
	}, nil
}
