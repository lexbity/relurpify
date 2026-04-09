package contextmgr

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestContextMgrHelpersAndExtractors(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	ctx := core.NewSharedContext(core.NewContext(), core.NewContextBudget(4096), nil)
	ctx.Set("x", strings.Repeat("a", 32))
	ctx.AddInteraction("user", "hello world", nil)

	if got := EstimateContextTokens(ctx); got <= 0 {
		t.Fatalf("expected token estimate to be positive, got %d", got)
	}
	if got := EstimateContextTokens(nil); got != 0 {
		t.Fatalf("expected nil context to estimate to zero, got %d", got)
	}
	if got := ApproximateTokens(strings.Repeat("x", 17)); got != 4 {
		t.Fatalf("unexpected approximate token count: %d", got)
	}
	if got := ApproximateTokens(""); got != 0 {
		t.Fatalf("expected empty content to estimate to zero, got %d", got)
	}
	if got := TrimLower("  MiXeD Case  "); got != "mixed case" {
		t.Fatalf("unexpected trim/lower result: %q", got)
	}
	if got := max(3, 7); got != 7 {
		t.Fatalf("unexpected max result: %d", got)
	}
	if got := DetailFull.String(); got != "full" {
		t.Fatalf("unexpected detail level string: %q", got)
	}
	if got := DetailLevel(99).String(); got != "unknown" {
		t.Fatalf("unexpected unknown detail level string: %q", got)
	}

	read, err := ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if read != "hello" {
		t.Fatalf("unexpected file content: %q", read)
	}

	refs := ExtractFileReferences("See ./pkg/foo.go, pkg/foo.go, ../bar/baz_test.go, and pkg/foo.go again.")
	if !reflect.DeepEqual(refs, []string{"pkg/foo.go", "../bar/baz_test.go"}) {
		t.Fatalf("unexpected file refs: %#v", refs)
	}
	symbols := ExtractSymbolReferences("makeThing(x); makeThing(y); loadData(z)")
	if !reflect.DeepEqual(symbols, []string{"makeThing", "loadData"}) {
		t.Fatalf("unexpected symbol refs: %#v", symbols)
	}
	keywords := ExtractKeywords(strings.Repeat("alpha ", 12))
	if len(strings.Fields(keywords)) != 10 {
		t.Fatalf("expected keywords to be truncated to 10 words, got %q", keywords)
	}
	if !ContainsInsensitive("Hello WORLD", "world") {
		t.Fatal("expected case-insensitive containment to match")
	}
	if countKeywords("refactor bug add implement", []string{"refactor", "bug", "add", "create"}) != 3 {
		t.Fatal("unexpected keyword count")
	}

	task := &core.Task{
		Context: map[string]any{
			"context_files": []any{" ./a.go ", "sub/c.go", "sub/c.go"},
			"workspace":     "/workspace/root",
		},
	}
	files := ExtractContextFiles(task)
	if !reflect.DeepEqual(files, []string{"a.go", "sub/c.go"}) {
		t.Fatalf("unexpected context files: %#v", files)
	}

	request := &ContextRequest{
		Files: []FileRequest{
			{Path: "relative.go", DetailLevel: DetailConcise},
			{Path: "/abs/keep.go", DetailLevel: DetailMinimal},
		},
	}
	ResolveContextRequestPaths(request, task)
	if got := request.Files[0].Path; got != "/workspace/root/relative.go" {
		t.Fatalf("unexpected resolved path: %q", got)
	}
	if got := request.Files[1].Path; got != "/abs/keep.go" {
		t.Fatalf("unexpected absolute path rewrite: %q", got)
	}
	AppendContextFiles(request, task, DetailSignatureOnly)
	if len(request.Files) != 4 {
		t.Fatalf("expected appended context files, got %d requests", len(request.Files))
	}
	if request.Files[2].Path != "a.go" || !request.Files[2].Pinned || request.Files[2].Priority != -1 {
		t.Fatalf("unexpected appended file request: %#v", request.Files[2])
	}
}

