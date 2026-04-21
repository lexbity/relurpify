package context_test

import (
	"testing"

	"codeburg.org/lexbit/relurpify/agents/chainer/context"
)

func TestBudgetManager_NewBudgetManager(t *testing.T) {
	manager := context.NewBudgetManager(4096)

	if manager == nil {
		t.Fatal("expected manager, got nil")
	}

	budget := manager.Budget()
	if budget == nil {
		t.Fatal("expected budget, got nil")
	}

	total := budget["total"].(int)
	if total != 4096 {
		t.Errorf("expected total 4096, got %d", total)
	}

	used := budget["used"].(int)
	if used != 0 {
		t.Errorf("expected used 0, got %d", used)
	}
}

func TestBudgetManager_Track(t *testing.T) {
	manager := context.NewBudgetManager(1000)

	// Track 100 tokens in LLM category
	err := manager.Track("llm", 100)
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}

	budget := manager.Budget()
	used := budget["used"].(int)
	if used != 100 {
		t.Errorf("expected used 100, got %d", used)
	}
}

func TestBudgetManager_WarningThreshold(t *testing.T) {
	manager := context.NewBudgetManager(1000)
	listener := &testBudgetListener{warningsCaught: 0}

	manager.AddListener(listener)

	// Set warning at 50%
	manager.SetWarningThreshold(50)

	// Track 400 tokens (40% usage - below warning)
	_ = manager.Track("llm", 400)
	if listener.warningsCaught != 0 {
		t.Errorf("expected no warning at 40%%, got %d", listener.warningsCaught)
	}

	// Track 100 more tokens (50% usage - at warning)
	_ = manager.Track("llm", 100)
	if listener.warningsCaught == 0 {
		t.Fatal("expected warning at 50%")
	}
}

func TestBudgetManager_ExceededThreshold(t *testing.T) {
	manager := context.NewBudgetManager(1000)
	listener := &testBudgetListener{}

	manager.AddListener(listener)

	// Track 900 tokens
	_ = manager.Track("llm", 900)

	// Track 150 more tokens (exceeds 1000)
	err := manager.Track("llm", 150)
	if err == nil {
		t.Fatal("expected error when exceeding budget")
	}

	if listener.exceedCaught == false {
		t.Fatal("expected exceeded listener to be called")
	}
}

func TestBudgetManager_Reset(t *testing.T) {
	manager := context.NewBudgetManager(1000)

	_ = manager.Track("llm", 500)

	budget := manager.Budget()
	used := budget["used"].(int)
	if used != 500 {
		t.Errorf("expected 500 used before reset, got %d", used)
	}

	err := manager.Reset()
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	budget = manager.Budget()
	used = budget["used"].(int)
	if used != 0 {
		t.Errorf("expected 0 used after reset, got %d", used)
	}
}

func TestBudgetManager_SetWarningThreshold(t *testing.T) {
	manager := context.NewBudgetManager(1000)

	// Valid thresholds
	for _, percent := range []int{1, 50, 99} {
		err := manager.SetWarningThreshold(percent)
		if err != nil {
			t.Errorf("valid threshold %d failed: %v", percent, err)
		}
	}

	// Invalid thresholds
	for _, percent := range []int{0, 100, 150, -10} {
		err := manager.SetWarningThreshold(percent)
		if err == nil {
			t.Errorf("invalid threshold %d should fail", percent)
		}
	}
}

func TestBudgetManager_EstimatedCompression(t *testing.T) {
	manager := context.NewBudgetManager(1000)

	// Track 500 tokens
	_ = manager.Track("llm", 500)

	// Estimated compression at 50% rate
	compression := manager.EstimatedCompression()
	if compression == 0 {
		t.Fatal("expected estimated compression > 0")
	}

	// Should be around 250 (50% of 500)
	if compression < 200 || compression > 300 {
		t.Errorf("expected compression ~250, got %d", compression)
	}
}

func TestBudgetManager_MultipleListeners(t *testing.T) {
	manager := context.NewBudgetManager(1000)

	listener1 := &testBudgetListener{}
	listener2 := &testBudgetListener{}

	manager.AddListener(listener1)
	manager.AddListener(listener2)

	manager.SetWarningThreshold(50)
	_ = manager.Track("llm", 600) // Trigger warning

	if listener1.warningsCaught == 0 || listener2.warningsCaught == 0 {
		t.Fatal("expected both listeners to receive warning")
	}
}

func TestBudgetManager_RemoveAllListeners(t *testing.T) {
	manager := context.NewBudgetManager(1000)
	listener := &testBudgetListener{}

	manager.AddListener(listener)
	manager.RemoveAllListeners()

	manager.SetWarningThreshold(50)
	_ = manager.Track("llm", 600)

	if listener.warningsCaught != 0 {
		t.Fatal("expected no warning after removing listeners")
	}
}

// Test listener implementation
type testBudgetListener struct {
	warningsCaught int
	exceedCaught   bool
}

func (l *testBudgetListener) OnBudgetWarning(remaining int, limit int) error {
	l.warningsCaught++
	return nil
}

func (l *testBudgetListener) OnBudgetExceeded(remaining int, limit int) error {
	l.exceedCaught = true
	return nil
}
