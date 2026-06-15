package toolwire

import (
	"encoding/json"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/model"
)

// RenderToolsToPrompt converts tool definitions into the fallback prompt wire format.
func RenderToolsToPrompt(tools []model.LLMToolSpec) string {
	if len(tools) == 0 {
		return "No tools available."
	}

	var b strings.Builder
	b.WriteString("You have access to the following tools.\n")
	b.WriteString("When you need a tool, return exactly one fenced JSON object:\n")
	b.WriteString("```tool\n{\"tool\":\"tool_name\",\"arguments\":{}}\n```\n\n")

	for _, tool := range tools {
		fmt.Fprintf(&b, "## %s\n", tool.Name)
		if strings.TrimSpace(tool.Description) != "" {
			fmt.Fprintf(&b, "%s\n", strings.TrimSpace(tool.Description))
		}
		payload := struct {
			Name        string        `json:"name"`
			Description string        `json:"description,omitempty"`
			InputSchema *model.Schema `json:"input_schema,omitempty"`
		}{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		}
		if encoded, err := json.Marshal(payload); err == nil {
			b.WriteString(string(encoded))
			b.WriteString("\n\n")
			continue
		}
		fmt.Fprintf(&b, "{\"name\":%q}\n\n", tool.Name)
	}

	return b.String()
}

// ParseToolCallsFromText extracts tool calls from raw LLM output and limits the
// result to max when max is positive.
func ParseToolCallsFromText(text string, max int) []model.ToolCall {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	var calls []model.ToolCall
	for _, candidate := range extractTopLevelJSONObjects(text) {
		if max > 0 && len(calls) >= max {
			break
		}
		call, ok := tryParseSingleToolCall(candidate)
		if ok {
			calls = append(calls, call)
		}
	}
	return calls
}

func extractTopLevelJSONObjects(text string) []string {
	var out []string
	start := -1
	depth := 0
	inString := false
	escaped := false

	for i := 0; i < len(text); i++ {
		ch := text[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				out = append(out, text[start:i+1])
				start = -1
			}
		}
	}
	return out
}

func tryParseSingleToolCall(jsonText string) (model.ToolCall, bool) {
	var raw struct {
		Tool       string         `json:"tool"`
		Name       string         `json:"name"`
		ToolName   string         `json:"tool_name"`
		Arguments  map[string]any `json:"arguments"`
		Args       map[string]any `json:"args"`
		Parameters map[string]any `json:"parameters"`
	}

	if err := json.Unmarshal([]byte(jsonText), &raw); err != nil {
		normalized := normalizeMultilineJSONStringLiterals(jsonText)
		if normalized == jsonText || json.Unmarshal([]byte(normalized), &raw) != nil {
			return model.ToolCall{}, false
		}
	}

	name := firstNonEmpty(raw.Tool, raw.Name, raw.ToolName)
	if name == "" {
		return model.ToolCall{}, false
	}

	args := raw.Arguments
	if args == nil {
		args = raw.Args
	}
	if args == nil {
		args = raw.Parameters
	}
	if args == nil {
		args = make(map[string]any)
	}
	if filePath, ok := args["file_path"].(string); ok && filePath != "" {
		if _, exists := args["path"]; !exists {
			args["path"] = filePath
		}
	}

	return model.ToolCall{
		Name: strings.TrimSpace(name),
		Args: args,
	}, true
}

func normalizeMultilineJSONStringLiterals(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	inString := false
	escaped := false
	changed := false

	for _, ch := range text {
		if escaped {
			b.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			b.WriteRune(ch)
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			b.WriteRune(ch)
			continue
		}
		if inString {
			switch ch {
			case '\n':
				b.WriteString(`\n`)
				changed = true
				continue
			case '\r':
				b.WriteString(`\r`)
				changed = true
				continue
			case '\t':
				b.WriteString(`\t`)
				changed = true
				continue
			}
		}
		b.WriteRune(ch)
	}

	if !changed {
		return text
	}
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
