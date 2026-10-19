package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

// RepositoryImpl implements the Repository interface using graphdb.
type RepositoryImpl struct {
	db *graphdb.Engine
}

// NewCompilerRepository creates a new compiler repository backed by graphdb.
func NewCompilerRepository(db *graphdb.Engine) *RepositoryImpl {
	return &RepositoryImpl{db: db}
}

// Close closes the repository and the underlying graphdb engine.
func (r *RepositoryImpl) Close() error {
	return r.db.Close(context.Background())
}

// Compilation record operations

// StoreCompilationRecord persists a compilation record as a graph node.
func (r *RepositoryImpl) StoreCompilationRecord(ctx context.Context, record CompilationRecord) error {
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
	return r.db.UpsertNode(ctx, node)
}

// ErrCompilationRecordNotFound is the typed miss returned by
// GetCompilationRecord; callers map it onto their not-found semantics.
var ErrCompilationRecordNotFound = errors.New("compilation record not found")

// GetCompilationRecord loads a compilation record by request ID.
func (r *RepositoryImpl) GetCompilationRecord(_ context.Context, requestID string) (*CompilationRecord, error) {
	node, ok := r.db.GetNode(requestID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrCompilationRecordNotFound, requestID)
	}
	var record CompilationRecord
	if err := json.Unmarshal(node.Props, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// ListCompilationRecords returns all persisted compilation records.
func (r *RepositoryImpl) ListCompilationRecords(_ context.Context, _ uint64) ([]CompilationRecord, error) {
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

// StoreCacheEntry persists a cache entry as a graph node.
func (r *RepositoryImpl) StoreCacheEntry(ctx context.Context, entry CacheEntry) error {
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
	return r.db.UpsertNode(ctx, node)
}

// GetCacheEntry loads a cache entry by key.
func (r *RepositoryImpl) GetCacheEntry(_ context.Context, key CacheKey) (*CacheEntry, error) {
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

// InvalidateCacheEntries deletes cache entries that depend on any of the given chunk IDs.
func (r *RepositoryImpl) InvalidateCacheEntries(ctx context.Context, chunkIDs []string) error {
	nodes := r.db.ListNodes(graphdb.NodeKindCompilerCache)
	for _, node := range nodes {
		var entry CacheEntry
		if err := json.Unmarshal(node.Props, &entry); err != nil {
			continue
		}
		// Check if this cache entry depends on any of the chunk IDs
		for _, chunkID := range chunkIDs {
			if _, depends := entry.Dependencies[knowledge.ChunkID(chunkID)]; depends {
				if err := r.db.DeleteNode(ctx, node.ID); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

// Compiler artifact operations

// StoreCompilerArtifact persists a compiler artifact as a graph node.
func (r *RepositoryImpl) StoreCompilerArtifact(ctx context.Context, artifact Artifact) error {
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
	return r.db.UpsertNode(ctx, node)
}

// GetCompilerArtifact loads a compiler artifact by artifact ID.
func (r *RepositoryImpl) GetCompilerArtifact(_ context.Context, artifactID string) (*Artifact, error) {
	node, ok := r.db.GetNode(artifactID)
	if !ok {
		return nil, fmt.Errorf("compiler artifact not found: %s", artifactID)
	}
	var artifact Artifact
	if err := json.Unmarshal(node.Props, &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

// ListCompilerArtifacts returns compiler artifacts, optionally filtered by request ID.
func (r *RepositoryImpl) ListCompilerArtifacts(_ context.Context, requestID string) ([]Artifact, error) {
	nodes := r.db.ListNodes(graphdb.NodeKindCompilerArtifact)
	artifacts := make([]Artifact, 0)
	for _, node := range nodes {
		var artifact Artifact
		if err := json.Unmarshal(node.Props, &artifact); err != nil {
			continue
		}
		if requestID == "" || artifact.RequestID == requestID {
			artifacts = append(artifacts, artifact)
		}
	}
	return artifacts, nil
}
