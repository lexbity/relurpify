package search

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	platformsearch "codeburg.org/lexbit/relurpify/platform/search"
)

func Constructors() map[string]ports.NativeToolConstructor {
	return map[string]ports.NativeToolConstructor{
		"search_grep":         func(basePath string) ports.Tool { return &platformsearch.GrepTool{BasePath: basePath} },
		"search_find_similar": func(basePath string) ports.Tool { return &platformsearch.SimilarityTool{BasePath: basePath} },
		"search_semantic":     func(basePath string) ports.Tool { return &platformsearch.SemanticSearchTool{BasePath: basePath} },
	}
}

func init() {
	for k, v := range Constructors() {
		ports.RegisterNative(k, v)
	}
}
