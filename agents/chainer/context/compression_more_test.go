package context

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

// TestCompressContext tests the CompressContext function
func TestCompressContext(t *testing.T) {
	t.Run("nil context", func(t *testing.T) {
		result, err := CompressContext(nil, StrategyAdaptive)
		if err == nil {
			t.Error("expected error for nil context")
		}
		if result != nil {
			t.Error("expected nil result for nil context")
		}
	})

	t.Run("adaptive compression", func(t *testing.T) {
		ctx := core.NewContext()
		ctx.Set("key1", "value1")
		ctx.Set("key2", "value2")

		result, err := CompressContext(ctx, StrategyAdaptive)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}

		// Check compression marker
		if marker, ok := result.Get("__compression_applied"); !ok || marker != "adaptive" {
			t.Error("expected adaptive compression marker")
		}
	})

	t.Run("aggressive compression", func(t *testing.T) {
		ctx := core.NewContext()
		ctx.Set("key1", "value1")

		result, err := CompressContext(ctx, StrategyAggressive)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if marker, ok := result.Get("__compression_applied"); !ok || marker != "aggressive" {
			t.Error("expected aggressive compression marker")
		}
	})

	t.Run("conservative compression", func(t *testing.T) {
		ctx := core.NewContext()
		ctx.Set("key1", "value1")

		result, err := CompressContext(ctx, StrategyConservative)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Conservative should return same context, no marker
		if _, ok := result.Get("__compression_applied"); ok {
			t.Error("expected no compression marker for conservative strategy")
		}
	})

	t.Run("unknown strategy", func(t *testing.T) {
		ctx := core.NewContext()
		result, err := CompressContext(ctx, CompressionStrategy("unknown"))
		if err == nil {
			t.Error("expected error for unknown strategy")
		}
		if result != nil {
			t.Error("expected nil result for unknown strategy")
		}
	})
}

// TestCompressAdaptive tests the compressAdaptive function directly
func TestCompressAdaptive(t *testing.T) {
	t.Run("nil context adaptive", func(t *testing.T) {
		result, err := compressAdaptive(nil)
		if err == nil {
			t.Error("expected error for nil context")
		}
		if result != nil {
			t.Error("expected nil result for nil context")
		}
	})

	t.Run("adaptive compression copies context", func(t *testing.T) {
		ctx := core.NewContext()
		ctx.Set("data", "value")

		result, err := compressAdaptive(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Result should be a clone
		if val, ok := result.Get("data"); !ok || val != "value" {
			t.Error("expected data to be preserved in compressed context")
		}

		// Modifying result should not affect original
		result.Set("newkey", "newvalue")
		if _, ok := ctx.Get("newkey"); ok {
			t.Error("modifying compressed context should not affect original")
		}
	})
}

// TestCompressAggressive tests the compressAggressive function directly
func TestCompressAggressive(t *testing.T) {
	t.Run("nil context aggressive", func(t *testing.T) {
		result, err := compressAggressive(nil)
		if err == nil {
			t.Error("expected error for nil context")
		}
		if result != nil {
			t.Error("expected nil result for nil context")
		}
	})

	t.Run("aggressive compression copies context", func(t *testing.T) {
		ctx := core.NewContext()
		ctx.Set("data", "value")

		result, err := compressAggressive(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if marker, ok := result.Get("__compression_applied"); !ok || marker != "aggressive" {
			t.Error("expected aggressive compression marker")
		}
	})
}

// TestCompressionListenerWarningCount tests the WarningCount method
func TestCompressionListenerWarningCount(t *testing.T) {
	t.Run("nil listener warning count", func(t *testing.T) {
		var nilListener *CompressionListener
		if nilListener.WarningCount() != 0 {
			t.Error("expected 0 for nil listener")
		}
	})

	t.Run("count warnings", func(t *testing.T) {
		listener := NewCompressionListener()

		// Simulate warnings by calling OnBudgetWarning
		listener.OnBudgetWarning(400, 1000) // 60% used
		listener.OnBudgetWarning(200, 1000) // 80% used, remaining dropped significantly

		if count := listener.WarningCount(); count != 2 {
			t.Errorf("expected 2 warnings, got %d", count)
		}
	})
}

// TestCompressionListenerOnBudgetExceededNil tests nil receiver
func TestCompressionListenerOnBudgetExceededNil(t *testing.T) {
	var nilListener *CompressionListener
	err := nilListener.OnBudgetExceeded(0, 1000)
	if err == nil {
		t.Error("expected error for nil listener")
	}
}

// TestCompressionListenerOnBudgetWarningNil tests nil receiver
func TestCompressionListenerOnBudgetWarningNil(t *testing.T) {
	var nilListener *CompressionListener
	err := nilListener.OnBudgetWarning(200, 1000)
	if err == nil {
		t.Error("expected error for nil listener")
	}
}
