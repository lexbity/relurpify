package context

import (
	"context"
	"fmt"
	"strings"

	archaeobkc "github.com/lexcodex/relurpify/archaeo/bkc"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
)

type bkcContextStrategy struct {
	base       contextmgr.ContextStrategy
	streamer   chunkStreamer
	seedChunks []contextmgr.ContextChunk
}

type chunkStreamer interface {
	Stream(ctx context.Context, seed archaeobkc.StreamSeed, budget int) (*archaeobkc.StreamResult, error)
	PlanningSeed(rootChunkIDs []string) archaeobkc.StreamSeed
	ArchaeologySeed(chunkIDs []string) archaeobkc.StreamSeed
}

func (s *bkcContextStrategy) SelectContext(task *core.Task, budget *core.ContextBudget) (*contextmgr.ContextRequest, error) {
	if s == nil || s.base == nil {
		return &contextmgr.ContextRequest{}, nil
	}
	return s.base.SelectContext(task, budget)
}

func (s *bkcContextStrategy) ShouldCompress(ctx *core.SharedContext) bool {
	if s == nil || s.base == nil {
		return false
	}
	return s.base.ShouldCompress(ctx)
}

func (s *bkcContextStrategy) DetermineDetailLevel(file string, relevance float64) contextmgr.DetailLevel {
	if s == nil || s.base == nil {
		return contextmgr.DetailDetailed
	}
	return s.base.DetermineDetailLevel(file, relevance)
}

func (s *bkcContextStrategy) ShouldExpandContext(ctx *core.SharedContext, lastResult *core.Result) bool {
	if s == nil || s.base == nil {
		return false
	}
	return s.base.ShouldExpandContext(ctx, lastResult)
}

func (s *bkcContextStrategy) PrioritizeContext(items []core.ContextItem) []core.ContextItem {
	if s == nil || s.base == nil {
		return append([]core.ContextItem(nil), items...)
	}
	return s.base.PrioritizeContext(items)
}

func (s *bkcContextStrategy) LoadChunks(task *core.Task, budget *core.ContextBudget) ([]contextmgr.ContextChunk, error) {
	if s == nil || len(s.seedChunks) == 0 {
		return nil, nil
	}
	if s.streamer == nil {
		return append([]contextmgr.ContextChunk(nil), s.seedChunks...), nil
	}
	rootChunkIDs := make([]string, 0, len(s.seedChunks))
	for _, chunk := range s.seedChunks {
		if strings.TrimSpace(chunk.ID) != "" {
			rootChunkIDs = append(rootChunkIDs, strings.TrimSpace(chunk.ID))
		}
	}
	if len(rootChunkIDs) == 0 {
		return nil, nil
	}
	budgetTokens := 1200
	if budget != nil && budget.MaxTokens > 0 {
		budgetTokens = budget.MaxTokens
	}
	result, err := s.streamer.Stream(context.Background(), s.streamer.ArchaeologySeed(rootChunkIDs), budgetTokens)
	if err != nil {
		return nil, err
	}
	if len(result.Chunks) == 0 {
		return append([]contextmgr.ContextChunk(nil), s.seedChunks...), nil
	}
	return archaeobkc.ToContextChunks(result.Chunks), nil
}

func contextChunksFromAny(raw any) []contextmgr.ContextChunk {
	switch typed := raw.(type) {
	case []contextmgr.ContextChunk:
		return append([]contextmgr.ContextChunk(nil), typed...)
	case []any:
		out := make([]contextmgr.ContextChunk, 0, len(typed))
		for _, item := range typed {
			if chunk, ok := contextChunkFromValue(item); ok {
				out = append(out, chunk)
			}
		}
		return out
	default:
		if chunk, ok := contextChunkFromValue(typed); ok {
			return []contextmgr.ContextChunk{chunk}
		}
		return nil
	}
}

func contextChunkFromValue(raw any) (contextmgr.ContextChunk, bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		return contextmgr.ContextChunk{}, false
	}
	id := strings.TrimSpace(fmt.Sprint(m["id"]))
	content := strings.TrimSpace(fmt.Sprint(m["content"]))
	if id == "" && content == "" {
		return contextmgr.ContextChunk{}, false
	}
	chunk := contextmgr.ContextChunk{
		ID:      id,
		Content: content,
	}
	switch tokens := m["token_estimate"].(type) {
	case int:
		chunk.TokenEstimate = tokens
	case int64:
		chunk.TokenEstimate = int(tokens)
	case float64:
		chunk.TokenEstimate = int(tokens)
	}
	if meta, ok := m["metadata"].(map[string]string); ok {
		chunk.Metadata = make(map[string]string, len(meta))
		for key, value := range meta {
			chunk.Metadata[key] = value
		}
	} else if metaAny, ok := m["metadata"].(map[string]any); ok {
		chunk.Metadata = make(map[string]string, len(metaAny))
		for key, value := range metaAny {
			chunk.Metadata[key] = strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return chunk, true
}

func uniqueContextChunks(chunks []contextmgr.ContextChunk) []contextmgr.ContextChunk {
	if len(chunks) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(chunks))
	out := make([]contextmgr.ContextChunk, 0, len(chunks))
	for _, chunk := range chunks {
		key := strings.TrimSpace(chunk.ID)
		if key == "" {
			key = strings.TrimSpace(chunk.Content)
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, chunk)
	}
	return out
}

func taskContextStringSlice(task *core.Task, key string) []string {
	if task == nil || task.Context == nil {
		return nil
	}
	value, ok := task.Context[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []string{text}
		}
		return nil
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return nil
		}
		return []string{text}
	}
}
