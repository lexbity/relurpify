package runtime

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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

var (
	// ErrReceiptConflict means the request identity was already used for another payload or target.
	ErrReceiptConflict = errors.New("lifecycle: receipt conflict")
	// ErrReceiptExpired means an unknown request is outside the admission window.
	ErrReceiptExpired = errors.New("lifecycle: receipt expired")
	// ErrJournalUnavailable means admission cannot establish authoritative state.
	ErrJournalUnavailable = errors.New("lifecycle: journal unavailable")
)

type receiptRecord struct {
	RequestID      RequestID     `json:"request_id"`
	Digest         string        `json:"digest"`
	Target         string        `json:"target"`
	AdmittedAt     time.Time     `json:"admitted_at"`
	ProtectedUntil time.Time     `json:"protected_until"`
	Status         ReceiptStatus `json:"status"`
	OperationID    string        `json:"operation_id"`
}

// ReceiptStatus is the persisted command admission result.
type ReceiptStatus string

// ReceiptStatusAccepted records storage admission, not dispatch or completion.
const ReceiptStatusAccepted ReceiptStatus = "accepted"

// SubmissionOutcome identifies a persisted admission and any conflicting reuse.
type SubmissionOutcome struct {
	Status      ReceiptStatus
	Receipt     receiptRecord
	OperationID string
	Conflict    bool
}

// JournalAdapter stores command admissions. It does not own or dispatch running
// executions. The runtime-owned dispatcher and durable event cursor required by
// P02 are not implemented by this storage primitive.
type JournalAdapter struct {
	db                 *graphdb.Engine
	workspace          WorkspaceID
	mu                 sync.Mutex
	lastAdmissionClock time.Time
	now                func() time.Time
}

// NewJournalAdapter binds admissions to one workspace namespace.
func NewJournalAdapter(db *graphdb.Engine, workspace WorkspaceID) *JournalAdapter {
	return &JournalAdapter{db: db, workspace: workspace, now: time.Now}
}

// generateRequestID returns a UUIDv7 request ID (RFC 9562 §5.7).
func generateRequestID() RequestID {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Sprintf("lifecycle: request ID entropy unavailable: %v", err))
	}
	var timestamp [8]byte
	binary.BigEndian.PutUint64(timestamp[:], uint64(time.Now().UnixMilli()))
	copy(raw[0:6], timestamp[2:8])
	raw[6] = (raw[6] & 0x0f) | 0x70
	raw[8] = (raw[8] & 0x3f) | 0x80
	return RequestID("req_" + hex.EncodeToString(raw[:]))
}

func (ja *JournalAdapter) receiptKey(id RequestID) string {
	return "admission:" + base64.RawURLEncoding.EncodeToString([]byte(ja.workspace)) + ":" + string(id)
}

// AdmitCommand commits a receipt, or returns the original receipt for an exact
// retry. Known receipts are checked before freshness and clock checks; different
// payloads always conflict, including after the admission window expires.
func (ja *JournalAdapter) AdmitCommand(ctx context.Context, id RequestID, digest, target string) (SubmissionOutcome, error) {
	outcome, _, err := ja.admit(ctx, id, digest, target, false)
	return outcome, err
}

