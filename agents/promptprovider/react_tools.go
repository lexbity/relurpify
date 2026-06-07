package promptprovider

import (
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/execution/prompt"
)

type reactToolsProvider struct{}

func (reactToolsProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if len(ctx.Tools) == 0 {
		return prompt.ContextChunk{}
	}
	type entry struct{ name, desc string }
	entries := make([]entry, 0, len(ctx.Tools))
	for _, t := range ctx.Tools {
		entries = append(entries, entry{name: t.Name(), desc: truncate(t.Description(), 120)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	var lines []string
	for _, e := range entries {
		if e.desc != "" {
			lines = append(lines, "- "+e.name+": "+e.desc)
		} else {
			lines = append(lines, "- "+e.name)
		}
	}
	return prompt.ContextChunk{Content: strings.Join(lines, "\n")}
}

func (reactToolsProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.tools",
		Description: "Lists available tool names and descriptions for the current phase.",
		Paradigms:   []string{"react"},
	}
}
