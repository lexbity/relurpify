package search

import "testing"

func TestMatchGlobRecursivePatternMatchesRootLevelFiles(t *testing.T) {
	if !MatchGlob("**/*.go", "mathutil.go") {
		t.Fatalf("expected recursive pattern to match root-level file")
	}
	if !MatchGlob("**/*.go", "nested/mathutil.go") {
		t.Fatalf("expected recursive pattern to match nested file")
	}
}
