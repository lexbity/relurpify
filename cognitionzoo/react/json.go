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
// truncated JSON object and repairs invalid escape sequences. It never reorders
// or drops content other than a trailing comma left by the cut and the
// backslash it adds to a literal escape; a snippet that is already valid JSON is
// returned unchanged.
//
// Invalid escapes are the regex case: a model writing
// re.sub(r'[^\w\s-]', …) inside a JSON string emits `\w`/`\s`, which JSON
// forbids. Left alone the whole decision is unparseable and the react loop's
// repair step (a second model call) can replace the intended action. Escaping
// the backslash (`\\w`) recovers the literal the model meant.
func balanceJSONSnippet(s string) string {
	if json.Valid([]byte(s)) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	inString := false
	escaped := false
	var stack []byte
	danglingComma := false
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if inString {
			if escaped {
				escaped = false
				b.WriteRune(r)
				continue
			}
			switch r {
			case '\\':
				if i+1 < len(runes) && isValidJSONEscape(runes, i+1) {
					escaped = true
					b.WriteRune(r)
				} else {
					// A literal backslash (regex \w, \s, \d, or a trailing
					// backslash): escape it so the string stays parseable.
					b.WriteString(`\\`)
				}
			case '"':
				inString = false
				b.WriteRune(r)
			default:
				b.WriteRune(r)
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
		b.WriteRune(r)
	}
	trimmed := b.String()
	if danglingComma {
		trimmed = strings.TrimRight(trimmed, " \t\n\r")
		trimmed = strings.TrimSuffix(trimmed, ",")
	}
	var out strings.Builder
	out.WriteString(trimmed)
	if inString {
		out.WriteByte('"')
	}
	for i := len(stack) - 1; i >= 0; i-- {
		out.WriteByte(stack[i])
	}
	return out.String()
}

// isValidJSONEscape reports whether the rune at index j (immediately after a
// backslash) begins a valid JSON escape: one of the simple escapes, or a
// Unicode escape sequence with four hex digits.
func isValidJSONEscape(runes []rune, j int) bool {
	if j >= len(runes) {
		return false
	}
	switch runes[j] {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		return true
	case 'u':
		if j+4 >= len(runes) {
			return false
		}
		for k := j + 1; k <= j+4; k++ {
			if !isHexRune(runes[k]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func isHexRune(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}
