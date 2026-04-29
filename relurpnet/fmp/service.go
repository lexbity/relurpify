package fmp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/named/rex/reconcile"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

type Service struct {
	Ownership  OwnershipStore
	Discovery  DiscoveryStore
	Trust      TrustBundleStore
	Boundaries BoundaryPolicyStore
	Forwarder  GatewayForwarder
	Runtime    RuntimeEndpoint
	Packager   ContextPackager
	Transfers  ChunkTransferManager
	Projector  CapabilityProjector
	Nexus      NexusAdapter
	Telemetry  core.Telemetry
	Audit      core.AuditLogger
	Limiter    OperationalLimiter
	Rollout    RolloutPolicy
	Log        event.Log
	Partition  string
	LeaseTTL   time.Duration
	Now        func() time.Time
	Signer     PayloadSigner
	Verifier   PayloadVerifier
	Mediator   *MediationController
	// Phase 6.3: PartitionDetector is optional; if set, guards against issuing/accepting handoffs during partition
	PartitionDetector PartitionDetector
	// Phase 6.5: CircuitBreakers is optional; if set, applies per-trust-domain circuit breaker logic
	CircuitBreakers CircuitBreakerStore
	// Phase 6.4: CompatibilityWindows is optional; if set, enforces version skew limits
	CompatibilityWindows CompatibilityWindowStore
}

type ExecutedHandoff struct {
	Accept  HandoffAccept           `json:"accept" yaml:"accept"`
	Attempt AttemptRecord           `json:"attempt" yaml:"attempt"`
	Receipt ResumeReceipt           `json:"receipt" yaml:"receipt"`
	Package *PortableContextPackage `json:"package,omitempty" yaml:"package,omitempty"`
}

// ReconcileLeaseResult captures summary statistics from lease reconciliation.
type ReconcileLeaseResult struct {
	Scanned  int `json:"scanned"`
	Orphaned int `json:"orphaned"`
	Skipped  int `json:"skipped"`
	Errors   int `json:"errors"`
}

// GCResult captures summary statistics from garbage collection.
type GCResult struct {
	ScannedObjects int `json:"scanned_objects"`
	DeletedObjects int `json:"deleted_objects"`
	Errors         int `json:"errors"`
}

func (s *Service) OpenChunkTransfer(ctx context.Context, lineageID string, manifest ContextManifest, sealed SealedContext) (*ChunkTransferSession, error) {
	if s.Transfers == nil {
		return nil, fmt.Errorf("chunk transfer manager unavailable")
	}
	session, err := s.Transfers.Open(ctx, manifest, sealed, s.nowUTC())
	if err != nil {
		return nil, err
	}
	if lineage, ok, err := s.Ownership.GetLineage(ctx, lineageID); err == nil && ok {
		s.emit(ctx, FrameworkEventFMPChunkOpened, lineage.Owner, map[string]any{
			"lineage_id":   lineageID,
			"transfer_id":  session.TransferID,
			"manifest_ref": session.ManifestRef,
			"total_chunks": session.TotalChunks,
		})
	}
	return session, nil
}

func (s *Service) ReadChunkTransfer(ctx context.Context, transferID string, maxChunks int) ([]ChunkFrame, *ChunkFlowControl, error) {
	if s.Transfers == nil {
		return nil, nil, fmt.Errorf("chunk transfer manager unavailable")
	}
	return s.Transfers.Read(ctx, transferID, maxChunks, s.nowUTC())
}

func (s *Service) AckChunkTransfer(ctx context.Context, lineageID string, ack ChunkAck) (*ChunkFlowControl, error) {
	if s.Transfers == nil {
		return nil, fmt.Errorf("chunk transfer manager unavailable")
	}
	control, err := s.Transfers.Ack(ctx, ack.TransferID, ack, s.nowUTC())
	if err != nil {
		return nil, err
	}
	if lineage, ok, err := s.Ownership.GetLineage(ctx, lineageID); err == nil && ok {
		s.emit(ctx, FrameworkEventFMPChunkAcked, lineage.Owner, map[string]any{
			"lineage_id":  lineageID,
			"transfer_id": ack.TransferID,
			"acked_index": ack.AckedIndex,
			"window_size": control.WindowSize,
			"remaining":   control.Remaining,
		})
	}
	return control, nil
}

func (s *Service) CancelChunkTransfer(ctx context.Context, lineageID, transferID, reason string) error {
	if s.Transfers == nil {
		return fmt.Errorf("chunk transfer manager unavailable")
	}
	if err := s.Transfers.Cancel(ctx, transferID, reason, s.nowUTC()); err != nil {
		return err
	}
	if lineage, ok, err := s.Ownership.GetLineage(ctx, lineageID); err == nil && ok {
		s.emit(ctx, FrameworkEventFMPChunkCancelled, lineage.Owner, map[string]any{
			"lineage_id":  lineageID,
			"transfer_id": transferID,
			"reason":      reason,
		})
	}
	return nil
}

func (s *Service) CreateLineage(ctx context.Context, lineage LineageRecord) error {
	if s.Ownership == nil {
		return fmt.Errorf("ownership store unavailable")
	}
	if err := lineage.Validate(); err != nil {
		return err
	}
	if lineage.CreatedAt.IsZero() {
		lineage.CreatedAt = s.nowUTC()
	}
	lineage.UpdatedAt = lineage.CreatedAt
	if err := s.Ownership.CreateLineage(ctx, lineage); err != nil {
		return err
	}
	s.emit(ctx, FrameworkEventFMPLineageCreated, lineage.Owner, map[string]any{
		"lineage_id": lineage.LineageID,
		"task_class": lineage.TaskClass,
	})
	return nil
}

func (s *Service) OfferHandoff(ctx context.Context, lineageID, attemptID, destinationExport, issuer string, query RuntimeQuery) (*HandoffOffer, *PortableContextPackage, *SealedContext, error) {
	if s.Ownership == nil || s.Packager == nil {
		return nil, nil, nil, fmt.Errorf("fmp service not configured")
	}
	// Phase 6.3: Check partition state before issuing new handoff offers
	if s.isPartitioned() {
		return nil, nil, nil, fmt.Errorf("ownership store partitioned: cannot issue new handoff offers")
	}
	lineage, ok, err := s.Ownership.GetLineage(ctx, lineageID)
	if err != nil {
		return nil, nil, nil, err
	}
	if !ok {
		return nil, nil, nil, fmt.Errorf("lineage %s not found", lineageID)
	}
	attempt, ok, err := s.Ownership.GetAttempt(ctx, attemptID)
	if err != nil {
		return nil, nil, nil, err
	}
	if !ok {
		return nil, nil, nil, fmt.Errorf("attempt %s not found", attemptID)
	}
	lease, err := s.Ownership.IssueLease(ctx, lineageID, attemptID, issuer, s.leaseTTL())
	if err != nil {
		return nil, nil, nil, err
	}
	if err := SignLeaseToken(s.Signer, lease); err != nil {
		return nil, nil, nil, err
	}
	pkg, err := s.Packager.BuildPackage(ctx, *lineage, *attempt, query)
	if err != nil {
		return nil, nil, nil, err
	}
	sealed, err := s.Packager.SealPackage(ctx, pkg.Manifest, pkg, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	runtimeDescriptor, _, err := s.resolveRuntimeDescriptor(ctx, attempt.RuntimeID)
	if err != nil {
		return nil, nil, nil, err
	}
	offer := &HandoffOffer{
		OfferID:                       lease.LeaseID,
		LineageID:                     lineageID,
		SourceAttemptID:               attemptID,
		SourceRuntimeID:               attempt.RuntimeID,
		SourceCompatibilityClass:      runtimeDescriptor.CompatibilityClass,
		DestinationExport:             destinationExport,
		ContextManifestRef:            pkg.Manifest.ContextID,
		ContextClass:                  pkg.Manifest.ContextClass,
		ContextSizeBytes:              pkg.Manifest.SizeBytes,
		SensitivityClass:              pkg.Manifest.SensitivityClass,
		RequestedCapabilityProjection: lineage.CapabilityEnvelope,
		LeaseToken:                    *lease,
		Expiry:                        lease.Expiry,
	}
	if err := SignHandoffOffer(s.Signer, offer); err != nil {
		return nil, nil, nil, err
	}
	if err := offer.Validate(); err != nil {
		return nil, nil, nil, err
	}
	attempt.State = AttemptStateHandoffOffered
	attempt.LeaseID = lease.LeaseID
	attempt.LeaseExpiry = lease.Expiry
	attempt.LastProgressTime = s.nowUTC()
	if err := s.Ownership.UpsertAttempt(ctx, *attempt); err != nil {
		return nil, nil, nil, err
	}
	s.emit(ctx, FrameworkEventFMPLeaseIssued, lineage.Owner, map[string]any{
		"lineage_id": lineageID,
		"lease_id":   lease.LeaseID,
	})
	s.emit(ctx, FrameworkEventFMPHandoffOffered, lineage.Owner, map[string]any{
		"lineage_id": lineageID,
		"offer_id":   offer.OfferID,
	})
	return offer, pkg, sealed, nil
}

func (s *Service) AcceptHandoff(ctx context.Context, offer HandoffOffer, destination ExportDescriptor, runtimeID string) (*HandoffAccept, error) {
	accept, refusal, err := s.TryAcceptHandoff(ctx, offer, destination, runtimeID)
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		s.emitRefusal(ctx, offer.LineageID, refusal)
		return nil, fmt.Errorf("resume refused: %s", refusal.Message)
	}
	return accept, nil
}

