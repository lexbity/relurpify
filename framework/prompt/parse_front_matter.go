package prompt

import (
	"fmt"
	"strconv"
	"strings"
)

type frontMatterResult struct {
	Schema    string
	ID        string
	Tags      []string
	Variables map[string]VariableDecl
}

func parseFrontMatter(lines []string, sourcePath string) (frontMatterResult, []string, error) {
	result := frontMatterResult{
		Variables: make(map[string]VariableDecl),
	}
	var warnings []string
	seenID := false

	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.ContainsRune(raw, '\t') {
			return result, warnings, fmt.Errorf("line %d: tabs are not allowed in front matter", i+1)
		}

		switch {
		case strings.HasPrefix(line, "schema "):
			if result.Schema != "" {
				return result, warnings, fmt.Errorf("line %d: duplicate schema statement", i+1)
			}
			value := strings.TrimSpace(strings.TrimPrefix(line, "schema "))
			if value != "framework.prompt/v2" {
				return result, warnings, fmt.Errorf("line %d: unsupported schema %q", i+1, value)
			}
			result.Schema = value
		case strings.HasPrefix(line, "id "):
			if seenID {
				return result, warnings, fmt.Errorf("line %d: duplicate id statement", i+1)
			}
			value := strings.TrimSpace(strings.TrimPrefix(line, "id "))
			if !validPromptID(value) {
				return result, warnings, fmt.Errorf("line %d: invalid id %q", i+1, value)
			}
			result.ID = value
			seenID = true
		case strings.HasPrefix(line, "tag "):
			tags, err := parseTagStatement(strings.TrimSpace(strings.TrimPrefix(line, "tag ")), i+1)
			if err != nil {
				return result, warnings, err
			}
			result.Tags = append(result.Tags, tags...)
		case strings.HasPrefix(line, "var "):
			name, value, err := parseVarStatement(strings.TrimSpace(strings.TrimPrefix(line, "var ")), i+1)
			if err != nil {
				return result, warnings, err
			}
			if _, exists := result.Variables[name]; exists {
				return result, warnings, fmt.Errorf("line %d: duplicate variable %q", i+1, name)
			}
			result.Variables[name] = VariableDecl{Default: value}
		default:
			return result, warnings, fmt.Errorf("line %d: unknown front matter statement %q", i+1, raw)
		}
	}

	if result.Schema == "" {
		return result, warnings, fmt.Errorf("%s: missing required schema statement", sourcePath)
	}
	if result.ID == "" {
		return result, warnings, fmt.Errorf("%s: missing required id statement", sourcePath)
	}
	result.Tags = dedupeStrings(result.Tags)
	return result, warnings, nil
}

func parseTagStatement(value string, line int) ([]string, error) {
	if value == "" {
		return nil, fmt.Errorf("line %d: empty tag statement", line)
	}
	if strings.HasPrefix(value, "[") {
		if !strings.HasSuffix(value, "]") {
			return nil, fmt.Errorf("line %d: malformed tag list", line)
		}
		inner := strings.TrimSpace(value[1 : len(value)-1])
		if inner == "" {
			return []string{}, nil
		}
		parts, err := splitCommaQuotedList(inner, line)
		if err != nil {
			return nil, err
		}
		return parts, nil
	}
	tag, err := parseQuotedString(value, line)
	if err != nil {
		return nil, err
	}
	return []string{tag}, nil
}

func parseVarStatement(value string, line int) (string, string, error) {
	eq := strings.Index(value, "=")
	if eq < 0 {
		return "", "", fmt.Errorf("line %d: malformed var statement", line)
	}
	name := strings.TrimSpace(value[:eq])
	if !validIdentifier(name) {
		return "", "", fmt.Errorf("line %d: invalid variable name %q", line, name)
	}
	rawValue := strings.TrimSpace(value[eq+1:])
	if rawValue == "" {
		return "", "", fmt.Errorf("line %d: var %q requires a quoted string value", line, name)
	}
	val, err := parseQuotedString(rawValue, line)
	if err != nil {
		return "", "", err
	}
	return name, val, nil
}

func splitCommaQuotedList(s string, line int) ([]string, error) {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			return nil, fmt.Errorf("line %d: malformed list item", line)
		}
		str, err := parseQuotedString(value, line)
		if err != nil {
			return nil, err
		}
		out = append(out, str)
	}
	return out, nil
}

func parseQuotedString(s string, line int) (string, error) {
	if len(s) < 2 {
		return "", fmt.Errorf("line %d: expected quoted string, got %q", line, s)
	}
	switch s[0] {
	case '"':
		v, err := strconv.Unquote(s)
		if err != nil {
			return "", fmt.Errorf("line %d: invalid quoted string %q", line, s)
		}
		return v, nil
	case '\'':
		if s[len(s)-1] != '\'' {
			return "", fmt.Errorf("line %d: invalid quoted string %q", line, s)
		}
		return strings.ReplaceAll(s[1:len(s)-1], "\\'", "'"), nil
	default:
		return "", fmt.Errorf("line %d: expected quoted string, got %q", line, s)
	}
}

func validPromptID(s string) bool {
	if s == "" {
		return false
	}
	parts := strings.Split(s, ".")
	for _, part := range parts {
		if !validIdentifier(part) {
			return false
		}
	}
	return true
}

func validIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
			if i == 0 {
				continue
			}
		case i > 0 && r >= '0' && r <= '9':
			continue
		default:
			return false
		}
	}
	return true
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
