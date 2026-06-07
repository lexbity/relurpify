package search

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformsearch "codeburg.org/lexbit/relurpify/platform/search"
)

func init() {
	ports.RegisterNative("search_grep", func(basePath string) ports.Tool {
		return &platformsearch.GrepTool{BasePath: basePath}
	})
	ports.RegisterNative("search_find_similar", func(basePath string) ports.Tool {
		return &platformsearch.SimilarityTool{BasePath: basePath}
	})
	ports.RegisterNative("search_semantic", func(basePath string) ports.Tool {
		return &platformsearch.SemanticSearchTool{BasePath: basePath}
	})
}
