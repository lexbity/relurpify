package contextmgr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkmemory "github.com/lexcodex/relurpify/framework/memory"
	frameworksearch "github.com/lexcodex/relurpify/framework/search"
)

type stubContextStrategy struct {
	request *ContextRequest
}

type stubMemoryStore struct {
	results []frameworkmemory.MemoryRecord
	err     error
}

type blockingASTParser struct {
	release <-chan struct{}
}

func (s *stubMemoryStore) Remember(context.Context, string, map[string]interface{}, frameworkmemory.MemoryScope) error {
	return nil
}

func (s *stubMemoryStore) Recall(context.Context, string, frameworkmemory.MemoryScope) (*frameworkmemory.MemoryRecord, bool, error) {
	return nil, false, nil
}

func (s *stubMemoryStore) Search(context.Context, string, frameworkmemory.MemoryScope) ([]frameworkmemory.MemoryRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]frameworkmemory.MemoryRecord(nil), s.results...), nil
}

func (s *stubMemoryStore) Forget(context.Context, string, frameworkmemory.MemoryScope) error {
	return nil
}

func (s *stubMemoryStore) Summarize(context.Context, frameworkmemory.MemoryScope) (string, error) {
	return "", nil
}

func (p *blockingASTParser) Parse(content string, path string) (*ast.ParseResult, error) {
	<-p.release
	now := time.Now().UTC()
	fileID := ast.GenerateFileID(path)
	root := &ast.Node{
		ID:        fileID + ":root",
		FileID:    fileID,
		Type:      ast.NodeTypePackage,
		Category:  ast.CategoryCode,
		Language:  "go",
		Name:      "sample",
		StartLine: 1,
		EndLine:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	fn := &ast.Node{
		ID:         fileID + ":fn",
		ParentID:   root.ID,
		FileID:     fileID,
		Type:       ast.NodeTypeFunction,
		Category:   ast.CategoryCode,
		Language:   "go",
		Name:       "Hello",
		Signature:  "func Hello()",
		IsExported: true,
		StartLine:  2,
		EndLine:    2,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return &ast.ParseResult{
		RootNode: root,
		Nodes:    []*ast.Node{root, fn},
		Edges: []*ast.Edge{{
			ID:       fileID + ":contains",
			SourceID: root.ID,
			TargetID: fn.ID,
			Type:     ast.EdgeTypeContains,
		}},
		Metadata: &ast.FileMetadata{
			ID:            fileID,
			Path:          path,
			RelativePath:  filepath.Base(path),
			Language:      "go",
			Category:      ast.CategoryCode,
			LineCount:     2,
			TokenCount:    len(content),
			ContentHash:   ast.HashContent(content),
			RootNodeID:    root.ID,
			NodeCount:     2,
			EdgeCount:     1,
			IndexedAt:     now,
			ParserVersion: "blocking-context-test",
		},
	}, nil
}

func (p *blockingASTParser) ParseIncremental(_ *ast.ParseResult, _ []ast.ContentChange) (*ast.ParseResult, error) {
	return nil, nil
}

func (p *blockingASTParser) Language() string          { return "go" }
func (p *blockingASTParser) Category() ast.Category    { return ast.CategoryCode }
func (p *blockingASTParser) SupportsIncremental() bool { return false }

func (s stubContextStrategy) SelectContext(task *core.Task, budget *core.ContextBudget) (*ContextRequest, error) {
	return s.request, nil
}

func (s stubContextStrategy) ShouldCompress(ctx *core.SharedContext) bool { return false }

func (s stubContextStrategy) DetermineDetailLevel(file string, relevance float64) DetailLevel {
	return DetailFull
}

func (s stubContextStrategy) ShouldExpandContext(ctx *core.SharedContext, lastResult *core.Result) bool {
	return false
}

func (s stubContextStrategy) PrioritizeContext(items []core.ContextItem) []core.ContextItem {
	return items
}

func TestProgressiveLoaderPromotesFileDetail(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	content := strings.Repeat("package sample\nfunc Example() string { return \"value\" }\n", 40)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	budget := core.NewContextBudget(16000)
	cm := NewContextManager(budget)
	loader := NewProgressiveLoader(cm, nil, nil, nil, budget, &core.SimpleSummarizer{})

	if err := loader.loadFile(FileRequest{Path: path, DetailLevel: DetailMinimal, Priority: 2}); err != nil {
		t.Fatalf("initial load: %v", err)
	}
	item := loader.fileItem(path)
	if item == nil {
		t.Fatal("expected file item after initial load")
	}
	detailedTokens := item.TokenCount()
	if got := loader.loadedFiles[path]; got != DetailMinimal {
		t.Fatalf("expected minimal level, got %v", got)
	}

	if err := loader.ExpandContext(path, DetailFull); err != nil {
		t.Fatalf("promote to full: %v", err)
	}
	item = loader.fileItem(path)
	if item == nil {
		t.Fatal("expected file item after promotion")
	}
	if got := loader.loadedFiles[path]; got != DetailFull {
		t.Fatalf("expected full level, got %v", got)
	}
	if item.TokenCount() <= detailedTokens {
		t.Fatalf("expected full detail to increase tokens, got detailed=%d full=%d", detailedTokens, item.TokenCount())
	}
	if len(cm.FileItems()) != 1 {
		t.Fatalf("expected upserted file item, got %d items", len(cm.FileItems()))
	}
}

func TestProgressiveLoaderDemotesFileDetailWithoutDuplicates(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.go")
	pathB := filepath.Join(dir, "b.go")
	content := strings.Repeat("package sample\nfunc Example() string { return \"value\" }\n", 80)
	for _, path := range []string{pathA, pathB} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write file %s: %v", path, err)
		}
	}

	budget := core.NewContextBudget(20000)
	cm := NewContextManager(budget)
	loader := NewProgressiveLoader(cm, nil, nil, nil, budget, &core.SimpleSummarizer{})

	for _, path := range []string{pathA, pathB} {
		if err := loader.loadFile(FileRequest{Path: path, DetailLevel: DetailFull, Priority: 2}); err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
	}
	beforeA := loader.fileItem(pathA).TokenCount()
	beforeB := loader.fileItem(pathB).TokenCount()

	freed, err := loader.DemoteToFree(1, nil)
	if err != nil {
		t.Fatalf("demote to free: %v", err)
	}
	if freed <= 0 {
		t.Fatalf("expected freed tokens, got %d", freed)
	}
	if len(cm.FileItems()) != 2 {
		t.Fatalf("expected two file items after demotion, got %d", len(cm.FileItems()))
	}
	afterA := loader.fileItem(pathA).TokenCount()
	afterB := loader.fileItem(pathB).TokenCount()
	if afterA >= beforeA && afterB >= beforeB {
		t.Fatalf("expected at least one file to shrink after demotion, before=(%d,%d) after=(%d,%d)", beforeA, beforeB, afterA, afterB)
	}
}

