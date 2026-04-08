package contextmgr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

type testContextItem struct {
	id         string
	tokens     int
	priority   int
	relevance  float64
	age        time.Duration
	itemType   core.ContextItemType
	compressTo int
}

func (i *testContextItem) TokenCount() int { return i.tokens }

func (i *testContextItem) RelevanceScore() float64 { return i.relevance }

func (i *testContextItem) Priority() int { return i.priority }

func (i *testContextItem) Compress() (core.ContextItem, error) {
	tokens := i.compressTo
	if tokens <= 0 || tokens >= i.tokens {
		tokens = i.tokens / 2
		if tokens <= 0 {
			tokens = 1
		}
	}
	return &testContextItem{
		id:         i.id,
		tokens:     tokens,
		priority:   i.priority + 1,
		relevance:  i.relevance * 0.9,
		age:        i.age,
		itemType:   i.itemType,
		compressTo: i.compressTo,
	}, nil
}

func (i *testContextItem) Type() core.ContextItemType { return i.itemType }

func (i *testContextItem) Age() time.Duration { return i.age }

type testPruningStrategy struct {
	compress []ContextItem
	prune    []ContextItem
}

func (s *testPruningStrategy) SelectForPruning(items []ContextItem, targetTokens int) []ContextItem {
	return append([]ContextItem(nil), s.prune...)
}

func (s *testPruningStrategy) SelectForCompression(items []ContextItem, targetTokens int) []ContextItem {
	return append([]ContextItem(nil), s.compress...)
}

type testContextStrategy struct {
	expand bool
}

func (s *testContextStrategy) SelectContext(task *core.Task, budget *core.ContextBudget) (*ContextRequest, error) {
	return &ContextRequest{}, nil
}

func (s *testContextStrategy) ShouldCompress(ctx *core.SharedContext) bool { return false }

func (s *testContextStrategy) DetermineDetailLevel(file string, relevance float64) DetailLevel {
	return DetailDetailed
}

func (s *testContextStrategy) ShouldExpandContext(ctx *core.SharedContext, lastResult *core.Result) bool {
	return s.expand
}

func (s *testContextStrategy) PrioritizeContext(items []core.ContextItem) []core.ContextItem {
	return append([]core.ContextItem(nil), items...)
}

func TestContextPolicyTextAndPathHelpers(t *testing.T) {
	task := &core.Task{
		Context: map[string]any{
			"context_files": []any{" ./a.go ", "/workspace/root/existing.go", "sub/c.go", "sub/c.go", 42},
			"workspace":     "/workspace/root",
		},
	}

	refs := ExtractFileReferences("See ./pkg/foo.go and pkg/foo.go plus ../bar/baz_test.go.")
	expectedRefs := []string{"pkg/foo.go", "../bar/baz_test.go"}
	if !reflect.DeepEqual(refs, expectedRefs) {
		t.Fatalf("unexpected file refs: got %v want %v", refs, expectedRefs)
	}

	files := ExtractContextFiles(task)
	expectedFiles := []string{"a.go", "/workspace/root/existing.go", "sub/c.go"}
	if !reflect.DeepEqual(files, expectedFiles) {
		t.Fatalf("unexpected explicit files: got %v want %v", files, expectedFiles)
	}

	request := &ContextRequest{
		Files: []FileRequest{
			{Path: "/workspace/root/existing.go", DetailLevel: DetailFull},
			{Path: "relative.go", DetailLevel: DetailConcise},
			{Path: "/abs/keep.go", DetailLevel: DetailMinimal},
		},
	}
	ResolveContextRequestPaths(request, task)
	wantPaths := []string{"/workspace/root/existing.go", "/workspace/root/relative.go", "/abs/keep.go"}
	for i, req := range request.Files {
		if req.Path != wantPaths[i] {
			t.Fatalf("unexpected resolved path at %d: got %q want %q", i, req.Path, wantPaths[i])
		}
	}

	AppendContextFiles(request, task, DetailSignatureOnly)
	if len(request.Files) != 5 {
		t.Fatalf("expected 5 file requests after append, got %d", len(request.Files))
	}
	if request.Files[3].Path != "a.go" || !request.Files[3].Pinned || request.Files[3].Priority != -1 {
		t.Fatalf("unexpected appended file request: %#v", request.Files[3])
	}
	if request.Files[4].Path != "sub/c.go" || !request.Files[4].Pinned {
		t.Fatalf("unexpected appended file request: %#v", request.Files[4])
	}

	symbols := ExtractSymbolReferences("foo(bar); foo(bar); buildThing(x); _hidden(y)")
	expectedSymbols := []string{"foo", "buildThing", "_hidden"}
	if !reflect.DeepEqual(symbols, expectedSymbols) {
		t.Fatalf("unexpected symbols: got %v want %v", symbols, expectedSymbols)
	}

	keywords := ExtractKeywords(strings.Repeat("alpha ", 12))
	if len(strings.Fields(keywords)) != 10 {
		t.Fatalf("expected ExtractKeywords to cap at 10 words, got %q", keywords)
	}
	if !ContainsInsensitive("Hello WORLD", "world") {
		t.Fatal("expected case-insensitive containment check to match")
	}
	if stringValue(123) != "123" || stringValue(nil) != "" {
		t.Fatalf("unexpected stringValue behavior")
	}
}

