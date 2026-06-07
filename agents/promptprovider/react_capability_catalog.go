package promptprovider

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/execution/prompt"
)

type reactCapabilityCatalogProvider struct{}

func (reactCapabilityCatalogProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if len(ctx.Capabilities) == 0 {
		return prompt.ContextChunk{}
	}
	var lines []string
	for _, cap := range ctx.Capabilities {
		if cap.Kind == agentspec.CapabilityKindTool {
			continue
		}
		label := strings.TrimSpace(cap.Name)
		if label == "" {
			label = cap.ID
		}
		desc := strings.TrimSpace(cap.Description)
		if desc == "" {
			desc = string(cap.Kind)
		}
		lines = append(lines, fmt.Sprintf("- %s [%s]: %s", label, cap.Kind, truncate(desc, 120)))
	}
	if len(lines) == 0 {
		return prompt.ContextChunk{}
	}
	sort.Strings(lines)
	if len(lines) > 6 {
		lines = lines[:6]
	}
	return prompt.ContextChunk{Content: strings.Join(lines, "\n")}
}

func (reactCapabilityCatalogProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.capability_catalog",
		Description: "Lists non-tool capabilities available in the current execution.",
		Paradigms:   []string{"react"},
	}
}
