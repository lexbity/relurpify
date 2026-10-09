package orchestrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
)

// Selection Decision Record persistence (D11, §3.6.4). Every dispatch leaves a
// redacted, workflow-scoped, queryable decision record; the write is
// synchronous under a 50 ms budget and failure never blocks the selection.
//
// Redaction is structural: the record has no utterance field. It carries a
// SHA-256 digest of the whitespace-normalized lowercase text plus the
// candidate-side matched keywords (public registry data) and the per-candidate
// component split.

const (
	// selectionPersistBudget is the synchronous write budget for one record
	// (FR-14 / NFR-4). On expiry the selection proceeds and
	// euclo.route.selection_persist_failed fires with budget_exceeded.
	selectionPersistBudget = 50 * time.Millisecond
	// selectionUpdateBudget bounds the route-completion state transition.
	// It is a failure-tolerant second write: on failure the record stays
	// "dispatched", which is truthful for an interrupted run.
	selectionUpdateBudget = 50 * time.Millisecond
)

// SelectionRecorder persists Selection Decision Records. A recorder without a
// lifecycle repository is a declared degraded mode: dispatch proceeds and no
// record is written (the record id key simply stays absent from the envelope).
type SelectionRecorder struct {
	lifecycle contextports.LifecycleRepository
}

// NewSelectionRecorder creates a recorder backed by the durable lifecycle
// repository. A nil repo yields a recorder whose writes are no-ops.
func NewSelectionRecorder(repo contextports.LifecycleRepository) *SelectionRecorder {
	return &SelectionRecorder{lifecycle: repo}
}

// configured reports whether the recorder can write records.
func (r *SelectionRecorder) configured() bool {
	return r != nil && r.lifecycle != nil
}

// persist builds and synchronously writes the selection decision record with
// the 50 ms budget (D11). On failure the selection proceeds and
// euclo.route.selection_persist_failed fires with the error class. On success
// the record id lands on the envelope so execution nodes can transition its
// execution_state on route completion. persist is called exactly once per
// dispatch, after selection (including any bounded Tier-2 consultation) and
// before route execution begins.
func (r *SelectionRecorder) persist(ctx context.Context, env *contextdata.Envelope, req RouteRequest, report *DryRunReport, selected CandidateRouteInfo, fallbackTaken bool) {
	if !r.configured() || report == nil {
		return
	}
	record := buildSelectionRecord(env, req, report, selected, fallbackTaken)
	decisionID, err := r.write(ctx, func(budgetCtx context.Context) (string, error) {
		return r.lifecycle.RecordSelectionDecision(budgetCtx, record)
	})
	if err != nil {
		if !req.TelemetryOff {
			reporting.EmitRouteSelectionPersistFailed(ctx, taskID(env), sessionID(env), decisionID, selectionPersistErrorClass(err))
		}
		return
	}
	if env != nil {
		euclostate.SetSelectionRecordID(env, decisionID)
	}
}

// updateExecutionState transitions the run's selection record to the final
// execution state once the route has run (D11). A nil/empty record id (no
// record written, or a non-persisted dispatch) is a no-op.
func (r *SelectionRecorder) updateExecutionState(ctx context.Context, env *contextdata.Envelope, success bool) {
	if !r.configured() || env == nil {
		return
	}
	decisionID, ok := euclostate.GetSelectionRecordID(env)
	if !ok || strings.TrimSpace(decisionID) == "" {
		return
	}
	state := contextports.SelectionExecutionStateCompleted
	if !success {
		state = contextports.SelectionExecutionStateFailed
	}
	_, _ = r.write(ctx, func(budgetCtx context.Context) (string, error) {
		return decisionID, r.lifecycle.UpdateSelectionExecutionState(budgetCtx, decisionID, state)
	})
}

// write runs fn under the record-write budget and returns its result. The
// budget bounds the synchronous store write rather than the caller's context:
// selection is never blocked on an unhealthy store beyond the budget.
func (r *SelectionRecorder) write(ctx context.Context, fn func(context.Context) (string, error)) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	budgetCtx, cancel := context.WithTimeout(ctx, selectionPersistBudget)
	defer cancel()
	return fn(budgetCtx)
}

