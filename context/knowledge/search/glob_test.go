package search

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchGlobTable(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{"empty pattern never matches", "", "anything.go", false},
		{"bare doublestar matches everything", "**", "any/path/file.go", true},
		{"bare doublestar matches empty value", "**", "", true},
		{"exact match", "main.go", "main.go", true},
		{"exact mismatch", "main.go", "other.go", false},
		{"single star stays within one segment", "*.go", "main.go", true},
		{"single star does not cross separators", "*.go", "dir/main.go", false},
		{"directory prefix single star", "src/*.go", "src/main.go", true},
		{"directory prefix single star rejects deeper nesting", "src/*.go", "src/deep/main.go", false},
		{"question mark matches one char", "ma?n.go", "main.go", true},
		{"question mark requires exactly one char", "ma?n.go", "maaan.go", false},
		{"leading doublestar segment matches root", "**/main.go", "main.go", true},
		{"leading doublestar segment matches nested", "**/main.go", "a/b/main.go", true},
		{"leading doublestar segment rejects other names", "**/main.go", "a/other.go", false},
		{"inner doublestar matches zero or more dirs", "a/**/b.go", "a/b.go", true},
		{"inner doublestar matches deep dirs", "a/**/b.go", "a/x/y/b.go", true},
		{"trailing doublestar matches subtree", "a/**", "a/b/c.go", true},
		{"trailing doublestar matches the dir itself", "a/**", "a/", true},
		{"doublestar requires separator semantics", "a/**", "ab/c.go", false},
		{"regex dot is literal", "a.b.txt", "a.b.txt", true},
		{"regex dot does not act as wildcard", "a.b.txt", "axbtxt", false},
		{"regex plus is literal", "c++.go", "c++.go", true},
		{"regex parens are literal", "f(x).go", "f(x).go", true},
		{"regex anchor chars are literal", "^a$", "^a$", true},
		{"regex alternation is literal", "a|b", "a|b", true},
		{"unterminated class falls back to false", "[abc", "abc", false},
		{"multiple single stars in one segment", "a*b*c", "aXbYc", true},
		{"multiple single stars require all parts", "a*b*c", "aXbY", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, MatchGlob(tc.pattern, tc.value),
				"MatchGlob(%q, %q)", tc.pattern, tc.value)
		})
	}
}

func TestGlobToRegexPublicAnchorsAndEscapes(t *testing.T) {
	cases := map[string]string{
		"main.go":     `^main\.go$`,
		"*.go":        `^[^/]*\.go$`,
		"**":          `^.*$`,
		"**/main.go":  `^(?:.*/)?main\.go$`,
		"a/**/b.go":   `^a/(?:.*/)?b\.go$`,
		"a/**":        `^a/.*$`,
		"ma?n":        `^ma.n$`,
		"literal[1]":  `^literal\[1\]$`,
		"curly{x}":    `^curly\{x\}$`,
		"back\\slash": `^back\\slash$`,
	}
	for pattern, want := range cases {
		require.Equal(t, want, globToRegexPublic(pattern), "pattern %q", pattern)
	}
}

// TestMatchGlobMatchesFilepathMatchForSimplePatterns: for patterns without
// '**', MatchGlob must agree with the standard library.
func TestMatchGlobMatchesFilepathMatchForSimplePatterns(t *testing.T) {
	patterns := []string{"*.go", "src/*.go", "main.?o", "a*b", "exact.txt"}
	values := []string{"main.go", "src/main.go", "src/deep/x.go", "axxxb", "exact.txt", "nope"}

	for _, p := range patterns {
		for _, v := range values {
			want, err := filepath.Match(p, v)
			require.NoError(t, err)
			require.Equal(t, want, MatchGlob(p, v), "MatchGlob(%q, %q) vs filepath.Match", p, v)
		}
	}
}

// TestGlobToRegexPublicProducesValidRegex: whatever the pattern, the built
// regex must compile and anchor to the full value.
func TestGlobToRegexPublicProducesValidRegex(t *testing.T) {
	for _, pattern := range []string{"*", "**", "**/x", "?*", "a**b**c", "[", "(((", "\\\\"} {
		expr := globToRegexPublic(pattern)
		require.True(t, strings.HasPrefix(expr, "^"), "pattern %q -> %q", pattern, expr)
		require.True(t, strings.HasSuffix(expr, "$"), "pattern %q -> %q", pattern, expr)
	}
}

// FuzzMatchGlob exercises the glob-to-regex translation against arbitrary
// untrusted patterns: it must never panic and must uphold its documented
// invariants.
func FuzzMatchGlob(f *testing.F) {
	f.Add("", "value")
	f.Add("**", "any/path")
	f.Add("**/main.go", "a/b/main.go")
	f.Add("src/*.go", "src/main.go")
	f.Add("a/**/b.go", "a/x/b.go")
	f.Add("ma?n.go", "main.go")
	f.Add("[unclosed", "abc")
	f.Add("{a,b}", "{a,b}")
	f.Add("***", "anything")

	f.Fuzz(func(t *testing.T, pattern, value string) {
		got := MatchGlob(pattern, value)

		// Invariant 1: empty patterns never match.
		if pattern == "" {
			require.False(t, got)
		}
		// Invariant 2: the bare doublestar matches everything.
		if pattern == "**" {
			require.True(t, got)
		}
		// Invariant 3: a match must be repeatable (no hidden state).
		require.Equal(t, got, MatchGlob(pattern, value), "MatchGlob(%q, %q) not deterministic", pattern, value)
		// Invariant 4: anything matching an exact literal pattern (no glob
		// metacharacters, no separator or escape normalization) equals the
		// pattern itself.
		if got && !strings.ContainsAny(pattern, `*?[\`) {
			require.Equal(t, pattern, value, "literal pattern %q matched different value %q", pattern, value)
		}
	})
}
