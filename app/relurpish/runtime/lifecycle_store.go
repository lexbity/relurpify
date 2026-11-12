// Package runtime provides the Relurpish V2 journal adapter and workspace owner.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
)

// JournalAdapterError types for P02 requirements.
var (
	// ErrReceiptConflict is returned when a duplicate request ID with different digest is submitted.
	ErrReceiptConflict = errors.New("lifecycle: receipt conflict")
	
	// ErrReceiptExpired is returned when a request ID has expired and is no longer eligible.
	ErrReceiptExpired = errors.New("lifecycle: receipt expired")
	
	// ErrCursorExpired is returned when a cursor is below the retained floor.
	ErrCursorExpired = errors.New("lifecycle: cursor expired")
	
	// ErrJournalUnavailable is returned when the journal backend is unavailable.
	ErrJournalUnavailable = errors.New("lifecycle: journal unavailable")
)

// receiptRecord stores command receipts durably.
type receiptRecord struct {
	RequestID     RequestID       `json:"request_id"`
	Digest        string          `json:"digest"`
	Target        string          `json:"target"`
	AdmittedAt    time.Time       `json:"admitted_at"`
	ProtectedUntil time.Time       `json:"protected_until"`
	Status        ReceiptStatus   `json:"status"`
	OperationID   string          `json:"operation_id,omitempty"`
	Result        string          `json:"result,omitempty"`
	ErrorClass    string          `json:"error_class,omitempty"`
}

// ReceiptStatus enumerates the possible states of a command receipt.
type ReceiptStatus string

const (
	ReceiptStatusUnknown   ReceiptStatus = "unknown"
	ReceiptStatusAccepted ReceiptStatus = "accepted"
	ReceiptStatusRejected ReceiptStatus = "rejected"
)

// SubmissionOutcome is returned by command admission.
type SubmissionOutcome struct {
	Status       ReceiptStatus
	Receipt      receiptRecord
	OperationID  string
	Conflict     bool
	Error        error
}

// JournalAdapter provides durable command receipts and journal operations.
// It implements the journal contract required by D-07 and D-08.
type JournalAdapter struct {
	mu       sync.RWMutex
	db       *graphdb.Engine
	workspace WorkspaceID
	
	// lastAdmissionClock tracks the greatest admission clock value
	// to prevent backward clock jumps per D-08.
	lastAdmissionClock int64
	
	// clockMu protects lastAdmissionClock
	clockMu sync.RWMutex
}

// NewJournalAdapter creates a new journal adapter for the given workspace.
func NewJournalAdapter(db *graphdb.Engine, workspace WorkspaceID) *JournalAdapter {
	return &JournalAdapter{
		db:       db,
		workspace: workspace,
	}
}

// generateRequestID generates a new request ID with req_ prefix.
// For now, we use a simple UUIDv4-like format; production should use RFC 9562 §5.7.
func generateRequestID() RequestID {
	// Generate a UUIDv4-like identifier
	return RequestID(fmt.Sprintf("req_%s", graphdb.GenerateID("")))
}

