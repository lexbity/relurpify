package lsp

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformlsp "codeburg.org/lexbit/relurpify/platform/lsp"
)

func init() {
	ports.RegisterNative("lsp_get_definition", func(basePath string) ports.Tool {
		return &platformlsp.DefinitionTool{}
	})
	ports.RegisterNative("lsp_get_references", func(basePath string) ports.Tool {
		return &platformlsp.ReferencesTool{}
	})
	ports.RegisterNative("lsp_get_hover", func(basePath string) ports.Tool {
		return &platformlsp.HoverTool{}
	})
	ports.RegisterNative("lsp_get_diagnostics", func(basePath string) ports.Tool {
		return &platformlsp.DiagnosticsTool{}
	})
	ports.RegisterNative("lsp_search_symbols", func(basePath string) ports.Tool {
		return &platformlsp.SearchSymbolsTool{}
	})
	ports.RegisterNative("lsp_document_symbols", func(basePath string) ports.Tool {
		return &platformlsp.DocumentSymbolsTool{}
	})
	ports.RegisterNative("lsp_format", func(basePath string) ports.Tool {
		return &platformlsp.FormatTool{}
	})
}
