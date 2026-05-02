package prompt

import (
	"fmt"
	"strings"
)

// fmResult holds the raw parsed front matter before mapping to PromptConfig.
type fmResult struct {
	APIVersion        string
	ID                string
	Name              string
	Description       string
	Extends           string
	FrameworkCritical bool
	RequiresProviders []string
	Paradigm          []string
	Agent             []string
	Domain            []string
	Kind              string
	Stability         string
	Variables         map[string]VariableDecl
	UnknownFields     []string
}

// parseFrontMatter parses the YAML-like block between the --- markers.
func parseFrontMatter(lines []string) (fmResult, error) {
	fm := fmResult{
		Variables: make(map[string]VariableDecl),
	}

	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}

		key, val, err := fmParseLine(line)
		if err != nil {
			return fm, fmt.Errorf("line %d: %w", i+1, err)
		}

		switch key {
		case "apiVersion":
			fm.APIVersion = val
			i++
		case "id":
			fm.ID = val
			i++
		case "name":
			fm.Name = fmUnquote(val)
			i++
		case "description":
			fm.Description = fmUnquote(val)
			i++
		case "extends":
			fm.Extends = val
			i++
		case "framework_critical":
			fm.FrameworkCritical = val == "true"
			i++
		case "requires_providers":
			list, n, err := fmParseList(lines, i, val)
			if err != nil {
				return fm, fmt.Errorf("requires_providers: %w", err)
			}
			fm.RequiresProviders = list
			i += n
		case "tags":
			n, err := fmParseTags(lines, i+1, &fm)
			if err != nil {
				return fm, fmt.Errorf("tags: %w", err)
			}
			i += n + 1
		case "variables":
			n, err := fmParseVariables(lines, i+1, fm.Variables)
			if err != nil {
				return fm, fmt.Errorf("variables: %w", err)
			}
			i += n + 1
		default:
			fm.UnknownFields = append(fm.UnknownFields, key)
			i++
		}
	}

	return fm, nil
}

// fmParseLine splits a "key: value" line.
func fmParseLine(line string) (key, val string, err error) {
	colon := strings.Index(line, ":")
	if colon < 0 {
		return "", "", fmt.Errorf("expected 'key: value', got %q", line)
	}
	key = strings.TrimSpace(line[:colon])
	val = strings.TrimSpace(line[colon+1:])
	return key, val, nil
}

// fmParseList parses an inline bracket list "[a, b]" or a block list.
// Returns items, lines-consumed (including the key line), and any error.
func fmParseList(lines []string, keyIdx int, inline string) ([]string, int, error) {
	if inline != "" {
		items, err := fmParseBracketList(inline)
		return items, 1, err
	}
	var items []string
	n := 1
	for keyIdx+n < len(lines) {
		trimmed := strings.TrimSpace(lines[keyIdx+n])
		if !strings.HasPrefix(trimmed, "- ") && trimmed != "-" {
			break
		}
		item := strings.TrimPrefix(trimmed, "- ")
		items = append(items, strings.TrimSpace(item))
		n++
	}
	return items, n, nil
}

func fmParseBracketList(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("expected bracket list, got %q", s)
	}
	inner := s[1 : len(s)-1]
	var items []string
	for _, part := range strings.Split(inner, ",") {
		item := strings.TrimSpace(part)
		if item != "" {
			items = append(items, item)
		}
	}
	return items, nil
}

// fmParseTags parses the indented tag sub-section.
// startIdx is the first line after "tags:".
// Returns lines consumed.
func fmParseTags(lines []string, startIdx int, fm *fmResult) (int, error) {
	n := 0
	for startIdx+n < len(lines) {
		line := lines[startIdx+n]
		if strings.TrimSpace(line) == "" {
			break
		}
		if !strings.HasPrefix(line, "  ") {
			break
		}
		inner := strings.TrimSpace(line)
		key, val, err := fmParseLine(inner)
		if err != nil {
			return n, err
		}
		switch key {
		case "paradigm":
			subLines := lines[startIdx+n:]
			list, consumed, err := fmParseList(subLines, 0, val)
			if err != nil {
				return n, err
			}
			fm.Paradigm = list
			n += consumed
			continue
		case "agent":
			subLines := lines[startIdx+n:]
			list, consumed, err := fmParseList(subLines, 0, val)
			if err != nil {
				return n, err
			}
			fm.Agent = list
			n += consumed
			continue
		case "domain":
			subLines := lines[startIdx+n:]
			list, consumed, err := fmParseList(subLines, 0, val)
			if err != nil {
				return n, err
			}
			fm.Domain = list
			n += consumed
			continue
		case "kind":
			fm.Kind = val
			n++
		case "stability":
			fm.Stability = val
			n++
		default:
			n++
		}
	}
	return n, nil
}

// fmParseVariables parses the variables sub-section.
// startIdx is the first line after "variables:".
// Returns lines consumed.
func fmParseVariables(lines []string, startIdx int, vars map[string]VariableDecl) (int, error) {
	n := 0
	for startIdx+n < len(lines) {
		line := lines[startIdx+n]
		if strings.TrimSpace(line) == "" {
			break
		}
		if !strings.HasPrefix(line, "  ") {
			break
		}
		// Variable name: "  varname:" (no value after colon for the name line)
		trimmed := strings.TrimSpace(line)
		colon := strings.Index(trimmed, ":")
		if colon < 0 {
			n++
			continue
		}
		varName := strings.TrimSpace(trimmed[:colon])
		afterColon := strings.TrimSpace(trimmed[colon+1:])
		if afterColon != "" {
			// Inline value — treat as unknown, skip.
			n++
			continue
		}
		n++
		var v VariableDecl
		// Parse sub-keys at 4-space indent: "    default: ..."
		for startIdx+n < len(lines) {
			subLine := lines[startIdx+n]
			if strings.TrimSpace(subLine) == "" {
				break
			}
			if !strings.HasPrefix(subLine, "    ") {
				break
			}
			subTrimmed := strings.TrimSpace(subLine)
			k, val, err := fmParseLine(subTrimmed)
			if err != nil {
				return n, err
			}
			if k == "default" {
				v.Default = fmUnquote(val)
			}
			n++
		}
		vars[varName] = v
	}
	return n, nil
}

// fmUnquote strips surrounding quotes from a string value if present.
func fmUnquote(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}
