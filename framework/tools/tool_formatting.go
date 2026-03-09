package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// RenderToolsToPrompt converts tool definitions into a schema-like string.
// This is used when the LLM does not support native tool calling API.
func RenderToolsToPrompt(tools []Tool) string {
	if len(tools) == 0 {
		return "No tools available."
	}
	var b strings.Builder
	b.WriteString("You have access to the following tools. To call a tool, return a JSON object with 'tool' (name) and 'arguments' (map).\n\n")

	for _, tool := range tools {
		b.WriteString(fmt.Sprintf("## %s\n", tool.Name()))
		b.WriteString(fmt.Sprintf("%s\n", tool.Description()))
		b.WriteString("Arguments:\n")
		params := tool.Parameters()
		if len(params) == 0 {
			b.WriteString("  (No arguments)\n")
		} else {
			for _, param := range params {
				req := "optional"
				if param.Required {
					req = "required"
				}
				b.WriteString(fmt.Sprintf("  - %s (%s, %s): %s\n", param.Name, param.Type, req, param.Description))
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("Example Call:\n")
	b.WriteString("```json\n{\"tool\": \"tool_name\", \"arguments\": {\"arg1\": \"value\"}}\n```\n")
	return b.String()
}

// ParseToolCallsFromText extracts potential tool calls from raw LLM output.
// It looks for JSON blocks that match the tool call schema.
func ParseToolCallsFromText(text string) []ToolCall {
	var calls []ToolCall

	// Attempt to find Markdown JSON blocks
	jsonBlockRegex := regexp.MustCompile("(?s)```json\\s*(.*?)```")
	matches := jsonBlockRegex.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		if len(match) > 1 {
			call, ok := tryParseSingleToolCall(match[1])
			if ok {
				calls = append(calls, call)
			}
		}
	}
	if len(calls) > 0 {
		return calls
	}

	// Scan for bare top-level JSON objects anywhere in the text. This handles
	// models that output tool calls as inline JSON mixed with prose (e.g.
	// {"name":"cli_go","arguments":{...}}\nSome explanation text).
	for _, candidate := range extractTopLevelJSONObjects(text) {
		if call, ok := tryParseSingleToolCall(candidate); ok {
			calls = append(calls, call)
		}
	}

	return calls
}

// extractTopLevelJSONObjects returns all balanced top-level {...} substrings
// found anywhere in text.
func extractTopLevelJSONObjects(text string) []string {
	var out []string
	depth := 0
	start := -1
	inString := false
	escaped := false
	for i, ch := range text {
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
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					out = append(out, text[start:i+1])
					start = -1
				}
			}
		}
	}
	return out
}

func tryParseSingleToolCall(jsonText string) (ToolCall, bool) {
	var raw struct {
		Tool      string                 `json:"tool"`
		Name      string                 `json:"name"` // alias for 'tool'
		Arguments map[string]interface{} `json:"arguments"`
		Args      map[string]interface{} `json:"args"` // alias for 'arguments'
	}

	if err := json.Unmarshal([]byte(jsonText), &raw); err != nil {
		normalized := normalizeMultilineJSONStringLiterals(jsonText)
		if normalized == jsonText || json.Unmarshal([]byte(normalized), &raw) != nil {
			return ToolCall{}, false
		}
	}

	name := raw.Tool
	if name == "" {
		name = raw.Name
	}
	if name == "" {
		return ToolCall{}, false
	}

	args := raw.Arguments
	if args == nil {
		args = raw.Args
	}
	if args == nil {
		args = make(map[string]interface{})
	}

	return ToolCall{
			Name: name,
			Args: args,
		},
		true
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
