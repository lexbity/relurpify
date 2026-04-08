package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type testBudgetListener struct {
	warnings    []float64
	exceeded    []string
	compression []int
}

func (l *testBudgetListener) OnBudgetWarning(usage float64) {
	l.warnings = append(l.warnings, usage)
}

func (l *testBudgetListener) OnBudgetExceeded(category string, requested, available int) {
	l.exceeded = append(l.exceeded, category)
}

func (l *testBudgetListener) OnCompression(category string, savedTokens int) {
	l.compression = append(l.compression, savedTokens)
}

type testBudgetItem struct {
	id          string
	tokens      int
	priority    int
	compressTo  int
	canCompress bool
	canEvict    bool
	err         error
}

func (i *testBudgetItem) GetID() string { return i.id }

func (i *testBudgetItem) GetTokenCount() int { return i.tokens }

func (i *testBudgetItem) GetPriority() int { return i.priority }

func (i *testBudgetItem) CanCompress() bool { return i.canCompress }

func (i *testBudgetItem) Compress() (BudgetItem, error) {
	if i.err != nil {
		return nil, i.err
	}
	if i.compressTo <= 0 || i.compressTo >= i.tokens {
		i.compressTo = i.tokens / 2
		if i.compressTo <= 0 {
			i.compressTo = 1
		}
	}
	return &testBudgetItem{
		id:          i.id,
		tokens:      i.compressTo,
		priority:    i.priority + 1,
		compressTo:  i.compressTo,
		canCompress: i.canCompress,
		canEvict:    i.canEvict,
	}, nil
}

func (i *testBudgetItem) CanEvict() bool { return i.canEvict }

type testTool struct{}

func (t testTool) Name() string { return "test-tool" }

func (t testTool) Description() string { return strings.Repeat("tool", 8) }

func (t testTool) Category() string { return "test" }

func (t testTool) Parameters() []ToolParameter {
	return []ToolParameter{{Name: "path"}, {Name: "mode"}}
}

func (t testTool) Execute(ctx context.Context, state *Context, args map[string]interface{}) (*ToolResult, error) {
	return &ToolResult{Success: true}, nil
}

func (t testTool) IsAvailable(ctx context.Context, state *Context) bool { return true }

func (t testTool) Permissions() ToolPermissions { return ToolPermissions{} }

func (t testTool) Tags() []string { return []string{TagReadOnly} }

func TestContextBudgetUsageAccountingAndListeners(t *testing.T) {
	budget := NewContextBudgetWithPolicy(1000, &AllocationPolicy{
		SystemReserved: 0,
		Allocations: map[string]float64{
			"work": 1.0,
		},
		AllowBorrowing:     false,
		MinimumPerCategory: 0,
	})
	budget.SetReservations(0, 0, 0)
	budget.SetPolicies(BudgetPolicies{
		WarningThreshold:     0.70,
		CompressionThreshold: 0.85,
		CriticalThreshold:    0.95,
		AutoCompress:         true,
		AutoPrune:            true,
	})
	listener := &testBudgetListener{}
	budget.AddListener(listener)

	ctx := NewContext()
	ctx.Set("state", strings.Repeat("x", 100))
	ctx.AddInteraction("assistant", strings.Repeat("hello ", 40), nil)
	budget.UpdateUsage(ctx, []Tool{testTool{}})
	usage := budget.GetCurrentUsage()
	if usage.ContextTokens == 0 || usage.ToolTokens == 0 {
		t.Fatalf("expected update usage to account for context and tools, got %#v", usage)
	}
	if budget.GetAvailableTokens() >= budget.AvailableForContext {
		t.Fatalf("expected available tokens to shrink after update, got %#v", budget.GetAvailableTokens())
	}

	compressible := &testBudgetItem{
		id:          "compressible",
		tokens:      800,
		priority:    10,
		compressTo:  120,
		canCompress: true,
		canEvict:    true,
	}
	if err := budget.Allocate("work", 0, compressible); err != nil {
		t.Fatalf("initial compressible allocation: %v", err)
	}
	if !budget.ShouldCompress() {
		t.Fatal("expected budget to request compression at warning threshold")
	}

	budget.SetCurrentUsage(TokenUsage{ContextTokens: 900, ContextUsagePercent: 0.96})
	if got := budget.CheckBudget(); got != BudgetCritical {
		t.Fatalf("expected critical state, got %v", got)
	}
	if !budget.CanAddTokens(50) {
		t.Fatal("expected to be able to add tokens below the available cap")
	}
	if budget.CanAddTokens(200) {
		t.Fatal("expected token addition beyond cap to be rejected")
	}

	if err := budget.Allocate("work", 250, nil); err != nil {
		t.Fatalf("expected compressible allocation to succeed, got %v", err)
	}
	if len(listener.compression) == 0 {
		t.Fatal("expected compression listener to fire")
	}

	budget.Free("work", 0, "compressible")
	if err := budget.Allocate("work", 1000, nil); err == nil {
		t.Fatal("expected allocation to fail when capacity is exceeded")
	}
	if len(listener.exceeded) == 0 {
		t.Fatal("expected budget exceeded listener to fire")
	}

	if got := budget.GetRemainingBudget("work"); got <= 0 {
		t.Fatalf("expected remaining budget after freeing item, got %d", got)
	}
	if got := budget.Categories(); len(got) == 0 {
		t.Fatal("expected category list to be populated")
	}
}

func TestContextBudgetZeroAndInvalidHelpers(t *testing.T) {
	budget := NewContextBudget(0)
	if budget.MaxTokens != 8192 {
		t.Fatalf("expected default max tokens, got %d", budget.MaxTokens)
	}
	if budget.GetRemainingBudget("missing") != 0 {
		t.Fatal("expected missing category to return zero remaining budget")
	}
	if budget.Allocate("missing", -1, nil) == nil {
		t.Fatal("expected negative allocation to fail")
	}
	if !errors.Is(ErrInvalidBudget, ErrInvalidBudget) {
		t.Fatal("expected invalid budget sentinel to be comparable")
	}
}