func (s *Service) TryAcceptHandoff(ctx context.Context, offer HandoffOffer, destination ExportDescriptor, runtimeID string) (*HandoffAccept, *TransferRefusal, error) {
	return s.tryAcceptHandoff(ctx, offer, destination, runtimeID, nil)
}

func (s *Service) tryAcceptHandoff(ctx context.Context, offer HandoffOffer, destination ExportDescriptor, runtimeID string, actor *AuthorizedActor) (*HandoffAccept, *TransferRefusal, error) {
	if err := offer.Validate(); err != nil {
		return nil, nil, err
	}
	if err := VerifyLeaseToken(s.Verifier, offer.LeaseToken); err != nil {
		return nil, &TransferRefusal{Code: RefusalUnauthorized, Message: err.Error()}, nil
	}
	if err := VerifyHandoffOffer(s.Verifier, offer); err != nil {
		return nil, &TransferRefusal{Code: RefusalUnauthorized, Message: err.Error()}, nil
	}
	if err := s.Ownership.ValidateLease(ctx, offer.LeaseToken, s.nowUTC()); err != nil {
		return nil, &TransferRefusal{Code: RefusalInvalidLease, Message: err.Error()}, nil
	}
	// Phase 6.2: Check for stale fencing epoch
	if sourceAttempt, ok, err := s.Ownership.GetAttempt(ctx, offer.SourceAttemptID); err == nil && ok {
		if offer.LeaseToken.FencingEpoch < sourceAttempt.FencingEpoch {
			return nil, &TransferRefusal{
				Code:    RefusalStaleEpoch,
				Message: fmt.Sprintf("offer epoch %d predates current fencing epoch %d", offer.LeaseToken.FencingEpoch, sourceAttempt.FencingEpoch),
			}, nil
		}
	}
	// Phase 6.2: Duplicate protection via provisional attempt ID
	// The provisional ID (lineage:runtime:resume) ensures idempotent re-accepts
	// are safe (they resolve to the same attempt record). No explicit check needed here
	// since idempotency is enforced at the reservation level.
	if err := destination.Validate(); err != nil {
		return nil, nil, err
	}
	provisionalAttemptID := offer.LineageID + ":" + runtimeID + ":resume"
	if registry, ok := s.Ownership.(HandoffOfferRegistry); ok {
		reservation := HandoffOfferReservation{
			LineageID:            offer.LineageID,
			OfferID:              offer.OfferID,
			LeaseID:              offer.LeaseToken.LeaseID,
			SourceAttemptID:      offer.SourceAttemptID,
			FencingEpoch:         offer.LeaseToken.FencingEpoch,
			ProvisionalAttemptID: provisionalAttemptID,
			RuntimeID:            runtimeID,
			CreatedAt:            s.nowUTC(),
		}
		existing, created, err := registry.ReserveHandoffOffer(ctx, reservation)
		if err != nil {
			return nil, nil, err
		}
		if !created && !matchingOfferReservation(existing, reservation) {
			return nil, &TransferRefusal{
				Code:    RefusalDuplicateHandoff,
				Message: fmt.Sprintf("offer %s already reserved for runtime %s", offer.OfferID, existing.RuntimeID),
			}, nil
		}
	}
	if lister, ok := s.Ownership.(AttemptLister); ok {
		attempts, err := lister.ListActiveAttemptsByLineage(ctx, offer.LineageID)
		if err != nil {
			return nil, nil, err
		}
		for _, attempt := range attempts {
			if strings.EqualFold(strings.TrimSpace(attempt.AttemptID), strings.TrimSpace(offer.SourceAttemptID)) {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(attempt.AttemptID), strings.TrimSpace(provisionalAttemptID)) {
				continue
			}
			return nil, &TransferRefusal{
				Code:    RefusalDuplicateHandoff,
				Message: fmt.Sprintf("lineage %s already has an active handoff attempt %s", offer.LineageID, attempt.AttemptID),
			}, nil
		}
	} else if checker, ok := s.Ownership.(DuplicateHandoffChecker); ok {
		if active, err := checker.HasActiveAttemptForLineage(ctx, offer.LineageID); err != nil {
			return nil, nil, err
		} else if active {
			return nil, &TransferRefusal{
				Code:    RefusalDuplicateHandoff,
				Message: fmt.Sprintf("lineage %s already has an active handoff attempt", offer.LineageID),
			}, nil
		}
	}
	lineage, ok, err := s.Ownership.GetLineage(ctx, offer.LineageID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, fmt.Errorf("lineage %s not found", offer.LineageID)
	}
	if refusal := s.validateExportPolicy(ctx, *lineage, offer, destination); refusal != nil {
		return nil, refusal, nil
	}
	runtimeDescriptor, refusal, err := s.resolveRuntimeDescriptor(ctx, runtimeID)
	if err != nil || refusal != nil {
		return nil, refusal, err
	}
	if refusal := s.validateRuntimeImportEligibility(runtimeDescriptor); refusal != nil {
		s.emit(ctx, FrameworkEventFMPRolloutBlocked, lineage.Owner, map[string]any{
			"lineage_id": offer.LineageID,
			"offer_id":   offer.OfferID,
			"runtime_id": runtimeDescriptor.RuntimeID,
			"node_id":    runtimeDescriptor.NodeID,
			"reason":     refusal.Message,
		})
		return nil, refusal, nil
	}
	if refusal := ValidateOfferCompatibility(runtimeDescriptor, offer, destination, s.nowUTC()); refusal != nil {
		return nil, refusal, nil
	}
	effectiveActor := lineage.Owner
	isOwner := true
	isDelegated := false
	if actor != nil {
		effectiveActor = actor.Subject
		isOwner = !actor.Delegated
		isDelegated = actor.Delegated
	}
	if s.Nexus.Policies != nil {
		decision, err := s.Nexus.Policies.EvaluateResume(ctx, ResumePolicyRequest{
			Lineage:      *lineage,
			Offer:        offer,
			Destination:  destination,
			SourceDomain: trustDomainForOffer(destination, offer),
			Actor:        effectiveActor,
			IsOwner:      isOwner,
			IsDelegated:  isDelegated,
			RouteMode:    resolveRouteMode(destination),
		})
		if err != nil {
			return nil, nil, err
		}
		if decision.Effect != "allow" {
			refusal := policyDecisionRefusal(decision)
			s.emit(ctx, FrameworkEventFMPPolicyDenied, lineage.Owner, map[string]any{
				"lineage_id": offer.LineageID,
				"offer_id":   offer.OfferID,
				"reason":     decision.Reason,
			})
			return nil, refusal, nil
		}
	}
	projection := lineage.CapabilityEnvelope
	if s.Projector != nil {
		projection, err = s.Projector.Project(ctx, *lineage, destination)
		if err != nil {
			return nil, nil, err
		}
	} else {
		projection, err = StrictCapabilityProjector{}.Project(ctx, *lineage, destination)
		if err != nil {
			return nil, nil, err
		}
	}
	accept := &HandoffAccept{
		OfferID:                      offer.OfferID,
		DestinationRuntimeID:         runtimeID,
		AcceptedContextClass:         offer.ContextClass,
		AcceptedCapabilityProjection: projection,
		ProvisionalAttemptID:         provisionalAttemptID,
		Expiry:                       s.nowUTC().Add(s.leaseTTL()),
	}
	if err := SignHandoffAccept(s.Signer, accept); err != nil {
		return nil, nil, err
	}
	if err := accept.Validate(); err != nil {
		return nil, nil, err
	}
	// Phase 6.3: Check partition state before accepting new handoffs
	if s.isPartitioned() {
		return nil, &TransferRefusal{
			Code:    RefusalAdmissionClosed,
			Message: "ownership store partitioned: no new handoffs accepted",
			RetryAt: s.nowUTC().Add(30 * time.Second),
		}, nil
	}
	// Phase 6.5: Check circuit breaker state for the trust domain
	if s.CircuitBreakers != nil {
		trustDomain := trustDomainForOffer(destination, offer)
		if state, err := s.CircuitBreakers.GetState(ctx, trustDomain); err == nil && state == CircuitOpen {
			return nil, &TransferRefusal{
				Code:    RefusalAdmissionClosed,
				Message: fmt.Sprintf("circuit breaker open for trust domain %s", trustDomain),
				RetryAt: s.nowUTC().Add(30 * time.Second),
			}, nil
		}
	}
	if refusal := s.reserveResumeSlot(ctx, accept.ProvisionalAttemptID, offer.ContextSizeBytes); refusal != nil {
		return nil, refusal, nil
	}
	s.emit(ctx, FrameworkEventFMPHandoffAccepted, lineage.Owner, map[string]any{
		"lineage_id": offer.LineageID,
		"offer_id":   offer.OfferID,
		"runtime_id": runtimeID,
	})
	return accept, nil, nil
}

