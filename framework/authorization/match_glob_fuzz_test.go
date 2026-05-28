package authorization

import (
	"testing"
)

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
