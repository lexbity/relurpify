package lsp

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestLSPToolPermissionsValidate(t *testing.T) {
	tools := []core.Tool{
		&DefinitionTool{},
		&ReferencesTool{},
		&HoverTool{},
		&DiagnosticsTool{},
		&SearchSymbolsTool{},
		&DocumentSymbolsTool{},
		&FormatTool{},
	}
	for _, tool := range tools {
		if err := tool.Permissions().Validate(); err != nil {
			t.Fatalf("%s permissions invalid: %v", tool.Name(), err)
		}
	}
}