func (s *Service) CommitHandoff(ctx context.Context, offer HandoffOffer, accept HandoffAccept, receipt ResumeReceipt) (*ResumeCommit, error) {
	if err := offer.Validate(); err != nil {
		return nil, err
	}
	if err := accept.Validate(); err != nil {
		return nil, err
	}
	if err := VerifyHandoffAccept(s.Verifier, accept); err != nil {
		return nil, err
	}
	if err := receipt.Validate(); err != nil {
		return nil, err
	}
	if err := VerifyResumeReceipt(s.Verifier, receipt); err != nil {
		return nil, err
	}
	commit := &ResumeCommit{
		LineageID:            offer.LineageID,
		OldAttemptID:         offer.SourceAttemptID,
		NewAttemptID:         accept.ProvisionalAttemptID,
		DestinationRuntimeID: accept.DestinationRuntimeID,
		ReceiptRef:           receipt.ReceiptID,
		CommitTime:           s.nowUTC(),
	}
	if err := SignResumeCommit(s.Signer, commit); err != nil {
		return nil, err
	}
	if err := s.Ownership.CommitHandoff(ctx, *commit); err != nil {
		return nil, err
	}
	s.releaseResumeSlot(ctx, accept.ProvisionalAttemptID)
	notice := FenceNotice{
		LineageID:    offer.LineageID,
		AttemptID:    offer.SourceAttemptID,
		FencingEpoch: offer.LeaseToken.FencingEpoch,
		Reason:       "resume committed",
		Issuer:       accept.DestinationRuntimeID,
		IssuedAt:     s.nowUTC(),
	}
	if err := SignFenceNotice(s.Signer, &notice); err != nil {
		return nil, err
	}
	if s.Runtime != nil {
		if err := s.Runtime.FenceAttempt(ctx, notice); err != nil {
			return nil, err
		}
	}
	if err := s.Ownership.Fence(ctx, notice); err != nil {
		return nil, err
	}
	lineage, ok, err := s.Ownership.GetLineage(ctx, offer.LineageID)
	if err == nil && ok {
		s.emit(ctx, FrameworkEventFMPResumeCommitted, lineage.Owner, map[string]any{
			"lineage_id":  offer.LineageID,
			"old_attempt": offer.SourceAttemptID,
			"new_attempt": accept.ProvisionalAttemptID,
		})
		s.emit(ctx, FrameworkEventFMPFenceIssued, lineage.Owner, map[string]any{
			"lineage_id": offer.LineageID,
			"attempt_id": offer.SourceAttemptID,
			"epoch":      notice.FencingEpoch,
		})
	}
	return commit, nil
}

// ReconcileExpiredLeases performs Phase 6 reconciliation on expired ownership leases.
// For each expired lease, if no commit receipt exists, the source attempt is orphaned.
func (s *Service) ReconcileExpiredLeases(ctx context.Context) (ReconcileLeaseResult, error) {
	result := ReconcileLeaseResult{}

	// Check if ownership store supports the extended interfaces
	lister, ok := s.Ownership.(AttemptLister)
	if !ok {
		// Graceful no-op: ownership store doesn't support extended interface
		return result, nil
	}

	checker, _ := s.Ownership.(CommitChecker)

	expired, err := lister.ListExpiredLeases(ctx, s.nowUTC())
	if err != nil {
		result.Errors++
		return result, fmt.Errorf("failed to list expired leases: %w", err)
	}
	result.Scanned = len(expired)

	for _, token := range expired {
		// Get the current attempt state
		attempt, ok, err := s.Ownership.GetAttempt(ctx, token.AttemptID)
		if err != nil {
			result.Errors++
			continue
		}
		if !ok {
			// Attempt no longer exists
			result.Skipped++
			continue
		}

		// Skip if already terminal
		switch attempt.State {
		case AttemptStateFenced, AttemptStateCompleted, AttemptStateFailed, AttemptStateOrphaned, AttemptStateCommittedRemote:
			result.Skipped++
			continue
		}

		// Check if a commit receipt exists (ownership transferred cleanly)
		if checker != nil {
			if hasCommit, err := checker.HasCommitForLineage(ctx, token.LineageID); err == nil && hasCommit {
				// Commit exists, ownership is resolved
				result.Skipped++
				continue
			}
		}

		// No commit exists: fence the source attempt and mark as orphaned
		notice := FenceNotice{
			LineageID:    token.LineageID,
			AttemptID:    token.AttemptID,
			FencingEpoch: token.FencingEpoch,
			Reason:       "lease_expired_no_receipt",
			Issuer:       s.Partition, // use partition ID as issuer
			IssuedAt:     s.nowUTC(),
		}
		if err := SignFenceNotice(s.Signer, &notice); err != nil {
			result.Errors++
			continue
		}
		if err := s.Ownership.Fence(ctx, notice); err != nil {
			result.Errors++
			continue
		}

		// Upsert the attempt as ORPHANED
		attempt.State = AttemptStateOrphaned
		attempt.Fenced = true
		attempt.FencingEpoch = notice.FencingEpoch
		if err := s.Ownership.UpsertAttempt(ctx, *attempt); err != nil {
			result.Errors++
			continue
		}

		// Emit event
		s.emit(ctx, FrameworkEventFMPFenceIssued, identity.SubjectRef{}, map[string]any{
			"lineage_id": token.LineageID,
			"attempt_id": token.AttemptID,
			"epoch":      notice.FencingEpoch,
			"reason":     "reconciliation: lease expired, no receipt",
		})
		result.Orphaned++
	}

	return result, nil
}

// GCContextObjects performs garbage collection on orphaned context objects.
// This method is optional and relies on optional interface support from the Packager.
func (s *Service) GCContextObjects(ctx context.Context) (GCResult, error) {
	result := GCResult{}

	// Check if packager supports optional GC interface
	type contextObjectGC interface {
		GCContextObjects(context.Context) (int, int, error) // (scanned, deleted, error)
	}

	gcr, ok := s.Packager.(contextObjectGC)
	if !ok {
		// Graceful no-op: packager doesn't support GC interface
		return result, nil
	}

	scanned, deleted, err := gcr.GCContextObjects(ctx)
	result.ScannedObjects = scanned
	result.DeletedObjects = deleted
	if err != nil {
		result.Errors = 1
		return result, fmt.Errorf("context object GC failed: %w", err)
	}

	return result, nil
}

