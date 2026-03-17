package context_test

import (
	"testing"

	"github.com/lexcodex/relurpify/agents/chainer/context"
	"github.com/lexcodex/relurpify/framework/core"
)

func TestCompressionListener_NewCompressionListener(t *testing.T) {
	listener := context.NewCompressionListener()

	if listener == nil {
		t.Fatal("expected listener, got nil")
	}

	if listener.WarningCount() != 0 {
		t.Errorf("expected 0 warnings initially, got %d", listener.WarningCount())
	}
}

func TestCompressionListener_OnBudgetWarning(t *testing.T) {
	listener := context.NewCompressionListener()

	err := listener.OnBudgetWarning(200, 1000)
	if err == nil {
		t.Fatal("expected error on warning")
	}

	if listener.WarningCount() != 1 {
		t.Errorf("expected 1 warning, got %d", listener.WarningCount())
	}

	// Second warning
	err = listener.OnBudgetWarning(100, 1000)
	if listener.WarningCount() != 2 {
		t.Errorf("expected 2 warnings, got %d", listener.WarningCount())
	}
}

func TestCompressionListener_OnBudgetExceeded(t *testing.T) {
	listener := context.NewCompressionListener()

	err := listener.OnBudgetExceeded(0, 1000)
	if err == nil {
		t.Fatal("expected error on exceed")
	}
}

func TestCompressionListener_Reset(t *testing.T) {
	listener := context.NewCompressionListener()

	listener.OnBudgetWarning(200, 1000)
	listener.OnBudgetWarning(100, 1000)

	if listener.WarningCount() != 2 {
		t.Errorf("expected 2 warnings before reset, got %d", listener.WarningCount())
	}

	listener.Reset()

	if listener.WarningCount() != 0 {
		t.Errorf("expected 0 warnings after reset, got %d", listener.WarningCount())
	}
}

func TestNewCompressionListenerWithStrategy(t *testing.T) {
	strategies := []context.CompressionStrategy{
		context.StrategyAdaptive,
		context.StrategyAggressive,
		context.StrategyConservative,
	}

	for _, strategy := range strategies {
		listener := context.NewCompressionListenerWithStrategy(strategy)
		if listener == nil {
			t.Errorf("failed to create listener with strategy: %s", strategy)
		}
	}
}

func TestCompressContext_Adaptive(t *testing.T) {
	ctx := core.NewContext()
	ctx.Set("key1", "value1")

	compressed, err := context.CompressContext(ctx, context.StrategyAdaptive)
	if err != nil {
		t.Fatalf("CompressContext failed: %v", err)
	}

	if compressed == nil {
		t.Fatal("expected compressed context, got nil")
	}

	// Verify compression flag
	if marker, ok := compressed.Get("__compression_applied"); !ok || marker != "adaptive" {
		t.Errorf("expected compression marker, got %v", marker)
	}
}

func TestCompressContext_Aggressive(t *testing.T) {
	ctx := core.NewContext()
	ctx.Set("key1", "value1")

	compressed, err := context.CompressContext(ctx, context.StrategyAggressive)
	if err != nil {
		t.Fatalf("CompressContext failed: %v", err)
	}

	if compressed == nil {
		t.Fatal("expected compressed context, got nil")
	}

	// Verify compression flag
	if marker, ok := compressed.Get("__compression_applied"); !ok || marker != "aggressive" {
		t.Errorf("expected compression marker, got %v", marker)
	}
}

func TestCompressContext_Conservative(t *testing.T) {
	ctx := core.NewContext()
	ctx.Set("key1", "value1")

	compressed, err := context.CompressContext(ctx, context.StrategyConservative)
	if err != nil {
		t.Fatalf("CompressContext failed: %v", err)
	}

	if compressed == nil {
		t.Fatal("expected compressed context, got nil")
	}

	// Conservative strategy returns unchanged (no compression marker)
	if marker, ok := compressed.Get("__compression_applied"); ok {
		t.Errorf("conservative strategy should not mark, got %v", marker)
	}
}

func TestCompressContext_NilContext(t *testing.T) {
	_, err := context.CompressContext(nil, context.StrategyAdaptive)
	if err == nil {
		t.Fatal("expected error for nil context")
	}
}

func TestCompressContext_InvalidStrategy(t *testing.T) {
	ctx := core.NewContext()

	_, err := context.CompressContext(ctx, "invalid_strategy")
	if err == nil {
		t.Fatal("expected error for invalid strategy")
	}
}

func TestCompressionListener_AdaptiveThreshold(t *testing.T) {
	listener := context.NewCompressionListener()

	// First warning at 200 remaining
	_ = listener.OnBudgetWarning(200, 1000)

	// Second call with still good remaining should not trigger
	err := listener.OnBudgetWarning(180, 1000) // Only 20 tokens difference
	if err != nil {
		t.Errorf("should not warn when remaining space still good, got: %v", err)
	}

	// But warning should trigger if remaining drops significantly
	_ = listener.OnBudgetWarning(50, 1000)
	if listener.WarningCount() != 2 {
		t.Errorf("expected 2 warnings, got %d", listener.WarningCount())
	}
}

func TestCompressionWorkflow(t *testing.T) {
	// Simulate real workflow: budget warning → compression → continue

	manager := context.NewBudgetManager(1000)
	listener := context.NewCompressionListener()

	manager.AddListener(listener)
	manager.SetWarningThreshold(75)

	// Track 700 tokens (70% - below warning)
	_ = manager.Track("llm", 700)
	if listener.WarningCount() != 0 {
		t.Fatal("should not warn at 70%")
	}

	// Track 100 more (80% - above warning)
	_ = manager.Track("llm", 100)
	if listener.WarningCount() == 0 {
		t.Fatal("should warn at 80%")
	}

	// Simulate compression (reclaim ~100 tokens)
	_ = manager.Reset() // Simulate successful compression

	// Verify budget was reset
	budget := manager.Budget()
	used := budget["used"].(int)
	if used != 0 {
		t.Errorf("expected reset budget, got %d used", used)
	}

	// Can now continue with fresh budget
	_ = manager.Track("llm", 100)
	if listener.WarningCount() != 1 { // Still just 1 from before reset
		t.Errorf("expected original warning count preserved, got %d", listener.WarningCount())
	}
}
