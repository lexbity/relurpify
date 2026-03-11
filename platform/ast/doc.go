// Package ast exposes AST-based code intelligence as agent-callable tools.
//
// ASTSymbolProvider (ast_symbol_provider.go) queries the framework/ast index
// to resolve symbol definitions, extracting name, kind, file, and line
// information without re-parsing source files. ast_tool.go wraps the provider
// as a registered capability tool, making symbol lookup available to any
// agent with the appropriate permission.
package ast
