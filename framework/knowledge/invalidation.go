package knowledge

import (
	"context"
	"strings"
)

// StaleChunkReporter can surface stale chunks to domain-specific consumers.
type StaleChunkReporter interface {
	ReportStaleChunks(ctx context.Context, chunkIDs []ChunkID, affectedPaths []string, reason string) error
}

// InvalidationPass reacts to revision drift and surfaces stale chunks.
type InvalidationPass struct {
	Store         *ChunkStore
	Staleness     *StalenessManager
	Events        *EventBus
	Tensions      any
	Reporter      StaleChunkReporter
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
				if err := p.SurfaceStaleChunks(ctx, chunkIDsFromStrings(payload.ChunkIDs), payload.AffectedPaths, payload.Reason); err != nil {
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
	if err := p.SurfaceStaleChunks(ctx, staled, payload.AffectedPaths, "code_revision_changed"); err != nil {
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
	return p.SurfaceStaleChunks(ctx, result.StaleDuringStream, nil, "stale_during_stream")
}

func (p *InvalidationPass) SurfaceStaleChunks(ctx context.Context, chunkIDs []ChunkID, affectedPaths []string, reason string) error {
	if p == nil || len(chunkIDs) == 0 {
		return nil
	}
	if p.Reporter != nil {
		return p.Reporter.ReportStaleChunks(ctx, chunkIDs, affectedPaths, reason)
	}
	return nil
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

func chunkIDsFromStrings(ids []string) []ChunkID {
	out := make([]ChunkID, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			out = append(out, ChunkID(strings.TrimSpace(id)))
		}
	}
	return out
}
