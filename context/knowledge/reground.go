package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// RegroundEntry is one durable state value restored at recipe restart. The
// knowledge-layer vocabulary matches the named/euclo/grounding port verbatim;
// app/envcomposition adapts between them.
type RegroundEntry struct {
	StateKey    string
	Value       any
	Epistemics  string
	Origin      string
	ChunkID     string
	SourceRunID string
	GroundedAt  time.Time
}

// RegroundRequest scopes a restart query over the grounded capture corpus.
type RegroundRequest struct {
	WorkspaceID string
	SessionID   string
	RecipeID    string
	MaxEntries  int
}

// RegroundResult is the ordered restore payload. Grounded=false with a nil
// error is a normal cold start, never an error.
type RegroundResult struct {
	Entries     []RegroundEntry
	SourceRunID string
	Grounded    bool
}

// defaultRegroundEntries bounds a reground result when MaxEntries is unset.
const defaultRegroundEntries = 64

// Reground returns the durable state captures of the most recent prior
// grounded run scoped to (WorkspaceID, RecipeID, SessionID). It is read-only,
// honors context cancellation, and bounds its output by MaxEntries.
func (g *GroundingService) Reground(ctx context.Context, req RegroundRequest) (RegroundResult, error) {
	if g == nil || g.store == nil || g.store.Graph == nil {
		return RegroundResult{}, fmt.Errorf("knowledge: reground store is required")
	}
	if err := ctx.Err(); err != nil {
		return RegroundResult{}, err
	}
	maxEntries := req.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultRegroundEntries
	}

	chunks, err := g.store.FindByWorkspace(req.WorkspaceID)
	if err != nil {
		return RegroundResult{}, err
	}

	type capturePair struct {
		chunk  KnowledgeChunk
		record GroundingRecord
		at     time.Time
	}
	var pairs []capturePair
	for _, chunk := range chunks {
		if kind, _ := chunk.Body.Fields["kind"].(string); kind != string(groundingKind) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return RegroundResult{}, err
		}
		if req.SessionID != "" && chunk.Provenance.SessionID != req.SessionID {
			continue
		}
		for _, record := range chunk.GroundedBy {
			if record.TaskID == "" {
				continue
			}
			if req.RecipeID != "" && record.RecipeID != req.RecipeID {
				continue
			}
			pairs = append(pairs, capturePair{chunk: chunk, record: record, at: chunk.AcquiredAt})
		}
	}
	if len(pairs) == 0 {
		return RegroundResult{}, nil
	}

	// The source run is the task with the newest capture timestamp; ties break
	// deterministically by task ID.
	runMaxAt := make(map[string]time.Time)
	for _, p := range pairs {
		if at, ok := runMaxAt[p.record.TaskID]; !ok || p.at.After(at) {
			runMaxAt[p.record.TaskID] = p.at
		}
	}
	var sourceRun string
	var sourceAt time.Time
	for runID, at := range runMaxAt {
		if sourceRun == "" || at.After(sourceAt) || (at.Equal(sourceAt) && runID < sourceRun) {
			sourceRun, sourceAt = runID, at
		}
	}

	runPairs := make([]capturePair, 0)
	for _, p := range pairs {
		if p.record.TaskID == sourceRun {
			runPairs = append(runPairs, p)
		}
	}
	sort.Slice(runPairs, func(i, j int) bool {
		if runPairs[i].at.Equal(runPairs[j].at) {
			return runPairs[i].chunk.ID < runPairs[j].chunk.ID
		}
		return runPairs[i].at.Before(runPairs[j].at)
	})

	// Final capture per state key: iterate oldest→newest, last wins.
	byKey := make(map[string]capturePair)
	var order []string
	for _, p := range runPairs {
		key, _ := p.chunk.Body.Fields["state_key"].(string)
		if key == "" {
			continue
		}
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
		}
		byKey[key] = p
	}

	entries := make([]RegroundEntry, 0, len(order))
	for _, key := range order {
		if err := ctx.Err(); err != nil {
			return RegroundResult{}, err
		}
		p := byKey[key]
		value, err := decodeCaptureValue(p.chunk)
		if err != nil {
			continue
		}
		epistemics, origin := g.restoreEpistemics(p.chunk)
		entries = append(entries, RegroundEntry{
			StateKey:    key,
			Value:       value,
			Epistemics:  string(epistemics),
			Origin:      origin,
			ChunkID:     string(p.chunk.ID),
			SourceRunID: sourceRun,
			GroundedAt:  p.at,
		})
		if len(entries) >= maxEntries {
			break
		}
	}
	return RegroundResult{Entries: entries, SourceRunID: sourceRun, Grounded: true}, nil
}

// restoreEpistemics re-derives the restore-time trust floor (FR-16): a stored
// `given` whose origin can no longer satisfy the identity rule is delivered as
// claimed, and the downgrade is observable.
func (g *GroundingService) restoreEpistemics(chunk KnowledgeChunk) (Epistemics, string) {
	origin := contextdata.OriginClass(chunk.OriginClass)
	if !origin.Valid() {
		origin = contextdata.OriginLLM
	}
	_, epistemics, downgraded := epistemicTrust(Epistemics(chunk.Epistemics), origin)
	if downgraded && g != nil && g.telemetry != nil {
		g.telemetry.Emit(telemetry.Event{
			Type:      telemetry.EventCaptureEpistemicsDowngraded,
			Message:   "restore epistemics downgraded",
			Timestamp: chunk.UpdatedAt,
			Metadata: map[string]any{
				"chunk_id": string(chunk.ID),
				"state_key": func() string {
					key, _ := chunk.Body.Fields["state_key"].(string)
					return key
				}(),
			},
		})
	}
	return epistemics, string(origin)
}

// decodeCaptureValue reconstructs the in-memory envelope value from the
// grounded chunk's canonical capture encoding.
func decodeCaptureValue(chunk KnowledgeChunk) (any, error) {
	var encoded captureEncoding
	if err := json.Unmarshal([]byte(chunk.Body.Raw), &encoded); err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(encoded.Value, &value); err != nil {
		return nil, err
	}
	return value, nil
}
