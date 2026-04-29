package contextstream

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

func TestRequestAndModeTypes(t *testing.T) {
	req := Request{
		ID:                    "req-1",
		Query:                 retrieval.RetrievalQuery{Text: "find docs"},
		MaxTokens:             128,
		EventLogSeq:           42,
		BudgetShortfallPolicy: "emit_partial",
		Mode:                  ModeBlocking,
	}
	if req.Mode != ModeBlocking {
		t.Fatalf("expected blocking mode, got %q", req.Mode)
	}
	if req.Query.Text != "find docs" {
		t.Fatalf("unexpected query text: %q", req.Query.Text)
	}
	if req.MaxTokens != 128 {
		t.Fatalf("unexpected max tokens: %d", req.MaxTokens)
	}
}

func TestTrimMetadataDefaults(t *testing.T) {
	meta := TrimMetadata{}
	if meta.Truncated {
		t.Fatal("expected zero-value trim metadata to be untrimmed")
	}
	if meta.BudgetTokens != 0 || meta.ShortfallTokens != 0 {
		t.Fatalf("unexpected trim metadata: %+v", meta)
	}
}
