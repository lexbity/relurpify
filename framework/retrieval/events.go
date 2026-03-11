package retrieval

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// RetrievalEvent records what the retriever found and why candidates were excluded.
type RetrievalEvent struct {
	QueryID          string            `json:"query_id"`
	Query            string            `json:"query"`
	FilterSummary    string            `json:"filter_summary"`
	SparseCandidates int               `json:"sparse_candidates"`
	DenseCandidates  int               `json:"dense_candidates"`
	FusedCandidates  int               `json:"fused_candidates"`
	ExcludedReasons  map[string]string `json:"excluded_reasons"`
	CacheTier        string            `json:"cache_tier,omitempty"`
	Timestamp        time.Time         `json:"timestamp"`
}

// PackingEvent records what was actually injected into context.
type PackingEvent struct {
	QueryID        string            `json:"query_id"`
	InjectedChunks []string          `json:"injected_chunks"`
	DroppedChunks  map[string]string `json:"dropped_chunks"`
	TokenBudget    int               `json:"token_budget"`
	TokensUsed     int               `json:"tokens_used"`
	Timestamp      time.Time         `json:"timestamp"`
}

func persistRetrievalEvent(ctx context.Context, db *sql.DB, event RetrievalEvent) error {
	if db == nil {
		return errors.New("retrieval event db required")
	}
	excludedJSON, err := json.Marshal(event.ExcludedReasons)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO retrieval_events
		(query_id, query_text, filter_summary, sparse_candidates, dense_candidates, fused_candidates, excluded_reasons_json, cache_tier, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(query_id) DO UPDATE SET
			query_text = excluded.query_text,
			filter_summary = excluded.filter_summary,
			sparse_candidates = excluded.sparse_candidates,
			dense_candidates = excluded.dense_candidates,
			fused_candidates = excluded.fused_candidates,
			excluded_reasons_json = excluded.excluded_reasons_json,
			cache_tier = excluded.cache_tier,
			created_at = excluded.created_at`,
		event.QueryID, event.Query, event.FilterSummary, event.SparseCandidates, event.DenseCandidates, event.FusedCandidates,
		string(excludedJSON), event.CacheTier, event.Timestamp.Format(time.RFC3339Nano),
	)
	return err
}

func persistPackingEvent(ctx context.Context, db *sql.DB, event PackingEvent) error {
	if db == nil {
		return errors.New("packing event db required")
	}
	injectedJSON, err := json.Marshal(event.InjectedChunks)
	if err != nil {
		return err
	}
	droppedJSON, err := json.Marshal(event.DroppedChunks)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO retrieval_packing_events
		(query_id, injected_chunks_json, dropped_chunks_json, token_budget, tokens_used, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(query_id) DO UPDATE SET
			injected_chunks_json = excluded.injected_chunks_json,
			dropped_chunks_json = excluded.dropped_chunks_json,
			token_budget = excluded.token_budget,
			tokens_used = excluded.tokens_used,
			created_at = excluded.created_at`,
		event.QueryID, string(injectedJSON), string(droppedJSON), event.TokenBudget, event.TokensUsed,
		event.Timestamp.Format(time.RFC3339Nano),
	)
	return err
}

func emitRetrievalTelemetry(telemetry core.Telemetry, retrievalEvent RetrievalEvent, packingEvent *PackingEvent) {
	if telemetry == nil {
		return
	}
	metadata := map[string]any{
		"query_id":          retrievalEvent.QueryID,
		"query":             retrievalEvent.Query,
		"filter_summary":    retrievalEvent.FilterSummary,
		"sparse_candidates": retrievalEvent.SparseCandidates,
		"dense_candidates":  retrievalEvent.DenseCandidates,
		"fused_candidates":  retrievalEvent.FusedCandidates,
		"excluded_reasons":  retrievalEvent.ExcludedReasons,
		"cache_tier":        retrievalEvent.CacheTier,
	}
	if packingEvent != nil {
		metadata["injected_chunks"] = packingEvent.InjectedChunks
		metadata["dropped_chunks"] = packingEvent.DroppedChunks
		metadata["token_budget"] = packingEvent.TokenBudget
		metadata["tokens_used"] = packingEvent.TokensUsed
	}
	telemetry.Emit(core.Event{
		Type:      core.EventCapabilityResult,
		Message:   "retrieval completed",
		Timestamp: retrievalEvent.Timestamp,
		Metadata:  metadata,
	})
}

func newQueryID(query RetrievalQuery, now time.Time) string {
	key := fmt.Sprintf("%s|%s|%s|%d", strings.Join(query.SourceTypes, ","), normalizeScope(query.Scope), query.Text, now.UnixNano())
	return "rq:" + shortStableHash(key)
}
