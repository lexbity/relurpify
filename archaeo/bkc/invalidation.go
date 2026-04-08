package bkc

import (
	"context"
	"fmt"
	"strings"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
)

// InvalidationPass reacts to revision drift and surfaces stale chunks as tensions.
type InvalidationPass struct {
	Store         *ChunkStore
	Staleness     *StalenessManager
	Events        *EventBus
	Tensions      archaeotensions.Service
	WorkspaceRoot string
}

func (p *InvalidationPass) Start(ctx context.Context) error {
	if p == nil || p.Events == nil {
		return nil
	}
	ch, unsub := p.Events.Subscribe(8)
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			switch event.Kind {
			case EventCodeRevisionChanged:
				payload, ok := event.Payload.(CodeRevisionChangedPayload)
				if !ok {
					continue
				}
				if err := p.HandleRevisionChanged(ctx, payload); err != nil {
					return err
				}
			case EventChunkStaled:
				payload, ok := event.Payload.(ChunkStaledPayload)
				if !ok {
					continue
				}
				if err := p.SurfaceStaleChunks(ctx, payload.ChunkIDs, payload.AffectedPaths, payload.Reason); err != nil {
					return err
				}
			}
		}
	}
}

func (p *InvalidationPass) Stop() error { return nil }

func (p *InvalidationPass) HandleRevisionChanged(ctx context.Context, payload CodeRevisionChangedPayload) error {
	if p == nil || p.Store == nil {
		return nil
	}
	matches, err := p.matchAffectedChunks(payload.AffectedPaths, payload.NewRevision)
	if err != nil || len(matches) == 0 {
		return err
	}
	manager := p.stalenessManager()
	staled, err := manager.BulkMarkStaleCollect(matches)
	if err != nil || len(staled) == 0 {
		return err
	}
	if err := p.SurfaceStaleChunks(ctx, chunkIDsToStrings(staled), payload.AffectedPaths, "code_revision_changed"); err != nil {
		return err
	}
	if p.Events != nil {
		p.Events.EmitChunkStaled(ChunkStaledPayload{
			WorkspaceRoot: firstNonEmpty(payload.WorkspaceRoot, p.WorkspaceRoot),
			ChunkIDs:      chunkIDsToStrings(staled),
			AffectedPaths: append([]string(nil), payload.AffectedPaths...),
			Reason:        "code_revision_changed",
		})
	}
	return nil
}

func (p *InvalidationPass) SurfaceStaleDuringStream(ctx context.Context, result *StreamResult) error {
	if result == nil || len(result.StaleDuringStream) == 0 {
		return nil
	}
	return p.SurfaceStaleChunks(ctx, chunkIDsToStrings(result.StaleDuringStream), nil, "stale_during_stream")
}

func (p *InvalidationPass) SurfaceStaleChunks(ctx context.Context, chunkIDs []string, affectedPaths []string, reason string) error {
	if p == nil || p.Store == nil || len(chunkIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(chunkIDs))
	for _, chunkID := range chunkIDs {
		chunkID = strings.TrimSpace(chunkID)
		if chunkID == "" {
			continue
		}
		if _, ok := seen[chunkID]; ok {
			continue
		}
		seen[chunkID] = struct{}{}
		chunk, ok, err := p.Store.Load(ChunkID(chunkID))
		if err != nil || !ok || chunk == nil {
			if err != nil {
				return err
			}
			continue
		}
		status := archaeodomain.TensionInferred
		if chunk.Freshness == FreshnessUnverified {
			status = archaeodomain.TensionConfirmed
		}
		if err := p.upsertStaleTension(ctx, *chunk, status, affectedPaths, reason); err != nil {
			return err
		}
	}
	return nil
}