// ReconcileAttemptFromOutcome updates FMP attempt state based on Rex reconciliation outcome.
// This bridges reconciliation decisions back to FMP ownership tracking.
func (s *Service) ReconcileAttemptFromOutcome(ctx context.Context, lineageID string, outcome *reconcile.Record) (*AttemptRecord, error) {
	if outcome == nil {
		return nil, nil
	}

	lineage, ok, err := s.Ownership.GetLineage(ctx, lineageID)
	if err != nil || !ok {
		return nil, err
	}

	// Find the attempt associated with this reconciliation outcome
	// Use the RunID as the attemptID if present
	attemptID := outcome.RunID
	if attemptID == "" {
		attemptID = outcome.ID
	}

	attempt, ok, err := s.Ownership.GetAttempt(ctx, attemptID)
	if err != nil || !ok {
		return nil, err
	}

	// Map reconciliation status to attempt state
	var newState AttemptState
	switch outcome.Status {
	case reconcile.StatusTerminal:
		newState = AttemptStateFailed
	case reconcile.StatusVerified:
		newState = AttemptStateCompleted
	case reconcile.StatusRepaired:
		newState = AttemptStateCompleted
	case reconcile.StatusOperatorReview:
		// Keep the current state
		newState = attempt.State
	default:
		newState = AttemptStateFailed
	}

	// Update the attempt
	attempt.State = newState
	attempt.LastProgressTime = s.nowUTC()

	if err := s.Ownership.UpsertAttempt(ctx, *attempt); err != nil {
		return nil, err
	}

	// Emit reconciliation outcome event
	s.emit(ctx, FrameworkEventFMPReconciliationOutcome, lineage.Owner, map[string]any{
		"lineage_id":       lineageID,
		"attempt_id":       attemptID,
		"outcome_status":   outcome.Status,
		"repair_summary":   outcome.RepairSummary,
		"resolution_notes": outcome.ResolutionNotes,
	})

	return attempt, nil
}

func (s *Service) ResumeHandoff(ctx context.Context, offer HandoffOffer, destination ExportDescriptor, runtimeID string, manifest ContextManifest, sealed SealedContext) (*ExecutedHandoff, *ResumeCommit, *TransferRefusal, error) {
	return s.resumeHandoff(ctx, offer, destination, runtimeID, manifest, sealed, nil, "")
}

func (s *Service) ResumeHandoffForNode(ctx context.Context, offer HandoffOffer, destination ExportDescriptor, runtimeID, nodeID string, actor identity.SubjectRef, manifest ContextManifest, sealed SealedContext) (*ExecutedHandoff, *ResumeCommit, *AuthorizedActor, *TransferRefusal, error) {
	authorized, err := s.AuthorizeResumeActor(ctx, offer.LineageID, actor, core.SessionOperationResume)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	executed, commit, refusal, err := s.resumeHandoff(ctx, offer, destination, runtimeID, manifest, sealed, authorized, nodeID)
	if err != nil || refusal != nil {
		return nil, nil, authorized, refusal, err
	}
	return executed, commit, authorized, nil, nil
}

func (s *Service) resumeHandoff(ctx context.Context, offer HandoffOffer, destination ExportDescriptor, runtimeID string, manifest ContextManifest, sealed SealedContext, authorized *AuthorizedActor, nodeID string) (*ExecutedHandoff, *ResumeCommit, *TransferRefusal, error) {
	if s.Runtime == nil {
		return nil, nil, nil, fmt.Errorf("runtime endpoint unavailable")
	}
	var (
		accept  *HandoffAccept
		refusal *TransferRefusal
		err     error
	)
	if authorized != nil {
		accept, refusal, err = s.tryAcceptHandoff(ctx, offer, destination, runtimeID, authorized)
	} else {
		accept, refusal, err = s.TryAcceptHandoff(ctx, offer, destination, runtimeID)
	}
	if err != nil || refusal != nil {
		return nil, nil, refusal, err
	}
	release := true
	defer func() {
		if release {
			s.releaseResumeSlot(ctx, accept.ProvisionalAttemptID)
		}
	}()
	if err := manifest.Validate(); err != nil {
		return nil, nil, nil, err
	}
	if err := VerifyContextManifest(s.Verifier, manifest); err != nil {
		return nil, nil, &TransferRefusal{Code: RefusalUnauthorized, Message: err.Error()}, nil
	}
	if err := sealed.Validate(); err != nil {
		return nil, nil, nil, err
	}
	if err := validateManifestForOffer(offer, manifest, sealed); err != nil {
		return nil, nil, nil, err
	}
	if descriptor, err := s.Runtime.Descriptor(ctx); err == nil {
		if compatErr := ValidateImportedContextCompatibility(descriptor, manifest, sealed); compatErr != nil {
			return nil, nil, &TransferRefusal{Code: RefusalIncompatibleRuntime, Message: compatErr.Error()}, nil
		}
		// Phase 6.4: Check version skew against compatibility window
		if s.CompatibilityWindows != nil {
			if window, ok, err := s.CompatibilityWindows.GetWindow(ctx, manifest.ContextClass); err == nil && ok {
				if refusal := ValidateVersionSkew(*window, manifest.SchemaVersion, descriptor.RuntimeVersion); refusal != nil {
					return nil, nil, refusal, nil
				}
			}
		}
	} else {
		return nil, nil, nil, err
	}
	lineage, ok, err := s.Ownership.GetLineage(ctx, offer.LineageID)
	if err != nil {
		return nil, nil, nil, err
	}
	if !ok {
		return nil, nil, nil, fmt.Errorf("lineage %s not found", offer.LineageID)
	}
	if authorized != nil {
		if err := s.validateContinuationAuthority(ctx, *lineage, *authorized, nodeID, runtimeID); err != nil {
			return nil, nil, nil, err
		}
	}
	if err := s.Runtime.ValidateContext(ctx, manifest, sealed); err != nil {
		return nil, nil, nil, err
	}
	pkg, err := s.Runtime.ImportContext(ctx, *lineage, manifest, sealed)
	if err != nil {
		return nil, nil, nil, err
	}
	if pkg == nil {
		return nil, nil, nil, fmt.Errorf("runtime import returned nil package")
	}
	attempt, err := s.Runtime.CreateAttempt(ctx, *lineage, *accept, pkg)
	if err != nil {
		return nil, nil, nil, err
	}
	if attempt == nil {
		return nil, nil, nil, fmt.Errorf("runtime create attempt returned nil attempt")
	}
	if strings.TrimSpace(attempt.AttemptID) == "" {
		attempt.AttemptID = accept.ProvisionalAttemptID
	}
	if attempt.LineageID == "" {
		attempt.LineageID = offer.LineageID
	}
	if attempt.RuntimeID == "" {
		attempt.RuntimeID = accept.DestinationRuntimeID
	}
	if attempt.State == "" {
		attempt.State = AttemptStateResumePending
	}
	if attempt.StartTime.IsZero() {
		attempt.StartTime = s.nowUTC()
	}
	accept.ProvisionalAttemptID = attempt.AttemptID
	if err := attempt.Validate(); err != nil {
		return nil, nil, nil, err
	}
	if err := validateImportedAttemptForContinuation(*attempt, *lineage, runtimeID); err != nil {
		return nil, nil, nil, err
	}
	if err := s.Ownership.UpsertAttempt(ctx, *attempt); err != nil {
		return nil, nil, nil, err
	}
	receipt, err := s.Runtime.IssueReceipt(ctx, *lineage, *attempt, pkg)
	if err != nil {
		return nil, nil, nil, err
	}
	if receipt == nil {
		return nil, nil, nil, fmt.Errorf("runtime receipt unavailable")
	}
	if receipt.AttemptID == "" {
		receipt.AttemptID = attempt.AttemptID
	}
	if receipt.LineageID == "" {
		receipt.LineageID = offer.LineageID
	}
	if receipt.RuntimeID == "" {
		receipt.RuntimeID = attempt.RuntimeID
	}
	if receipt.ImportedContextID == "" {
		receipt.ImportedContextID = manifest.ContextID
	}
	if receipt.StartTime.IsZero() {
		receipt.StartTime = s.nowUTC()
	}
	if err := SignResumeReceipt(s.Signer, receipt); err != nil {
		return nil, nil, nil, err
	}
	if err := validateReceiptForCommit(offer, *accept, manifest, *receipt); err != nil {
		return nil, nil, nil, err
	}
	if receipt.Status != ReceiptStatusRunning {
		return nil, nil, &TransferRefusal{Code: RefusalAdmissionClosed, Message: fmt.Sprintf("runtime receipt status %s does not allow commit", receipt.Status)}, nil
	}
	if authorized != nil {
		s.emit(ctx, FrameworkEventFMPContinuationBound, lineage.Owner, map[string]any{
			"lineage_id":         lineage.LineageID,
			"tenant_id":          lineage.TenantID,
			"session_id":         lineage.SessionID,
			"owner_id":           lineage.Owner.ID,
			"actor_id":           authorized.Subject.ID,
			"actor_kind":         authorized.Subject.Kind,
			"delegated":          authorized.Delegated,
			"runtime_id":         attempt.RuntimeID,
			"node_id":            nodeID,
			"attempt_id":         attempt.AttemptID,
			"receipt_id":         receipt.ReceiptID,
			"trust_class":        lineage.TrustClass,
			"manifest_ref":       manifest.ContextID,
			"destination_export": destination.ExportName,
		})
	}
	commit, err := s.CommitHandoff(ctx, offer, *accept, *receipt)
	if err != nil {
		// Record failure for circuit breaker (Phase 6.5)
		if s.CircuitBreakers != nil {
			trustDomain := trustDomainForOffer(destination, offer)
			_ = s.CircuitBreakers.RecordFailure(ctx, trustDomain, s.nowUTC())
		}
		return nil, nil, nil, err
	}
	// Record success for circuit breaker (Phase 6.5)
	if s.CircuitBreakers != nil {
		trustDomain := trustDomainForOffer(destination, offer)
		_ = s.CircuitBreakers.RecordSuccess(ctx, trustDomain, s.nowUTC())
	}
	release = false
	return &ExecutedHandoff{
		Accept:  *accept,
		Attempt: *attempt,
		Receipt: *receipt,
		Package: pkg,
	}, commit, nil, nil
}