func TestProgressiveLoaderPreservesProtectedFilesDuringDemotion(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	pathA := filepath.Join(dir, "protected.go")
	pathB := filepath.Join(dir, "other.go")
	content := strings.Repeat("package sample\nfunc Example() string { return \"value\" }\n", 80)
	for _, path := range []string{pathA, pathB} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write file %s: %v", path, err)
		}
	}

	budget := core.NewContextBudget(20000)
	cm := NewContextManager(budget)
	loader := NewProgressiveLoader(cm, nil, nil, nil, budget, &core.SimpleSummarizer{})

	for _, path := range []string{pathA, pathB} {
		if err := loader.loadFile(FileRequest{Path: path, DetailLevel: DetailFull, Priority: 2}); err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
	}
	beforeProtected := loader.fileItem(pathA).TokenCount()
	beforeOther := loader.fileItem(pathB).TokenCount()

	freed, err := loader.DemoteToFree(1, map[string]struct{}{pathA: {}})
	if err != nil {
		t.Fatalf("demote to free with protected set: %v", err)
	}
	if freed <= 0 {
		t.Fatalf("expected freed tokens, got %d", freed)
	}
	afterProtected := loader.fileItem(pathA).TokenCount()
	afterOther := loader.fileItem(pathB).TokenCount()
	if afterProtected != beforeProtected {
		t.Fatalf("expected protected file to remain unchanged, before=%d after=%d", beforeProtected, afterProtected)
	}
	if afterOther >= beforeOther {
		t.Fatalf("expected unprotected file to shrink, before=%d after=%d", beforeOther, afterOther)
	}
}