func TestContextPolicyGraphMemoryHelpers(t *testing.T) {
	state := core.NewContext()
	state.Set("architect.current_step", core.PlanStep{
		Files: []string{"keep.go", "also_keep.go"},
	})
	state.Set("react.last_tool_result", map[string]interface{}{
		"path": "tool/path.go",
	})

	protected := protectedFileSet(state)
	for _, path := range []string{"keep.go", "also_keep.go", "tool/path.go"} {
		if _, ok := protected[path]; !ok {
			t.Fatalf("expected protected file set to include %q", path)
		}
	}

	if got := extractPathFromToolResult(map[string]interface{}{"path": " direct.go "}); got != "direct.go" {
		t.Fatalf("unexpected direct tool result path: %q", got)
	}
	if got := extractPathFromToolResult(map[string]interface{}{
		"payload": map[string]interface{}{
			"data": map[string]interface{}{"path": "nested.go"},
		},
	}); got != "nested.go" {
		t.Fatalf("unexpected nested tool result path: %q", got)
	}

	now := time.Now().UTC()
	item := memoryItemFromPublicationResult(map[string]any{
		"summary":   "design constraint",
		"text":      "full design constraint",
		"source":    "retrieval",
		"record_id": "record-1",
		"kind":      "document",
		"reference": map[string]any{
			"kind":    string(core.ContextReferenceRetrievalEvidence),
			"id":      "ref-1",
			"uri":     "memory://ref-1",
			"version": "v1",
			"detail":  "evidence",
		},
	}, core.ContextReferenceRuntimeMemory, now)
	if item == nil || item.Reference == nil || item.Reference.URI != "memory://ref-1" {
		t.Fatalf("unexpected memory publication item: %#v", item)
	}

	fallback := memoryItemsFromGraphPublication(state, "graph.declarative_memory_payload", "graph.procedural_memory_refs", core.ContextReferenceRuntimeMemory, now)
	if fallback != nil {
		t.Fatalf("expected no items when graph payload/ref keys are absent, got %#v", fallback)
	}

	state.Set("graph.declarative_memory_payload", map[string]any{
		"results": []map[string]any{
			{
				"summary": "doc summary",
				"reference": map[string]any{
					"kind": string(core.ContextReferenceRetrievalEvidence),
					"uri":  "memory://doc-1",
				},
			},
		},
	})
	items := graphMemoryContextItems(state)
	if len(items) != 1 || items[0].Reference == nil || items[0].Reference.URI != "memory://doc-1" {
		t.Fatalf("unexpected graph memory items: %#v", items)
	}

	key := graphMemoryItemKey(items[0])
	if key == "" {
		t.Fatal("expected graph memory item key")
	}
	keys := graphMemoryItemKeySet([]ContextItem{items[0], &testContextItem{id: "skip", tokens: 1, itemType: core.ContextTypeFile}})
	if len(keys) != 1 {
		t.Fatalf("expected one memory key in set, got %#v", keys)
	}
	if !graphMemoryItemExists([]ContextItem{items[0]}, items[0]) {
		t.Fatal("expected graph memory item existence check to match")
	}
	if graphMemoryItemExists([]ContextItem{items[0]}, &core.MemoryContextItem{Source: "other", Summary: "other"}) {
		t.Fatal("expected unmatched graph memory item to be absent")
	}
	if !graphMemoryItemExists(nil, nil) {
		t.Fatal("expected nil candidate to be treated as existing")
	}

	reference := publicationReference(map[string]any{
		"record_id": "pub-1",
		"kind":      "note",
	}, core.ContextReferenceRuntimeMemory)
	if reference == nil || reference.ID != "pub-1" || reference.Detail != "note" {
		t.Fatalf("unexpected publication reference: %#v", reference)
	}

	override := publicationReference(map[string]any{
		"reference": map[string]any{
			"kind":    string(core.ContextReferenceWorkflowArtifact),
			"id":      "workflow-1",
			"uri":     "artifact://workflow-1",
			"version": "v2",
			"detail":  "artifact",
		},
	}, core.ContextReferenceRuntimeMemory)
	if override == nil || override.Kind != core.ContextReferenceWorkflowArtifact || override.URI != "artifact://workflow-1" {
		t.Fatalf("unexpected overridden publication reference: %#v", override)
	}
}

