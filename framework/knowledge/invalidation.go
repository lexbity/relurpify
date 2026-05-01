package knowledge

import (
	"context"
	"strings"
	"time"
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
	const debounceWindow = 50 * time.Millisecond
	var (
		timer         *time.Timer
		timerC        <-chan time.Time
		pending       = make(map[ChunkID]struct{})
		pendingPaths  = make(map[string]struct{})
		pendingReason string
	)
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}
	schedule := func() {
		if timer == nil {
			timer = time.NewTimer(debounceWindow)
			timerC = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(debounceWindow)
		timerC = timer.C
	}
	flush := func() error {
		if len(pending) == 0 {
			stopTimer()
			return nil
		}
		ids := make([]ChunkID, 0, len(pending))
		for id := range pending {
			ids = append(ids, id)
		}
		paths := make([]string, 0, len(pendingPaths))
		for path := range pendingPaths {
			paths = append(paths, path)
		}
		reason := pendingReason
		pending = make(map[ChunkID]struct{})
		pendingPaths = make(map[string]struct{})
		pendingReason = ""
		stopTimer()

		manager := p.stalenessManager()
		propagated, err := manager.PropagateSync(ids, 0)
		if err != nil {
			return err
		}
		if err := p.SurfaceStaleChunks(ctx, ids, paths, reason); err != nil {
			return err
		}
		if len(propagated) > 0 {
			if err := p.SurfaceStaleChunks(ctx, propagated, paths, reason); err != nil {
				return err
			}
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			stopTimer()
			return nil
		case <-timerC:
			if err := flush(); err != nil {
				stopTimer()
				return err
			}
		case event, ok := <-ch:
			if !ok {
				stopTimer()
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
				for _, id := range chunkIDsFromStrings(payload.ChunkIDs) {
					pending[id] = struct{}{}
				}
				for _, path := range payload.AffectedPaths {
					if strings.TrimSpace(path) != "" {
						pendingPaths[strings.TrimSpace(path)] = struct{}{}
					}
				}
				if pendingReason == "" {
					pendingReason = payload.Reason
				}
				schedule()
				if len(pending) == 0 {
					continue
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
	if p == nil || p.Store == nil {
		return nil, nil
	}
	pathSet := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			pathSet[path] = struct{}{}
		}
	}
	if len(pathSet) == 0 {
		return nil, nil
	}
	matches := make([]ChunkID, 0)
	seen := make(map[ChunkID]struct{})
	for path := range pathSet {
		chunks, err := p.Store.FindByFilePath(path)
		if err != nil {
			return nil, err
		}
		for _, chunk := range chunks {
			if strings.TrimSpace(newRevision) != "" && strings.TrimSpace(chunk.Provenance.CodeStateRef) == strings.TrimSpace(newRevision) {
				continue
			}
			if _, ok := seen[chunk.ID]; ok {
				continue
			}
			seen[chunk.ID] = struct{}{}
			matches = append(matches, chunk.ID)
		}
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