func TestResolveContextRequestPathsUsesWorkspaceRoot(t *testing.T) {
	request := &ContextRequest{
		Files: []FileRequest{
			{Path: "testsuite/fixture/a.go"},
			{Path: "/tmp/already-absolute.go"},
		},
	}
	task := &core.Task{
		Context: map[string]any{
			"workspace": "/tmp/workspace-root",
		},
	}

	ResolveContextRequestPaths(request, task)

	if got := request.Files[0].Path; got != filepath.Join("/tmp/workspace-root", "testsuite/fixture/a.go") {
		t.Fatalf("expected workspace-relative path, got %q", got)
	}
	if got := request.Files[1].Path; got != "/tmp/already-absolute.go" {
		t.Fatalf("expected absolute path unchanged, got %q", got)
	}
}

func TestProgressiveLoaderInitialLoadResolvesRelativePathsFromWorkspace(t *testing.T) {
	t.Helper()
	liveRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(liveRoot, "testsuite/fixture"), 0o755); err != nil {
		t.Fatalf("mkdir live fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(liveRoot, "testsuite/fixture/sample.txt"), []byte("live repo"), 0o644); err != nil {
		t.Fatalf("write live fixture: %v", err)
	}

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "testsuite/fixture"), 0o755); err != nil {
		t.Fatalf("mkdir workspace fixture: %v", err)
	}
	want := "derived workspace"
	if err := os.WriteFile(filepath.Join(workspace, "testsuite/fixture/sample.txt"), []byte(want), 0o644); err != nil {
		t.Fatalf("write workspace fixture: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()
	if err := os.Chdir(liveRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	budget := core.NewContextBudget(8000)
	cm := NewContextManager(budget)
	loader := NewProgressiveLoader(cm, nil, nil, nil, budget, &core.SimpleSummarizer{})
	task := &core.Task{
		Instruction: "Inspect testsuite/fixture/sample.txt",
		Context: map[string]any{
			"workspace": workspace,
		},
	}
	request := &ContextRequest{
		Files: []FileRequest{
			{Path: "testsuite/fixture/sample.txt", DetailLevel: DetailFull},
		},
	}

	if err := loader.InitialLoad(task, stubContextStrategy{request: request}); err != nil {
		t.Fatalf("initial load: %v", err)
	}

	item := loader.fileItem(filepath.Join(workspace, "testsuite/fixture/sample.txt"))
	if item == nil {
		t.Fatal("expected workspace file item to be loaded")
	}
	if !strings.Contains(item.Content, want) {
		t.Fatalf("expected workspace content %q, got %q", want, item.Content)
	}
	if strings.Contains(item.Content, "live repo") {
		t.Fatalf("loaded content from cwd instead of workspace: %q", item.Content)
	}
}

func TestProgressiveLoaderExecuteASTQueryWaitsForIndexReadiness(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	store, err := ast.NewSQLiteStore(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatalf("sqlite init failed: %v", err)
	}
	defer store.Close()

	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc Hello() {}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: dir, ParallelWorkers: 1})
	release := make(chan struct{})
	manager.RegisterParser(&blockingASTParser{release: release})
	if err := manager.StartIndexing(context.Background()); err != nil {
		t.Fatalf("start indexing: %v", err)
	}

	budget := core.NewContextBudget(8000)
	cm := NewContextManager(budget)
	loader := NewProgressiveLoader(cm, manager, nil, nil, budget, &core.SimpleSummarizer{})

	done := make(chan error, 1)
	go func() {
		done <- loader.ExecuteContextRequest(&ContextRequest{
			ASTQueries: []ASTQuery{{
				Type: ASTQueryListSymbols,
				Filter: ASTFilter{
					Types: []ast.NodeType{ast.NodeTypeFunction},
				},
			}},
		}, "initial")
	}()

	select {
	case err := <-done:
		t.Fatalf("ast query returned before index was ready: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("execute request failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for execute request to finish")
	}

	items := cm.GetItemsByType(core.ContextTypeToolResult)
	if len(items) != 1 {
		t.Fatalf("expected one AST tool result item, got %d", len(items))
	}
	resultItem, ok := items[0].(*core.ToolResultContextItem)
	if !ok {
		t.Fatalf("expected tool result context item, got %T", items[0])
	}
	if resultItem.Result == nil || !resultItem.Result.Success {
		t.Fatalf("expected successful AST result, got %#v", resultItem.Result)
	}
}

func TestProgressiveLoaderExecuteSearchQueryLoadsMatchingFiles(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "match.go")
	content := "package sample\nfunc Hello() string { return \"needle\" }\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	codeIndex, err := frameworkmemory.NewCodeIndex(dir, filepath.Join(dir, "code_index.json"))
	if err != nil {
		t.Fatalf("new code index: %v", err)
	}
	if err := codeIndex.BuildIndex(context.Background()); err != nil {
		t.Fatalf("build code index: %v", err)
	}
	searchEngine := frameworksearch.NewSearchEngine(nil, codeIndex)

	budget := core.NewContextBudget(8000)
	cm := NewContextManager(budget)
	loader := NewProgressiveLoader(cm, nil, searchEngine, nil, budget, &core.SimpleSummarizer{})

	if err := loader.ExecuteContextRequest(&ContextRequest{
		SearchQueries: []SearchQuery{{
			Text:       "needle",
			Mode:       frameworksearch.SearchKeyword,
			MaxResults: 1,
		}},
	}, "initial"); err != nil {
		t.Fatalf("execute search query: %v", err)
	}

	item := loader.fileItem(path)
	if item == nil {
		t.Fatal("expected matching file to be loaded into context")
	}
	if !strings.Contains(item.Content, "needle") {
		t.Fatalf("expected loaded file content to include search match, got %q", item.Content)
	}
}

