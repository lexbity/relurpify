package agents

import contextmgr "codeburg.org/lexbit/relurpify/framework/contextmgr"

type (
	ContextStrategy  = contextmgr.ContextStrategy
	ContextRequest   = contextmgr.ContextRequest
	FileRequest      = contextmgr.FileRequest
	DetailLevel      = contextmgr.DetailLevel
	ASTQuery         = contextmgr.ASTQuery
	ASTQueryType     = contextmgr.ASTQueryType
	ASTFilter        = contextmgr.ASTFilter
	MemoryQuery      = contextmgr.MemoryQuery
	SearchQuery      = contextmgr.SearchQuery
	ContextLoadEvent = contextmgr.ContextLoadEvent
)

const (
	DetailFull          = contextmgr.DetailFull
	DetailDetailed      = contextmgr.DetailDetailed
	DetailConcise       = contextmgr.DetailConcise
	DetailMinimal       = contextmgr.DetailMinimal
	DetailSignatureOnly = contextmgr.DetailSignatureOnly

	ASTQueryListSymbols     = contextmgr.ASTQueryListSymbols
	ASTQueryGetSignature    = contextmgr.ASTQueryGetSignature
	ASTQueryFindCallers     = contextmgr.ASTQueryFindCallers
	ASTQueryFindCallees     = contextmgr.ASTQueryFindCallees
	ASTQueryGetDependencies = contextmgr.ASTQueryGetDependencies
)

func NewAggressiveStrategy() *contextmgr.ProfiledStrategy {
	return contextmgr.NewAggressiveStrategy()
}
func NewConservativeStrategy() *contextmgr.ProfiledStrategy {
	return contextmgr.NewConservativeStrategy()
}
func NewAdaptiveStrategy() *contextmgr.AdaptiveStrategy { return contextmgr.NewAdaptiveStrategy() }

func ExtractFileReferences(text string) []string   { return contextmgr.ExtractFileReferences(text) }
func ExtractSymbolReferences(text string) []string { return contextmgr.ExtractSymbolReferences(text) }
func ExtractKeywords(text string) string           { return contextmgr.ExtractKeywords(text) }
func ContainsInsensitive(text, substr string) bool {
	return contextmgr.ContainsInsensitive(text, substr)
}