func (s *Service) validateContinuationAuthority(ctx context.Context, lineage LineageRecord, authorized AuthorizedActor, nodeID, runtimeID string) error {
	if err := s.ensureTenantAndOwner(ctx, lineage.TenantID, lineage.Owner); err != nil {
		return err
	}
	if strings.TrimSpace(lineage.SessionID) != "" && s.Nexus.Sessions != nil {
		boundary, err := s.Nexus.Sessions.GetBoundaryBySessionID(ctx, lineage.SessionID)
		if err != nil {
			return err
		}
		if boundary == nil {
			return fmt.Errorf("session %s not found for lineage %s", lineage.SessionID, lineage.LineageID)
		}
		if !strings.EqualFold(boundary.TenantID, lineage.TenantID) {
			return fmt.Errorf("session %s tenant %s does not match lineage tenant %s", lineage.SessionID, boundary.TenantID, lineage.TenantID)
		}
		if !subjectRefsEqual(subjectRefFromDelegation(boundary.Owner), lineage.Owner) {
			return fmt.Errorf("session %s owner changed for lineage %s", lineage.SessionID, lineage.LineageID)
		}
		if boundary.TrustClass != "" && lineage.TrustClass != "" && boundary.TrustClass != lineage.TrustClass {
			return fmt.Errorf("session %s trust class %s does not match lineage trust class %s", lineage.SessionID, boundary.TrustClass, lineage.TrustClass)
		}
		if authorized.Delegated {
			delegations, err := s.loadSessionDelegations(ctx, lineage.SessionID)
			if err != nil {
				return err
			}
			allowed := false
			eventActor := core.EventActor{
				ID:          authorized.Subject.ID,
				TenantID:    authorized.Subject.TenantID,
				SubjectKind: string(authorized.Subject.Kind),
			}
			for _, delegation := range delegations {
				if delegation.Allows(eventActor, core.SessionOperationResume, s.nowUTC()) {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("delegation no longer valid for actor %s on session %s", authorized.Subject.ID, lineage.SessionID)
			}
		}
	}
	if strings.TrimSpace(nodeID) != "" {
		if err := s.validateDestinationNode(ctx, lineage.TenantID, nodeID); err != nil {
			return err
		}
	}
	if s.Runtime != nil {
		descriptor, err := s.Runtime.Descriptor(ctx)
		if err != nil {
			return err
		}
		if strings.TrimSpace(runtimeID) != "" && strings.TrimSpace(descriptor.RuntimeID) != "" && !strings.EqualFold(descriptor.RuntimeID, runtimeID) {
			return fmt.Errorf("runtime id %s does not match destination runtime %s", descriptor.RuntimeID, runtimeID)
		}
		if strings.TrimSpace(nodeID) != "" && strings.TrimSpace(descriptor.NodeID) != "" && !strings.EqualFold(descriptor.NodeID, nodeID) {
			return fmt.Errorf("runtime node %s does not match enrolled node %s", descriptor.NodeID, nodeID)
		}
	}
	return nil
}

// isPartitioned returns true if the ownership store is partitioned (Phase 6.3).
func (s *Service) isPartitioned() bool {
	if s == nil || s.PartitionDetector == nil {
		return false
	}
	return s.PartitionDetector.IsPartitioned()
}

// listLiveNodeAds returns node advertisements, filtering expired entries if LiveDiscoveryStore is available.
func (s *Service) listLiveNodeAds(ctx context.Context) ([]NodeAdvertisement, error) {
	if s.Discovery == nil {
		return nil, nil
	}
	if live, ok := s.Discovery.(LiveDiscoveryStore); ok {
		return live.ListLiveNodeAdvertisements(ctx, s.nowUTC())
	}
	return s.Discovery.ListNodeAdvertisements(ctx)
}

// listLiveRuntimeAds returns runtime advertisements, filtering expired entries if LiveDiscoveryStore is available.
func (s *Service) listLiveRuntimeAds(ctx context.Context) ([]RuntimeAdvertisement, error) {
	if s.Discovery == nil {
		return nil, nil
	}
	if live, ok := s.Discovery.(LiveDiscoveryStore); ok {
		return live.ListLiveRuntimeAdvertisements(ctx, s.nowUTC())
	}
	return s.Discovery.ListRuntimeAdvertisements(ctx)
}

// listLiveExportAds returns export advertisements, filtering expired entries if LiveDiscoveryStore is available.
func (s *Service) listLiveExportAds(ctx context.Context) ([]ExportAdvertisement, error) {
	if s.Discovery == nil {
		return nil, nil
	}
	if live, ok := s.Discovery.(LiveDiscoveryStore); ok {
		return live.ListLiveExportAdvertisements(ctx, s.nowUTC())
	}
	return s.Discovery.ListExportAdvertisements(ctx)
}

func (s *Service) nowUTC() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) leaseTTL() time.Duration {
	if s != nil && s.LeaseTTL > 0 {
		return s.LeaseTTL
	}
	return 5 * time.Minute
}

func matchingOfferReservation(existing *HandoffOfferReservation, current HandoffOfferReservation) bool {
	if existing == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(existing.OfferID), strings.TrimSpace(current.OfferID)) &&
		strings.EqualFold(strings.TrimSpace(existing.LeaseID), strings.TrimSpace(current.LeaseID)) &&
		strings.EqualFold(strings.TrimSpace(existing.SourceAttemptID), strings.TrimSpace(current.SourceAttemptID)) &&
		strings.EqualFold(strings.TrimSpace(existing.ProvisionalAttemptID), strings.TrimSpace(current.ProvisionalAttemptID)) &&
		strings.EqualFold(strings.TrimSpace(existing.RuntimeID), strings.TrimSpace(current.RuntimeID)) &&
		existing.FencingEpoch == current.FencingEpoch
}

func (s *Service) emit(ctx context.Context, eventType string, owner identity.SubjectRef, payload map[string]any) {
	if s == nil {
		return
	}
	s.emitObservability(ctx, eventType, owner, payload)
	if s.Log == nil {
		return
	}
	body := mustJSON(payload)
	partition := s.partition()
	_, _ = s.Log.Append(ctx, partition, []core.FrameworkEvent{{
		Timestamp: time.Now().UTC(),
		Type:      eventType,
		Payload:   body,
		Partition: partition,
		Actor: core.EventActor{
			Kind:        string(owner.Kind),
			ID:          owner.ID,
			TenantID:    owner.TenantID,
			SubjectKind: string(owner.Kind),
		},
	}})
}

func (s *Service) AdvertiseNode(ctx context.Context, ad NodeAdvertisement) error {
	if s.Discovery == nil {
		return fmt.Errorf("discovery store unavailable")
	}
	if ad.ExpiresAt.IsZero() {
		ad.ExpiresAt = s.nowUTC().Add(5 * time.Minute)
	}
	return s.Discovery.UpsertNodeAdvertisement(ctx, ad)
}

func (s *Service) AdvertiseRuntime(ctx context.Context, ad RuntimeAdvertisement) error {
	if s.Discovery == nil {
		return fmt.Errorf("discovery store unavailable")
	}
	if err := SignRuntimeDescriptor(s.Signer, &ad.Runtime); err != nil {
		return err
	}
	if strings.TrimSpace(ad.Signature) == "" {
		ad.Signature = ad.Runtime.Signature
	}
	if err := validateAuthoritativeRuntime(ad); err != nil {
		return err
	}
	if ad.ExpiresAt.IsZero() {
		ad.ExpiresAt = s.nowUTC().Add(5 * time.Minute)
	}
	return s.Discovery.UpsertRuntimeAdvertisement(ctx, ad)
}

