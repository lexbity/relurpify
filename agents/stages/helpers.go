package stages

import (
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

func taskInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.Instruction)
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
				Truncated: m["truncated"] == true,
			})
		}
	}
	var b strings.Builder
	remaining := maxBytes
	for _, file := range files {
		if file.Path == "" || file.Content == "" {
			continue
		}
		header := fmt.Sprintf("File: %s\n", file.Path)
		if len(header) >= remaining {
			break
		}
		b.WriteString(header)
		remaining -= len(header)
		content := file.Content
		if len(content) > remaining {
			content = content[:remaining]
		}
		b.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
		remaining = maxBytes - b.Len()
		if remaining <= 0 {
			break
		}
	}
	return strings.TrimSpace(b.String())
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
