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
	files := normalizeContextFiles(raw)
	var b strings.Builder
	remaining := maxBytes
	for _, file := range files {
		if strings.TrimSpace(file.Path) == "" {
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

func renderContextFileEntry(file contextFileContent, maxBytes int) string {
	if maxBytes <= 0 || file.Path == "" {
		return ""
	}
	header := fmt.Sprintf("File: %s", file.Path)
	if detail := strings.TrimSpace(fmt.Sprint(file.Reference["detail"])); detail != "" && detail != "<nil>" {
		header += fmt.Sprintf(" [detail=%s]", detail)
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

type contextFileContent struct {
	Path      string
	Content   string
	Summary   string
	Truncated bool
	Reference map[string]any
}

func normalizeContextFiles(raw any) []contextFileContent {
	switch v := raw.(type) {
	case []contextFileContent:
		return append([]contextFileContent(nil), v...)
	case []any:
		out := make([]contextFileContent, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, contextFileContent{
				Path:      strings.TrimSpace(fmt.Sprint(m["path"])),
				Content:   fmt.Sprint(m["content"]),
				Summary:   strings.TrimSpace(fmt.Sprint(m["summary"])),
				Truncated: m["truncated"] == true,
				Reference: asStringAnyMap(m["reference"]),
			})
		}
		return out
	case []map[string]any:
		out := make([]contextFileContent, 0, len(v))
		for _, m := range v {
			out = append(out, contextFileContent{
				Path:      strings.TrimSpace(fmt.Sprint(m["path"])),
				Content:   fmt.Sprint(m["content"]),
				Summary:   strings.TrimSpace(fmt.Sprint(m["summary"])),
				Truncated: m["truncated"] == true,
				Reference: asStringAnyMap(m["reference"]),
			})
		}
		return out
	default:
		return nil
	}
}

func asStringAnyMap(raw any) map[string]any {
	m, ok := raw.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
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
