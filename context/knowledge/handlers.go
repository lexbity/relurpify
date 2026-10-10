// knowledge runner-job handlers (S8): thin adapters wiring the runner's
// day-one job kinds to the knowledge domain's real machinery. Both satisfy
// the H1–H5 handler contract structurally:
//
//	H1 idempotent — bootstrap is a derived-structure rebuild; the sweep is
//	   a pure staleness re-evaluation over current file state.
//	H2 ctx-honoring — both delegate to ctx-carrying calls and return within
//	   the shutdown grace budget.
//	H3 checkpoints — the sweep checkpoints per batch of examined chunks, so
//	   a long sweep resumes from the last batch instead of restarting.
//	H4 authorization — the runner grants no implicit scopes; these handlers
//	   touch only the workspace's derived structures under the workspace
//	   service scope the composition root granted.
//	H5 telemetry — progress emits through the injected sink, stamped with
//	   the job's CorrelateID (the runner's executor stamps it).
package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/jobs"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// NewBootstrapHandler builds the knowledge.bootstrap runner job: a full AST
// workspace index pass. Indexing is a derived-structure rebuild, so the
// handler is idempotent (H1) and resume-from-checkpoint re-runs the pass —
// the checkpoint records completion stats, not partial state, because the
// index manager has no partial-batch seam and inventing one here would fake
// resumability it cannot deliver.
func NewBootstrapHandler(indexManager *ast.IndexManager, tel telemetry.Telemetry) jobs.Handler {
	return &bootstrapHandler{indexManager: indexManager, tel: tel}
}

type bootstrapHandler struct {
	indexManager *ast.IndexManager
	tel          telemetry.Telemetry
}

func (h *bootstrapHandler) Handle(ctx context.Context, j jobs.Job) (jobs.Checkpoint, error) {
	if h == nil || h.indexManager == nil {
		return jobs.Checkpoint{}, fmt.Errorf("knowledge.bootstrap: index manager unavailable")
	}
	if err := h.indexManager.IndexWorkspaceContext(ctx); err != nil {
		return jobs.Checkpoint{}, fmt.Errorf("knowledge.bootstrap: index workspace: %w", err)
	}
	stats, err := h.indexManager.Stats()
	indexed := 0
	if err == nil && stats != nil {
		indexed = stats.TotalFiles
	}
	h.emit(ctx, j, "knowledge.bootstrap complete", map[string]any{"indexed_files": indexed})
	now := time.Now().UTC()
	return jobs.Checkpoint{
		ID:      j.ID + ":bootstrap",
		JobID:   j.ID,
		State:   map[string]any{"indexed_files": indexed, "completed_at": now.Format(time.RFC3339)},
		Token:   j.ResumeToken,
		Created: now,
	}, nil
}

