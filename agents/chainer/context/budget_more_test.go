package context

import (
	"testing"
)

// TestNewBudgetManager tests the budget manager constructor
func TestNewBudgetManager(t *testing.T) {
	bm := NewBudgetManager(1000)
	if bm == nil {
		t.Fatal("expected non-nil budget manager")
	}

	// Verify initial state via Budget() call
	metrics := bm.Budget()
	if metrics == nil {
		t.Fatal("expected non-nil metrics")
	}

	if metrics["total"] != 1000 {
		t.Errorf("expected total 1000, got %v", metrics["total"])
	}

	if metrics["used"] != 0 {
		t.Errorf("expected used 0, got %v", metrics["used"])
	}

	if metrics["remaining"] != 1000 {
		t.Errorf("expected remaining 1000, got %v", metrics["remaining"])
	}
}

// TestBudgetManagerBudgetNil tests Budget on nil receiver
func TestBudgetManagerBudgetNil(t *testing.T) {
	var nilBM *SimpleBudgetTracker
	if nilBM.Budget() != nil {
		t.Error("expected nil budget for nil tracker")
	}
}

// TestBudgetManagerTrack tests the Track method
func TestBudgetManagerTrack(t *testing.T) {
	t.Run("track tokens", func(t *testing.T) {
		bm := NewBudgetManager(1000)

		err := bm.Track("llm", 300)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		metrics := bm.Budget()
		if metrics["used"] != 300 {
			t.Errorf("expected used 300, got %v", metrics["used"])
		}

		// Track more
		err = bm.Track("embedding", 200)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		metrics = bm.Budget()
		if metrics["used"] != 500 {
			t.Errorf("expected used 500, got %v", metrics["used"])
		}
	})

	t.Run("track exceeds budget", func(t *testing.T) {
		bm := NewBudgetManager(100)

		err := bm.Track("llm", 150)
		if err == nil {
			t.Error("expected error when exceeding budget")
		}

		metrics := bm.Budget()
		// Used should still be recorded even if exceeded
		if metrics["used"] != 150 {
			t.Errorf("expected used 150, got %v", metrics["used"])
		}
	})

	t.Run("nil tracker track", func(t *testing.T) {
		var nilBM *SimpleBudgetTracker
		err := nilBM.Track("llm", 100)
		if err == nil {
			t.Error("expected error on nil tracker")
		}
	})
}

// TestBudgetManagerWarningThreshold tests the warning threshold functionality
func TestBudgetManagerWarningThreshold(t *testing.T) {
	t.Run("default threshold warning", func(t *testing.T) {
		bm := NewBudgetManager(100)
		warningTriggered := false
		bm.AddListener(&testListener{
			onWarning: func(remaining, limit int) error {
				warningTriggered = true
				return nil
			},
		})

		// Track 85% (above 80% default warning threshold)
		bm.Track("llm", 85)

		if !warningTriggered {
			t.Error("expected warning to be triggered at 85%")
		}
	})

	t.Run("custom threshold", func(t *testing.T) {
		bm := NewBudgetManager(100)
		err := bm.SetWarningThreshold(50)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		warningTriggered := false
		bm.AddListener(&testListener{
			onWarning: func(remaining, limit int) error {
				warningTriggered = true
				return nil
			},
		})

		// Track 60% (above 50% custom threshold)
		bm.Track("llm", 60)

		if !warningTriggered {
			t.Error("expected warning to be triggered at 60% with 50% threshold")
		}
	})

	t.Run("invalid threshold", func(t *testing.T) {
		bm := NewBudgetManager(100)

		if err := bm.SetWarningThreshold(0); err == nil {
			t.Error("expected error for 0 threshold")
		}

		if err := bm.SetWarningThreshold(100); err == nil {
			t.Error("expected error for 100 threshold")
		}

		if err := bm.SetWarningThreshold(101); err == nil {
			t.Error("expected error for 101 threshold")
		}
	})
}

// TestBudgetManagerReset tests the Reset method
func TestBudgetManagerReset(t *testing.T) {
	t.Run("reset clears state", func(t *testing.T) {
		bm := NewBudgetManager(1000)
		bm.Track("llm", 500)

		err := bm.Reset()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		metrics := bm.Budget()
		if metrics["used"] != 0 {
			t.Errorf("expected used 0 after reset, got %v", metrics["used"])
		}
	})

	t.Run("nil tracker reset", func(t *testing.T) {
		var nilBM *SimpleBudgetTracker
		err := nilBM.Reset()
		if err == nil {
			t.Error("expected error on nil tracker reset")
		}
	})
}

// TestBudgetManagerRemoveAllListeners tests listener removal
func TestBudgetManagerRemoveAllListeners(t *testing.T) {
	bm := NewBudgetManager(100)

	listenerCallCount := 0
	bm.AddListener(&testListener{
		onWarning: func(remaining, limit int) error {
			listenerCallCount++
			return nil
		},
	})

	// Remove all listeners
	bm.RemoveAllListeners()

	// Track tokens that would trigger warning
	bm.Track("llm", 85)

	if listenerCallCount != 0 {
		t.Error("expected listener not to be called after removal")
	}
}

// TestBudgetManagerEstimatedCompression tests compression estimation
func TestBudgetManagerEstimatedCompression(t *testing.T) {
	t.Run("estimate with usage", func(t *testing.T) {
		bm := NewBudgetManager(1000)
		bm.Track("llm", 600)

		// Estimate should be 50% of used tokens
		estimate := bm.EstimatedCompression()
		if estimate != 300 {
			t.Errorf("expected estimate 300, got %d", estimate)
		}
	})

	t.Run("nil tracker estimate", func(t *testing.T) {
		var nilBM *SimpleBudgetTracker
		if nilBM.EstimatedCompression() != 0 {
			t.Error("expected 0 for nil tracker")
		}
	})
}

// testListener is a test helper for budget listener tests
type testListener struct {
	onWarning  func(remaining, limit int) error
	onExceeded func(remaining, limit int) error
}

func (l *testListener) OnBudgetWarning(remaining, limit int) error {
	if l.onWarning != nil {
		return l.onWarning(remaining, limit)
	}
	return nil
}

func (l *testListener) OnBudgetExceeded(remaining, limit int) error {
	if l.onExceeded != nil {
		return l.onExceeded(remaining, limit)
	}
	return nil
}
