package authorization

import (
	"testing"
)

// goldenGlobVectors defines a shared set of (pattern, value, expected) cases
// that both governance/authorization.matchGlob and the search package's MatchGlob
// must agree on. Add vectors here when adding new glob patterns to either package.
var goldenGlobVectors = []struct {
	pattern  string
	value    string
	expected bool
}{
	// Exact match
	{"foo", "foo", true},
	{"foo", "bar", false},
	{"/workspace/file.txt", "/workspace/file.txt", true},
	{"/workspace/file.txt", "/workspace/other.txt", false},

	// Empty pattern (search.MatchGlob returns false for empty)
	{"", "foo", false},
	{"", "", false},

	// Single star: [^/]*
	{"*.go", "main.go", true},
	{"*.go", "main.go.bak", false},
	{"/workspace/*.go", "/workspace/main.go", true},
	{"/workspace/*.go", "/workspace/src/main.go", false},
	{"*.md", "README.md", true},

	// Single char wildcard (matches exactly one non-/ char)
	{"file.???", "file.goo", true},
	{"file.??", "file.go", true},
	{"file.?", "file.g", true},
	{"file.?", "file.go", false},
	{"?", "x", true},
	{"???", "abc", true},
	{"???", "ab", false},

	// Doublestar: ** matches everything
	{"**", "anything", true},
	{"**", "", true},
	{"**/*.go", "/workspace/src/main.go", true},
	{"**/*.go", "main.go", true},
	{"**/*.go", "main.py", false},

	// Trailing /** matches everything below
	{"src/**", "src/file.go", true},
	{"src/**", "src/a/b/c/file.go", true},
	{"src/**", "src", true},
	{"/workspace/**", "/workspace/file.txt", true},

	// **/ matches zero or more directory levels
	{"a/**/b", "a/b", true},
	{"a/**/b", "a/x/b", true},
	{"a/**/b", "a/x/y/b", true},
	{"a/**/b", "a/b/c", false},

	// Character class
	{"[abc]", "a", true},
	{"[abc]", "d", false},
	{"file[0-9].go", "file5.go", true},
	{"file[0-9].go", "filea.go", false},

	// Brace expansion: Not supported by Go's filepath.Match — treated as literal.
	// Include the vector to document this is not a supported syntax here.
	{"{a,b,c}", "{a,b,c}", true},
	{"{a,b,c}", "a", false},

	// Mixed patterns
	{"src/**/*.md", "src/README.md", true},
	{"src/**/*.md", "src/docs/guide/README.md", true},
	{"src/**/*.md", "src/docs/guide/readme.txt", false},

	// Escaped special characters
	// filepath.Match: '\\' + char

	// Edge: pattern with no wildcards that doesn't match
	{"foo/bar", "foo/baz", false},

	// Edge: trailing slash
	// Note: "dir/" does not match "dir" in filepath.Match
	{"dir/", "dir/", true},

	// Edge: ** not matching across segment boundary without /
	{"**", "a/b/c", true},
}

func TestMatchGlob_goldenVectors(t *testing.T) {
	for _, tc := range goldenGlobVectors {
		got := matchGlob(tc.pattern, tc.value)
		if got != tc.expected {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.pattern, tc.value, got, tc.expected)
		}
	}
}

// TestMatchGlob_permissionMatchAll verifies the special-case short circuit for "**".
func TestMatchGlob_permissionMatchAll(t *testing.T) {
	if !matchGlob("**", "") {
		t.Error(`matchGlob("**", "") should be true`)
	}
	if !matchGlob("**", "anything/at/all") {
		t.Error(`matchGlob("**", "anything/at/all") should be true`)
	}
}