func (ja *JournalAdapter) admit(ctx context.Context, id RequestID, digest, target string, withRun bool) (SubmissionOutcome, *agentlifecycle.WorkflowRunRecord, error) {
	ja.mu.Lock()
	defer ja.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return SubmissionOutcome{}, nil, err
	}
	if ja.db == nil || ja.workspace == "" {
		return SubmissionOutcome{}, nil, ErrJournalUnavailable
	}
	if digest == "" || target == "" || len(digest) > 256 || len(target) > 256 {
		return SubmissionOutcome{}, nil, errors.New("lifecycle: bounded digest and target required")
	}
	if id == "" {
		id = generateRequestID()
	}
	issued, ok := extractIssuanceTime(id)
	if !ok {
		return SubmissionOutcome{}, nil, fmt.Errorf("invalid request ID: %s", id)
	}
	now := ja.now().UTC()
	receipt := receiptRecord{RequestID: id, Digest: digest, Target: target, AdmittedAt: now, ProtectedUntil: now.Add(10 * time.Minute), Status: ReceiptStatusAccepted, OperationID: string(id)}
	var run *agentlifecycle.WorkflowRunRecord
	if withRun {
		run = &agentlifecycle.WorkflowRunRecord{RunID: graphdb.GenerateID("run"), WorkflowID: string(ja.workspace), Status: "queued", StartedAt: now}
		receipt.OperationID = run.RunID
	}
	encoded, err := encodeReceipt(receipt)
	if err != nil {
		return SubmissionOutcome{}, nil, err
	}
	nodes := []graphdb.NodeRecord{{ID: ja.receiptKey(id), Kind: "receipt", Props: encoded}}
	if run != nil {
		data, err := json.Marshal(run)
		if err != nil {
			return SubmissionOutcome{}, nil, err
		}
		nodes = append(nodes, graphdb.NodeRecord{ID: run.RunID, Kind: "run", Props: data})
	}
	var decisionErr error
	var existing bool
	// Receipt and run commit together. Validation is also the authoritative read
	// barrier for exact retries, avoiding cache reads after an indeterminate write.
	err = ja.db.ApplyBatchValidated(ctx, graphdb.GraphBatch{Nodes: nodes}, func(get graphdb.NodeGetter) error {
		if node, found := get(ja.receiptKey(id)); found {
			if node.Kind != "receipt" {
				decisionErr = ErrJournalUnavailable
				return decisionErr
			}
			known, _, decodeErr := decodeReceipt(node.Props)
			if decodeErr != nil || known.RequestID != id || known.OperationID == "" {
				decisionErr = ErrJournalUnavailable
				return decisionErr
			}
			if known.Digest != digest || known.Target != target {
				decisionErr = ErrReceiptConflict
				return decisionErr
			}
			if withRun {
				record, found := get(known.OperationID)
				if !found || record.Kind != "run" {
					decisionErr = ErrJournalUnavailable
					return decisionErr
				}
				var stored agentlifecycle.WorkflowRunRecord
				if json.Unmarshal(record.Props, &stored) != nil || stored.RunID != known.OperationID || stored.WorkflowID != string(ja.workspace) {
					decisionErr = ErrJournalUnavailable
					return decisionErr
				}
				run = &stored
			}
			receipt, existing = known, true
			return graphdb.ErrBatchConflict // read-only retry: never write the candidate
		}
		if now.Sub(issued) > 5*time.Minute || issued.After(now.Add(30*time.Second)) {
			decisionErr = ErrReceiptExpired
			return decisionErr
		}
		if !ja.lastAdmissionClock.IsZero() && now.Before(ja.lastAdmissionClock.Add(-30*time.Second)) {
			decisionErr = errors.New("lifecycle: admission clock moved backward")
			return decisionErr
		}
		if run != nil {
			if _, found := get(run.RunID); found {
				decisionErr = ErrJournalUnavailable
				return decisionErr
			}
		}
		return nil
	})
	if decisionErr != nil {
		return SubmissionOutcome{Conflict: errors.Is(decisionErr, ErrReceiptConflict)}, nil, decisionErr
	}
	if err != nil && (!existing || !errors.Is(err, graphdb.ErrBatchConflict)) {
		return SubmissionOutcome{}, nil, err
	}
	if !existing && now.After(ja.lastAdmissionClock) {
		ja.lastAdmissionClock = now
	}
	return SubmissionOutcome{Status: receipt.Status, Receipt: receipt, OperationID: receipt.OperationID}, run, nil
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

// RunAdmissionStore atomically stores run intent with its receipt. It is a
// storage primitive, not a workspace actor: it starts no goroutines, dispatches
// no effects, and has no second in-memory run authority to reconcile.
type RunAdmissionStore struct{ journal *JournalAdapter }

// NewRunAdmissionStore binds run admissions to a workspace and graph engine.
func NewRunAdmissionStore(workspace WorkspaceID, db *graphdb.Engine) *RunAdmissionStore {
	return &RunAdmissionStore{journal: NewJournalAdapter(db, workspace)}
}

// AdmitRun returns the same persisted run for every exact retry, including after
// reopening. Receipt admission and queued run creation are one graph transaction.
func (s *RunAdmissionStore) AdmitRun(ctx context.Context, id RequestID, digest string) (*agentlifecycle.WorkflowRunRecord, error) {
	_, run, err := s.journal.admit(ctx, id, digest, "run", true)
	return run, err
}