func TestContextMgrStrategiesAndPruning(t *testing.T) {
	task := &core.Task{
		Instruction: "refactor agents/architect/architect_agent.go and add tests for named/rex/route/route.go",
		Context: map[string]any{
			"context_files": []string{"framework/core/context.go"},
			"workspace":     "/workspace/root",
		},
	}
	budget := core.NewContextBudget(12000)
	shared := core.NewSharedContext(core.NewContext(), budget, nil)
	for i := 0; i < 12; i++ {
		shared.AddInteraction("assistant", "I am not sure about the best approach", nil)
	}

	aggressive := NewAggressiveStrategy()
	request, err := aggressive.SelectContext(task, budget)
	if err != nil {
		t.Fatalf("aggressive SelectContext: %v", err)
	}
	if request.MaxTokens != budget.AvailableForContext/4 {
		t.Fatalf("unexpected aggressive token budget: %d", request.MaxTokens)
	}
	if len(request.Files) == 0 || request.Files[0].DetailLevel != DetailSignatureOnly {
		t.Fatalf("expected aggressive strategy to prefer signature-only files, got %#v", request.Files)
	}
	if !aggressive.ShouldCompress(shared) {
		t.Fatal("expected aggressive strategy to compress with enough history")
	}
	if got := aggressive.DetermineDetailLevel("file.go", 0.95); got != DetailDetailed {
		t.Fatalf("unexpected aggressive detail level: %v", got)
	}
	if !aggressive.ShouldExpandContext(shared, &core.Result{Success: false, Data: map[string]any{"error_type": "insufficient_context"}}) {
		t.Fatal("expected aggressive strategy to expand on insufficient context errors")
	}

	conservative := NewConservativeStrategy()
	request, err = conservative.SelectContext(task, budget)
	if err != nil {
		t.Fatalf("conservative SelectContext: %v", err)
	}
	if request.MaxTokens != budget.AvailableForContext*3/4 {
		t.Fatalf("unexpected conservative token budget: %d", request.MaxTokens)
	}
	if len(request.MemoryQueries) == 0 || len(request.SearchQueries) == 0 {
		t.Fatalf("expected conservative strategy to prefer memory, file loading, and search, got %#v", request)
	}
	if got := request.SearchQueries[0]; got.Text != task.Instruction || got.MaxResults != 20 {
		t.Fatalf("unexpected conservative search query: %#v", got)
	}
	if got := conservative.DetermineDetailLevel("file.go", 0.9); got != DetailFull {
		t.Fatalf("unexpected conservative detail level: %v", got)
	}
	if !conservative.ShouldExpandContext(shared, &core.Result{Success: true, Data: map[string]any{"tool_used": "search"}}) {
		t.Fatal("expected conservative strategy to expand after search")
	}

	adaptive := NewAdaptiveStrategy()
	if _, err := adaptive.SelectContext(nil, budget); err == nil {
		t.Fatal("expected nil task to be rejected")
	}
	if _, err := adaptive.SelectContext(task, nil); err == nil {
		t.Fatal("expected nil budget to be rejected")
	}
	adaptive.contextLoadHistory = []ContextLoadEvent{{Success: true}, {Success: true}, {Success: true}, {Success: true}}
	adaptive.currentMode = ModeBalanced
	if got := adaptive.ShouldCompress(shared); !got {
		t.Fatal("expected adaptive strategy to compress above balanced threshold")
	}
	if got := adaptive.DetermineDetailLevel("file.go", 0.7); got != DetailDetailed {
		t.Fatalf("unexpected adaptive balanced detail level: %v", got)
	}
	if adaptive.ShouldExpandContext(shared, nil) {
		t.Fatal("expected nil last result to avoid expansion")
	}
	if !adaptive.ShouldExpandContext(shared, &core.Result{Success: false}) {
		t.Fatal("expected failed result to trigger expansion")
	}
	if !adaptive.ShouldExpandContext(shared, &core.Result{Success: true, Data: map[string]any{"llm_output": "we need more information"}}) {
		t.Fatal("expected uncertainty markers to trigger expansion")
	}
	adaptive.currentMode = ModeAggressive
	if got := adaptive.DetermineDetailLevel("file.go", 0.95); got != DetailDetailed {
		t.Fatalf("unexpected adaptive aggressive detail level: %v", got)
	}
	adaptive.currentMode = ModeConservative
	if got := adaptive.DetermineDetailLevel("file.go", 0.55); got != DetailDetailed {
		t.Fatalf("unexpected adaptive conservative detail level: %v", got)
	}

	items := []core.ContextItem{
		&testContextItem{id: "low", tokens: 20, relevance: 0.1, priority: 1, age: time.Hour, itemType: core.ContextTypeObservation},
		&testContextItem{id: "high", tokens: 20, relevance: 0.9, priority: 8, age: 10 * time.Minute, itemType: core.ContextTypeObservation},
		&testContextItem{id: "zero", tokens: 20, relevance: 0.05, priority: 0, age: 5 * time.Hour, itemType: core.ContextTypeObservation},
	}
	aggressiveOrder := aggressive.PrioritizeContext(items)
	if aggressiveOrder[0].Age() > aggressiveOrder[1].Age() {
		t.Fatal("expected aggressive ordering to favor recency")
	}
	conservativeOrder := conservative.PrioritizeContext(items)
	if conservativeOrder[0].RelevanceScore() < conservativeOrder[1].RelevanceScore() {
		t.Fatal("expected conservative ordering to favor relevance")
	}
	adaptiveOrder := adaptive.PrioritizeContext(items)
	if len(adaptiveOrder) != len(items) {
		t.Fatal("expected adaptive ordering to preserve cardinality")
	}
}

func TestContextMgrPruningStrategies(t *testing.T) {
	items := []ContextItem{
		&testContextItem{id: "keep", tokens: 40, relevance: 0.9, priority: 0, age: time.Hour, itemType: core.ContextTypeObservation},
		&testContextItem{id: "prune", tokens: 30, relevance: 0.05, priority: 1, age: 4 * time.Hour, itemType: core.ContextTypeObservation},
		&testContextItem{id: "compress", tokens: 50, relevance: 0.2, priority: 2, age: 5 * time.Hour, itemType: core.ContextTypeObservation},
	}

	relevance := NewRelevanceBasedStrategy()
	pruned := relevance.SelectForPruning(items, 20)
	if len(pruned) != 1 || pruned[0].Priority() == 0 {
		t.Fatalf("expected low-relevance, non-priority-zero item to be selected for pruning, got %#v", pruned)
	}
	compressed := relevance.SelectForCompression(items, 20)
	if len(compressed) == 0 {
		t.Fatal("expected relevance-based compression to select items")
	}

	lru := &LRUStrategy{}
	lruPruned := lru.SelectForPruning(items, 20)
	if len(lruPruned) == 0 || lruPruned[0].Age() < lruPruned[len(lruPruned)-1].Age() {
		t.Fatalf("expected LRU pruning to start with oldest items, got %#v", lruPruned)
	}
	if got := lru.SelectForCompression(items, 20); len(got) == 0 {
		t.Fatal("expected LRU compression to select items")
	}

	hybrid := NewHybridStrategy()
	hybridPruned := hybrid.SelectForPruning(items, 20)
	if len(hybridPruned) == 0 {
		t.Fatal("expected hybrid strategy to select items")
	}
	if got := hybrid.SelectForCompression(items, 20); len(got) == 0 {
		t.Fatal("expected hybrid compression to select items")
	}
}
