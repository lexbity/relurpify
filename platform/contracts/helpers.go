package contracts

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// NormalizeStringSlice coerces common decoded tool-call array shapes into a
// Go string slice. This helper is available for use by platform tools.
func NormalizeStringSlice(value interface{}) ([]string, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []string:
		return append([]string(nil), typed...), nil
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			out = append(out, fmt.Sprint(entry))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected array, got %T", value)
	}
}

// MatchGlob supports both filepath.Match and the '**' recursive glob pattern.
func MatchGlob(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	pattern = filepath.ToSlash(pattern)
	value = filepath.ToSlash(value)
	if !strings.Contains(pattern, "**") {
		ok, err := filepath.Match(pattern, value)
		if err != nil {
			return false
		}
		return ok
	}
	regexPattern := globToRegex(pattern)
	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return false
	}
	return regex.MatchString(value)
}

func globToRegex(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		switch ch {
		case '*':
			peek := ""
			if i+1 < len(runes) {
				peek = string(runes[i+1])
			}
			if peek == "*" {
				if i+2 < len(runes) && runes[i+2] == '/' {
					b.WriteString("(?:.*/)?")
					i += 2
				} else {
					b.WriteString(".*")
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString(".")
		case '.', '+', '(', ')', '|', '^', '$', '[', ']', '{', '}', '\\':
			b.WriteRune('\\')
			b.WriteRune(ch)
		default:
			b.WriteRune(ch)
		}
	}
	b.WriteString("$")
	return b.String()
}
