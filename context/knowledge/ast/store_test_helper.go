package ast

import (
	"context"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

// TestStore wraps a GraphIndexStore and its underlying GraphDB engine.
type TestStore struct {
	IndexStore
	engine *graphdb.Engine
}

// Close closes the underlying GraphDB engine.
func (s *TestStore) Close() error {
	if s == nil || s.engine == nil {
		return nil
	}
	return s.engine.Close(context.Background())
}

// NewTestStore creates a GraphIndexStore backed by a temporary GraphDB engine for testing.
func NewTestStore(dbPath string) (*TestStore, error) {
	dbDir := filepath.Dir(dbPath)
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(dbDir))
	if err != nil {
		return nil, err
	}
	return &TestStore{
		IndexStore: NewGraphIndexStore(engine),
		engine:     engine,
	}, nil
}

func newTestIndexManager(t *testing.T) (*IndexManager, string) {
	t.Helper()
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, "index.db"))
	if err != nil {
		t.Fatalf("store init failed: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir, ParallelWorkers: 1})
	return manager, tmpDir
}