func (h *bootstrapHandler) emit(ctx context.Context, j jobs.Job, message string, metadata map[string]any) {
	if h.tel == nil {
		return
	}
	ev := telemetry.Event{
		Type:      telemetry.EventJobClaimed,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(ctx, &ev)
	h.tel.Emit(ev)
}

// refreshSweepCheckpoint is the resume payload the refresh handler writes
// after each examined batch (H3): resume continues after the last examined
// chunk ID instead of restarting the sweep.
type refreshSweepCheckpoint struct {
	LastChunkID string `json:"last_chunk_id"`
	MarkedStale int    `json:"marked_stale"`
}

// NewRefreshHandler builds the knowledge.refresh runner job: the staleness
// sweep that is the sole staleness driver while no realtime producer exists
// (Q12 r3). Each chunk with a file_path is re-evaluated against current
// workspace state — the file missing or modified after the chunk's
// provenance timestamp marks it stale via the staleness manager.
func NewRefreshHandler(store *ChunkStore, staleness *StalenessManager, workspaceRoot string, ckpt jobs.Store, tel telemetry.Telemetry) jobs.Handler {
	return &refreshHandler{store: store, staleness: staleness, workspaceRoot: workspaceRoot, ckpt: ckpt, tel: tel, batch: 500}
}

type refreshHandler struct {
	store         *ChunkStore
	staleness     *StalenessManager
	workspaceRoot string
	ckpt          jobs.Store // the durable jobs store: checkpoint persistence
	tel           telemetry.Telemetry
	batch         int // examined chunks per checkpoint; 500 per §5.6
}

func (h *refreshHandler) Handle(ctx context.Context, j jobs.Job) (jobs.Checkpoint, error) {
	if h == nil || h.store == nil || h.staleness == nil {
		return jobs.Checkpoint{}, fmt.Errorf("knowledge.refresh: store and staleness manager required")
	}
	batch := h.batch
	if batch <= 0 {
		batch = 500
	}

	// Resume (H3): start after the last examined chunk from the prior
	// checkpoint, when the executor handed one back.
	var progress refreshSweepCheckpoint
	if j.ResumeToken != "" && h.ckpt != nil {
		if cp, cpErr := h.ckpt.LoadCheckpoint(ctx, j.ID); cpErr == nil && cp != nil {
			if state, ok := cp.State.(refreshSweepCheckpoint); ok {
				progress = state
			}
		}
	}

	chunks, err := h.store.FindAll()
	if err != nil {
		return jobs.Checkpoint{}, fmt.Errorf("knowledge.refresh: load chunks: %w", err)
	}
	marked := 0
	examinedSinceCheckpoint := 0
	for _, chunk := range chunks {
		if string(chunk.ID) <= progress.LastChunkID {
			continue
		}
		if stale, evalErr := h.evaluate(ctx, chunk); evalErr == nil && stale {
			if err := h.staleness.MarkStale(ctx, chunk.ID); err != nil {
				return jobs.Checkpoint{}, fmt.Errorf("knowledge.refresh: mark stale %s: %w", chunk.ID, err)
			}
			marked++
		}
		progress.LastChunkID = string(chunk.ID)
		examinedSinceCheckpoint++
		if examinedSinceCheckpoint >= batch {
			progress.MarkedStale = marked
			cp := jobs.Checkpoint{
				ID:      j.ID + ":sweep",
				JobID:   j.ID,
				State:   progress,
				Token:   j.ResumeToken,
				Created: time.Now().UTC(),
			}
			if h.ckpt != nil {
				if err := h.ckpt.SaveCheckpoint(ctx, cp); err != nil {
					return jobs.Checkpoint{}, fmt.Errorf("knowledge.refresh: checkpoint: %w", err)
				}
				h.emit(ctx, j, "knowledge.refresh batch", map[string]any{"examined": examinedSinceCheckpoint, "marked_stale": marked})
			}
			examinedSinceCheckpoint = 0
		}
		if ctx.Err() != nil {
			return jobs.Checkpoint{}, ctx.Err() // H2: honor cancellation
		}
	}
	h.emit(ctx, j, "knowledge.refresh complete", map[string]any{"marked_stale": marked})
	now := time.Now().UTC()
	return jobs.Checkpoint{
		ID:      j.ID + ":sweep",
		JobID:   j.ID,
		State:   progress,
		Token:   j.ResumeToken,
		Created: now,
	}, nil
}

func (h *refreshHandler) emit(ctx context.Context, j jobs.Job, message string, metadata map[string]any) {
	if h.tel == nil {
		return
	}
	ev := telemetry.Event{
		Type:      telemetry.EventJobClaimed,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(ctx, &ev)
	h.tel.Emit(ev)
}

// evaluate reports whether the chunk's source file has drifted past the
// chunk's provenance: the file is gone, or it was modified after the chunk
// was compiled. Chunks without a file path are never file-stale.
func (h *refreshHandler) evaluate(ctx context.Context, chunk KnowledgeChunk) (bool, error) {
	raw, ok := chunk.Body.Fields["file_path"]
	if !ok {
		return false, nil
	}
	rel, ok := raw.(string)
	if !ok || strings.TrimSpace(rel) == "" {
		return false, nil
	}
	path := rel
	if !filepath.IsAbs(path) && h.workspaceRoot != "" {
		path = filepath.Join(h.workspaceRoot, rel)
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err // unreadable: leave freshness to the next sweep
	}
	if !chunk.Provenance.Timestamp.IsZero() && info.ModTime().After(chunk.Provenance.Timestamp) {
		return true, nil
	}
	return false, nil
}
