package ast

import (
	"path/filepath"

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
	return s.engine.Close()
}

// NewTestStore creates a GraphIndexStore backed by a temporary GraphDB engine for testing.
func NewTestStore(dbPath string) (*TestStore, error) {
	dbDir := filepath.Dir(dbPath)
	engine, err := graphdb.Open(graphdb.DefaultOptions(dbDir))
	if err != nil {
		return nil, err
	}
	return &TestStore{
		IndexStore: NewGraphIndexStore(engine),
		engine:     engine,
	}, nil
}
