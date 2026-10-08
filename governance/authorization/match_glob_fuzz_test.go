package authorization

import (
	"testing"
)

func FuzzMatchCommandGlob(f *testing.F) {
	seeds := []struct {
		pattern string
		value   string
	}{
		{"git push*", "git push --force"},
		{"curl http://evil.com*", "curl http://evil.com/x"},
		{`curl \*literal`, "curl *literal"},
		{"*", "anything at all"},
		{"?", "x"},
		{"", "anything"},
	}
	for _, s := range seeds {
		f.Add(s.pattern, s.value)
	}
	f.Fuzz(func(t *testing.T, pattern, value string) {
		// Must never panic on adversarial input.
		MatchCommandGlob(pattern, value)
		// `\*` is a literal match, not a wildcard.
		if MatchCommandGlob(`\*`, "*") != true {
			t.Errorf("escaped star must match a literal star")
		}
	})
}

func FuzzMatchGlob(f *testing.F) {
	seeds := []struct {
		pattern string
		value   string
	}{
		{"**/*.go", "/workspace/src/main.go"},
		{"*.md", "README.md"},
		{"src/**", "src/a/b/c/file.go"},
		{"[abc]", "a"},
		{"{a,b,c}", "a"},
		{"?", "x"},
		{"/workspace/**", "/workspace/file.txt"},
	}
	for _, s := range seeds {
		f.Add(s.pattern, s.value)
	}
	f.Fuzz(func(t *testing.T, pattern, value string) {
		matchGlob(pattern, value)
	})
}
