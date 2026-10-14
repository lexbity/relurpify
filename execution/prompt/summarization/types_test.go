package summarization

import (
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/context/knowledge"
)

func TestSummarizerKindConstants(t *testing.T) {
	kinds := map[SummarizerKind]string{
		SummarizerKindAST:     "ast",
		SummarizerKindHeading: "heading",
		SummarizerKindProse:   "prose",
	}
	for kind, want := range kinds {
		if string(kind) != want {
			t.Errorf("kind %q != %q", kind, want)
		}
	}
}

func TestGenerationCapErrorMessage(t *testing.T) {
	err := &GenerationCapError{Generation: 5, Cap: 3}
	want := "generation cap exceeded: generation 5 >= cap 3"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
	var capErr *GenerationCapError
	if !errors.As(err, &capErr) {
		t.Error("GenerationCapError must be discoverable via errors.As")
	}
	if capErr.Generation != 5 || capErr.Cap != 3 {
		t.Errorf("decoded fields = %d/%d, want 5/3", capErr.Generation, capErr.Cap)
	}
}

func TestComputeCoverageHashIsDeterministicAndOrderSensitive(t *testing.T) {
	a := chunkWithBody("a", "go", "one")
	b := chunkWithBody("b", "go", "two")

	ab1 := ComputeCoverageHash([]knowledge.KnowledgeChunk{a, b})
	ab2 := ComputeCoverageHash([]knowledge.KnowledgeChunk{a, b})
	if ab1 != ab2 {
		t.Fatalf("hash not deterministic: %q vs %q", ab1, ab2)
	}
	if len(ab1) != 32 {
		t.Fatalf("hash length = %d, want 32 hex chars (16 bytes)", len(ab1))
	}

	ba := ComputeCoverageHash([]knowledge.KnowledgeChunk{b, a})
	if ba == ab1 {
		t.Error("hash must depend on chunk order")
	}

	onlyA := ComputeCoverageHash([]knowledge.KnowledgeChunk{a})
	if onlyA == ab1 {
		t.Error("hash must depend on the chunk set")
	}

	// Version participates: same ID, different version hashes differently.
	aV2 := a
	aV2.Version = a.Version + 1
	if ComputeCoverageHash([]knowledge.KnowledgeChunk{aV2}) == onlyA {
		t.Error("hash must depend on chunk version")
	}
}

func TestCountTokensApproximatesBytesOverFour(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"abc", 0},          // 3 bytes floor to zero
		{"abcd", 1},         // 4 bytes
		{"abcdefgh", 2},     // 8 bytes
		{strings.Repeat("x", 400), 100},
	}
	for _, tc := range cases {
		if got := CountTokens(tc.text); got != tc.want {
			t.Errorf("CountTokens(%q bytes=%d) = %d, want %d", truncForLog(tc.text), len(tc.text), got, tc.want)
		}
	}
}

func truncForLog(s string) string {
	if len(s) > 16 {
		return s[:16] + "..."
	}
	return s
}
