package framework_test

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestSimpleCompressionStrategyCompress(t *testing.T) {
	strategy := core.NewSimpleCompressionStrategy()
	llm := &stubLLM{text: `Summary: Completed refactor.
Key Facts: [{"type":"decision","content":"Refactored module","relevance":0.9}]`}
	interactions := []core.Interaction{
		{ID: 1, Role: "user", Content: "Please refactor the module", Timestamp: time.Now()},
		{ID: 2, Role: "assistant", Content: "Working on it", Timestamp: time.Now()},
	}
	cc, err := strategy.Compress(interactions, llm)
	if err != nil {
		t.Fatalf("compress returned error: %v", err)
	}
	if cc == nil {
		t.Fatal("expected compressed context")
	}
	if cc.Summary == "" {
		t.Fatal("expected summary to be populated")
	}
	if len(cc.KeyFacts) == 0 {
		t.Fatal("expected key facts to be extracted")
	}
	if cc.OriginalTokens <= cc.CompressedTokens {
		t.Fatalf("expected compression to reduce tokens, got original=%d compressed=%d", cc.OriginalTokens, cc.CompressedTokens)
	}
}

func TestSimpleCompressionStrategyShouldCompress(t *testing.T) {
	strategy := core.NewSimpleCompressionStrategy()
	ctx := core.NewContext()
	for i := 0; i < 6; i++ {
		ctx.AddInteraction("user", "message", nil)
	}
	if !strategy.ShouldCompress(ctx, nil) {
		t.Fatal("expected compression recommendation when history exceeds threshold")
	}
	shortCtx := core.NewContext()
	for i := 0; i < 2; i++ {
		shortCtx.AddInteraction("user", "message", nil)
	}
	budget := core.NewArtifactBudget(1000)
	usage := budget.GetCurrentUsage()
	usage.ArtifactUsagePercent = 0.5
	budget.SetCurrentUsage(usage)
	if strategy.ShouldCompress(shortCtx, budget) {
		t.Fatal("expected compression to stay disabled when usage below threshold")
	}
	usage = budget.GetCurrentUsage()
	usage.ArtifactUsagePercent = 0.9
	budget.SetCurrentUsage(usage)
	if !strategy.ShouldCompress(shortCtx, budget) {
		t.Fatal("expected compression once usage exceeds threshold")
	}
}

func TestContextCompressHistory(t *testing.T) {
	ctx := core.NewContext()
	strategy := core.NewSimpleCompressionStrategy()
	for i := 0; i < 15; i++ {
		ctx.AddInteraction("user", "long message content", nil)
	}
	compressed, history := ctx.GetFullHistory()
	if len(compressed) == 0 {
		t.Fatal("expected compressed history entries")
	}
	if len(history) != strategy.KeepRecentCount {
		t.Fatalf("expected recent history length %d, got %d", strategy.KeepRecentCount, len(history))
	}
}
