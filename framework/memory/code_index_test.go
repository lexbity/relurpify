package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCodeIndexBuildIndexHonorsPathFilter(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed.go")
	denied := filepath.Join(root, "secret.go")
	if err := os.WriteFile(allowed, []byte("package sample\nfunc Allowed() {}\n"), 0o644); err != nil {
		t.Fatalf("write allowed file: %v", err)
	}
	if err := os.WriteFile(denied, []byte("package sample\nfunc Secret() {}\n"), 0o644); err != nil {
		t.Fatalf("write denied file: %v", err)
	}

	index, err := NewCodeIndex(root, filepath.Join(root, "code_index.json"))
	if err != nil {
		t.Fatalf("new code index: %v", err)
	}
	index.SetPathFilter(func(path string, isDir bool) bool {
		if isDir {
			return true
		}
		return filepath.Base(path) != "secret.go"
	})

	if err := index.BuildIndex(context.Background()); err != nil {
		t.Fatalf("build index: %v", err)
	}

	files := index.ListFiles()
	if len(files) != 1 {
		t.Fatalf("expected only one indexed file, got %v", files)
	}
	if files[0] != "allowed.go" {
		t.Fatalf("expected allowed.go to be indexed, got %v", files)
	}
	if _, ok := index.GetFileMetadata("secret.go"); ok {
		t.Fatal("expected denied file to be excluded from the code index")
	}
	if results := index.SearchChunks("Secret", 10); len(results) != 0 {
		t.Fatalf("expected denied file chunks to be excluded from search, got %d results", len(results))
	}
}

func TestCodeIndexUpdateIncrementalHonorsPathFilterWithoutDeadlock(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed.go")
	denied := filepath.Join(root, "secret.go")
	if err := os.WriteFile(allowed, []byte("package sample\nfunc Allowed() {}\n"), 0o644); err != nil {
		t.Fatalf("write allowed file: %v", err)
	}
	if err := os.WriteFile(denied, []byte("package sample\nfunc Secret() {}\n"), 0o644); err != nil {
		t.Fatalf("write denied file: %v", err)
	}

	index, err := NewCodeIndex(root, filepath.Join(root, "code_index.json"))
	if err != nil {
		t.Fatalf("new code index: %v", err)
	}
	index.SetPathFilter(func(path string, isDir bool) bool {
		if isDir {
			return true
		}
		return filepath.Base(path) != "secret.go"
	})

	done := make(chan error, 1)
	go func() {
		done <- index.UpdateIncremental([]string{"allowed.go", "secret.go"})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("update incremental: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("update incremental timed out; possible deadlock")
	}

	files := index.ListFiles()
	if len(files) != 1 || files[0] != "allowed.go" {
		t.Fatalf("expected only allowed.go after incremental update, got %v", files)
	}
	if _, ok := index.GetFileMetadata("secret.go"); ok {
		t.Fatal("expected denied file to remain excluded after incremental update")
	}
}

func TestCodeIndexUpdateIncrementalRemovesDeletedFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc Hello() {}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	index, err := NewCodeIndex(root, filepath.Join(root, "code_index.json"))
	if err != nil {
		t.Fatalf("new code index: %v", err)
	}
	if err := index.BuildIndex(context.Background()); err != nil {
		t.Fatalf("build index: %v", err)
	}
	if _, ok := index.GetFileMetadata("sample.go"); !ok {
		t.Fatal("expected sample.go to be indexed before deletion")
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	if err := index.UpdateIncremental([]string{"sample.go"}); err != nil {
		t.Fatalf("update incremental: %v", err)
	}

	if _, ok := index.GetFileMetadata("sample.go"); ok {
		t.Fatal("expected deleted file to be removed from the index")
	}
	if results := index.SearchChunks("Hello", 10); len(results) != 0 {
		t.Fatalf("expected deleted file chunks to be removed from search, got %d results", len(results))
	}
}
