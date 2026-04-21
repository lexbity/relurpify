package stages

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func taskInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.Instruction)
}

func workspaceRoot(task *core.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	raw, ok := task.Context["workspace"]
	if !ok || raw == nil {
		return ""
	}
	s, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func renderContextFiles(task *core.Task, maxBytes int) string {
	if task == nil || task.Context == nil {
		return ""
	}
	raw, ok := task.Context["context_file_contents"]
	if !ok || raw == nil {
		return ""
	}
	if maxBytes <= 0 {
		maxBytes = 3000
	}
	var files []core.ContextFileContent
	switch v := raw.(type) {
	case []core.ContextFileContent:
		files = append(files, v...)
	case []any:
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			files = append(files, core.ContextFileContent{
				Path:      strings.TrimSpace(fmt.Sprint(m["path"])),
				Content:   fmt.Sprint(m["content"]),
				Summary:   strings.TrimSpace(fmt.Sprint(m["summary"])),
				Truncated: m["truncated"] == true,
			})
		}
	}
	var b strings.Builder
	remaining := maxBytes
	for _, file := range files {
		if file.Path == "" {
			continue
		}
		entry := renderContextFileEntry(file, remaining)
		if entry == "" {
			continue
		}
		if len(entry) > remaining {
			entry = entry[:remaining]
		}
		if len(entry) == 0 {
			break
		}
		b.WriteString(entry)
		if !strings.HasSuffix(entry, "\n") {
			b.WriteString("\n")
		}
		remaining = maxBytes - b.Len()
		if remaining <= 0 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func renderContextFileEntry(file core.ContextFileContent, maxBytes int) string {
	if maxBytes <= 0 || file.Path == "" {
		return ""
	}
	header := fmt.Sprintf("File: %s", file.Path)
	if file.Reference != nil && file.Reference.Detail != "" {
		header += fmt.Sprintf(" [detail=%s]", file.Reference.Detail)
	}
	header += "\n"
	remaining := maxBytes - len(header)
	if remaining <= 0 {
		return ""
	}
	body := strings.TrimSpace(file.Content)
	if body == "" {
		body = strings.TrimSpace(file.Summary)
	}
	if body == "" {
		body = "reference only"
	}
	if len(body) > remaining {
		body = body[:remaining]
	}
	return header + body
}

func filePaths(selection FileSelection) []string {
	out := make([]string, 0, len(selection.RelevantFiles))
	for _, file := range selection.RelevantFiles {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		out = append(out, file.Path)
	}
	return out
}
