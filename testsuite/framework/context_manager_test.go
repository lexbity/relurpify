package framework_test

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextmgr"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/perfstats"
)

type fakeContextItem struct {
	tokens    int
	relevance float64
	priority  int
	age       time.Duration
}

func (f *fakeContextItem) TokenCount() int         { return f.tokens }
func (f *fakeContextItem) RelevanceScore() float64 { return f.relevance }
func (f *fakeContextItem) Priority() int           { return f.priority }
func (f *fakeContextItem) Compress() (core.ContextItem, error) {
	return &fakeContextItem{
		tokens:    f.tokens / 2,
		relevance: f.relevance * 0.8,
		priority:  f.priority + 1,
		age:       f.age,
	}, nil
}
func (f *fakeContextItem) Type() core.ContextItemType { return core.ContextTypeMemory }
func (f *fakeContextItem) Age() time.Duration         { return f.age }

func TestContextManagerAddItem(t *testing.T) {
	budget := core.NewContextBudget(8000)
	budget.SetReservations(500, 500, 500)
	manager := contextmgr.NewContextManager(budget)
	item := &fakeContextItem{tokens: 10, relevance: 1.0, priority: 1, age: time.Hour}
	if err := manager.AddItem(item); err != nil {
		t.Fatalf("AddItem returned error: %v", err)
	}
	if len(manager.GetItems()) != 1 {
		t.Fatal("expected item to be added")
	}
}

func TestContextManagerCompression(t *testing.T) {
	budget := core.NewContextBudget(1000)
	budget.SetReservations(0, 0, 0)
	manager := contextmgr.NewContextManager(budget)
	if err := manager.AddItem(&fakeContextItem{tokens: 100, relevance: 0.05, priority: 5, age: time.Hour}); err != nil {
		t.Fatalf("AddItem returned error: %v", err)
	}
	usage := budget.GetCurrentUsage()
	usage.ContextUsagePercent = 0.85
	budget.SetCurrentUsage(usage)
	if err := manager.MakeSpace(20); err != nil {
		t.Fatalf("expected MakeSpace to succeed via compression, got %v", err)
	}
}

func TestContextManagerPrune(t *testing.T) {
	budget := core.NewContextBudget(1000)
	budget.SetReservations(0, 0, 0)
	manager := contextmgr.NewContextManager(budget)
	if err := manager.AddItem(&fakeContextItem{tokens: 100, relevance: 0.0, priority: 5, age: 48 * time.Hour}); err != nil {
		t.Fatalf("AddItem returned error: %v", err)
	}
	if err := manager.AddItem(&fakeContextItem{tokens: 80, relevance: 0.0, priority: 6, age: 24 * time.Hour}); err != nil {
		t.Fatalf("AddItem returned error: %v", err)
	}
	usage := budget.GetCurrentUsage()
	usage.ContextUsagePercent = 0.95
	budget.SetCurrentUsage(usage)
	if err := manager.MakeSpace(50); err != nil {
		t.Fatalf("expected pruning to succeed, got %v", err)
	}
	stats := manager.GetStats()
	if stats.TotalItems == 0 {
		t.Fatal("expected some items remaining after pruning")
	}
}

func TestContextManagerCommonMutationsAvoidBudgetRescans(t *testing.T) {
	perfstats.Reset()
	budget := core.NewContextBudget(8000)
	budget.SetReservations(0, 0, 0)
	manager := contextmgr.NewContextManager(budget)

	if err := manager.AddItem(&fakeContextItem{tokens: 40, relevance: 1.0, priority: 1, age: time.Hour}); err != nil {
		t.Fatalf("AddItem returned error: %v", err)
	}
	if err := manager.UpsertFileItem(&core.FileContextItem{
		Path:         "a.go",
		Content:      "package sample\n",
		Summary:      "sample",
		LastAccessed: time.Now().UTC(),
		Relevance:    0.8,
		PriorityVal:  1,
	}); err != nil {
		t.Fatalf("UpsertFileItem returned error: %v", err)
	}

	stats := manager.GetStats()
	if stats.TotalItems != 2 {
		t.Fatalf("unexpected total items: %+v", stats)
	}
	if stats.TotalTokens == 0 || stats.ItemsByType[core.ContextTypeMemory] != 1 || stats.ItemsByType[core.ContextTypeFile] != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	snapshot := perfstats.Get()
	if snapshot.ContextBudgetRescanCount != 0 {
		t.Fatalf("expected no budget rescans for common mutations, got %+v", snapshot)
	}
}
