package lsp

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformlsp "codeburg.org/lexbit/relurpify/platform/lsp"
)

func Constructors() map[string]ports.NativeToolConstructor {
	return map[string]ports.NativeToolConstructor{
		"lsp_get_definition":  func(basePath string) ports.Tool { return &platformlsp.DefinitionTool{} },
		"lsp_get_references":  func(basePath string) ports.Tool { return &platformlsp.ReferencesTool{} },
		"lsp_get_hover":       func(basePath string) ports.Tool { return &platformlsp.HoverTool{} },
		"lsp_get_diagnostics": func(basePath string) ports.Tool { return &platformlsp.DiagnosticsTool{} },
		"lsp_search_symbols":  func(basePath string) ports.Tool { return &platformlsp.SearchSymbolsTool{} },
		"lsp_document_symbols": func(basePath string) ports.Tool { return &platformlsp.DocumentSymbolsTool{} },
		"lsp_format":          func(basePath string) ports.Tool { return &platformlsp.FormatTool{} },
	}
}

func init() {
	for k, v := range Constructors() {
		ports.RegisterNative(k, v)
	}
}
