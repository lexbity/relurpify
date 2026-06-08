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

// ASTQueryHandler implements the AST query capability for searching code structure.
type ASTQueryHandler struct {
	env agentenv.WorkspaceEnvironment
}

// NewASTQueryHandler creates a new AST query handler.
func NewASTQueryHandler(env agentenv.WorkspaceEnvironment) *ASTQueryHandler {
	return &ASTQueryHandler{env: env}
}

// Descriptor returns the capability descriptor for the AST query handler.
func (h *ASTQueryHandler) Descriptor(ctx context.Context, env ports.State) capability.CapabilityDescriptor {
	return capability.CapabilityDescriptor{
		ID:            "euclo:cap.ast_query",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          "AST Query",
		Version:       "1.0.0",
		Description:   "Queries the AST index to find symbols, functions, classes, and other code structure elements",
		Category:      "code_analysis",
		Tags:          []string{"ast", "query", "read-only"},
		Source: capability.CapabilitySource{
			Scope: taxonomy.CapabilityScopeBuiltin,
		},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		RiskClasses:   []taxonomy.RiskClass{taxonomy.RiskClassReadOnly},
		EffectClasses: []taxonomy.EffectClass{},
		InputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"query": {
					Type:        "string",
					Description: "Symbol name or pattern to search for",
				},
				"types": {
					Type:        "array",
					Description: "Filter by node types (e.g., function, class, struct)",
					Items: &schemacoerce.Schema{
						Type: "string",
					},
				},
				"languages": {
					Type:        "array",
					Description: "Filter by programming languages",
					Items: &schemacoerce.Schema{
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
		OutputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
				"success": {
					Type:        "boolean",
					Description: "True if query executed successfully",
				},
				"matches": {
					Type:        "array",
					Description: "Matching AST nodes",
					Items: &schemacoerce.Schema{
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
func (h *ASTQueryHandler) Invoke(ctx context.Context, st ports.State, args map[string]interface{}) (*ports.ToolResult, error) {
	env := contextdata.EnvelopeFromState(st)
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

	return &ports.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"success":     true,
			"matches":     matches,
			"total_found": len(nodes),
		},
	}, nil
}