func TestContextPolicyHandleSignalsAndDiagnose(t *testing.T) {
	dir := t.TempDir()
	expandPath := filepath.Join(dir, "expand.go")
	drillPath := filepath.Join(dir, "drill.go")
	if err := os.WriteFile(expandPath, []byte("package main\nfunc expandRef() {}\n"), 0o600); err != nil {
		t.Fatalf("write expand file: %v", err)
	}
	if err := os.WriteFile(drillPath, []byte("package main\nfunc drillRef() {}\n"), 0o600); err != nil {
		t.Fatalf("write drill file: %v", err)
	}

	manager := NewContextManager(core.NewContextBudget(8000))
	loader := NewProgressiveLoader(manager, nil, nil, nil, core.NewContextBudget(8000), nil)
	policy := &ContextPolicy{
		ContextManager:     manager,
		Progressive:        loader,
		Strategy:           &testContextStrategy{expand: true},
		ProgressiveEnabled: true,
	}

	state := core.NewContext()
	state.AddInteraction("assistant", "I am not sure about "+expandPath+" and buildThing()", nil)
	result := &core.Result{
		Data: map[string]any{
			"file": drillPath,
		},
	}
	shared := core.NewSharedContext(state, nil, nil)

	policy.HandleSignals(state, shared, result)

	if got := loader.loadedFiles[expandPath]; got != DetailDetailed {
		t.Fatalf("expected expansion path to load at detailed level, got %v", got)
	}
	if got := loader.loadedFiles[drillPath]; got != DetailFull {
		t.Fatalf("expected drill path to load at full level, got %v", got)
	}
	if len(loader.loadHistory) != 1 {
		t.Fatalf("expected one recorded context request for symbol lookup, got %d", len(loader.loadHistory))
	}
	if loader.loadHistory[0].ItemsLoaded != 0 || loader.loadHistory[0].Success {
		t.Fatalf("expected AST request to fail gracefully without an index, got %#v", loader.loadHistory[0])
	}

	diagnosed, err := policy.Diagnose(context.Background(), core.PlanStep{ID: "step-1"}, errors.New("boom"), func(ctx context.Context, step core.PlanStep, err error) (string, error) {
		if step.ID != "step-1" {
			t.Fatalf("unexpected step passed to diagnosis function: %#v", step)
		}
		return "diagnosed", nil
	})
	if err != nil || diagnosed != "diagnosed" {
		t.Fatalf("unexpected diagnosis result: %q, %v", diagnosed, err)
	}

	diagnosed, err = policy.Diagnose(context.Background(), core.PlanStep{}, nil, nil)
	if err != nil || diagnosed != "" {
		t.Fatalf("expected nil diagnosis function to return empty result, got %q, %v", diagnosed, err)
	}
}