func TestProgressiveLoaderExecuteMemoryQueryAddsMemoryContextItem(t *testing.T) {
	t.Helper()
	budget := core.NewContextBudget(8000)
	cm := NewContextManager(budget)
	loader := NewProgressiveLoader(cm, nil, nil, &stubMemoryStore{
		results: []frameworkmemory.MemoryRecord{
			{Key: "incident", Scope: frameworkmemory.MemoryScopeProject, Value: map[string]interface{}{"summary": "rollback failure"}},
			{Key: "playbook", Scope: frameworkmemory.MemoryScopeProject, Value: map[string]interface{}{"summary": "recovery steps"}},
		},
	}, budget, &core.SimpleSummarizer{})

	if err := loader.ExecuteContextRequest(&ContextRequest{
		MemoryQueries: []MemoryQuery{{
			Scope:      frameworkmemory.MemoryScopeProject,
			Query:      "rollback",
			MaxResults: 1,
		}},
	}, "initial"); err != nil {
		t.Fatalf("execute memory query: %v", err)
	}

	items := cm.GetItemsByType(core.ContextTypeMemory)
	if len(items) != 1 {
		t.Fatalf("expected one memory item, got %d", len(items))
	}
	item, ok := items[0].(*core.MemoryContextItem)
	if !ok {
		t.Fatalf("expected memory context item, got %T", items[0])
	}
	if !strings.Contains(item.Content, "Relevant agent memories:") {
		t.Fatalf("expected memory heading, got %q", item.Content)
	}
	if !strings.Contains(item.Content, "incident") {
		t.Fatalf("expected top result to be included, got %q", item.Content)
	}
	if strings.Contains(item.Content, "playbook") {
		t.Fatalf("expected MaxResults limit to exclude second result, got %q", item.Content)
	}
}

func TestProgressiveLoaderExecuteMemoryQueryNoopsWithoutStore(t *testing.T) {
	t.Helper()
	budget := core.NewContextBudget(8000)
	cm := NewContextManager(budget)
	loader := NewProgressiveLoader(cm, nil, nil, nil, budget, &core.SimpleSummarizer{})

	if err := loader.ExecuteContextRequest(&ContextRequest{
		MemoryQueries: []MemoryQuery{{
			Scope: frameworkmemory.MemoryScopeProject,
			Query: "rollback",
		}},
	}, "initial"); err != nil {
		t.Fatalf("execute memory query without store: %v", err)
	}

	items := cm.GetItemsByType(core.ContextTypeMemory)
	if len(items) != 0 {
		t.Fatalf("expected no memory items, got %d", len(items))
	}
}
