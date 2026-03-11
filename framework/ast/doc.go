// Package ast provides AST parsing and code indexing for Go and Markdown
// source files, enabling agents to navigate workspace symbols without loading
// entire files into context.
//
// # Parsers
//
// Language-specific parsers (parser_go.go, parser_markdown.go) extract
// structured symbol trees from source files. parser.go provides the common
// Parser interface and language auto-detection via language_detector.go.
//
// # Index
//
// IndexManager coordinates incremental re-indexing as files change.
// IndexStore persists symbol records in a SQLite database (sqlite_store.go)
// for fast lookup by name, kind, or file path. Agents query the index to
// locate definitions, references, and documentation anchors before deciding
// which files to read.
//
// # Symbol types
//
// symbols.go defines the Symbol type hierarchy: functions, methods, types,
// constants, variables, and Markdown headings. node.go represents the raw
// AST node from which a symbol was extracted.
package ast
