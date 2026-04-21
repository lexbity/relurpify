package stages

import (
	"bytes"
	"fmt"
	"text/template"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// FilterState returns only the requested state keys from context.
// This ensures input isolation: each link sees only its declared InputKeys.
func FilterState(ctx *core.Context, keys []string) map[string]any {
	filtered := make(map[string]any, len(keys))
	if ctx == nil {
		return filtered
	}
	for _, key := range keys {
		if value, ok := ctx.Get(key); ok {
			filtered[key] = value
		}
	}
	return filtered
}

// RenderPrompt executes a template with task instruction and filtered input state.
// Template context includes:
//   - .Instruction: task.Instruction
//   - .Input: map[string]any of filtered state keys
func RenderPrompt(templateSrc string, instruction string, inputState map[string]any) (string, error) {
	if templateSrc == "" {
		return "", fmt.Errorf("chainer: template required")
	}
	tpl, err := template.New("link").Parse(templateSrc)
	if err != nil {
		return "", fmt.Errorf("chainer: parse template: %w", err)
	}
	var buf bytes.Buffer
	data := struct {
		Instruction string
		Input       map[string]any
	}{
		Instruction: instruction,
		Input:       inputState,
	}
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("chainer: execute template: %w", err)
	}
	return buf.String(), nil
}

// RecordInteraction adds an LLM response to the context's interaction history.
// Metadata includes the link name and any additional tags for debugging.
func RecordInteraction(ctx *core.Context, role, content string, metadata map[string]any) {
	if ctx == nil {
		return
	}
	ctx.AddInteraction(role, content, metadata)
}

// TaskInstruction extracts the instruction from a task, or returns empty string if nil.
func TaskInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return task.Instruction
}
