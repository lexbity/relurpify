package framework_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestSharedContextDowngradesOnBudgetWarning(t *testing.T) {
	ctx := core.NewContext()
	budget := core.NewContextBudget(256)
	summarizer := &core.SimpleSummarizer{}
	sc := core.NewSharedContext(ctx, budget, summarizer)

	path := filepath.Join(t.TempDir(), "file.go")
	content := strings.Repeat("func example() {}\n", 50)
	fc, err := sc.AddFile(path, content, "go", core.DetailFull)
	if err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}
	if fc.Level != core.DetailFull {
		t.Fatalf("expected DetailFull, got %v", fc.Level)
	}

	// Simulate budget pressure.
	sc.OnBudgetWarning(0.9)
	if fc.Level != core.DetailSummary {
		t.Fatalf("expected file downgraded to summary, got %v", fc.Level)
	}
}

func TestSharedContextRefreshConversationSummary(t *testing.T) {
	ctx := core.NewContext()
	sc := core.NewSharedContext(ctx, nil, &core.SimpleSummarizer{})
	sc.AddInteraction("user", "Add new API endpoint", nil)
	sc.AddInteraction("assistant", "Implemented handler", nil)

	sc.RefreshConversationSummary()
	if sc.GetConversationSummary() == "" {
		t.Fatalf("expected conversation summary to be populated")
	}
}

func TestSharedContextRehydratesFromCachedRawContent(t *testing.T) {
	ctx := core.NewContext()
	budget := core.NewContextBudget(512)
	summarizer := &core.SimpleSummarizer{}
	sc := core.NewSharedContext(ctx, budget, summarizer)

	path := filepath.Join(t.TempDir(), "file.go")
	content := strings.Repeat("func example() {}\n", 50)
	fc, err := sc.AddFile(path, content, "go", core.DetailFull)
	if err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}
	if err := sc.DowngradeOldFiles(core.DetailSummary, 1); err != nil {
		t.Fatalf("DowngradeOldFiles failed: %v", err)
	}
	if fc.Content != "" || fc.RawContent == "" {
		t.Fatalf("expected downgraded file to keep raw content cached, got %+v", fc)
	}
	if err := os.Remove(path); err == nil {
		if _, err := sc.EnsureFileLevel(path, core.DetailFull); err != nil {
			t.Fatalf("EnsureFileLevel should use cached raw content, got %v", err)
		}
		if fc.Content == "" {
			t.Fatalf("expected content restored from cache")
		}
	}
}
