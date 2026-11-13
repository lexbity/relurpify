// Package runtime provides the Relurpish V2 journal adapter and workspace owner.
package runtime

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
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
	RequestID      RequestID     `json:"request_id"`
	Digest         string        `json:"digest"`
	Target         string        `json:"target"`
	AdmittedAt     time.Time     `json:"admitted_at"`
	ProtectedUntil time.Time     `json:"protected_until"`
	Status         ReceiptStatus `json:"status"`
	OperationID    string        `json:"operation_id,omitempty"`
	Result         string        `json:"result,omitempty"`
	ErrorClass     string        `json:"error_class,omitempty"`
}

// ReceiptStatus enumerates the possible states of a command receipt.
type ReceiptStatus string

// Receipt status values for admitted commands.
const (
	ReceiptStatusUnknown  ReceiptStatus = "unknown"
	ReceiptStatusAccepted ReceiptStatus = "accepted"
	ReceiptStatusRejected ReceiptStatus = "rejected"
)

// SubmissionOutcome is returned by command admission.
type SubmissionOutcome struct {
	Status      ReceiptStatus
	Receipt     receiptRecord
	OperationID string
	Conflict    bool
	Error       error
}

// JournalAdapter provides durable command receipts and journal operations.
// It implements the journal contract required by D-07 and D-08.
type JournalAdapter struct {
	db        *graphdb.Engine
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
		db:        db,
		workspace: workspace,
	}
}

// generateRequestID generates a UUIDv7 request ID with the req_ prefix per
// RFC 9562 §5.7 and D-08.
func generateRequestID() RequestID {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Sprintf("lifecycle: request ID entropy unavailable: %v", err))
	}
	var timestamp [8]byte
	binary.BigEndian.PutUint64(timestamp[:], uint64(time.Now().UnixMilli()))
	copy(raw[0:6], timestamp[2:8])
	raw[6] = (raw[6] & 0x0f) | 0x70 // version 7
	raw[8] = (raw[8] & 0x3f) | 0x80 // RFC 9562 variant
	return RequestID("req_" + hex.EncodeToString(raw[:]))
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

	// Check if request is within the 5-minute freshness window and the
	// 30-second future-skew allowance (D-08).
	issuanceTime, ok := extractIssuanceTime(requestID)
	if !ok {
		return SubmissionOutcome{}, fmt.Errorf("invalid request ID: %s", requestID)
	}
	now := time.Now().UTC()
	if now.Sub(issuanceTime) > 5*time.Minute || issuanceTime.After(now.Add(30*time.Second)) {
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
		RequestID:      requestID,
		Digest:         digest,
		Target:         target,
		AdmittedAt:     now,
		ProtectedUntil: now.Add(10 * time.Minute),
		Status:         ReceiptStatusAccepted,
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

	// Create new receipt and encode it as a graph node.
	encoded, err := encodeReceipt(receipt)
	if err != nil {
		return SubmissionOutcome{}, err
	}
	receiptNode := graphdb.NodeRecord{
		ID:     string(requestID),
		Kind:   graphdb.NodeKind("receipt"),
		Props:  encoded,
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

// isValidRequestID checks that a request ID has the req_ prefix, a 128-bit
// hex payload, UUID version 7, and the RFC 9562 variant.
func isValidRequestID(id RequestID) bool {
	s := string(id)
	if len(s) != 4+32 || s[:4] != "req_" {
		return false
	}
	raw, err := hex.DecodeString(s[4:])
	if err != nil || len(raw) != 16 {
		return false
	}
	return raw[6]>>4 == 7 && raw[8]&0xc0 == 0x80
}

// extractIssuanceTime decodes the UUIDv7 millisecond timestamp from a request ID.
func extractIssuanceTime(id RequestID) (time.Time, bool) {
	s := string(id)
	if !isValidRequestID(id) {
		return time.Time{}, false
	}
	raw, err := hex.DecodeString(s[4:])
	if err != nil {
		return time.Time{}, false
	}
	var timestamp [8]byte
	copy(timestamp[2:], raw[0:6])
	ms := binary.BigEndian.Uint64(timestamp[:])
	return time.UnixMilli(int64(ms)).UTC(), true //nolint:gosec // 48-bit UUIDv7 timestamp fits int64
}

func encodeReceipt(r receiptRecord) (json.RawMessage, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("encode receipt %s: %w", r.RequestID, err)
	}
	return json.RawMessage(data), nil
}

func decodeReceipt(props json.RawMessage) (receiptRecord, bool, error) {
	var r receiptRecord
	if err := json.Unmarshal(props, &r); err != nil {
		return receiptRecord{}, false, err
	}
	return r, true, nil
}

// WorkspaceOwner implements the workspace actor per D-07.
// It serializes metadata/admission and journal commits.
type WorkspaceOwner struct {
	mu          sync.Mutex
	workspaceID WorkspaceID
	journal     *JournalAdapter
	activeRuns  map[string]*agentlifecycle.WorkflowRunRecord
	stopCh      chan struct{}
	wg          sync.WaitGroup
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
	run := &agentlifecycle.WorkflowRunRecord{
		RunID:      graphdb.GenerateID("run"),
		WorkflowID: string(wo.workspaceID),
		Status:     "queued",
		StartedAt:  time.Now().UTC(),
	}

	// Persist the run durably before publishing it in memory. A failed commit
	// must not leave an admitted run that only exists in this process.
	encoded, err := json.Marshal(run)
	if err != nil {
		return nil, fmt.Errorf("encode run %s: %w", run.RunID, err)
	}
	node := graphdb.NodeRecord{
		ID:     run.RunID,
		Kind:   graphdb.NodeKind("run"),
		Props:  json.RawMessage(encoded),
		Labels: []string{"run", fmt.Sprintf("workspace_%s", wo.workspaceID)},
	}
	err = wo.journal.db.ApplyBatchValidated(ctx, graphdb.GraphBatch{Nodes: []graphdb.NodeRecord{node}}, func(get graphdb.NodeGetter) error {
		if _, exists := get(run.RunID); exists {
			return graphdb.ErrBatchConflict
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("persist run %s: %w", run.RunID, err)
	}

	wo.activeRuns[run.RunID] = run

	return run, nil
}

// Close stops the workspace owner and waits for pending work.
func (wo *WorkspaceOwner) Close() error {
	close(wo.stopCh)
	wo.wg.Wait()
	return nil
}