func (p *InvalidationPass) upsertStaleTension(ctx context.Context, chunk KnowledgeChunk, status archaeodomain.TensionStatus, affectedPaths []string, reason string) error {
	if p.Tensions.Store == nil || strings.TrimSpace(chunk.Provenance.WorkflowID) == "" {
		return nil
	}
	description := fmt.Sprintf("knowledge chunk %s became stale", chunk.ID)
	if r := strings.TrimSpace(reason); r != "" {
		description = fmt.Sprintf("knowledge chunk %s became stale: %s", chunk.ID, r)
	}
	commentRefs := p.invalidatedEdgeRefs(chunk)
	_, err := p.Tensions.CreateOrUpdate(ctx, archaeotensions.CreateInput{
		WorkflowID:         chunk.Provenance.WorkflowID,
		SourceRef:          string(chunk.ID),
		Kind:               "bkc_chunk_stale",
		Description:        description,
		Severity:           "medium",
		Status:             status,
		AnchorRefs:         affectedPathsFromChunk(chunk, affectedPaths),
		SymbolScope:        symbolScopeFromChunk(chunk),
		BlastRadiusNodeIDs: append([]string(nil), affectedPaths...),
		BasedOnRevision:    chunk.Provenance.CodeStateRef,
		CommentRefs:        commentRefs,
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *InvalidationPass) invalidatedEdgeRefs(chunk KnowledgeChunk) []string {
	if p == nil || p.Store == nil {
		return nil
	}
	edges, err := p.Store.LoadEdgesFrom(chunk.ID, EdgeKindDependsOnCodeState, EdgeKindRequiresContext, EdgeKindAmplifies)
	if err != nil || len(edges) == 0 {
		return nil
	}
	commentRefs := make([]string, 0, len(edges))
	for _, edge := range edges {
		ref := strings.TrimSpace(string(edge.Kind)) + ":"
		if edge.ToChunk != "" {
			ref += string(edge.ToChunk)
		} else if edge.Meta != nil {
			if sourceRef, ok := edge.Meta["code_state_ref"].(string); ok && strings.TrimSpace(sourceRef) != "" {
				ref += strings.TrimSpace(sourceRef)
			} else if sourceRef, ok := edge.Meta["source_ref"].(string); ok && strings.TrimSpace(sourceRef) != "" {
				ref += strings.TrimSpace(sourceRef)
			}
		}
		if strings.TrimSpace(ref) == ":" {
			continue
		}
		commentRefs = append(commentRefs, ref)
	}
	if len(commentRefs) == 0 {
		return nil
	}
	return commentRefs
}

func (p *InvalidationPass) matchAffectedChunks(paths []string, newRevision string) ([]ChunkID, error) {
	chunks, err := p.Store.FindAll()
	if err != nil || len(chunks) == 0 {
		return nil, err
	}
	pathSet := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			pathSet[path] = struct{}{}
		}
	}
	matches := make([]ChunkID, 0)
	for _, chunk := range chunks {
		if len(pathSet) > 0 && !chunkTouchesPaths(chunk, pathSet) {
			continue
		}
		if strings.TrimSpace(newRevision) != "" && strings.TrimSpace(chunk.Provenance.CodeStateRef) == strings.TrimSpace(newRevision) {
			continue
		}
		matches = append(matches, chunk.ID)
	}
	return matches, nil
}

func (p *InvalidationPass) stalenessManager() *StalenessManager {
	if p.Staleness != nil {
		return p.Staleness
	}
	return &StalenessManager{Store: p.Store, Propagate: true, MaxDepth: 3}
}

func chunkTouchesPaths(chunk KnowledgeChunk, pathSet map[string]struct{}) bool {
	if len(pathSet) == 0 {
		return true
	}
	if path, _ := chunk.Body.Fields["file_path"].(string); path != "" {
		if _, ok := pathSet[strings.TrimSpace(path)]; ok {
			return true
		}
	}
	for _, source := range chunk.Provenance.Sources {
		if _, ok := pathSet[strings.TrimSpace(source.Ref)]; ok {
			return true
		}
	}
	for _, value := range stringValues(chunk.Body.Fields["symbol_scope"]) {
		if _, ok := pathSet[strings.TrimSpace(value)]; ok {
			return true
		}
	}
	return false
}

func affectedPathsFromChunk(chunk KnowledgeChunk, affectedPaths []string) []string {
	if len(affectedPaths) > 0 {
		return append([]string(nil), affectedPaths...)
	}
	out := make([]string, 0, 1)
	if path, _ := chunk.Body.Fields["file_path"].(string); strings.TrimSpace(path) != "" {
		out = append(out, strings.TrimSpace(path))
	}
	return out
}

func symbolScopeFromChunk(chunk KnowledgeChunk) []string {
	scope := stringValues(chunk.Body.Fields["symbol_scope"])
	if len(scope) > 0 {
		return scope
	}
	if symbolID, _ := chunk.Body.Fields["symbol_id"].(string); strings.TrimSpace(symbolID) != "" {
		return []string{strings.TrimSpace(symbolID)}
	}
	return nil
}

func stringValues(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{strings.TrimSpace(v)}
	default:
		return nil
	}
}

func chunkIDsToStrings(ids []ChunkID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(string(id)) != "" {
			out = append(out, string(id))
		}
	}
	return out
}
