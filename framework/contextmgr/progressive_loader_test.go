package contextmgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

type stubContextStrategy struct {
	request *ContextRequest
}

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
	loader := NewProgressiveLoader(cm, nil, nil, budget, &core.SimpleSummarizer{})

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
	loader := NewProgressiveLoader(cm, nil, nil, budget, &core.SimpleSummarizer{})

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
	loader := NewProgressiveLoader(cm, nil, nil, budget, &core.SimpleSummarizer{})

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
	loader := NewProgressiveLoader(cm, nil, nil, budget, &core.SimpleSummarizer{})
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
