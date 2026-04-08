package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type recordingSummarizer struct {
	calls []SummaryLevel
}

func (r *recordingSummarizer) Summarize(content string, level SummaryLevel) (string, error) {
	r.calls = append(r.calls, level)
	if content == "" {
		return "", nil
	}
	return content[:min(len(content), 24)], nil
}

func (r *recordingSummarizer) SummarizeFile(path string, content string, level SummaryLevel) (*FileSummary, error) {
	return &FileSummary{Path: path, Level: level, Summary: content}, nil
}

func (r *recordingSummarizer) SummarizeDirectory(path string, files []FileSummary, level SummaryLevel) (*DirectorySummary, error) {
	return &DirectorySummary{Path: path, Level: level}, nil
}

func (r *recordingSummarizer) SummarizeChunk(chunk CodeChunk, content string, level SummaryLevel) (*ChunkSummary, error) {
	return &ChunkSummary{ChunkID: chunk.ID, Level: level, Summary: content}, nil
}

func TestSharedContextWorkingSetLifecycleAndMutationHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	content := "package sample\n\nfunc Example() {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	summarizer := &recordingSummarizer{}
	sc := NewSharedContext(nil, NewContextBudget(4096), summarizer)

	if _, err := sc.AddFile("", content, "go", DetailFull); err == nil {
		t.Fatal("AddFile error = nil, want missing path")
	}

	fc, err := sc.AddFile(path, content, "go", DetailSummary)
	if err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if fc.Summary == "" {
		t.Fatal("expected summarizer to populate summary")
	}
	if got := sc.GetTokenUsage(); got.Total == 0 {
		t.Fatal("expected token usage to account for added file")
	}

	ref, ok := sc.FileReference(path)
	if !ok {
		t.Fatal("expected file reference")
	}
	if ref.Detail != "summary" || ref.Metadata["language"] != "go" {
		t.Fatalf("unexpected file reference: %+v", ref)
	}
	if refs := sc.WorkingSetReferences(); len(refs) != 1 {
		t.Fatalf("expected 1 working set reference, got %d", len(refs))
	}

	hydrated, err := sc.HydrateFileReference(ContextReference{Kind: ContextReferenceFile, URI: path}, DetailSignature)
	if err != nil {
		t.Fatalf("HydrateFileReference(signature): %v", err)
	}
	if hydrated.Level != DetailSignature || hydrated.Content == "" {
		t.Fatalf("expected signature hydration, got %+v", hydrated)
	}

	full, err := sc.EnsureFileLevel(path, DetailFull)
	if err != nil {
		t.Fatalf("EnsureFileLevel(full): %v", err)
	}
	if full.Level != DetailFull || full.Content == "" {
		t.Fatalf("expected full hydration, got %+v", full)
	}

	if _, err := sc.EnsureFileLevel("missing.go", DetailSummary); err == nil {
		t.Fatal("EnsureFileLevel error = nil, want missing file")
	}

	sc.RecordMutation("sample.go", "set", "agent-1", &DerivationChain{})
	sc.RecordMutation("sample.go", "delete", "agent-2", nil)
	sc.RecordMutation("other.go", "set", "agent-3", nil)
	if got := sc.MutationHistory("sample.go"); len(got) != 2 {
		t.Fatalf("expected 2 sample mutations, got %d", len(got))
	}
	if got := sc.RecentMutations(2); len(got) != 2 {
		t.Fatalf("expected 2 recent mutations, got %d", len(got))
	}

	sc.SetChangeLogSummary("changes")
	if got := sc.GetChangeLogSummary(); got != "changes" {
		t.Fatalf("expected change log summary, got %q", got)
	}

	sc.SetChangeLogSummary("")
	sc.mu.Lock()
	sc.conversationSummary = "conversation"
	sc.mu.Unlock()
	if got := sc.GetChangeLogSummary(); got != "conversation" {
		t.Fatalf("expected fallback to conversation summary, got %q", got)
	}

	sc.mu.Lock()
	sc.history = []Interaction{{Role: "user", Content: "hello world"}}
	sc.compressedHistory = nil
	sc.mu.Unlock()
	sc.RefreshConversationSummary()
	if got := sc.GetConversationSummary(); got == "" {
		t.Fatal("expected refreshed conversation summary")
	}
}

func TestSharedContextDowngradeAndHistoryHelpers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	sc := NewSharedContext(nil, NewContextBudget(4096), &recordingSummarizer{})
	fc, err := sc.AddFile(path, content, "go", DetailSignature)
	if err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	fc.Level = DetailFull
	fc.LastAccessed = time.Now().Add(-2 * time.Hour)
	if err := sc.DowngradeOldFiles(DetailSummary, 128); err != nil {
		t.Fatalf("DowngradeOldFiles: %v", err)
	}
	if got, ok := sc.GetFile(path); !ok || got == nil {
		t.Fatal("expected file to remain tracked")
	}
	if got, ok := sc.GetFile(path); !ok || got.Level != DetailSummary {
		t.Fatalf("expected file to be downgraded to summary, got %+v", got)
	}

	if name := detailLevelName(DetailFull); name != "full" {
		t.Fatalf("unexpected detail level name: %q", name)
	}
	if got := linesAround("a\nb\nc\nd\ne\nf", 3); got != "a\nb\nc" {
		t.Fatalf("unexpected linesAround result: %q", got)
	}
	if got := splitLines("a\nb\n"); len(got) != 2 {
		t.Fatalf("unexpected splitLines result: %#v", got)
	}
	if got := joinLines([]string{"a", "b"}); got != "a\nb" {
		t.Fatalf("unexpected joinLines result: %q", got)
	}

	item := newFileBudgetItem(&FileContext{Path: path, Content: content, Level: DetailSignature, LastAccessed: time.Now().Add(-time.Hour)}, &recordingSummarizer{})
	if !item.CanCompress() || item.CanEvict() == false {
		t.Fatal("expected unpinned file item to compress and evict")
	}
	if _, err := item.Compress(); err != nil {
		t.Fatalf("file item compress: %v", err)
	}
	if item.GetTokenCount() == 0 {
		t.Fatal("expected refreshed token count")
	}
}
