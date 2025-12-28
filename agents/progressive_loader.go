package agents

import (
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/search"
)

type ProgressiveLoader = contextmgr.ProgressiveLoader

func NewProgressiveLoader(
	contextManager *contextmgr.ContextManager,
	indexManager *ast.IndexManager,
	searchEngine *search.SearchEngine,
	budget *core.ContextBudget,
	summarizer core.Summarizer,
) *ProgressiveLoader {
	return contextmgr.NewProgressiveLoader(contextManager, indexManager, searchEngine, budget, summarizer)
}
