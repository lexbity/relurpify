package fmp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

)

// OwnershipStore is part of the Phase 1 frozen FMP surface.
// Later phases should swap the in-memory implementation for a durable
// coordination-backed store behind this interface.
type OwnershipStore interface {
	CreateLineage(ctx context.Context, lineage LineageRecord) error
	GetLineage(ctx context.Context, lineageID string) (*LineageRecord, bool, error)
	UpsertAttempt(ctx context.Context, attempt AttemptRecord) error
	GetAttempt(ctx context.Context, attemptID string) (*AttemptRecord, bool, error)
	IssueLease(ctx context.Context, lineageID, attemptID, issuer string, ttl time.Duration) (*LeaseToken, error)
	ValidateLease(ctx context.Context, lease LeaseToken, now time.Time) error
	CommitHandoff(ctx context.Context, commit ResumeCommit) error
	Fence(ctx context.Context, notice FenceNotice) error
}

type HandoffOfferReservation struct {
	LineageID            string    `json:"lineage_id"`
	OfferID              string    `json:"offer_id"`
	LeaseID              string    `json:"lease_id"`
	SourceAttemptID      string    `json:"source_attempt_id"`
	FencingEpoch         int64     `json:"fencing_epoch"`
	ProvisionalAttemptID string    `json:"provisional_attempt_id"`
	RuntimeID            string    `json:"runtime_id"`
	CreatedAt            time.Time `json:"created_at"`
}

type InMemoryOwnershipStore struct {
	mu             sync.RWMutex
	lineages       map[string]LineageRecord
	attempts       map[string]AttemptRecord
	leaseByLineage map[string]LeaseToken
	offerByID      map[string]HandoffOfferReservation
}

func (s *InMemoryOwnershipStore) ListLineages(_ context.Context) ([]LineageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LineageRecord, 0, len(s.lineages))
	for _, lineage := range s.lineages {
		out = append(out, lineage)
	}
	return out, nil
}

func (s *InMemoryOwnershipStore) CreateLineage(_ context.Context, lineage LineageRecord) error {
	if err := lineage.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lineages == nil {
		s.lineages = map[string]LineageRecord{}
	}
	if _, ok := s.lineages[lineage.LineageID]; ok {
		return fmt.Errorf("lineage %s already exists", lineage.LineageID)
	}
	s.lineages[lineage.LineageID] = lineage
	return nil
}

func (s *InMemoryOwnershipStore) GetLineage(_ context.Context, lineageID string) (*LineageRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lineage, ok := s.lineages[lineageID]
	if !ok {
		return nil, false, nil
	}
	copy := lineage
	return &copy, true, nil
}

func (s *InMemoryOwnershipStore) UpsertAttempt(_ context.Context, attempt AttemptRecord) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attempts == nil {
		s.attempts = map[string]AttemptRecord{}
	}
	s.attempts[attempt.AttemptID] = attempt
	return nil
}

func (s *InMemoryOwnershipStore) GetAttempt(_ context.Context, attemptID string) (*AttemptRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	attempt, ok := s.attempts[attemptID]
	if !ok {
		return nil, false, nil
	}
	copy := attempt
	return &copy, true, nil
}

