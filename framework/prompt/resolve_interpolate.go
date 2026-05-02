package prompt

import "strings"

// interpolate replaces {variable_name} references in s.
// Resolution order: rtVars (runtime overrides), then defaults.
// Escaped braces \{ and \} are replaced with literal { and }.
// Unknown references resolve to "".
func interpolate(s string, rtVars, defaults map[string]string) string {
	if !strings.ContainsRune(s, '{') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			next := s[i+1]
			if next == '{' || next == '}' {
				b.WriteByte(next)
				i += 2
				continue
			}
		}
		if c == '{' {
			j := i + 1
			for j < len(s) && s[j] != '}' && s[j] != '{' && s[j] != '\n' {
				j++
			}
			if j < len(s) && s[j] == '}' {
				name := s[i+1 : j]
				if v, ok := rtVars[name]; ok {
					b.WriteString(v)
				} else if v, ok := defaults[name]; ok {
					b.WriteString(v)
				}
				// else: empty string (no output)
				i = j + 1
				continue
			}
			// No matching '}' — treat as literal.
			b.WriteByte(c)
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}