// buildSelectionRecord constructs the redacted record from the deterministic
// report, the selected candidate, the tier-2 outcome, and the request. The raw
// utterance never enters the record.
func buildSelectionRecord(env *contextdata.Envelope, req RouteRequest, report *DryRunReport, selected CandidateRouteInfo, fallbackTaken bool) contextports.SelectionDecisionRecord {
	candidates := make([]contextports.SelectionDecisionCandidate, 0, len(report.Candidates))
	for _, candidate := range report.Candidates {
		candidates = append(candidates, contextports.SelectionDecisionCandidate{
			RouteID:         strings.TrimSpace(string(candidate.RouteID)),
			Kind:            strings.TrimSpace(candidate.RouteKind),
			Availability:    strings.TrimSpace(string(candidate.Availability)),
			Score:           candidate.RankScore,
			Components:      cloneComponentMap(candidate.Components),
			MatchedKeywords: append([]string(nil), candidate.MatchedKeywords...),
		})
	}
	degradation := ""
	if report.DecidedBy == decidedByDefaultRecipe {
		degradation = "default_recipe"
	}
	record := contextports.SelectionDecisionRecord{
		Schema:          contextports.SelectionDecisionSchema,
		WorkflowID:      workflowIDFromEnvelope(env),
		RunID:           runIDFromEnvelope(env),
		UtteranceDigest: utteranceDigestFor(env, req),
		TokenCount:      tokenCountFor(env, req),
		Family:          strings.TrimSpace(report.Request.FamilyID),
		Candidates:      candidates,
		Selected: contextports.SelectionDecisionSelected{
			RouteID: strings.TrimSpace(string(selected.RouteID)),
			Kind:    strings.TrimSpace(selected.RouteKind),
		},
		DecidedBy:      strings.TrimSpace(report.DecidedBy),
		FallbackTaken:  fallbackTaken,
		Degradation:    degradation,
		Tier2:          tier2ForRecord(report.Tier2),
		ExecutionState: contextports.SelectionExecutionStateDispatched,
		CreatedAt:      time.Now().UTC(),
	}
	return record
}

// tier2ForRecord maps the typed Tier-2 info onto the record's redacted tier-2
// block (D10).
func tier2ForRecord(tier2 euclotypes.Tier2Info) contextports.SelectionDecisionTier2 {
	return contextports.SelectionDecisionTier2{
		Used:        tier2.Used,
		Outcome:     strings.TrimSpace(tier2.Outcome),
		Model:       strings.TrimSpace(tier2.Model),
		LatencyMs:   tier2.LatencyMs,
		Confidence:  tier2.Confidence,
		CandidateID: strings.TrimSpace(tier2.CandidateID),
	}
}

func cloneComponentMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// utteranceDigestFor computes the redacted utterance digest: SHA-256 over the
// whitespace-normalized lowercase text, hex-encoded and prefixed with "sha256:".
// This is the only utterance trace a selection record or event may carry (D11).
func utteranceDigestFor(env *contextdata.Envelope, req RouteRequest) string {
	text := strings.Join(strings.Fields(strings.ToLower(utteranceForGate(env, req))), " ")
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// tokenCountFor counts the deterministic route-search tokens of the dispatch
// (the same vocabulary the scorers see), so token_count is reproducible.
func tokenCountFor(env *contextdata.Envelope, req RouteRequest) int {
	return len(routeSearchTokens(env, req))
}

// workflowIDFromEnvelope derives the workflow scope for the record. The
// composition root seeds the envelope's task identity; when the runtime tracks
// lifecycle records, its task ID is the workflow the dispatch belongs to. An
// empty result still yields a valid record (scoped by the record id itself).
func workflowIDFromEnvelope(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	return strings.TrimSpace(env.TaskIDSnapshot())
}

// runIDFromEnvelope derives the run scope for the record. The runtime
// correlates one turn's dispatch to its run via the session identity.
func runIDFromEnvelope(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	return strings.TrimSpace(env.SessionIDSnapshot())
}

// selectionPersistErrorClass maps a record-write error to a stable telemetry
// class for euclo.route.selection_persist_failed (budget_exceeded vs
// store_error). Provenance loss is loud, never blocking.
func selectionPersistErrorClass(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "budget_exceeded"
	}
	return "store_error"
}
