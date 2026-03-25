package rewoo

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
)

func sharedContextPromptBlock(shared *core.SharedContext, policy *contextmgr.ContextPolicy) string {
	sections := make([]string, 0, 2)
	if fileSection := sharedFileContextPromptBlock(shared); fileSection != "" {
		sections = append(sections, fileSection)
	}
	if itemSection := managedReferencePromptBlock(policy); itemSection != "" {
		sections = append(sections, itemSection)
	}
	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func sharedFileContextPromptBlock(shared *core.SharedContext) string {
	if shared == nil {
		return ""
	}
	refs := shared.WorkingSetReferences()
	if len(refs) == 0 {
		return ""
	}
	lines := make([]string, 0, len(refs))
	for _, ref := range refs {
		path := strings.TrimSpace(ref.URI)
		if path == "" {
			path = strings.TrimSpace(ref.ID)
		}
		if path == "" {
			continue
		}
		line := path
		if ref.Detail != "" {
			line += fmt.Sprintf(" [detail=%s]", ref.Detail)
		}
		if fc, ok := shared.GetFile(path); ok && fc != nil {
			snippet := strings.TrimSpace(fc.Summary)
			if snippet == "" {
				snippet = strings.TrimSpace(fc.Content)
			}
			if snippet != "" {
				line += ": " + truncatePromptSnippet(snippet, 180)
			}
		}
		lines = append(lines, "- "+line)
	}
	if len(lines) == 0 {
		return ""
	}
	sort.Strings(lines)
	return "Shared file context:\n" + strings.Join(lines, "\n")
}

func managedReferencePromptBlock(policy *contextmgr.ContextPolicy) string {
	if policy == nil || policy.ContextManager == nil {
		return ""
	}
	items := policy.ContextManager.GetItems()
	if len(items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(items))
	seen := make(map[string]struct{})
	for _, item := range items {
		refItem, ok := item.(core.ReferenceCapableContextItem)
		if !ok {
			continue
		}
		refs := refItem.References()
		if len(refs) == 0 {
			continue
		}
		for _, ref := range refs {
			key := string(ref.Kind) + "|" + ref.ID + "|" + ref.URI
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			line := formatReferencePromptLine(ref, item)
			if line != "" {
				lines = append(lines, "- "+line)
			}
		}
	}
	if len(lines) == 0 {
		return ""
	}
	sort.Strings(lines)
	return "Reference context:\n" + strings.Join(lines, "\n")
}

func formatReferencePromptLine(ref core.ContextReference, item core.ContextItem) string {
	parts := make([]string, 0, 3)
	label := strings.TrimSpace(ref.URI)
	if label == "" {
		label = strings.TrimSpace(ref.ID)
	}
	if label == "" {
		label = string(ref.Kind)
	}
	parts = append(parts, fmt.Sprintf("%s (%s)", label, ref.Kind))
	if ref.Detail != "" {
		parts = append(parts, "detail="+ref.Detail)
	}
	if summary := contextItemPromptSummary(item); summary != "" {
		parts = append(parts, summary)
	} else {
		parts = append(parts, "reference only")
	}
	return strings.Join(parts, " | ")
}

func contextItemPromptSummary(item core.ContextItem) string {
	switch typed := item.(type) {
	case *core.FileContextItem:
		if typed.Summary != "" {
			return truncatePromptSnippet(typed.Summary, 180)
		}
		if typed.Content != "" {
			return truncatePromptSnippet(typed.Content, 180)
		}
	case *core.MemoryContextItem:
		if typed.Summary != "" {
			return truncatePromptSnippet(typed.Summary, 180)
		}
		if typed.Content != "" {
			return truncatePromptSnippet(typed.Content, 180)
		}
	case *core.RetrievalContextItem:
		if typed.Summary != "" {
			return truncatePromptSnippet(typed.Summary, 180)
		}
		if typed.Content != "" {
			return truncatePromptSnippet(typed.Content, 180)
		}
	}
	return ""
}

func truncatePromptSnippet(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
