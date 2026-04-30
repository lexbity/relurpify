package relurpicabilities

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// SymbolTraceHandler implements the symbol trace capability for call graph analysis.
type SymbolTraceHandler struct {
	env agentenv.WorkspaceEnvironment
}

// NewSymbolTraceHandler creates a new symbol trace handler.
func NewSymbolTraceHandler(env agentenv.WorkspaceEnvironment) *SymbolTraceHandler {
	return &SymbolTraceHandler{env: env}
}

// Descriptor returns the capability descriptor for the symbol trace handler.
func (h *SymbolTraceHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:cap.symbol_trace",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "Symbol Trace",
		Version:       "1.0.0",
		Description:   "Traces call relationships for a symbol to find callers and callees",
		Category:      "code_analysis",
		Tags:          []string{"callgraph", "trace", "read-only"},
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeBuiltin,
		},
		TrustClass:    core.TrustClassBuiltinTrusted,
		RiskClasses:   []core.RiskClass{core.RiskClassReadOnly},
		EffectClasses: []core.EffectClass{},
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"symbol": {
					Type:        "string",
					Description: "Symbol name to trace",
				},
			},
			Required: []string{"symbol"},
		},
		OutputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
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
					Items: &core.Schema{
						Type: "object",
					},
				},
				"callers": {
					Type:        "array",
					Description: "Functions that call this symbol",
					Items: &core.Schema{
						Type: "object",
					},
				},
			},
		},
	}
}

// Invoke executes the symbol trace and returns call graph information.
func (h *SymbolTraceHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
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

	return &core.CapabilityExecutionResult{
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
