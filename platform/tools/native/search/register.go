package search

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	platformsearch "codeburg.org/lexbit/relurpify/platform/search"
)

func init() {
	contracts.RegisterNative("search_grep", func(basePath string) contracts.Tool {
		return &platformsearch.GrepTool{BasePath: basePath}
	})
	contracts.RegisterNative("search_find_similar", func(basePath string) contracts.Tool {
		return &platformsearch.SimilarityTool{BasePath: basePath}
	})
	contracts.RegisterNative("search_semantic", func(basePath string) contracts.Tool {
		return &platformsearch.SemanticSearchTool{BasePath: basePath}
	})
}
