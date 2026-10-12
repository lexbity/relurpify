package react

import (
	"encoding/json"
	"strings"
)

// ExtractJSON returns the outermost JSON object inside a string response. When
// no braces are present it returns an empty JSON object so downstream
// unmarshalling still succeeds.
//
// Small models routinely emit an EOS before the closing constructs of the
// decision object (the live scratch-turn failure). A truncated object is
// balanced — open strings and brackets are closed, a dangling trailing comma
// is dropped — so the result parses; well-formed input is returned
// byte-identical.
func ExtractJSON(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end >= start {
		return balanceJSONSnippet(raw[start : end+1])
	}
	return "{}"
}

// ExtractJSONSnippet returns the substring containing the JSON payload if
// present. When delimiters are missing it returns an empty string so callers
// can surface a more helpful error. Like ExtractJSON, a truncated object is
// balanced so the result parses.
func ExtractJSONSnippet(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end >= start {
		return balanceJSONSnippet(raw[start : end+1])
	}
	return ""
}

// balanceJSONSnippet closes the open string/bracket constructs of a possibly
// truncated JSON object. It never reorders or drops content other than a
// trailing comma left by the cut; a snippet that is already valid JSON is
// returned unchanged.
func balanceJSONSnippet(s string) string {
	if json.Valid([]byte(s)) {
		return s
	}
	inString := false
	escaped := false
	var stack []byte
	danglingComma := false
	for _, r := range s {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch r {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			danglingComma = false
			continue
		}
		switch r {
		case '"':
			inString = true
			danglingComma = false
		case '{':
			stack = append(stack, '}')
			danglingComma = false
		case '[':
			stack = append(stack, ']')
			danglingComma = false
		case '}':
			if n := len(stack); n > 0 && stack[n-1] == '}' {
				stack = stack[:n-1]
			}
			danglingComma = false
		case ']':
			if n := len(stack); n > 0 && stack[n-1] == ']' {
				stack = stack[:n-1]
			}
			danglingComma = false
		case ',':
			danglingComma = true
		case ' ', '\t', '\n', '\r':
		default:
			danglingComma = false
		}
	}
	trimmed := s
	if danglingComma {
		trimmed = strings.TrimRight(s, " \t\n\r")
		trimmed = strings.TrimSuffix(trimmed, ",")
	}
	var b strings.Builder
	b.WriteString(trimmed)
	if inString {
		b.WriteByte('"')
	}
	for i := len(stack) - 1; i >= 0; i-- {
		b.WriteByte(stack[i])
	}
	return b.String()
}