func (s *InMemoryOwnershipStore) IssueLease(_ context.Context, lineageID, attemptID, issuer string, ttl time.Duration) (*LeaseToken, error) {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	now := time.Now().UTC()
	token := LeaseToken{
		LeaseID:      lineageID + ":" + attemptID + ":" + now.Format(time.RFC3339Nano),
		LineageID:    lineageID,
		AttemptID:    attemptID,
		Issuer:       issuer,
		IssuedAt:     now,
		Expiry:       now.Add(ttl),
		FencingEpoch: 1,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.leaseByLineage == nil {
		s.leaseByLineage = map[string]LeaseToken{}
	}
	if existing, ok := s.leaseByLineage[lineageID]; ok && existing.FencingEpoch >= token.FencingEpoch {
		token.FencingEpoch = existing.FencingEpoch + 1
	}
	s.leaseByLineage[lineageID] = token
	return &token, nil
}

func (s *InMemoryOwnershipStore) ValidateLease(_ context.Context, lease LeaseToken, now time.Time) error {
	if err := lease.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, ok := s.leaseByLineage[lease.LineageID]
	if !ok {
		return fmt.Errorf("lease not found for lineage %s", lease.LineageID)
	}
	if current.LeaseID != lease.LeaseID {
		return fmt.Errorf("lease %s superseded", lease.LeaseID)
	}
	if now.After(current.Expiry) {
		return fmt.Errorf("lease %s expired", lease.LeaseID)
	}
	return nil
}

func (s *InMemoryOwnershipStore) CommitHandoff(_ context.Context, commit ResumeCommit) error {
	if err := commit.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lineage, ok := s.lineages[commit.LineageID]
	if !ok {
		return fmt.Errorf("lineage %s not found", commit.LineageID)
	}
	newAttempt, ok := s.attempts[commit.NewAttemptID]
	if !ok {
		return fmt.Errorf("attempt %s not found", commit.NewAttemptID)
	}
	lineage.CurrentOwnerAttempt = commit.NewAttemptID
	lineage.CurrentOwnerRuntime = commit.DestinationRuntimeID
	lineage.UpdatedAt = commit.CommitTime.UTC()
	lineage.LineageVersion++
	s.lineages[commit.LineageID] = lineage
	newAttempt.State = AttemptStateRunning
	newAttempt.LastProgressTime = commit.CommitTime.UTC()
	s.attempts[commit.NewAttemptID] = newAttempt
	if oldAttempt, ok := s.attempts[commit.OldAttemptID]; ok {
		oldAttempt.State = AttemptStateCommittedRemote
		oldAttempt.Fenced = true
		if lease, ok := s.leaseByLineage[commit.LineageID]; ok && lease.AttemptID == commit.OldAttemptID {
			oldAttempt.FencingEpoch = lease.FencingEpoch
			delete(s.leaseByLineage, commit.LineageID)
		}
		s.attempts[commit.OldAttemptID] = oldAttempt
	}
	return nil
}

func (s *InMemoryOwnershipStore) Fence(_ context.Context, notice FenceNotice) error {
	if err := notice.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, ok := s.attempts[notice.AttemptID]
	if !ok {
		return fmt.Errorf("attempt %s not found", notice.AttemptID)
	}
	attempt.Fenced = true
	attempt.FencingEpoch = notice.FencingEpoch
	if attempt.State != AttemptStateCommittedRemote {
		attempt.State = AttemptStateFenced
	}
	s.attempts[notice.AttemptID] = attempt
	return nil
}

// Phase 6 optional interfaces below

// DuplicateHandoffChecker is an optional extension to OwnershipStore for detecting concurrent handoff offers.
type DuplicateHandoffChecker interface {
	HasActiveAttemptForLineage(ctx context.Context, lineageID string) (bool, error)
}

// AttemptLister is an optional extension to OwnershipStore for reconciliation queries.
type AttemptLister interface {
	ListActiveAttemptsByLineage(ctx context.Context, lineageID string) ([]AttemptRecord, error)
	ListExpiredLeases(ctx context.Context, now time.Time) ([]LeaseToken, error)
}

// CommitChecker is an optional extension to OwnershipStore for checking commit existence.
type CommitChecker interface {
	HasCommitForLineage(ctx context.Context, lineageID string) (bool, error)
}

// HandoffOfferRegistry stores accepted-offer reservations for idempotency and duplicate rejection.
type HandoffOfferRegistry interface {
	ReserveHandoffOffer(ctx context.Context, reservation HandoffOfferReservation) (*HandoffOfferReservation, bool, error)
}

// HasActiveAttemptForLineage returns true if the lineage has any non-terminal,
// non-fenced attempt in one of the handoff-related states.
func (s *InMemoryOwnershipStore) HasActiveAttemptForLineage(_ context.Context, lineageID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, attempt := range s.attempts {
		if attempt.LineageID != lineageID || attempt.Fenced {
			continue
		}
		switch attempt.State {
		case AttemptStateHandoffOffered, AttemptStateHandoffAccepted, AttemptStateResumePending:
			return true, nil
		}
	}
	return false, nil
}

// ListActiveAttemptsByLineage returns all attempts for the lineage that are not terminal or fenced.
func (s *InMemoryOwnershipStore) ListActiveAttemptsByLineage(_ context.Context, lineageID string) ([]AttemptRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []AttemptRecord
	for _, attempt := range s.attempts {
		if attempt.LineageID != lineageID || attempt.Fenced {
			continue
		}
		switch attempt.State {
		case AttemptStateCompleted, AttemptStateFailed, AttemptStateOrphaned, AttemptStateCommittedRemote, AttemptStateFenced:
			continue
		default:
			out = append(out, attempt)
		}
	}
	return out, nil
}

// ListExpiredLeases returns all leases that have expired.
func (s *InMemoryOwnershipStore) ListExpiredLeases(_ context.Context, now time.Time) ([]LeaseToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []LeaseToken
	for _, token := range s.leaseByLineage {
		if now.After(token.Expiry) {
			out = append(out, token)
		}
	}
	return out, nil
}

// HasCommitForLineage returns true if there is a commit record for the lineage.
// In the in-memory implementation, we track this with a simple presence check.
func (s *InMemoryOwnershipStore) HasCommitForLineage(ctx context.Context, lineageID string) (bool, error) {
	// For the in-memory store, we don't track commits separately.
	// In practice, if the lineage exists and its current owner runtime is set,
	// a commit has occurred. This is a simplification; the SQLite implementation
	// will check fmp_resume_commits table.
	s.mu.RLock()
	defer s.mu.RUnlock()
	lineage, ok := s.lineages[lineageID]
	if !ok {
		return false, nil
	}
	// If both current owner attempt and runtime are set, a commit has happened
	return lineage.CurrentOwnerAttempt != "" && lineage.CurrentOwnerRuntime != "", nil
}

func (s *InMemoryOwnershipStore) ReserveHandoffOffer(_ context.Context, reservation HandoffOfferReservation) (*HandoffOfferReservation, bool, error) {
	if strings.TrimSpace(reservation.OfferID) == "" {
		return nil, false, fmt.Errorf("offer id required")
	}
	if reservation.CreatedAt.IsZero() {
		reservation.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.offerByID == nil {
		s.offerByID = map[string]HandoffOfferReservation{}
	}
	key := strings.TrimSpace(reservation.OfferID)
	if existing, ok := s.offerByID[key]; ok {
		copy := existing
		return &copy, false, nil
	}
	s.offerByID[key] = reservation
	copy := reservation
	return &copy, true, nil
}
