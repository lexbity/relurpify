package reconcile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	StatusVerified       Status = "verified"
	StatusRepaired       Status = "repaired"
	StatusOperatorReview Status = "operator_review"
	StatusTerminal       Status = "terminal"
)

type Outcome string

const (
	OutcomeVerified Outcome = "verified"
	OutcomeRepaired Outcome = "repaired"
	OutcomeOperator Outcome = "operator_review"
	OutcomeTerminal Outcome = "terminal"
)

type Record struct {
	ID              string    `json:"id"`
	WorkflowID      string    `json:"workflow_id"`
	RunID           string    `json:"run_id,omitempty"`
	Reason          string    `json:"reason"`
	Status          Status    `json:"status"`
	Ambiguous       bool      `json:"ambiguous"`
	SuppressRetry   bool      `json:"suppress_retry"`
	RepairSummary   string    `json:"repair_summary,omitempty"`
	ResolutionNotes string    `json:"resolution_notes,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Reconciler is the v2 rex reconciliation contract.
type Reconciler interface {
	RecordAmbiguity(workflowID, runID, reason string) Record
	Resolve(record Record, outcome Outcome, notes string) Record
	ShouldRetry(record Record) bool
}

// InMemoryReconciler provides local semantics for ambiguity handling.
type InMemoryReconciler struct {
	mu      sync.RWMutex
	records map[string]Record
}

func (r *InMemoryReconciler) RecordAmbiguity(workflowID, runID, reason string) Record {
	r.ensure()
	now := time.Now().UTC()
	record := Record{
		ID:            reconcileID(workflowID, runID, reason),
		WorkflowID:    workflowID,
		RunID:         runID,
		Reason:        reason,
		Status:        StatusOperatorReview,
		Ambiguous:     true,
		SuppressRetry: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	r.mu.Lock()
	r.records[record.ID] = record
	r.mu.Unlock()
	return record
}

func (r *InMemoryReconciler) Resolve(record Record, outcome Outcome, notes string) Record {
	r.ensure()
	record.ResolutionNotes = notes
	record.UpdatedAt = time.Now().UTC()
	switch outcome {
	case OutcomeVerified:
		record.Status = StatusVerified
		record.SuppressRetry = false
	case OutcomeRepaired:
		record.Status = StatusRepaired
		record.SuppressRetry = false
		record.RepairSummary = notes
	case OutcomeOperator:
		record.Status = StatusOperatorReview
		record.SuppressRetry = true
	case OutcomeTerminal:
		record.Status = StatusTerminal
		record.SuppressRetry = true
	default:
		record.Status = StatusTerminal
		record.SuppressRetry = true
	}
	r.mu.Lock()
	r.records[record.ID] = record
	r.mu.Unlock()
	return record
}

func (r *InMemoryReconciler) ShouldRetry(record Record) bool {
	return !record.SuppressRetry && (record.Status == StatusVerified || record.Status == StatusRepaired)
}

func (r *InMemoryReconciler) Get(recordID string) (Record, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.records[recordID]
	return record, ok
}

type ProtectedWrite struct {
	Resource string `json:"resource"`
	Token    uint64 `json:"token"`
}

type ProtectedWriter interface {
	Reserve(context.Context, string) (ProtectedWrite, error)
	Validate(ProtectedWrite, ProtectedWrite) error
}

type InMemoryProtectedWriter struct {
	mu     sync.Mutex
	tokens map[string]uint64
}

func (w *InMemoryProtectedWriter) Reserve(_ context.Context, resource string) (ProtectedWrite, error) {
	resource = normalizeResource(resource)
	if resource == "" {
		return ProtectedWrite{}, errors.New("protected write resource required")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.tokens == nil {
		w.tokens = map[string]uint64{}
	}
	w.tokens[resource]++
	return ProtectedWrite{Resource: resource, Token: w.tokens[resource]}, nil
}

func (w *InMemoryProtectedWriter) Validate(expected, received ProtectedWrite) error {
	return ValidateProtectedWrite(expected, received)
}

func ValidateProtectedWrite(expected, received ProtectedWrite) error {
	if normalizeResource(expected.Resource) == "" || normalizeResource(received.Resource) == "" {
		return errors.New("protected write resource required")
	}
	if normalizeResource(expected.Resource) != normalizeResource(received.Resource) || received.Token < expected.Token {
		return errors.New("stale fencing token")
	}
	return nil
}

type OutboxIntent struct {
	Key        string         `json:"key"`
	WorkflowID string         `json:"workflow_id,omitempty"`
	RunID      string         `json:"run_id,omitempty"`
	Kind       string         `json:"kind,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type Outbox interface {
	Append(context.Context, OutboxIntent) error
	List(context.Context, string) ([]OutboxIntent, error)
}

type InMemoryOutbox struct {
	mu      sync.RWMutex
	intents map[string][]OutboxIntent
}

func (o *InMemoryOutbox) Append(_ context.Context, intent OutboxIntent) error {
	intent.Key = normalizeResource(intent.Key)
	if intent.Key == "" {
		return fmt.Errorf("outbox key required")
	}
	if intent.CreatedAt.IsZero() {
		intent.CreatedAt = time.Now().UTC()
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.intents == nil {
		o.intents = map[string][]OutboxIntent{}
	}
	o.intents[intent.Key] = append(o.intents[intent.Key], intent)
	return nil
}

func (o *InMemoryOutbox) List(_ context.Context, key string) ([]OutboxIntent, error) {
	key = normalizeResource(key)
	o.mu.RLock()
	defer o.mu.RUnlock()
	intents := o.intents[key]
	out := make([]OutboxIntent, len(intents))
	copy(out, intents)
	return out, nil
}

func reconcileID(workflowID, runID, reason string) string {
	return fmt.Sprintf("%s:%s:%s", normalizeResource(workflowID), normalizeResource(runID), normalizeResource(reason))
}

func normalizeResource(value string) string {
	return strings.TrimSpace(value)
}

func (r *InMemoryReconciler) ensure() {
	if r.records != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.records == nil {
		r.records = map[string]Record{}
	}
}