func TestContextManagerMakeSpaceAndStats(t *testing.T) {
	budget := core.NewContextBudget(8000)
	cm := NewContextManager(budget)
	noteItem := &testContextItem{id: "note", tokens: 300, priority: 3, relevance: 0.9, age: time.Hour, itemType: core.ContextTypeObservation, compressTo: 100}
	if err := cm.AddItem(noteItem); err != nil {
		t.Fatalf("add item: %v", err)
	}
	cm.strategy = &testPruningStrategy{
		compress: []ContextItem{noteItem},
	}
	file := &core.FileContextItem{
		Path:         "pkg/file.go",
		Content:      strings.Repeat("x", 200),
		Relevance:    0.7,
		PriorityVal:  2,
		LastAccessed: time.Now().Add(-time.Hour),
	}
	if err := cm.UpsertFileItem(file); err != nil {
		t.Fatalf("upsert file: %v", err)
	}

	stats := cm.GetStats()
	if stats.TotalItems != 2 || stats.ItemsByType[core.ContextTypeObservation] != 1 || stats.ItemsByType[core.ContextTypeFile] != 1 {
		t.Fatalf("unexpected initial stats: %#v", stats)
	}
	if len(cm.FileItems()) != 1 {
		t.Fatalf("expected one tracked file, got %d", len(cm.FileItems()))
	}

	budget.SetCurrentUsage(core.TokenUsage{ContextTokens: 4500, ContextUsagePercent: 0.9})
	if err := cm.makeSpaceLocked(100); err != nil {
		t.Fatalf("expected compression to make space, got %v", err)
	}
	observations := cm.GetItemsByType(core.ContextTypeObservation)
	if len(observations) != 1 || observations[0].TokenCount() >= 300 {
		t.Fatalf("expected compressed observation item to shrink, got %#v", observations)
	}

	replacement := &core.FileContextItem{
		Path:         "pkg/file.go",
		Content:      strings.Repeat("y", 300),
		Relevance:    0.8,
		PriorityVal:  4,
		LastAccessed: time.Now(),
	}
	if err := cm.UpsertFileItem(replacement); err != nil {
		t.Fatalf("upsert replacement file: %v", err)
	}
	fileItems := cm.FileItems()
	if len(fileItems) != 1 || fileItems[0].Content != replacement.Content {
		t.Fatalf("expected file item replacement to take effect: %#v", fileItems)
	}

	cm.strategy = &testPruningStrategy{
		prune: []ContextItem{cm.GetItemsByType(core.ContextTypeObservation)[0]},
	}
	budget.SetCurrentUsage(core.TokenUsage{ContextTokens: 4700, ContextUsagePercent: 0.98})
	if err := cm.makeSpaceLocked(100); err != nil {
		t.Fatalf("expected pruning to make space under critical budget, got %v", err)
	}
	if len(cm.GetItems()) != 1 {
		t.Fatalf("expected pruning to remove one item, got %d", len(cm.GetItems()))
	}

	cm.Clear()
	cleared := cm.GetStats()
	if cleared.TotalItems != 0 || cleared.TotalTokens != 0 || len(cleared.ItemsByType) != 0 {
		t.Fatalf("expected clear to reset stats, got %#v", cleared)
	}
}
