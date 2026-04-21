package framework_test

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestContextBudgetUpdateUsage(t *testing.T) {
	ctx := core.NewContext()
	ctx.AddInteraction("user", "hello world", nil)
	budget := core.NewContextBudget(8000)
	budget.UpdateUsage(ctx, nil)
	usage := budget.GetCurrentUsage()
	if usage.ContextTokens == 0 {
		t.Fatal("expected context tokens to be tracked")
	}
	if usage.TotalTokens == 0 {
		t.Fatal("expected total tokens to be computed")
	}
}

func TestContextBudgetReservations(t *testing.T) {
	budget := core.NewContextBudget(4000)
	budget.SetReservations(500, 500, 500)
	if budget.AvailableForContext != 2500 {
		t.Fatalf("expected available context 2500, got %d", budget.AvailableForContext)
	}
}

func TestContextBudgetStates(t *testing.T) {
	budget := core.NewContextBudget(1000)
	usage := budget.GetCurrentUsage()
	usage.ContextUsagePercent = 0.95
	budget.SetCurrentUsage(usage)
	if budget.CheckBudget() != core.BudgetCritical {
		t.Fatal("expected critical budget state")
	}
	usage = budget.GetCurrentUsage()
	usage.ContextUsagePercent = 0.5
	budget.SetCurrentUsage(usage)
	if budget.CheckBudget() != core.BudgetOK {
		t.Fatal("expected OK budget state")
	}
}
