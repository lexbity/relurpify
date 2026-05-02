package relurpicabilities

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ASTQueryHandler implements the AST query capability for searching code structure.
type ASTQueryHandler struct {
	env agentenv.WorkspaceEnvironment
}

// NewASTQueryHandler creates a new AST query handler.
func NewASTQueryHandler(env agentenv.WorkspaceEnvironment) *ASTQueryHandler {
	return &ASTQueryHandler{env: env}
}

// Descriptor returns the capability descriptor for the AST query handler.
func (h *ASTQueryHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:cap.ast_query",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "AST Query",
		Version:       "1.0.0",
		Description:   "Queries the AST index to find symbols, functions, classes, and other code structure elements",
		Category:      "code_analysis",
		Tags:          []string{"ast", "query", "read-only"},
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeBuiltin,
		},
		TrustClass:    core.TrustClassBuiltinTrusted,
		RiskClasses:   []core.RiskClass{core.RiskClassReadOnly},
		EffectClasses: []core.EffectClass{},
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"query": {
					Type:        "string",
					Description: "Symbol name or pattern to search for",
				},
				"types": {
					Type:        "array",
					Description: "Filter by node types (e.g., function, class, struct)",
					Items: &core.Schema{
						Type: "string",
					},
				},
				"languages": {
					Type:        "array",
					Description: "Filter by programming languages",
					Items: &core.Schema{
						Type: "string",
					},
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum number of results (default: 20)",
				},
			},
			Required: []string{"query"},
		},
		OutputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"success": {
					Type:        "boolean",
					Description: "True if query executed successfully",
				},
				"matches": {
					Type:        "array",
					Description: "Matching AST nodes",
					Items: &core.Schema{
						Type: "object",
					},
				},
				"total_found": {
					Type:        "integer",
					Description: "Total number of matches found",
				},
			},
		},
	}
}

// Invoke executes the AST query and returns matching nodes.
func (h *ASTQueryHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	// Extract arguments
	query, ok := stringArg(args, "query")
	if !ok || query == "" {
		return failResult("query argument is required and must be non-empty"), nil
	}

	limit, _ := intArg(args, "limit", 20)

	// Check for IndexManager
	if h.env.IndexManager == nil {
		return failResult("IndexManager not available in environment"), nil
	}

	// Build node query
	nodeQuery := ast.NodeQuery{
		NamePattern: query,
		Limit:       limit,
	}

	// Add type filters if provided
	if types, ok := args["types"].([]interface{}); ok {
		for _, t := range types {
			if typeStr, ok := t.(string); ok {
				nodeQuery.Types = append(nodeQuery.Types, ast.NodeType(typeStr))
			}
		}
	}

	// Add language filters if provided
	if languages, ok := args["languages"].([]interface{}); ok {
		for _, lang := range languages {
			if langStr, ok := lang.(string); ok {
				nodeQuery.Languages = append(nodeQuery.Languages, langStr)
			}
		}
	}

	// Execute query
	nodes, err := h.env.IndexManager.SearchNodes(nodeQuery)
	if err != nil {
		return failResult(fmt.Sprintf("query failed: %v", err)), nil
	}

	// Convert nodes to match entries
	matches := nodesToMatchEntries(nodes)

	// Write retrieval reference to envelope
	writeRetrievalReferences(env, query, nodes)

	return &contracts.CapabilityExecutionResult{
		Success: true,
		Data: map[string]interface{}{
			"success":     true,
			"matches":     matches,
			"total_found": len(nodes),
		},
	}, nil
}
