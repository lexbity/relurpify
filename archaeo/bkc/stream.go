package bkc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
)

// StreamSeed identifies starting chunk IDs for backward streaming.
type StreamSeed struct {
	ChunkIDs []ChunkID
}

// StreamResult contains ordered chunks and stale exclusions.
type StreamResult struct {
	Chunks            []KnowledgeChunk `json:"chunks,omitempty"`
	StaleDuringStream []ChunkID        `json:"stale_during_stream,omitempty"`
	StaleGapMessages  []string         `json:"stale_gap_messages,omitempty"`
	TokenTotal        int              `json:"token_total,omitempty"`
}

// Streamer performs dependency-first chunk streaming within a token budget.
type Streamer struct {
	Store *ChunkStore
	Graph *ChunkGraph
}

func (s *Streamer) Stream(ctx context.Context, seed StreamSeed, budget int) (*StreamResult, error) {
	if s == nil || s.Store == nil {
		return &StreamResult{}, nil
	}
	if len(seed.ChunkIDs) == 0 || budget <= 0 {
		return &StreamResult{}, nil
	}
	graph := s.Graph
	if graph == nil {
		graph = &ChunkGraph{Store: s.Store}
	}
	ordered, err := graph.OrderRequiresContext(seed.ChunkIDs)
	if err != nil {
		return nil, err
	}
	result := &StreamResult{}
	seen := make(map[ChunkID]struct{}, len(ordered))
	for _, chunk := range ordered {
		if _, ok := seen[chunk.ID]; ok {
			continue
		}
		seen[chunk.ID] = struct{}{}
		if chunk.Freshness == FreshnessStale {
			result.StaleDuringStream = append(result.StaleDuringStream, chunk.ID)
			result.StaleGapMessages = append(result.StaleGapMessages, fmt.Sprintf(
				"chunk %s (%.0f tokens, source: %s) was skipped - stale",
				chunk.ID,
				float64(chunk.TokenEstimate),
				firstProvenanceRef(chunk.Provenance),
			))
			continue
		}
		if chunk.TokenEstimate <= 0 {
			chunk.TokenEstimate = estimateTokens(chunk.Body.Raw)
		}
		if result.TokenTotal+chunk.TokenEstimate > budget {
			break
		}
		result.Chunks = append(result.Chunks, chunk)
		result.TokenTotal += chunk.TokenEstimate
	}
	if result.TokenTotal < budget {
		enrichment, err := s.loadAmplifies(seed.ChunkIDs, budget-result.TokenTotal, seen)
		if err != nil {
			return nil, err
		}
		for _, chunk := range enrichment {
			result.Chunks = append(result.Chunks, chunk)
			result.TokenTotal += chunk.TokenEstimate
		}
	}
	return result, nil
}

func (s *Streamer) loadAmplifies(seed []ChunkID, remaining int, seen map[ChunkID]struct{}) ([]KnowledgeChunk, error) {
	if remaining <= 0 {
		return nil, nil
	}
	type candidate struct {
		chunk  KnowledgeChunk
		weight float64
	}
	var candidates []candidate
	for _, seedID := range seed {
		edges, err := s.Store.LoadEdgesFrom(seedID, EdgeKindAmplifies)
		if err != nil {
			return nil, err
		}
		for _, edge := range edges {
			if edge.ToChunk == "" {
				continue
			}
			if _, ok := seen[edge.ToChunk]; ok {
				continue
			}
			chunk, ok, err := s.Store.Load(edge.ToChunk)
			if err != nil || !ok || chunk == nil {
				if err != nil {
					return nil, err
				}
				continue
			}
			if chunk.Freshness == FreshnessStale {
				continue
			}
			if chunk.TokenEstimate <= 0 {
				chunk.TokenEstimate = estimateTokens(chunk.Body.Raw)
			}
			candidates = append(candidates, candidate{chunk: *chunk, weight: edge.Weight})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].weight == candidates[j].weight {
			return candidates[i].chunk.ID < candidates[j].chunk.ID
		}
		return candidates[i].weight > candidates[j].weight
	})
	out := make([]KnowledgeChunk, 0, len(candidates))
	total := 0
	for _, entry := range candidates {
		if _, ok := seen[entry.chunk.ID]; ok {
			continue
		}
		if total+entry.chunk.TokenEstimate > remaining {
			break
		}
		seen[entry.chunk.ID] = struct{}{}
		out = append(out, entry.chunk)
		total += entry.chunk.TokenEstimate
	}
	return out, nil
}

func firstProvenanceRef(p ChunkProvenance) string {
	if len(p.Sources) > 0 {
		if ref := strings.TrimSpace(p.Sources[0].Ref); ref != "" {
			return ref
		}
	}
	if ref := strings.TrimSpace(p.CodeStateRef); ref != "" {
		return ref
	}
	return "unknown"
}

func (s *Streamer) ChatSeed(files []string) (StreamSeed, error) {
	chunks, err := s.Store.FindAll()
	if err != nil {
		return StreamSeed{}, err
	}
	allow := make(map[string]struct{}, len(files))
	for _, file := range files {
		allow[file] = struct{}{}
	}
	seed := StreamSeed{}
	for _, chunk := range chunks {
		filePath, _ := chunk.Body.Fields["file_path"].(string)
		if _, ok := allow[filePath]; ok {
			seed.ChunkIDs = append(seed.ChunkIDs, chunk.ID)
		}
	}
	return seed, nil
}

func (s *Streamer) PlanningSeed(rootChunkIDs []string) StreamSeed {
	seed := StreamSeed{ChunkIDs: make([]ChunkID, 0, len(rootChunkIDs))}
	for _, id := range rootChunkIDs {
		if id != "" {
			seed.ChunkIDs = append(seed.ChunkIDs, ChunkID(id))
		}
	}
	return seed
}

func (s *Streamer) PlanningSeedForVersion(plan *archaeodomain.VersionedLivingPlan) StreamSeed {
	if plan == nil {
		return StreamSeed{}
	}
	return s.PlanningSeed(plan.RootChunkIDs)
}

func (s *Streamer) ArchaeologySeed(chunkIDs []string) StreamSeed {
	return s.PlanningSeed(chunkIDs)
}

func (s *Streamer) DebugSeed(files []string, tensionRefs []string) (StreamSeed, error) {
	seed, err := s.ChatSeed(files)
	if err != nil {
		return StreamSeed{}, err
	}
	chunks, err := s.Store.FindAll()
	if err != nil {
		return StreamSeed{}, err
	}
	allow := make(map[string]struct{}, len(tensionRefs))
	for _, ref := range tensionRefs {
		allow[ref] = struct{}{}
	}
	for _, chunk := range chunks {
		for _, source := range chunk.Provenance.Sources {
			if _, ok := allow[source.Ref]; ok {
				seed.ChunkIDs = append(seed.ChunkIDs, chunk.ID)
				break
			}
		}
	}
	return seed, nil
}
