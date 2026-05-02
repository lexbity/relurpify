package promptprovider

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/prompt"
)

type reactContextFilesProvider struct{}

func (reactContextFilesProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Task == nil || ctx.Task.Context == nil {
		return prompt.ContextChunk{}
	}
	raw, ok := ctx.Task.Context["context_file_contents"]
	if !ok || raw == nil {
		return prompt.ContextChunk{}
	}
	switch v := raw.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return prompt.ContextChunk{}
		}
		return prompt.ContextChunk{Content: s}
	case map[string]any:
		return prompt.ContextChunk{Content: renderFileMap(v)}
	case map[string]string:
		return prompt.ContextChunk{Content: renderStringFileMap(v)}
	default:
		items := toSliceOfAny(raw)
		if len(items) > 0 {
			return prompt.ContextChunk{Content: renderFileList(items)}
		}
		s := strings.TrimSpace(fmt.Sprint(raw))
		if s == "" {
			return prompt.ContextChunk{}
		}
		return prompt.ContextChunk{Content: s}
	}
}

func (reactContextFilesProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.context_files",
		Description: "Supplies context file contents from task.Context[\"context_file_contents\"].",
		Paradigms:   []string{"react"},
	}
}

func renderFileMap(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	var parts []string
	for path, content := range m {
		s := strings.TrimSpace(fmt.Sprint(content))
		if s == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("// %s\n%s", path, s))
	}
	return strings.Join(parts, "\n\n")
}

func renderStringFileMap(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	var parts []string
	for path, content := range m {
		s := strings.TrimSpace(content)
		if s == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("// %s\n%s", path, s))
	}
	return strings.Join(parts, "\n\n")
}

func renderFileList(items []any) string {
	var parts []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path := extractStringField(m, "path")
		if path == "" {
			path = extractStringField(m, "name")
		}
		content := extractStringField(m, "content")
		if content == "" {
			content = extractStringField(m, "text")
		}
		if content == "" {
			continue
		}
		entry := strings.TrimSpace(content)
		if path != "" {
			entry = fmt.Sprintf("// %s\n%s", path, entry)
		}
		parts = append(parts, entry)
	}
	return strings.Join(parts, "\n\n")
}
