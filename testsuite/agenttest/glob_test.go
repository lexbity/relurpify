package agenttest

import (
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/config"
)

func TestMatchGlob(t *testing.T) {
	t.Helper()
	testRunsGlob := filepath.ToSlash(filepath.Join(config.DirName, "test_runs", "**"))
	testRunReport := filepath.ToSlash(filepath.Join(config.DirName, "test_runs", "x", "report.json"))
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"a/b/*.go", "a/b/main.go", true},
		{"a/b/*.go", "a/b/c/main.go", false},
		{"a/**/main.go", "a/b/c/main.go", true},
		{"**/*.md", "docs/README.md", true},
		{"docs/**", "docs/index.html", true},
		{"docs/**", "src/docs/index.html", false},
		{testRunsGlob, testRunReport, true},
	}
	for _, tc := range cases {
		if got := matchGlob(tc.pattern, tc.path); got != tc.want {
			t.Fatalf("matchGlob(%q, %q)=%v want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}
