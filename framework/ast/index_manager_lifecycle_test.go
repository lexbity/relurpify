package ast

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

)

type blockingParser struct {
	release <-chan struct{}
}

func (p *blockingParser) Parse(content string, path string) (*ParseResult, error) {
	<-p.release
	now := time.Now().UTC()
	fileID := GenerateFileID(path)
	root := &Node{
		ID:        fileID + ":root",
		FileID:    fileID,
		Type:      NodeTypePackage,
		Category:  CategoryCode,
		Language:  "go",
		Name:      "sample",
		StartLine: 1,
		EndLine:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return &ParseResult{
		RootNode: root,
		Nodes:    []*Node{root},
		Edges:    nil,
		Metadata: &FileMetadata{
			ID:            fileID,
			Path:          path,
			RelativePath:  filepath.Base(path),
			Language:      "go",
			Category:      CategoryCode,
			LineCount:     1,
			TokenCount:    len(content),
			ContentHash:   HashContent(content),
			RootNodeID:    root.ID,
			NodeCount:     1,
			IndexedAt:     now,
			ParserVersion: "blocking-test",
		},
	}, nil
}

func (p *blockingParser) ParseIncremental(_ *ParseResult, _ []ContentChange) (*ParseResult, error) {
	return nil, nil
}

func (p *blockingParser) Language() string          { return "go" }
func (p *blockingParser) Category() Category        { return CategoryCode }
func (p *blockingParser) SupportsIncremental() bool { return false }

func newTestIndexManager(t *testing.T) (*IndexManager, string) {
	t.Helper()
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	if err != nil {
		t.Fatalf("sqlite init failed: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir, ParallelWorkers: 1})
	return manager, tmpDir
}

func TestIndexManagerWaitUntilReadyBlocksUntilAsyncIndexCompletes(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	release := make(chan struct{})
	manager.RegisterParser(&blockingParser{release: release})

	if err := manager.StartIndexing(context.Background()); err != nil {
		t.Fatalf("start indexing: %v", err)
	}
	if manager.Ready() {
		t.Fatal("manager should not report ready while indexing is blocked")
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- manager.WaitUntilReady(context.Background())
	}()

	select {
	case err := <-waitDone:
		t.Fatalf("wait returned before indexing completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("wait failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async indexing to complete")
	}

	if !manager.Ready() {
		t.Fatal("manager should report ready after async indexing completes")
	}
	meta, err := manager.Store().GetFileByPath(path)
	if err != nil {
		t.Fatalf("fetch indexed file: %v", err)
	}
	if meta == nil {
		t.Fatal("expected file metadata after indexing")
	}
}

func TestIndexManagerWaitUntilReadyReturnsCancellationError(t *testing.T) {
	manager, _ := newTestIndexManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := manager.StartIndexing(ctx); err != nil {
		t.Fatalf("start indexing: %v", err)
	}
	err := manager.WaitUntilReady(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if manager.Ready() {
		t.Fatal("manager should not report ready after canceled indexing")
	}
}

func TestIndexManagerCloseWaitsForAsyncIndexToFinish(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	if err != nil {
		t.Fatalf("sqlite init failed: %v", err)
	}

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir, ParallelWorkers: 1})
	path := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	release := make(chan struct{})
	manager.RegisterParser(&blockingParser{release: release})

	if err := manager.StartIndexing(context.Background()); err != nil {
		t.Fatalf("start indexing: %v", err)
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- manager.Close()
	}()

	select {
	case err := <-closeDone:
		t.Fatalf("close returned before indexing completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("close failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for close to finish")
	}
}

func TestIndexManagerIndexWorkspaceMarksReady(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := manager.IndexWorkspace(); err != nil {
		t.Fatalf("index workspace: %v", err)
	}
	if !manager.Ready() {
		t.Fatal("manager should report ready after synchronous indexing")
	}
	if err := manager.WaitUntilReady(context.Background()); err != nil {
		t.Fatalf("wait after sync indexing: %v", err)
	}
	meta, err := manager.Store().GetFileByPath(path)
	if err != nil {
		t.Fatalf("fetch indexed file: %v", err)
	}
	if meta == nil {
		t.Fatal("expected indexed file metadata")
	}
}

func TestIndexManagerIndexWorkspaceParallelReturnsWithoutDeadlockOnErrors(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	if err != nil {
		t.Fatalf("sqlite init failed: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir, ParallelWorkers: 2})
	for i := 0; i < 8; i++ {
		path := filepath.Join(tmpDir, "bad", fmt.Sprintf("script_%d.py", i))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("print('hi')\n"), 0o644); err != nil {
			t.Fatalf("write file %d: %v", i, err)
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- manager.IndexWorkspaceContext(context.Background())
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected indexing error for unsupported files")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for parallel indexing to finish")
	}
}

func TestIndexManagerRefreshFilesRemovesDeletedFiles(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc Hello() {}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := manager.IndexFile(path); err != nil {
		t.Fatalf("index file: %v", err)
	}
	meta, err := manager.Store().GetFileByPath(path)
	if err != nil || meta == nil {
		t.Fatalf("expected indexed file before delete, err=%v", err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	if err := manager.RefreshFiles([]string{path}); err != nil {
		t.Fatalf("refresh files: %v", err)
	}

	meta, err = manager.Store().GetFileByPath(path)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get file by path: %v", err)
	}
	if meta != nil {
		t.Fatal("expected deleted file to be removed from AST index")
	}
}
