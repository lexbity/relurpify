package ast

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

const (
	String_lsp_ast_tool = "string"
	Symbol_lsp_ast_tool = "symbol"
	Type_lsp_ast_tool = "type"
)


const astToolReadyTimeout = 2 * time.Second

// ASTTool exposes the AST index for querying.
type ASTTool struct {
	manager *IndexManager
}

// NewASTTool constructs a tool backed by an IndexManager.
func NewASTTool(manager *IndexManager) *ASTTool {
	return &ASTTool{manager: manager}
}

func (t *ASTTool) Name() string { return "query_ast" }
func (t *ASTTool) Description() string {
	return "Query the universal AST index to explore symbols, callers, callees, and dependencies without loading entire files."
}
func (t *ASTTool) Category() string { return "search" }
func (t *ASTTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{
		{Name: "action", Type: String_lsp_ast_tool, Description: "list_symbols|get_signature|find_callers|find_callees|get_imports|get_dependencies|search", Required: true},
		{Name: Symbol_lsp_ast_tool, Type: String_lsp_ast_tool, Description: "Target symbol name", Required: false},
		{Name: Type_lsp_ast_tool, Type: String_lsp_ast_tool, Description: "Filter by node type", Required: false},
		{Name: "category", Type: String_lsp_ast_tool, Description: "Filter by category", Required: false},
		{Name: "exported_only", Type: "boolean", Description: "Only include exported symbols", Required: false},
	}
}

func (t *ASTTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	if t.manager == nil {
		return nil, fmt.Errorf("ast index unavailable")
	}
	if err := t.waitUntilReady(ctx, astToolReadyTimeout); err != nil {
		return nil, err
	}
	action := fmt.Sprint(args["action"])
	switch action {
	case "list_symbols", "search":
		return t.handleList(args)
	case "get_signature":
		return t.handleSignature(args)
	case "find_callers":
		return t.handleCallers(args)
	case "find_callees":
		return t.handleCallees(args)
	case "get_imports":
		return t.handleImports(args)
	case "get_dependencies":
		return t.handleDependencies(args)
	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
}

func (t *ASTTool) waitUntilReady(ctx context.Context, timeout time.Duration) error {
	if t == nil || t.manager == nil || t.manager.Ready() {
		return nil
	}
	waitCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(waitCtx, timeout)
		defer cancel()
	}
	if err := t.manager.WaitUntilReady(waitCtx); err != nil {
		return fmt.Errorf("wait for ast index readiness: %w", err)
	}
	return nil
}

func (t *ASTTool) querySymbol(args map[string]any) (*Node, error) {
	symbol := fmt.Sprint(args[Symbol_lsp_ast_tool])
	if symbol == "" {
		return nil, fmt.Errorf("symbol parameter required")
	}
	nodes, err := t.manager.QuerySymbol(symbol)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("symbol %s not found", symbol)
	}
	return nodes[0], nil
}

func (t *ASTTool) handleList(args map[string]any) (*ports.ToolResult, error) {
	query := NodeQuery{Limit: 100}
	if nodeType := fmt.Sprint(args[Type_lsp_ast_tool]); nodeType != "" {
		query.Types = []NodeType{NodeType(nodeType)}
	}
	if category := fmt.Sprint(args["category"]); category != "" {
		query.Categories = []Category{Category(category)}
	}
	if exportedOnly, ok := args["exported_only"].(bool); ok {
		query.IsExported = &exportedOnly
	}
	nodes, err := t.manager.SearchNodes(query)
	if err != nil {
		return nil, err
	}
	return successResult(map[string]any{
		"symbols": summarizeNodes(nodes),
		"count":   len(nodes),
	}), nil
}

func (t *ASTTool) handleSignature(args map[string]any) (*ports.ToolResult, error) {
	node, err := t.querySymbol(args)
	if err != nil {
		return nil, err
	}
	return successResult(map[string]any{
		"name":       node.Name,
		Type_lsp_ast_tool:       node.Type,
		"signature":  node.Signature,
		"doc_string": node.DocString,
		"file_id":    node.FileID,
		"line":       node.StartLine,
		"exported":   node.IsExported,
	}), nil
}

func (t *ASTTool) handleCallers(args map[string]any) (*ports.ToolResult, error) {
	node, err := t.querySymbol(args)
	if err != nil {
		return nil, err
	}
	callers, err := t.manager.Store().GetCallers(node.ID)
	if err != nil {
		return nil, err
	}
	return successResult(map[string]any{
		Symbol_lsp_ast_tool:  node.Name,
		"callers": summarizeNodes(callers),
	}), nil
}

func (t *ASTTool) handleCallees(args map[string]any) (*ports.ToolResult, error) {
	node, err := t.querySymbol(args)
	if err != nil {
		return nil, err
	}
	callees, err := t.manager.Store().GetCallees(node.ID)
	if err != nil {
		return nil, err
	}
	return successResult(map[string]any{
		Symbol_lsp_ast_tool:  node.Name,
		"callees": summarizeNodes(callees),
	}), nil
}

func (t *ASTTool) handleImports(args map[string]any) (*ports.ToolResult, error) {
	node, err := t.querySymbol(args)
	if err != nil {
		return nil, err
	}
	imports, err := t.manager.Store().GetImports(node.ID)
	if err != nil {
		return nil, err
	}
	return successResult(map[string]any{
		Symbol_lsp_ast_tool:  node.Name,
		"imports": summarizeNodes(imports),
	}), nil
}

func (t *ASTTool) handleDependencies(args map[string]any) (*ports.ToolResult, error) {
	symbol := fmt.Sprint(args[Symbol_lsp_ast_tool])
	if symbol == "" {
		return nil, fmt.Errorf("symbol parameter required")
	}
	graph, err := t.manager.GetDependencyGraph(symbol)
	if err != nil {
		return nil, err
	}
	return successResult(map[string]any{
		Symbol_lsp_ast_tool:       symbol,
		"dependencies": summarizeNodes(graph.Dependencies),
		"dependents":   summarizeNodes(graph.Dependents),
	}), nil
}

func (t *ASTTool) IsAvailable(ctx context.Context) bool {
	return t.manager != nil
}

func (t *ASTTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{
		Permissions: &permissions.PermissionSet{},
	}
}
func (t *ASTTool) Tags() []string {
	return []string{toolcapabilities.TagReadOnly, "ast", Symbol_lsp_ast_tool, "recovery"}
}

func successResult(data map[string]any) *ports.ToolResult {
	return &ports.ToolResult{
		Success: true,
		Data:    data,
	}
}

func summarizeNodes(nodes []*Node) []map[string]any {
	result := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		result = append(result, map[string]any{
			"id":        node.ID,
			"name":      node.Name,
			Type_lsp_ast_tool:      node.Type,
			"signature": node.Signature,
			"file_id":   node.FileID,
			"line":      node.StartLine,
			"exported":  node.IsExported,
		})
	}
	return result
}
