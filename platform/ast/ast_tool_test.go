package ast

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	frameworkast "codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type blockingParser struct {
	release <-chan struct{}
}

func (p *blockingParser) Parse(content string, path string) (*frameworkast.ParseResult, error) {
	<-p.release
	now := time.Now().UTC()
	fileID := frameworkast.GenerateFileID(path)
	root := &frameworkast.Node{
		ID:        fileID + ":root",
		FileID:    fileID,
		Type:      frameworkast.NodeTypePackage,
		Category:  frameworkast.CategoryCode,
		Language:  "go",
		Name:      "sample",
		StartLine: 1,
		EndLine:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	fn := &frameworkast.Node{
		ID:         fileID + ":func:Hello",
		FileID:     fileID,
		ParentID:   root.ID,
		Type:       frameworkast.NodeTypeFunction,
		Category:   frameworkast.CategoryCode,
		Language:   "go",
		Name:       "Hello",
		Signature:  "func Hello()",
		StartLine:  2,
		EndLine:    2,
		IsExported: true,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return &frameworkast.ParseResult{
		RootNode: root,
		Nodes:    []*frameworkast.Node{root, fn},
		Metadata: &frameworkast.FileMetadata{
			ID:            fileID,
			Path:          path,
			RelativePath:  filepath.Base(path),
			Language:      "go",
			Category:      frameworkast.CategoryCode,
			LineCount:     2,
			TokenCount:    len(content),
			ContentHash:   frameworkast.HashContent(content),
			RootNodeID:    root.ID,
			NodeCount:     2,
			IndexedAt:     now,
			ParserVersion: "blocking-test",
		},
	}, nil
}

func (p *blockingParser) ParseIncremental(_ *frameworkast.ParseResult, _ []frameworkast.ContentChange) (*frameworkast.ParseResult, error) {
	return nil, nil
}

func (p *blockingParser) Language() string                { return "go" }
func (p *blockingParser) Category() frameworkast.Category { return frameworkast.CategoryCode }
func (p *blockingParser) SupportsIncremental() bool       { return false }

func TestASTToolExecuteWaitsForAsyncIndexReadiness(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := frameworkast.NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	if err != nil {
		t.Fatalf("sqlite init failed: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	path := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc Hello() {}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	release := make(chan struct{})
	manager := frameworkast.NewIndexManager(store, frameworkast.IndexConfig{WorkspacePath: tmpDir, ParallelWorkers: 1})
	manager.RegisterParser(&blockingParser{release: release})
	if err := manager.StartIndexing(context.Background()); err != nil {
		t.Fatalf("start indexing: %v", err)
	}

	tool := NewASTTool(manager)
	done := make(chan error, 1)
	go func() {
		_, err := tool.Execute(context.Background(), core.NewContext(), map[string]any{
			"action": "get_signature",
			"symbol": "Hello",
		})
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("execute returned before index ready: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("execute failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for execute to finish")
	}
}

func TestASTToolExecuteReturnsBoundedWaitErrorWhenContextExpires(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := frameworkast.NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	if err != nil {
		t.Fatalf("sqlite init failed: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	path := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc Hello() {}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	release := make(chan struct{})
	manager := frameworkast.NewIndexManager(store, frameworkast.IndexConfig{WorkspacePath: tmpDir, ParallelWorkers: 1})
	manager.RegisterParser(&blockingParser{release: release})
	if err := manager.StartIndexing(context.Background()); err != nil {
		t.Fatalf("start indexing: %v", err)
	}
	defer close(release)

	tool := NewASTTool(manager)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = tool.Execute(ctx, core.NewContext(), map[string]any{
		"action": "get_signature",
		"symbol": "Hello",
	})
	if err == nil {
		t.Fatal("expected readiness wait error")
	}
}