func (s *Service) AdvertiseExport(ctx context.Context, ad ExportAdvertisement) error {
	if s.Discovery == nil {
		return fmt.Errorf("discovery store unavailable")
	}
	if err := SignExportDescriptor(s.Signer, &ad.Export); err != nil {
		return err
	}
	if strings.TrimSpace(ad.Signature) == "" {
		ad.Signature = ad.Export.Signature
	}
	runtimeAd, err := s.resolveRegisteredRuntimeAdvertisement(ctx, ad.TrustDomain, ad.RuntimeID)
	if err != nil {
		return err
	}
	if runtimeAd == nil {
		return fmt.Errorf("runtime %s in trust domain %s is not registered", ad.RuntimeID, ad.TrustDomain)
	}
	if !strings.EqualFold(runtimeAd.Runtime.NodeID, ad.NodeID) {
		return fmt.Errorf("export node_id must match registered runtime node_id")
	}
	if ad.ExpiresAt.IsZero() {
		ad.ExpiresAt = s.nowUTC().Add(5 * time.Minute)
	}
	return s.Discovery.UpsertExportAdvertisement(ctx, ad)
}

func (s *Service) ResolveRoutes(ctx context.Context, req RouteSelectionRequest) ([]RouteCandidate, error) {
	if s.Discovery == nil {
		return nil, fmt.Errorf("discovery store unavailable")
	}
	if err := s.Discovery.DeleteExpired(ctx, s.nowUTC()); err != nil {
		return nil, err
	}
	exports, err := s.listLiveExportAds(ctx)
	if err != nil {
		return nil, err
	}
	runtimes, err := s.listLiveRuntimeAds(ctx)
	if err != nil {
		return nil, err
	}
	runtimeByQualified := make(map[string]RuntimeAdvertisement, len(runtimes))
	for _, runtime := range runtimes {
		runtimeByQualified[qualifiedRuntimeName(runtime.TrustDomain, runtime.Runtime.RuntimeID)] = runtime
	}

	explicitDomain := ""
	unqualifiedExport := strings.TrimSpace(req.ExportName)
	if IsQualifiedExportName(req.ExportName) {
		var parseErr error
		explicitDomain, unqualifiedExport, parseErr = ParseQualifiedExportName(req.ExportName)
		if parseErr != nil {
			return nil, parseErr
		}
	}
	lineage, err := s.resolveRouteSelectionLineage(ctx, req)
	if err != nil {
		return nil, err
	}

	candidates := make([]RouteCandidate, 0)
	for _, ad := range exports {
		if explicitDomain != "" {
			if !strings.EqualFold(ad.TrustDomain, explicitDomain) || !strings.EqualFold(ad.Export.ExportName, unqualifiedExport) {
				continue
			}
		} else {
			if !strings.EqualFold(ad.Export.ExportName, unqualifiedExport) {
				continue
			}
			if ad.Imported && !req.AllowRemote {
				continue
			}
		}
		if refusal := validateRouteAdvertisement(ad, req, s.nowUTC()); refusal != nil {
			continue
		}
		runtimeKey := qualifiedRuntimeName(ad.TrustDomain, ad.RuntimeID)
		runtimeAd, ok := runtimeByQualified[runtimeKey]
		if !ok {
			continue
		}
		runtime := &runtimeAd.Runtime
		if refusal := validateRuntimeForRoute(runtimeAd.Runtime, req, ad.Export, s.nowUTC()); refusal != nil {
			continue
		}
		if refusal := s.validateRuntimeImportEligibility(runtimeAd.Runtime); refusal != nil {
			s.emit(ctx, FrameworkEventFMPRolloutBlocked, identity.SubjectRef{}, map[string]any{
				"runtime_id":   runtimeAd.Runtime.RuntimeID,
				"node_id":      runtimeAd.Runtime.NodeID,
				"trust_domain": ad.TrustDomain,
				"reason":       refusal.Message,
			})
			continue
		}
		refusal, err := s.validateRoutePolicySelection(ctx, req, lineage, ad.TrustDomain, ad.Imported, ad.Export)
		if err != nil {
			return nil, err
		}
		if refusal != nil {
			continue
		}
		export := ad.Export
		if strings.TrimSpace(export.TrustDomain) == "" {
			export.TrustDomain = ad.TrustDomain
		}
		candidates = append(candidates, RouteCandidate{
			QualifiedExport: QualifiedExportName(ad.TrustDomain, ad.Export.ExportName),
			TrustDomain:     ad.TrustDomain,
			NodeID:          ad.NodeID,
			RuntimeID:       ad.RuntimeID,
			Imported:        ad.Imported,
			RouteMode:       resolveRouteMode(ad.Export),
			Export:          export,
			Runtime:         runtime,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return compareRouteCandidates(candidates[i], candidates[j])
	})
	return candidates, nil
}

func (s *Service) resolveRouteSelectionLineage(ctx context.Context, req RouteSelectionRequest) (*LineageRecord, error) {
	if strings.TrimSpace(req.LineageID) == "" || s.Ownership == nil {
		return nil, nil
	}
	lineage, ok, err := s.Ownership.GetLineage(ctx, req.LineageID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("lineage %s not found", req.LineageID)
	}
	return lineage, nil
}

func (s *Service) validateRoutePolicySelection(ctx context.Context, req RouteSelectionRequest, lineage *LineageRecord, trustDomain string, imported bool, destination ExportDescriptor) (*TransferRefusal, error) {
	tenantID := strings.TrimSpace(req.TenantID)
	if lineage != nil && strings.TrimSpace(lineage.TenantID) != "" {
		tenantID = lineage.TenantID
	}
	if refusal := s.validateTenantExportEnablement(ctx, tenantID, destination.ExportName); refusal != nil {
		return refusal, nil
	}
	if imported {
		if refusal := s.validateTenantFederationPolicy(ctx, tenantID, trustDomain, resolveRouteMode(destination), req.ContextSizeBytes); refusal != nil {
			return refusal, nil
		}
	}
	owner := routeSelectionOwner(req, lineage)
	if len(destination.AcceptedIdentities) > 0 {
		if subjectRefEmpty(owner) {
			return &TransferRefusal{Code: RefusalUnauthorized, Message: "route selection requires lineage owner for export identity policy"}, nil
		}
		allowed := false
		for _, candidate := range destination.AcceptedIdentities {
			if candidate == owner {
				allowed = true
				break
			}
		}
		if !allowed {
			return &TransferRefusal{Code: RefusalUnauthorized, Message: "lineage owner not authorized for export"}, nil
		}
	}
	if s.Nexus.Policies == nil {
		return nil, nil
	}
	policyReq, ok := buildRouteResumePolicyRequest(req, lineage, destination, owner)
	if !ok {
		return nil, nil
	}
	decision, err := s.Nexus.Policies.EvaluateResume(ctx, policyReq)
	if err != nil {
		return nil, err
	}
	if decision.Effect != "allow" {
		return policyDecisionRefusal(decision), nil
	}
	return nil, nil
}

func trustDomainForOffer(destination ExportDescriptor, offer HandoffOffer) string {
	if strings.TrimSpace(destination.TrustDomain) != "" {
		return strings.TrimSpace(destination.TrustDomain)
	}
	if IsQualifiedExportName(offer.DestinationExport) {
		trustDomain, _, err := ParseQualifiedExportName(offer.DestinationExport)
		if err == nil {
			return trustDomain
		}
	}
	return ""
}

func (s *Service) emitRefusal(ctx context.Context, lineageID string, refusal *TransferRefusal) {
	if refusal == nil {
		return
	}
	owner := identity.SubjectRef{}
	if lineage, ok, err := s.Ownership.GetLineage(ctx, lineageID); err == nil && ok {
		owner = lineage.Owner
	}
	s.emit(ctx, FrameworkEventFMPPolicyDenied, owner, map[string]any{
		"lineage_id": lineageID,
		"code":       refusal.Code,
		"reason":     refusal.Message,
	})
}

func (s *Service) resolveRuntimeDescriptor(ctx context.Context, runtimeID string) (RuntimeDescriptor, *TransferRefusal, error) {
	if s.Runtime == nil {
		return RuntimeDescriptor{}, nil, nil
	}
	descriptor, err := s.Runtime.Descriptor(ctx)
	if err != nil {
		return RuntimeDescriptor{}, nil, err
	}
	if strings.TrimSpace(runtimeID) != "" && descriptor.RuntimeID != "" && !strings.EqualFold(descriptor.RuntimeID, runtimeID) {
		return RuntimeDescriptor{}, nil, fmt.Errorf("runtime id %s does not match descriptor %s", runtimeID, descriptor.RuntimeID)
	}
	return descriptor, nil, nil
}

func (s *Service) validateExportPolicy(ctx context.Context, lineage LineageRecord, offer HandoffOffer, destination ExportDescriptor) *TransferRefusal {
	if refusal := s.validateTenantExportEnablement(ctx, lineage.TenantID, destination.ExportName); refusal != nil {
		return refusal
	}
	trustDomain := strings.TrimSpace(destination.TrustDomain)
	if trustDomain == "" && IsQualifiedExportName(offer.DestinationExport) {
		if parsedTrustDomain, _, err := ParseQualifiedExportName(offer.DestinationExport); err == nil {
			trustDomain = parsedTrustDomain
		}
	}
	if trustDomain != "" {
		if refusal := s.validateTenantFederationPolicy(ctx, lineage.TenantID, trustDomain, resolveRouteMode(destination), offer.ContextSizeBytes); refusal != nil {
			return refusal
		}
		if len(lineage.AllowedFederationTargets) > 0 && !containsFoldString(lineage.AllowedFederationTargets, trustDomain) {
			return &TransferRefusal{Code: RefusalUnauthorized, Message: "trust domain not allowed by lineage federation policy"}
		}
	}
	if destination.ExportName != "" && !strings.EqualFold(destination.ExportName, offer.DestinationExport) {
		return &TransferRefusal{Code: RefusalUnauthorized, Message: "destination export mismatch"}
	}
	if !exportAllowsRouteMode(destination, resolveRouteMode(destination)) {
		return &TransferRefusal{Code: RefusalUnauthorized, Message: "route mode not allowed by export transport policy"}
	}
	if len(destination.AcceptedContextClasses) > 0 && !containsFoldString(destination.AcceptedContextClasses, offer.ContextClass) {
		return &TransferRefusal{Code: RefusalUnsupportedContext, Message: "destination does not accept context class"}
	}
	if destination.MaxContextSize > 0 && offer.ContextSizeBytes > destination.MaxContextSize {
		return &TransferRefusal{Code: RefusalContextTooLarge, Message: "context exceeds destination max size"}
	}
	if destination.SensitivityLimit != "" && sensitivityRank(offer.SensitivityClass) > sensitivityRank(destination.SensitivityLimit) {
		return &TransferRefusal{Code: RefusalSensitivityDenied, Message: "sensitivity exceeds destination limit"}
	}
	if len(destination.AcceptedIdentities) > 0 {
		allowed := false
		for _, candidate := range destination.AcceptedIdentities {
			if candidate == lineage.Owner {
				allowed = true
				break
			}
		}
		if !allowed {
			return &TransferRefusal{Code: RefusalUnauthorized, Message: "lineage owner not authorized for export"}
		}
	}
	if len(destination.AllowedTaskClasses) > 0 && !containsFoldString(destination.AllowedTaskClasses, lineage.TaskClass) {
		return &TransferRefusal{Code: RefusalUnauthorized, Message: "task class not allowed by export"}
	}
	if !capabilityEnvelopeSubset(offer.RequestedCapabilityProjection, lineage.CapabilityEnvelope) {
		return &TransferRefusal{Code: RefusalUnauthorized, Message: "requested capability projection broadens lineage authority"}
	}
	return nil
}

func validateRuntimeAgainstOffer(runtime RuntimeDescriptor, offer HandoffOffer, destination ExportDescriptor) *TransferRefusal {
	return ValidateOfferCompatibility(runtime, offer, destination, time.Now().UTC())
}

func policyDecisionRefusal(decision core.PolicyDecision) *TransferRefusal {
	switch decision.Effect {
	case "deny":
		return &TransferRefusal{Code: RefusalUnauthorized, Message: fallbackMessage(decision.Reason, "denied by policy")}
	case "require_approval":
		return &TransferRefusal{Code: RefusalAdmissionClosed, Message: fallbackMessage(decision.Reason, "approval required")}
	default:
		return nil
	}
}

func resolveRouteMode(destination ExportDescriptor) RouteMode {
	if destination.RouteMode != "" {
		return destination.RouteMode
	}
	return RouteModeGateway
}

func containsFoldString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func sensitivityRank(value SensitivityClass) int {
	switch value {
	case SensitivityClassLow:
		return 1
	case SensitivityClassModerate:
		return 2
	case SensitivityClassHigh:
		return 3
	case SensitivityClassRestricted:
		return 4
	default:
		return 0
	}
}

func fallbackMessage(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func validateManifestForOffer(offer HandoffOffer, manifest ContextManifest, sealed SealedContext) error {
	if !strings.EqualFold(strings.TrimSpace(manifest.ContextID), strings.TrimSpace(offer.ContextManifestRef)) {
		return fmt.Errorf("manifest context id %s does not match offer %s", manifest.ContextID, offer.ContextManifestRef)
	}
	if !strings.EqualFold(strings.TrimSpace(manifest.ContextID), strings.TrimSpace(sealed.ContextManifestRef)) {
		return fmt.Errorf("sealed context ref %s does not match manifest %s", sealed.ContextManifestRef, manifest.ContextID)
	}
	if !strings.EqualFold(strings.TrimSpace(manifest.LineageID), strings.TrimSpace(offer.LineageID)) {
		return fmt.Errorf("manifest lineage id %s does not match offer %s", manifest.LineageID, offer.LineageID)
	}
	if !strings.EqualFold(strings.TrimSpace(manifest.AttemptID), strings.TrimSpace(offer.SourceAttemptID)) {
		return fmt.Errorf("manifest attempt id %s does not match offer %s", manifest.AttemptID, offer.SourceAttemptID)
	}
	if !strings.EqualFold(strings.TrimSpace(manifest.ContextClass), strings.TrimSpace(offer.ContextClass)) {
		return fmt.Errorf("manifest context class %s does not match offer %s", manifest.ContextClass, offer.ContextClass)
	}
	return nil
}

func validateReceiptForCommit(offer HandoffOffer, accept HandoffAccept, manifest ContextManifest, receipt ResumeReceipt) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(receipt.LineageID), strings.TrimSpace(offer.LineageID)) {
		return fmt.Errorf("receipt lineage id %s does not match offer %s", receipt.LineageID, offer.LineageID)
	}
	if !strings.EqualFold(strings.TrimSpace(receipt.AttemptID), strings.TrimSpace(accept.ProvisionalAttemptID)) {
		return fmt.Errorf("receipt attempt id %s does not match accept %s", receipt.AttemptID, accept.ProvisionalAttemptID)
	}
	if !strings.EqualFold(strings.TrimSpace(receipt.RuntimeID), strings.TrimSpace(accept.DestinationRuntimeID)) {
		return fmt.Errorf("receipt runtime id %s does not match accept %s", receipt.RuntimeID, accept.DestinationRuntimeID)
	}
	if !strings.EqualFold(strings.TrimSpace(receipt.ImportedContextID), strings.TrimSpace(manifest.ContextID)) {
		return fmt.Errorf("receipt imported context id %s does not match manifest %s", receipt.ImportedContextID, manifest.ContextID)
	}
	return nil
}

