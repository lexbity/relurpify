package framework_test

import (
	"github.com/lexcodex/relurpify/framework/core"
	"path/filepath"
	"strings"
	"testing"
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
