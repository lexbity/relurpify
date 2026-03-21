package fmp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// OwnershipStore is part of the Phase 1 frozen FMP surface.
// Later phases should swap the in-memory implementation for a durable
// coordination-backed store behind this interface.
type OwnershipStore interface {
	CreateLineage(ctx context.Context, lineage core.LineageRecord) error
	GetLineage(ctx context.Context, lineageID string) (*core.LineageRecord, bool, error)
	UpsertAttempt(ctx context.Context, attempt core.AttemptRecord) error
	GetAttempt(ctx context.Context, attemptID string) (*core.AttemptRecord, bool, error)
	IssueLease(ctx context.Context, lineageID, attemptID, issuer string, ttl time.Duration) (*core.LeaseToken, error)
	ValidateLease(ctx context.Context, lease core.LeaseToken, now time.Time) error
	CommitHandoff(ctx context.Context, commit core.ResumeCommit) error
	Fence(ctx context.Context, notice core.FenceNotice) error
}

type InMemoryOwnershipStore struct {
	mu             sync.RWMutex
	lineages       map[string]core.LineageRecord
	attempts       map[string]core.AttemptRecord
	leaseByLineage map[string]core.LeaseToken
}

func (s *InMemoryOwnershipStore) ListLineages(_ context.Context) ([]core.LineageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.LineageRecord, 0, len(s.lineages))
	for _, lineage := range s.lineages {
		out = append(out, lineage)
	}
	return out, nil
}

func (s *InMemoryOwnershipStore) CreateLineage(_ context.Context, lineage core.LineageRecord) error {
	if err := lineage.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lineages == nil {
		s.lineages = map[string]core.LineageRecord{}
	}
	if _, ok := s.lineages[lineage.LineageID]; ok {
		return fmt.Errorf("lineage %s already exists", lineage.LineageID)
	}
	s.lineages[lineage.LineageID] = lineage
	return nil
}

func (s *InMemoryOwnershipStore) GetLineage(_ context.Context, lineageID string) (*core.LineageRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lineage, ok := s.lineages[lineageID]
	if !ok {
		return nil, false, nil
	}
	copy := lineage
	return &copy, true, nil
}

func (s *InMemoryOwnershipStore) UpsertAttempt(_ context.Context, attempt core.AttemptRecord) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attempts == nil {
		s.attempts = map[string]core.AttemptRecord{}
	}
	s.attempts[attempt.AttemptID] = attempt
	return nil
}

func (s *InMemoryOwnershipStore) GetAttempt(_ context.Context, attemptID string) (*core.AttemptRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	attempt, ok := s.attempts[attemptID]
	if !ok {
		return nil, false, nil
	}
	copy := attempt
	return &copy, true, nil
}

func (s *InMemoryOwnershipStore) IssueLease(_ context.Context, lineageID, attemptID, issuer string, ttl time.Duration) (*core.LeaseToken, error) {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	now := time.Now().UTC()
	token := core.LeaseToken{
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
		s.leaseByLineage = map[string]core.LeaseToken{}
	}
	if existing, ok := s.leaseByLineage[lineageID]; ok && existing.FencingEpoch >= token.FencingEpoch {
		token.FencingEpoch = existing.FencingEpoch + 1
	}
	s.leaseByLineage[lineageID] = token
	return &token, nil
}

func (s *InMemoryOwnershipStore) ValidateLease(_ context.Context, lease core.LeaseToken, now time.Time) error {
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

func (s *InMemoryOwnershipStore) CommitHandoff(_ context.Context, commit core.ResumeCommit) error {
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
	newAttempt.State = core.AttemptStateRunning
	newAttempt.LastProgressTime = commit.CommitTime.UTC()
	s.attempts[commit.NewAttemptID] = newAttempt
	if oldAttempt, ok := s.attempts[commit.OldAttemptID]; ok {
		oldAttempt.State = core.AttemptStateCommittedRemote
		s.attempts[commit.OldAttemptID] = oldAttempt
	}
	return nil
}

func (s *InMemoryOwnershipStore) Fence(_ context.Context, notice core.FenceNotice) error {
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
	attempt.State = core.AttemptStateFenced
	s.attempts[notice.AttemptID] = attempt
	return nil
}