func validateImportedAttemptForContinuation(attempt AttemptRecord, lineage LineageRecord, runtimeID string) error {
	if !strings.EqualFold(strings.TrimSpace(attempt.LineageID), strings.TrimSpace(lineage.LineageID)) {
		return fmt.Errorf("attempt lineage id %s does not match lineage %s", attempt.LineageID, lineage.LineageID)
	}
	if strings.TrimSpace(runtimeID) != "" && !strings.EqualFold(strings.TrimSpace(attempt.RuntimeID), strings.TrimSpace(runtimeID)) {
		return fmt.Errorf("attempt runtime id %s does not match destination runtime %s", attempt.RuntimeID, runtimeID)
	}
	return nil
}

func validateRouteAdvertisement(ad ExportAdvertisement, req RouteSelectionRequest, now time.Time) *TransferRefusal {
	if !ad.ExpiresAt.IsZero() && now.After(ad.ExpiresAt) {
		return &TransferRefusal{Code: RefusalAdmissionClosed, Message: "export advertisement expired"}
	}
	routeMode := resolveRouteMode(ad.Export)
	if req.RequiredRouteMode != "" && !strings.EqualFold(string(routeMode), string(req.RequiredRouteMode)) {
		return &TransferRefusal{Code: RefusalUnauthorized, Message: "export route mode does not match requested route mode"}
	}
	if !exportAllowsRouteMode(ad.Export, routeMode) {
		return &TransferRefusal{Code: RefusalUnauthorized, Message: "export route mode not allowed by transport paths"}
	}
	if !ad.Export.AdmissionSummary.Available {
		return &TransferRefusal{Code: RefusalAdmissionClosed, Message: fallbackMessage(ad.Export.AdmissionSummary.Reason, "export unavailable")}
	}
	if len(ad.Export.AcceptedContextClasses) > 0 && !containsFoldString(ad.Export.AcceptedContextClasses, req.ContextClass) {
		return &TransferRefusal{Code: RefusalUnsupportedContext, Message: "export does not accept context class"}
	}
	if ad.Export.MaxContextSize > 0 && req.ContextSizeBytes > ad.Export.MaxContextSize {
		return &TransferRefusal{Code: RefusalContextTooLarge, Message: "context exceeds export max size"}
	}
	if ad.Export.SensitivityLimit != "" && sensitivityRank(req.SensitivityClass) > sensitivityRank(ad.Export.SensitivityLimit) {
		return &TransferRefusal{Code: RefusalSensitivityDenied, Message: "sensitivity exceeds export limit"}
	}
	if len(ad.Export.AllowedTaskClasses) > 0 && !containsFoldString(ad.Export.AllowedTaskClasses, req.TaskClass) {
		return &TransferRefusal{Code: RefusalUnauthorized, Message: "export does not allow requested task class"}
	}
	return nil
}

