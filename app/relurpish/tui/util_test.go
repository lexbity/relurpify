package tui

import "testing"

func TestFuzzyMatchScore(t *testing.T) {
	ok, score := fuzzyMatchScore("cmd", "command")
	if !ok {
		t.Fatalf("expected match")
	}
	if score <= 0 {
		t.Fatalf("expected positive score, got %d", score)
	}
	if ok, _ := fuzzyMatchScore("zz", "command"); ok {
		t.Fatalf("expected no match")
	}
}
