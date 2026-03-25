package core

import (
	"testing"
	"time"
)

func TestFileContextItemReferencesAndCompressionPreserveReference(t *testing.T) {
	item := &FileContextItem{
		Path:    "sample.go",
		Content: "package sample\nfunc Example() {}\n",
		Summary: "sample summary",
		Reference: &ContextReference{
			Kind:   ContextReferenceFile,
			ID:     "sample.go",
			URI:    "sample.go",
			Detail: "full",
		},
		LastAccessed: time.Now().UTC(),
	}

	refs := item.References()
	if len(refs) != 1 || refs[0].Kind != ContextReferenceFile || refs[0].URI != "sample.go" {
		t.Fatalf("unexpected references: %+v", refs)
	}
	if !item.HasInlinePayload() {
		t.Fatal("expected file item to report inline payload")
	}

	compressed, err := item.Compress()
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	file, ok := compressed.(*FileContextItem)
	if !ok {
		t.Fatalf("expected compressed file context item, got %T", compressed)
	}
	if file.Content != "" {
		t.Fatalf("expected compressed file item to drop inline content, got %q", file.Content)
	}
	if file.Reference == nil || file.Reference.URI != "sample.go" {
		t.Fatalf("expected reference to survive compression, got %+v", file.Reference)
	}
}

func TestMemoryContextItemFallsBackToSummaryAndReference(t *testing.T) {
	item := &MemoryContextItem{
		Source:  "memory:test",
		Summary: "remembered summary",
		Reference: &ContextReference{
			Kind:   ContextReferenceRuntimeMemory,
			ID:     "test",
			Detail: "query-results",
		},
		LastAccessed: time.Now().UTC(),
	}

	if item.TokenCount() == 0 {
		t.Fatal("expected summary-backed token count")
	}
	refs := item.References()
	if len(refs) != 1 || refs[0].Kind != ContextReferenceRuntimeMemory {
		t.Fatalf("unexpected references: %+v", refs)
	}
	if !item.HasInlinePayload() {
		t.Fatal("expected summary to count as inline payload")
	}
}

func TestRetrievalContextItemCompressPreservesReference(t *testing.T) {
	item := &RetrievalContextItem{
		Source:  "retrieval_evidence",
		Content: "retrieved evidence text",
		Reference: &ContextReference{
			Kind:   ContextReferenceRetrievalEvidence,
			ID:     "chunk-1",
			URI:    "doc-1",
			Detail: "packed",
		},
		LastAccessed: time.Now().UTC(),
	}

	compressed, err := item.Compress()
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	retrievalItem, ok := compressed.(*RetrievalContextItem)
	if !ok {
		t.Fatalf("expected retrieval context item, got %T", compressed)
	}
	if retrievalItem.Content != "" {
		t.Fatalf("expected compressed retrieval item to drop content, got %q", retrievalItem.Content)
	}
	if retrievalItem.Reference == nil || retrievalItem.Reference.Kind != ContextReferenceRetrievalEvidence {
		t.Fatalf("expected retrieval reference to survive compression, got %+v", retrievalItem.Reference)
	}
}
