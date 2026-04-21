package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

type retrieverSemanticAdapter struct {
	retriever *retrieval.Retriever
}

func resolveSemanticEmbedder(backend llm.ManagedBackend, cfg Config, inferenceModel string) (retrieval.Embedder, error) {
	return retrieval.NewEmbedder(backend, embedderCfgFromRuntimeConfig(cfg, inferenceModel))
}

func (a *retrieverSemanticAdapter) Query(ctx context.Context, query string, limit int) ([]search.VectorMatch, error) {
	if a == nil || a.retriever == nil {
		return nil, nil
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	result, err := a.retriever.RetrieveCandidates(ctx, retrieval.RetrievalQuery{
		Text:  query,
		Scope: "workspace",
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	chunkMeta := make(map[string]retrieval.PrefilterChunk, len(result.Prefilter.Chunks))
	for _, chunk := range result.Prefilter.Chunks {
		chunkMeta[chunk.ChunkID] = chunk
	}
	matches := make([]search.VectorMatch, 0, len(result.Fused))
	for _, candidate := range result.Fused {
		meta := map[string]any{
			"chunk_id": candidate.ChunkID,
			"doc_id":   candidate.DocID,
		}
		if chunk, ok := chunkMeta[candidate.ChunkID]; ok {
			meta["path"] = chunk.CanonicalURI
			meta["source_type"] = chunk.SourceType
		}
		matches = append(matches, search.VectorMatch{
			ID:       candidate.ChunkID,
			Content:  candidate.Text,
			Metadata: meta,
			Score:    candidate.FusedScore,
		})
	}
	return matches, nil
}

func openRetrievalDB(path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("retrieval db path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if err := retrieval.EnsureSchema(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func ingestCodeIndex(ctx context.Context, workspace string, codeIndex core.CodeIndex, pipeline *retrieval.IngestionPipeline) error {
	if codeIndex == nil || pipeline == nil {
		return nil
	}
	files := append([]string(nil), codeIndex.ListFiles()...)
	sort.Strings(files)
	for _, relPath := range files {
		fullPath := relPath
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(workspace, relPath)
		}
		if _, err := os.Stat(fullPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		if _, err := pipeline.IngestFile(ctx, fullPath, "workspace", []string{"code"}); err != nil {
			return fmt.Errorf("ingest %s: %w", fullPath, err)
		}
	}
	return nil
}
