package ast

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
)

// DocumentSymbolToolProvider wraps the lsp_document_symbols tool so AST
// indexing can source symbols through the existing permission and proxy
// infrastructure.
type DocumentSymbolToolProvider struct {
	tool ports.Tool
}

// NewDocumentSymbolToolProvider builds a provider that executes the wrapped
// tool directly. The supplied tool should be fetched from a CapabilityRegistry
// so it carries the permission manager wrapper.
func NewDocumentSymbolToolProvider(tool ports.Tool) *DocumentSymbolToolProvider {
	if tool == nil {
		return nil
	}
	return &DocumentSymbolToolProvider{tool: tool}
}

// DocumentSymbols implements ast.DocumentSymbolProvider.
func (p *DocumentSymbolToolProvider) DocumentSymbols(ctx context.Context, path string) ([]DocumentSymbol, error) {
	if p == nil || p.tool == nil {
		return nil, fmt.Errorf("document symbol tool unavailable")
	}
	res, err := p.tool.Execute(ctx, map[string]interface{}{"file": path})
	if err != nil {
		return nil, err
	}
	raw, ok := res.Data["symbols"]
	if !ok {
		return nil, fmt.Errorf("document symbols payload missing")
	}
	info, err := castSymbolInformation(raw)
	if err != nil {
		return nil, err
	}
	return convertSymbolInformation(info), nil
}

// AttachASTSymbolProvider inspects the registry for the LSP document symbols
// tool and wires it into the AST indexer when present.
func AttachASTSymbolProvider(manager *IndexManager, registry *registry.CapabilityRegistry) {
	if manager == nil || registry == nil {
		return
	}
	tool, ok := registry.Get("lsp_document_symbols")
	if !ok || tool == nil {
		return
	}
	if !tool.IsAvailable(context.Background()) {
		return
	}
	provider := NewDocumentSymbolToolProvider(tool)
	manager.UseSymbolProvider(provider)
}

type rawSymbolInfo struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Location string `json:"location"`
}

func castSymbolInformation(raw interface{}) ([]rawSymbolInfo, error) {
	if raw == nil {
		return nil, fmt.Errorf("empty symbol payload")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal raw symbols: %w", err)
	}
	var list []rawSymbolInfo
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("failed to unmarshal raw symbols: %w", err)
	}
	return list, nil
}

func convertSymbolInformation(input []rawSymbolInfo) []DocumentSymbol {
	result := make([]DocumentSymbol, 0, len(input))
	for _, sym := range input {
		line := extractLine(sym.Location)
		nodeType := mapSymbolKind(sym.Kind)
		result = append(result, DocumentSymbol{
			Name:      sym.Name,
			Kind:      nodeType,
			StartLine: line,
			EndLine:   line,
		})
	}
	return result
}

func extractLine(location string) int {
	parts := strings.Split(location, ":")
	if len(parts) < 2 {
		return 1
	}
	line, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 1
	}
	// LSP lines are zero-based; shift to one-based for AST storage.
	return line + 1
}

func mapSymbolKind(kind string) NodeType {
	switch kind {
	case "5": // Class
		return NodeTypeClass
	case "6": // Method
		return NodeTypeMethod
	case "7", "8": // Property/Field
		return NodeTypeVariable
	case "9": // Constructor
		return NodeTypeFunction
	case "10": // Enum
		return NodeTypeEnum
	case "11": // Interface
		return NodeTypeInterface
	case "12": // Function
		return NodeTypeFunction
	case "13": // Variable
		return NodeTypeVariable
	case "14": // Constant
		return NodeTypeConstant
	case "23": // Struct
		return NodeTypeStruct
	default:
		return NodeTypeSection
	}
}
