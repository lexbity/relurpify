package authorization

import (
	"regexp"
	"strings"
)

// MatchCommandGlob matches a command-text glob pattern against a joined command
// line. It is the command-authorization counterpart of the path matcher
// (matchGlob); the two grammars are intentionally distinct (D-8).
//
// Normative grammar:
//
//   - matches any run of any characters, including spaces and '/'
//     ?      matches exactly one character
//     \x     matches the literal character x (escape)
//     other  literal (regular-expression metacharacters are quoted)
//
// The match is anchored (the whole line must match) and case-sensitive.
func MatchCommandGlob(pattern, cmdline string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if pattern == permissionMatchAll {
		return true
	}
	regexStr := commandGlobToRegex(pattern)
	compiled, err := globRegexCache.get(regexStr)
	if err != nil {
		return false
	}
	return compiled.MatchString(cmdline)
}

// commandGlobToRegex translates the command-text glob grammar to an anchored
// regular expression. The `(?s)` flag lets `.` match newlines so commands with
// embedded line continuations still match.
func commandGlobToRegex(pattern string) string {
	var b strings.Builder
	b.WriteString(`(?s)^`)
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '\\':
			if i+1 < len(pattern) {
				i++
				b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			} else {
				b.WriteString(regexp.QuoteMeta("\\"))
			}
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return b.String()
}
