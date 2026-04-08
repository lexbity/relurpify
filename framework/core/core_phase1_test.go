package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubLanguageModel struct {
	lastPrompt string
	response   *LLMResponse
	err        error
}

type stubCompressionStrategy struct {
	compressed     *CompressedContext
	estimate       int
	keepRecent     int
	lastInputCount int
}

func (s *stubLanguageModel) Generate(ctx context.Context, prompt string, options *LLMOptions) (*LLMResponse, error) {
	s.lastPrompt = prompt
	if s.err != nil {
		return nil, s.err
	}
	if s.response != nil {
		return s.response, nil
	}
	return &LLMResponse{Text: "Summary: stub"}, nil
}

func (s *stubLanguageModel) GenerateStream(ctx context.Context, prompt string, options *LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (s *stubLanguageModel) Chat(ctx context.Context, messages []Message, options *LLMOptions) (*LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *stubLanguageModel) ChatWithTools(ctx context.Context, messages []Message, tools []LLMToolSpec, options *LLMOptions) (*LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *stubCompressionStrategy) Compress(interactions []Interaction, llm LanguageModel) (*CompressedContext, error) {
	s.lastInputCount = len(interactions)
	if s.compressed != nil {
		cc := *s.compressed
		return &cc, nil
	}
	return &CompressedContext{Summary: "stub summary"}, nil
}

func (s *stubCompressionStrategy) ShouldCompress(ctx *Context, budget *ContextBudget) bool {
	return true
}

func (s *stubCompressionStrategy) EstimateTokens(cc *CompressedContext) int {
	if s.estimate != 0 {
		return s.estimate
	}
	if cc == nil {
		return 0
	}
	return estimateTokens(cc.Summary) + estimateTokens(cc.KeyFacts)
}

func (s *stubCompressionStrategy) KeepRecent() int {
	if s.keepRecent != 0 {
		return s.keepRecent
	}
	return 1
}

func TestContextStateSnapshotDeepCopiesAndMergeConflicts(t *testing.T) {
	var nilCtx *Context
	if got := nilCtx.StateSnapshot(); got != nil {
		t.Fatalf("expected nil snapshot for nil context, got %#v", got)
	}

	ctx := NewContext()
	ctx.Set("nested", map[string]interface{}{
		"outer": map[string]interface{}{"value": "base"},
	})
	ctx.recordMergeConflict("route", "losing", "winning", "state")

	snapshot := ctx.StateSnapshot()
	nested := snapshot["nested"].(map[string]interface{})
	outer := nested["outer"].(map[string]interface{})
	outer["value"] = "mutated"

	original, ok := ctx.Get("nested")
	if !ok {
		t.Fatal("expected nested state to exist")
	}
	originalOuter := original.(map[string]interface{})["outer"].(map[string]interface{})
	if got := originalOuter["value"]; got != "base" {
		t.Fatalf("expected original state to stay unchanged, got %#v", got)
	}

	conflicts := ctx.MergeConflicts()
	if len(conflicts) != 1 {
		t.Fatalf("expected one merge conflict, got %d", len(conflicts))
	}
	if conflicts[0].Key != "route" || conflicts[0].ConflictArea != "state" {
		t.Fatalf("unexpected merge conflict record: %#v", conflicts[0])
	}
	conflicts[0].Key = "changed"
	if got := ctx.MergeConflicts()[0].Key; got != "route" {
		t.Fatalf("expected merge conflicts to be returned by copy, got %q", got)
	}
}

func TestContextHistoryCompressionAndRendering(t *testing.T) {
	ctx := NewContext()
	ctx.AddInteraction("system", "bootstrap", nil)
	ctx.AddInteraction("user", "first", nil)
	ctx.AddInteraction("assistant", "second", nil)

	latest, ok := ctx.LatestInteraction()
	if !ok || latest.Content != "second" {
		t.Fatalf("unexpected latest interaction: %#v, ok=%v", latest, ok)
	}

	ctx.TrimHistory(2)
	history := ctx.History()
	if len(history) != 2 {
		t.Fatalf("expected 2 interactions after trim, got %d", len(history))
	}
	if history[0].Content != "first" || history[1].Content != "second" {
		t.Fatalf("unexpected trimmed history: %#v", history)
	}

	compressCtx := NewContext()
	compressCtx.AddInteraction("system", "bootstrap", nil)
	compressCtx.AddInteraction("user", strings.Repeat("alpha ", 8), nil)
	compressCtx.AddInteraction("assistant", strings.Repeat("beta ", 8), nil)
	compressCtx.AddInteraction("user", "final question", nil)

	strategy := &stubCompressionStrategy{
		compressed: &CompressedContext{
			Summary:  "condensed summary",
			KeyFacts: []KeyFact{{Type: "decision", Content: "use stub summary"}},
		},
		estimate:   17,
		keepRecent: 1,
	}

	if err := compressCtx.CompressHistory(1, nil, strategy); err != nil {
		t.Fatalf("CompressHistory: %v", err)
	}
	if strategy.lastInputCount != 3 {
		t.Fatalf("expected 3 interactions to be compressed, got %d", strategy.lastInputCount)
	}

	compressed, recent := compressCtx.GetFullHistory()
	if len(compressed) != 1 || len(recent) != 1 {
		t.Fatalf("unexpected history split: compressed=%d recent=%d", len(compressed), len(recent))
	}
	if got := compressed[0].Summary; got != "condensed summary" {
		t.Fatalf("unexpected compressed summary: %q", got)
	}
	if got := compressed[0].CompressedTokens; got != 17 {
		t.Fatalf("expected compressed token estimate to be populated, got %d", got)
	}

	rendered := compressCtx.GetContextForLLM()
	if !strings.Contains(rendered, "=== Previous Context (Compressed) ===") {
		t.Fatalf("expected compressed section in LLM context, got %q", rendered)
	}
	if !strings.Contains(rendered, "condensed summary") || !strings.Contains(rendered, "final question") {
		t.Fatalf("expected rendered context to include summary and recent tail, got %q", rendered)
	}

	stats := compressCtx.GetCompressionStats()
	if stats.CompressionEvents != 1 || stats.TotalInteractionsCompressed != 3 {
		t.Fatalf("unexpected compression stats: %#v", stats)
	}
	if stats.CompressedChunks != 1 || stats.CurrentHistorySize != 1 {
		t.Fatalf("unexpected history stats: %#v", stats)
	}
	if stats.TotalTokensSaved <= 0 {
		t.Fatalf("expected tokens saved to be positive, got %#v", stats)
	}

	if err := compressCtx.CompressHistory(1, nil, nil); err == nil {
		t.Fatal("expected nil compression strategy to fail")
	}
}

func TestSimpleCompressionStrategyHelpers(t *testing.T) {
	strategy := NewSimpleCompressionStrategy()

	ctx := NewContext()
	for i := 0; i < strategy.MinInteractionsTrigger-1; i++ {
		ctx.AddInteraction("user", "turn", nil)
	}
	if strategy.ShouldCompress(ctx, nil) {
		t.Fatal("expected compression to stay off below the trigger threshold")
	}
	ctx.AddInteraction("assistant", "turn", nil)
	budget := NewContextBudget(1000)
	budget.SetCurrentUsage(TokenUsage{ContextUsagePercent: 0.65})
	if strategy.ShouldCompress(ctx, budget) {
		t.Fatal("expected compression to stay off below the budget threshold")
	}
	budget.SetCurrentUsage(TokenUsage{ContextUsagePercent: 0.8})
	if !strategy.ShouldCompress(ctx, budget) {
		t.Fatal("expected compression to activate once both thresholds are met")
	}

	if got := strategy.KeepRecent(); got != 5 {
		t.Fatalf("unexpected keep-recent count: %d", got)
	}
	if got := strategy.EstimateTokens(nil); got != 0 {
		t.Fatalf("expected nil compressed context to estimate to zero, got %d", got)
	}
	if got := strategy.EstimateTokens(&CompressedContext{Summary: "summary", KeyFacts: []KeyFact{{Type: "decision", Content: "fact"}}}); got <= 0 {
		t.Fatalf("expected compressed token estimate to be positive, got %d", got)
	}

	prompt := strategy.buildPrompt([]Interaction{{Role: "user", Content: strings.Repeat("x", 240)}})
	if !strings.Contains(prompt, "...") {
		t.Fatalf("expected long interaction content to be truncated in prompt, got %q", prompt)
	}

	parsed, err := strategy.parseCompressionResponse(
		"Summary: condensed\nKey Facts: [{\"type\":\"decision\",\"content\":\"done\",\"relevance\":0.9}]",
		[]Interaction{{Role: "user", Content: "hello"}},
	)
	if err != nil {
		t.Fatalf("parseCompressionResponse(valid): %v", err)
	}
	if parsed.Summary != "condensed" || len(parsed.KeyFacts) != 1 {
		t.Fatalf("unexpected parsed response: %#v", parsed)
	}

	fallback, err := strategy.parseCompressionResponse(
		"Summary: condensed\nKey Facts: not-json",
		[]Interaction{{Role: "user", Content: "hello"}},
	)
	if err != nil {
		t.Fatalf("parseCompressionResponse(fallback): %v", err)
	}
	if len(fallback.KeyFacts) != 1 || fallback.KeyFacts[0].Type != "summary" {
		t.Fatalf("expected fallback key fact to be generated, got %#v", fallback.KeyFacts)
	}

	llm := &stubLanguageModel{
		response: &LLMResponse{
			Text: "Summary: generated\nKey Facts: [{\"type\":\"decision\",\"content\":\"processed\",\"relevance\":1.0}]",
		},
	}
	compressed, err := strategy.Compress([]Interaction{{Role: "user", Content: "describe"}}, llm)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if compressed.Summary != "generated" || len(compressed.KeyFacts) != 1 {
		t.Fatalf("unexpected compressed result: %#v", compressed)
	}
	if !strings.Contains(llm.lastPrompt, "Summarize these agent interactions") {
		t.Fatalf("expected compression prompt to be built, got %q", llm.lastPrompt)
	}

	if _, err := strategy.Compress([]Interaction{{Role: "user", Content: "describe"}}, nil); err == nil {
		t.Fatal("expected compression to fail without a language model")
	}
	if _, err := strategy.Compress(nil, llm); err == nil {
		t.Fatal("expected compression to fail with no interactions")
	}
}

func TestContextBudgetBorrowingAndUsageClone(t *testing.T) {
	budget := NewContextBudgetWithPolicy(200, &AllocationPolicy{
		SystemReserved: 0,
		Allocations: map[string]float64{
			"alpha": 0.5,
			"beta":  0.5,
		},
		AllowBorrowing:     true,
		MinimumPerCategory: 0,
	})

	if got := budget.Categories(); len(got) != 3 || got[0] != "alpha" || got[1] != "beta" || got[2] != "system" {
		t.Fatalf("unexpected categories: %#v", got)
	}

	usage := budget.GetUsage()
	if usage == nil || usage.TotalTokens != 200 {
		t.Fatalf("unexpected initial usage snapshot: %#v", usage)
	}
	usage.Categories["alpha"].MaxTokens = 1
	if fresh := budget.GetUsage(); fresh.Categories["alpha"].MaxTokens == 1 {
		t.Fatal("expected GetUsage to return a defensive copy")
	}

	if err := budget.Allocate("beta", 40, nil); err != nil {
		t.Fatalf("beta allocation: %v", err)
	}
	if err := budget.Allocate("alpha", 120, nil); err != nil {
		t.Fatalf("alpha allocation should borrow capacity: %v", err)
	}

	usage = budget.GetUsage()
	if usage.Categories["alpha"].MaxTokens <= 100 {
		t.Fatalf("expected alpha category to receive borrowed capacity, got %#v", usage.Categories["alpha"])
	}
	if usage.Categories["beta"].MaxTokens >= 100 {
		t.Fatalf("expected beta category to donate capacity, got %#v", usage.Categories["beta"])
	}

	if remaining := budget.GetRemainingBudget("alpha"); remaining != 0 {
		t.Fatalf("expected alpha to be fully consumed, got %d", remaining)
	}
	if remaining := budget.GetRemainingBudget("missing"); remaining != 0 {
		t.Fatalf("expected missing category to report zero remaining tokens, got %d", remaining)
	}
}

func TestContextBudgetCompressionListenerAndLegacyUsage(t *testing.T) {
	budget := NewContextBudgetWithPolicy(100, &AllocationPolicy{
		SystemReserved: 0,
		Allocations: map[string]float64{
			"work": 1.0,
		},
		AllowBorrowing:     false,
		MinimumPerCategory: 0,
	})
	listener := &testBudgetListener{}
	budget.AddListener(listener)

	first := &testBudgetItem{
		id:          "first",
		tokens:      60,
		priority:    10,
		compressTo:  20,
		canCompress: true,
		canEvict:    true,
	}
	second := &testBudgetItem{
		id:          "second",
		tokens:      60,
		priority:    5,
		compressTo:  20,
		canCompress: true,
		canEvict:    true,
	}

	if err := budget.Allocate("work", 0, first); err != nil {
		t.Fatalf("initial allocation: %v", err)
	}
	if err := budget.Allocate("work", 0, second); err != nil {
		t.Fatalf("allocation after compression should succeed: %v", err)
	}
	if len(listener.compression) == 0 {
		t.Fatal("expected compression listener to be notified")
	}

	if got := budget.GetUsage(); got == nil || got.Categories["work"].UsedTokens != 80 {
		t.Fatalf("expected compressed category usage to be updated, got %#v", got)
	}

	budget.Free("work", 0, "first")
	if got := budget.GetUsage(); got.Categories["work"].ItemCount != 1 {
		t.Fatalf("expected free-by-item-id to remove the item, got %#v", got.Categories["work"])
	}

	ctx := NewContext()
	ctx.Set("state", strings.Repeat("x", 32))
	ctx.AddInteraction("assistant", strings.Repeat("hello ", 16), nil)
	budget.SetReservations(10, 20, 30)
	budget.UpdateUsage(ctx, []Tool{testTool{}})
	current := budget.GetCurrentUsage()
	if current.TotalTokens == 0 || current.ContextTokens == 0 || current.ToolTokens == 0 {
		t.Fatalf("expected legacy usage accounting to be populated, got %#v", current)
	}
	if budget.GetAvailableTokens() <= 0 {
		t.Fatalf("expected available legacy tokens to be positive, got %#v", budget.GetAvailableTokens())
	}
}