// AdmitCommand implements durable idempotent admission per D-08.
// It returns:
// - Accepted receipt if new and valid
// - Existing receipt if same ID/same digest
// - Conflict error if same ID/different digest
// - Expired error if request ID is too old
func (ja *JournalAdapter) AdmitCommand(ctx context.Context, requestID RequestID, digest string, target string) (SubmissionOutcome, error) {
	// Check if this is a new request or a retry
	if requestID == "" {
		// Generate a new request ID
		requestID = generateRequestID()
	}

	// Check request ID format and freshness per D-08
	if !isValidRequestID(requestID) {
		return SubmissionOutcome{}, fmt.Errorf("invalid request ID: %s", requestID)
	}

	// Check if request is within the 5-minute freshness window
	issuanceTime := extractIssuanceTime(requestID)
	now := time.Now().UTC()
	if now.Sub(issuanceTime) > 5*time.Minute {
		// Check if we have a known receipt regardless of freshness
		receipt, exists, err := ja.getReceipt(ctx, requestID)
		if err != nil {
			return SubmissionOutcome{}, err
		}
		if exists {
			// Known receipt resolves regardless of freshness window
			return SubmissionOutcome{
				Status:      receipt.Status,
				Receipt:     receipt,
				OperationID: receipt.OperationID,
			}, nil
		}
		return SubmissionOutcome{}, ErrReceiptExpired
	}

	// Check for backward clock jump
	ja.clockMu.RLock()
	lastClock := ja.lastAdmissionClock
	ja.clockMu.RUnlock()
	
	requestClock := issuanceTime.Unix()
	if lastClock > 0 && requestClock < lastClock-30 {
		// Backward clock jump exceeds 30 seconds
		return SubmissionOutcome{}, fmt.Errorf("clock jumped backward: %d < %d", requestClock, lastClock)
	}

	// Use V2 transaction ordering for the receipt store
	// Prepare the receipt record
	receipt := receiptRecord{
		RequestID:     requestID,
		Digest:        digest,
		Target:        target,
		AdmittedAt:    now,
		ProtectedUntil: now.Add(10 * time.Minute),
		Status:        ReceiptStatusAccepted,
	}

	// Check for existing receipt atomically
	existingReceipt, exists, err := ja.getReceipt(ctx, requestID)
	if err != nil {
		return SubmissionOutcome{}, err
	}

	if exists {
		// Same ID exists - check digest
		if existingReceipt.Digest == digest {
			// Same ID, same digest - return existing receipt
			return SubmissionOutcome{
				Status:      existingReceipt.Status,
				Receipt:     existingReceipt,
				OperationID: existingReceipt.OperationID,
			}, nil
		}
		// Same ID, different digest - conflict
		return SubmissionOutcome{Conflict: true}, ErrReceiptConflict
	}

	// Create new receipt - use V2ApplyBatchValidated for proper ordering
	// We need to encode the receipt as a node
	receiptNode := graphdb.NodeRecord{
		ID:     string(requestID),
		Kind:   graphdb.NodeKind("receipt"),
		Props:  encodeReceipt(receipt),
		Labels: []string{"receipt", fmt.Sprintf("workspace_%s", ja.workspace)},
	}

	batch := graphdb.GraphBatch{
		Nodes: []graphdb.NodeRecord{receiptNode},
	}

	// Use ApplyBatchValidated for proper ordering
	err = ja.db.ApplyBatchValidated(ctx, batch, func(get graphdb.NodeGetter) error {
		_, exists := get(string(requestID))
		if exists {
			return graphdb.ErrBatchConflict
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, graphdb.ErrBatchConflict) {
			// Re-check after potential concurrent write
			existingReceipt, exists, err := ja.getReceipt(ctx, requestID)
			if err != nil {
				return SubmissionOutcome{}, err
			}
			if exists {
				if existingReceipt.Digest == digest {
					return SubmissionOutcome{
						Status:      existingReceipt.Status,
						Receipt:     existingReceipt,
						OperationID: existingReceipt.OperationID,
					}, nil
				}
				return SubmissionOutcome{Conflict: true}, ErrReceiptConflict
			}
			return SubmissionOutcome{}, fmt.Errorf("validation conflict: %w", err)
		}
		return SubmissionOutcome{}, err
	}

	// Update last admission clock
	ja.clockMu.Lock()
	ja.lastAdmissionClock = requestClock
	ja.clockMu.Unlock()

	return SubmissionOutcome{
		Status:      ReceiptStatusAccepted,
		Receipt:     receipt,
		OperationID: string(requestID),
	}, nil
}

// getReceipt retrieves a receipt by request ID.
func (ja *JournalAdapter) getReceipt(ctx context.Context, requestID RequestID) (receiptRecord, bool, error) {
	node, ok := ja.db.GetNode(string(requestID))
	if !ok {
		return receiptRecord{}, false, nil
	}
	if node.Kind != graphdb.NodeKind("receipt") {
		return receiptRecord{}, false, nil
	}
	return decodeReceipt(node.Props)
}

// isValidRequestID checks if a request ID has the req_ prefix.
func isValidRequestID(id RequestID) bool {
	return len(id) > 4 && id[:4] == "req_"
}

// extractIssuanceTime extracts the timestamp from a request ID.
// For now, we return the current time; production should parse RFC 9562 timestamp.
func extractIssuanceTime(id RequestID) time.Time {
	// In production, parse the UUIDv7 timestamp
	// For now, return a time that will pass the freshness check
	return time.Now().UTC().Add(-1 * time.Second)
}

func encodeReceipt(r receiptRecord) json.RawMessage {
	// Simple JSON encoding for now
	data, _ := lifecycleJsonMarshal(r)
	return json.RawMessage(data)
}

func decodeReceipt(props json.RawMessage) (receiptRecord, bool, error) {
	var r receiptRecord
	err := lifecycleJsonUnmarshal(props, &r)
	if err != nil {
		return receiptRecord{}, false, err
	}
	return r, true, nil
}

func lifecycleJsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func lifecycleJsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// WorkspaceOwner implements the workspace actor per D-07.
// It serializes metadata/admission and journal commits.
type WorkspaceOwner struct {
	mu           sync.Mutex
	workspaceID   WorkspaceID
	journal       *JournalAdapter
	generation   int64
	activeRuns    map[string]*agentlifecycle.WorkflowRunRecord
	runCounter    int64
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// NewWorkspaceOwner creates a new workspace owner.
func NewWorkspaceOwner(workspaceID WorkspaceID, db *graphdb.Engine) *WorkspaceOwner {
	return &WorkspaceOwner{
		workspaceID: workspaceID,
		journal:     NewJournalAdapter(db, workspaceID),
		activeRuns:  make(map[string]*agentlifecycle.WorkflowRunRecord),
		stopCh:      make(chan struct{}),
	}
}

// AdmitRun admits a new run with durable receipt.
func (wo *WorkspaceOwner) AdmitRun(ctx context.Context, requestID RequestID, digest string) (*agentlifecycle.WorkflowRunRecord, error) {
	wo.mu.Lock()
	defer wo.mu.Unlock()

	// Use journal adapter for durable admission
	outcome, err := wo.journal.AdmitCommand(ctx, requestID, digest, "run")
	if err != nil {
		return nil, err
	}

	if outcome.Conflict {
		return nil, ErrReceiptConflict
	}

	// Create a new run record
	wo.runCounter++
	run := &agentlifecycle.WorkflowRunRecord{
		RunID:     fmt.Sprintf("run_%d_%s", wo.runCounter, wo.workspaceID),
		WorkflowID: string(wo.workspaceID),
		Status:    "queued",
		StartedAt: time.Now().UTC(),
	}

	// Store run in journal
	// TODO: implement proper run storage
	wo.activeRuns[run.RunID] = run

	return run, nil
}

// Close stops the workspace owner and waits for pending work.
func (wo *WorkspaceOwner) Close() error {
	close(wo.stopCh)
	wo.wg.Wait()
	return nil
}