func buildRouteResumePolicyRequest(req RouteSelectionRequest, lineage *LineageRecord, destination ExportDescriptor, owner identity.SubjectRef) (ResumePolicyRequest, bool) {
	actor := req.Actor
	if subjectRefEmpty(actor) {
		if lineage != nil && !subjectRefEmpty(lineage.Owner) {
			actor = lineage.Owner
		} else if !subjectRefEmpty(owner) {
			actor = owner
		}
	}
	if subjectRefEmpty(actor) {
		return ResumePolicyRequest{}, false
	}
	lineageRecord := LineageRecord{
		LineageID:        req.LineageID,
		TenantID:         req.TenantID,
		TaskClass:        req.TaskClass,
		ContextClass:     req.ContextClass,
		SensitivityClass: req.SensitivityClass,
		Owner:            owner,
		SessionID:        req.SessionID,
		TrustClass:       req.TrustClass,
	}
	if lineage != nil {
		lineageRecord = *lineage
		if !subjectRefEmpty(owner) {
			lineageRecord.Owner = owner
		}
	}
	isOwner := req.IsOwner || (!subjectRefEmpty(lineageRecord.Owner) && actor == lineageRecord.Owner)
	return ResumePolicyRequest{
		Lineage: lineageRecord,
		Offer: HandoffOffer{
			LineageID:         lineageRecord.LineageID,
			DestinationExport: destination.ExportName,
			ContextClass:      req.ContextClass,
			ContextSizeBytes:  req.ContextSizeBytes,
			SensitivityClass:  req.SensitivityClass,
		},
		Destination: destination,
		Actor:       actor,
		IsOwner:     isOwner,
		IsDelegated: req.IsDelegated,
		RouteMode:   resolveRouteMode(destination),
	}, true
}

func routeSelectionOwner(req RouteSelectionRequest, lineage *LineageRecord) identity.SubjectRef {
	if !subjectRefEmpty(req.Owner) {
		return req.Owner
	}
	if lineage != nil {
		return lineage.Owner
	}
	return identity.SubjectRef{}
}

func subjectRefEmpty(subject identity.SubjectRef) bool {
	return strings.TrimSpace(subject.TenantID) == "" && strings.TrimSpace(subject.ID) == "" && strings.TrimSpace(string(subject.Kind)) == ""
}

func (s *Service) validateTenantExportEnablement(ctx context.Context, tenantID, exportName string) *TransferRefusal {
	if s.Nexus.Exports == nil || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(exportName) == "" {
		return nil
	}
	enabled, configured, err := s.Nexus.Exports.IsExportEnabled(ctx, tenantID, exportName)
	if err != nil {
		return &TransferRefusal{Code: RefusalAdmissionClosed, Message: err.Error()}
	}
	if configured && !enabled {
		return &TransferRefusal{Code: RefusalAdmissionClosed, Message: "export disabled for tenant"}
	}
	return nil
}

func (s *Service) validateTenantFederationPolicy(ctx context.Context, tenantID, trustDomain string, routeMode RouteMode, sizeBytes int64) *TransferRefusal {
	if s.Nexus.Federation == nil || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(trustDomain) == "" {
		return nil
	}
	policy, err := s.Nexus.Federation.GetTenantFederationPolicy(ctx, tenantID)
	if err != nil {
		return &TransferRefusal{Code: RefusalAdmissionClosed, Message: err.Error()}
	}
	if policy == nil {
		return nil
	}
	if len(policy.AllowedTrustDomains) > 0 && !containsFoldString(policy.AllowedTrustDomains, trustDomain) {
		return &TransferRefusal{Code: RefusalUnauthorized, Message: "trust domain not allowed for tenant"}
	}
	if len(policy.AllowedRouteModes) > 0 && !containsRouteMode(policy.AllowedRouteModes, routeMode) {
		return &TransferRefusal{Code: RefusalUnauthorized, Message: "route mode not allowed for tenant"}
	}
	if routeMode == RouteModeMediated && !policy.AllowMediation {
		return &TransferRefusal{Code: RefusalUnauthorized, Message: "mediation mode not allowed for tenant"}
	}
	if policy.MaxTransferBytes > 0 && sizeBytes > policy.MaxTransferBytes {
		return &TransferRefusal{Code: RefusalTransferBudget, Message: "transfer exceeds tenant federation budget"}
	}
	return nil
}

func validateRuntimeForRoute(runtime RuntimeDescriptor, req RouteSelectionRequest, export ExportDescriptor, now time.Time) *TransferRefusal {
	if runtime.MaxContextSize > 0 && req.ContextSizeBytes > runtime.MaxContextSize {
		return &TransferRefusal{Code: RefusalContextTooLarge, Message: "context exceeds runtime max size"}
	}
	if len(runtime.SupportedContextClasses) > 0 && !containsFoldString(runtime.SupportedContextClasses, req.ContextClass) {
		return &TransferRefusal{Code: RefusalUnsupportedContext, Message: "runtime does not support context class"}
	}
	requiredCompat := req.RequiredCompatibilityClass
	if requiredCompat != "" && !strings.EqualFold(runtime.CompatibilityClass, requiredCompat) {
		return &TransferRefusal{Code: RefusalIncompatibleRuntime, Message: "runtime compatibility mismatch"}
	}
	if len(export.RequiredCompatibilityClasses) > 0 && !containsFoldString(export.RequiredCompatibilityClasses, runtime.CompatibilityClass) {
		return &TransferRefusal{Code: RefusalIncompatibleRuntime, Message: "runtime compatibility not accepted by export"}
	}
	if !runtime.ExpiresAt.IsZero() && now.After(runtime.ExpiresAt) {
		return &TransferRefusal{Code: RefusalAdmissionClosed, Message: "runtime advertisement expired"}
	}
	return nil
}

func compareRouteCandidates(left, right RouteCandidate) bool {
	leftLocal := !left.Imported
	rightLocal := !right.Imported
	if leftLocal != rightLocal {
		return leftLocal
	}
	if left.RouteMode != right.RouteMode {
		return left.RouteMode == RouteModeDirect
	}
	return left.QualifiedExport < right.QualifiedExport
}

func exportAllowsRouteMode(export ExportDescriptor, mode RouteMode) bool {
	if mode == "" {
		return true
	}
	if len(export.AllowedTransportPaths) == 0 {
		return true
	}
	for _, candidate := range export.AllowedTransportPaths {
		if strings.EqualFold(string(candidate), string(mode)) {
			return true
		}
	}
	return false
}

func mustJSON(v any) []byte {
	body, _ := jsonMarshal(v)
	return body
}

var jsonMarshal = func(v any) ([]byte, error) {
	return json.Marshal(v)
}
