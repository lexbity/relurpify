package stages

import (
	// Re-export LinkStage from chainer to avoid circular imports.
	// LinkStage is implemented in chainer/stage_adapter.go because:
	// - LinkStage needs to know about chainer.Link
	// - stages needs to know about Link for helper functions
	// - If LinkStage was in stages, chainer would need to import stages,
	//   creating a circular dependency
	//
	// Solution: LinkStage lives in the chainer package, stages provides helpers
	chaineradapter "codeburg.org/lexbit/relurpify/agents/chainer"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// LinkStage is re-exported from the chainer package to preserve the public API.
// See chainer/stage_adapter.go for the implementation.
type LinkStage = chaineradapter.LinkStage

// NewLinkStage is re-exported from the chainer package.
func NewLinkStage(link *chaineradapter.Link, model core.LanguageModel) *LinkStage {
	return chaineradapter.NewLinkStage(link, model)
}

// NewLinkStageWithOptions is re-exported from the chainer package.
func NewLinkStageWithOptions(link *chaineradapter.Link, model core.LanguageModel, opts *core.LLMOptions) *LinkStage {
	return chaineradapter.NewLinkStageWithOptions(link, model, opts)
}
