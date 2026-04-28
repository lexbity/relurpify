package ast

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
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
func (t *ASTTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "action", Type: "string", Description: "list_symbols|get_signature|find_callers|find_callees|get_imports|get_dependencies|search", Required: true},
		{Name: "symbol", Type: "string", Description: "Target symbol name", Required: false},
		{Name: "type", Type: "string", Description: "Filter by node type", Required: false},
		{Name: "category", Type: "string", Description: "Filter by category", Required: false},
		{Name: "exported_only", Type: "boolean", Description: "Only include exported symbols", Required: false},
	}
}

func (t *ASTTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
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
	if waitCtx == nil {
		waitCtx = context.Background()
	}
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

func (t *ASTTool) querySymbol(args map[string]interface{}) (*Node, error) {
	symbol := fmt.Sprint(args["symbol"])
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

func (t *ASTTool) handleList(args map[string]interface{}) (*contracts.ToolResult, error) {
	query := NodeQuery{Limit: 100}
	if nodeType := fmt.Sprint(args["type"]); nodeType != "" {
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
	return successResult(map[string]interface{}{
		"symbols": summarizeNodes(nodes),
		"count":   len(nodes),
	}), nil
}

func (t *ASTTool) handleSignature(args map[string]interface{}) (*contracts.ToolResult, error) {
	node, err := t.querySymbol(args)
	if err != nil {
		return nil, err
	}
	return successResult(map[string]interface{}{
		"name":       node.Name,
		"type":       node.Type,
		"signature":  node.Signature,
		"doc_string": node.DocString,
		"file_id":    node.FileID,
		"line":       node.StartLine,
		"exported":   node.IsExported,
	}), nil
}

func (t *ASTTool) handleCallers(args map[string]interface{}) (*contracts.ToolResult, error) {
	node, err := t.querySymbol(args)
	if err != nil {
		return nil, err
	}
	callers, err := t.manager.Store().GetCallers(node.ID)
	if err != nil {
		return nil, err
	}
	return successResult(map[string]interface{}{
		"symbol":  node.Name,
		"callers": summarizeNodes(callers),
	}), nil
}

func (t *ASTTool) handleCallees(args map[string]interface{}) (*contracts.ToolResult, error) {
	node, err := t.querySymbol(args)
	if err != nil {
		return nil, err
	}
	callees, err := t.manager.Store().GetCallees(node.ID)
	if err != nil {
		return nil, err
	}
	return successResult(map[string]interface{}{
		"symbol":  node.Name,
		"callees": summarizeNodes(callees),
	}), nil
}

func (t *ASTTool) handleImports(args map[string]interface{}) (*contracts.ToolResult, error) {
	node, err := t.querySymbol(args)
	if err != nil {
		return nil, err
	}
	imports, err := t.manager.Store().GetImports(node.ID)
	if err != nil {
		return nil, err
	}
	return successResult(map[string]interface{}{
		"symbol":  node.Name,
		"imports": summarizeNodes(imports),
	}), nil
}

func (t *ASTTool) handleDependencies(args map[string]interface{}) (*contracts.ToolResult, error) {
	symbol := fmt.Sprint(args["symbol"])
	if symbol == "" {
		return nil, fmt.Errorf("symbol parameter required")
	}
	graph, err := t.manager.GetDependencyGraph(symbol)
	if err != nil {
		return nil, err
	}
	return successResult(map[string]interface{}{
		"symbol":       symbol,
		"dependencies": summarizeNodes(graph.Dependencies),
		"dependents":   summarizeNodes(graph.Dependents),
	}), nil
}

func (t *ASTTool) IsAvailable(ctx context.Context) bool {
	return t.manager != nil
}

func (t *ASTTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{
		Permissions: &contracts.PermissionSet{},
	}
}
func (t *ASTTool) Tags() []string { return []string{contracts.TagReadOnly, "ast", "symbol", "recovery"} }

func successResult(data map[string]interface{}) *contracts.ToolResult {
	return &contracts.ToolResult{
		Success: true,
		Data:    data,
	}
}

func summarizeNodes(nodes []*Node) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id":        node.ID,
			"name":      node.Name,
			"type":      node.Type,
			"signature": node.Signature,
			"file_id":   node.FileID,
			"line":      node.StartLine,
			"exported":  node.IsExported,
		})
	}
	return result
}
