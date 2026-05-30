package lsp

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformlsp "codeburg.org/lexbit/relurpify/platform/lsp"
)

func init() {
	contracts.RegisterNative("lsp_get_definition", func(basePath string) contracts.Tool {
		return &platformlsp.DefinitionTool{}
	})
	contracts.RegisterNative("lsp_get_references", func(basePath string) contracts.Tool {
		return &platformlsp.ReferencesTool{}
	})
	contracts.RegisterNative("lsp_get_hover", func(basePath string) contracts.Tool {
		return &platformlsp.HoverTool{}
	})
	contracts.RegisterNative("lsp_get_diagnostics", func(basePath string) contracts.Tool {
		return &platformlsp.DiagnosticsTool{}
	})
	contracts.RegisterNative("lsp_search_symbols", func(basePath string) contracts.Tool {
		return &platformlsp.SearchSymbolsTool{}
	})
	contracts.RegisterNative("lsp_document_symbols", func(basePath string) contracts.Tool {
		return &platformlsp.DocumentSymbolsTool{}
	})
	contracts.RegisterNative("lsp_format", func(basePath string) contracts.Tool {
		return &platformlsp.FormatTool{}
	})
}
