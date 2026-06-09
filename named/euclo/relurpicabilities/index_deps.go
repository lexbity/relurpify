package relurpicabilities

import "codeburg.org/lexbit/relurpify/context/knowledge/ast"

// SymbolQuerier resolves a symbol name to AST nodes.
type SymbolQuerier interface {
	QuerySymbol(name string) ([]*ast.Node, error)
}

// NodeSearcher searches nodes in the AST index by query.
type NodeSearcher interface {
	SearchNodes(query ast.NodeQuery) ([]*ast.Node, error)
}

// GraphReader reads call and dependency graphs for a symbol.
type GraphReader interface {
	GetCallGraph(symbol string) (*ast.CallGraph, error)
	GetDependencyGraph(symbol string) (*ast.DependencyGraph, error)
}

// EdgeStore provides direct access to edges and nodes in the index store.
type EdgeStore interface {
	GetEdgesByType(edgeType ast.EdgeType) ([]*ast.Edge, error)
	GetNode(nodeID string) (*ast.Node, error)
	GetFile(fileID string) (*ast.FileMetadata, error)
	GetFileByPath(path string) (*ast.FileMetadata, error)
}

// IndexDeps bundles the index-family dependencies a handler needs.
type IndexDeps struct {
	Searcher  NodeSearcher
	Grapher   GraphReader
	Store     EdgeStore
	Workspace string
}
