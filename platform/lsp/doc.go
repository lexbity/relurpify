// Package lsp integrates Language Server Protocol (LSP) capabilities into the
// Relurpify agent tool surface.
//
// lsp.go implements an LSP client that communicates with language servers
// over JSON-RPC (sourcegraph/jsonrpc2). lsp_process_client.go launches and
// manages the language server subprocess for a given workspace and language.
//
// Exposed agent tools include: go-to-definition, hover documentation,
// find-references, document symbols, and workspace diagnostics. These tools
// give agents IDE-quality code intelligence without requiring file parsing in
// the agent loop itself.
package lsp
