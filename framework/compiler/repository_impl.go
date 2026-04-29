package compiler

import (
	"context"
	"encoding/json"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// CompilerRepositoryImpl implements the Repository interface using graphdb.
type CompilerRepositoryImpl struct {
	db *graphdb.Engine
}

// NewCompilerRepository creates a new compiler repository backed by graphdb.
func NewCompilerRepository(db *graphdb.Engine) *CompilerRepositoryImpl {
	return &CompilerRepositoryImpl{db: db}
}

// Close closes the repository and the underlying graphdb engine.
func (r *CompilerRepositoryImpl) Close() error {
	return r.db.Close()
}

// Compilation record operations

func (r *CompilerRepositoryImpl) StoreCompilationRecord(ctx context.Context, record CompilationRecord) error {
	props, err := json.Marshal(record)
	if err != nil {
		return err
	}

	node := graphdb.NodeRecord{
		ID:     record.RequestID,
		Kind:   graphdb.NodeKindCompilerCompilation,
		Props:  props,
		Labels: []string{"compilation"},
	}
	return r.db.UpsertNode(node)
}

func (r *CompilerRepositoryImpl) GetCompilationRecord(ctx context.Context, requestID string) (*CompilationRecord, error) {
	node, ok := r.db.GetNode(requestID)
	if !ok {
		return nil, fmt.Errorf("compilation record not found: %s", requestID)
	}
	var record CompilationRecord
	if err := json.Unmarshal(node.Props, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *CompilerRepositoryImpl) ListCompilationRecords(ctx context.Context, eventLogSeq uint64) ([]CompilationRecord, error) {
	nodes := r.db.ListNodes(graphdb.NodeKindCompilerCompilation)
	records := make([]CompilationRecord, 0, len(nodes))
	for _, node := range nodes {
		var record CompilationRecord
		if err := json.Unmarshal(node.Props, &record); err != nil {
			continue // Skip malformed records
		}
		records = append(records, record)
	}
	return records, nil
}

// Cache entry operations

func (r *CompilerRepositoryImpl) StoreCacheEntry(ctx context.Context, entry CacheEntry) error {
	keyStr := entry.Key.String()
	props, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	node := graphdb.NodeRecord{
		ID:     keyStr,
		Kind:   graphdb.NodeKindCompilerCache,
		Props:  props,
		Labels: []string{"cache"},
	}
	return r.db.UpsertNode(node)
}

func (r *CompilerRepositoryImpl) GetCacheEntry(ctx context.Context, key CacheKey) (*CacheEntry, error) {
	keyStr := key.String()
	node, ok := r.db.GetNode(keyStr)
	if !ok {
		return nil, fmt.Errorf("cache entry not found: %s", keyStr)
	}
	var entry CacheEntry
	if err := json.Unmarshal(node.Props, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (r *CompilerRepositoryImpl) InvalidateCacheEntries(ctx context.Context, chunkIDs []string) error {
	nodes := r.db.ListNodes(graphdb.NodeKindCompilerCache)
	for _, node := range nodes {
		var entry CacheEntry
		if err := json.Unmarshal(node.Props, &entry); err != nil {
			continue
		}
		// Check if this cache entry depends on any of the chunk IDs
		for _, chunkID := range chunkIDs {
			if _, depends := entry.Dependencies[knowledge.ChunkID(chunkID)]; depends {
				if err := r.db.DeleteNode(node.ID); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

// Compiler artifact operations

func (r *CompilerRepositoryImpl) StoreCompilerArtifact(ctx context.Context, artifact CompilerArtifact) error {
	props, err := json.Marshal(artifact)
	if err != nil {
		return err
	}

	node := graphdb.NodeRecord{
		ID:     artifact.ArtifactID,
		Kind:   graphdb.NodeKindCompilerArtifact,
		Props:  props,
		Labels: []string{"compiler_artifact"},
	}
	return r.db.UpsertNode(node)
}

func (r *CompilerRepositoryImpl) GetCompilerArtifact(ctx context.Context, artifactID string) (*CompilerArtifact, error) {
	node, ok := r.db.GetNode(artifactID)
	if !ok {
		return nil, fmt.Errorf("compiler artifact not found: %s", artifactID)
	}
	var artifact CompilerArtifact
	if err := json.Unmarshal(node.Props, &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

func (r *CompilerRepositoryImpl) ListCompilerArtifacts(ctx context.Context, requestID string) ([]CompilerArtifact, error) {
	nodes := r.db.ListNodes(graphdb.NodeKindCompilerArtifact)
	artifacts := make([]CompilerArtifact, 0)
	for _, node := range nodes {
		var artifact CompilerArtifact
		if err := json.Unmarshal(node.Props, &artifact); err != nil {
			continue
		}
		if requestID == "" || artifact.RequestID == requestID {
			artifacts = append(artifacts, artifact)
		}
	}
	return artifacts, nil
}
