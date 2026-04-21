package runtime

import (
	"fmt"
	"strings"
	"time"

	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
)

func ApplyUnitOfWorkTransition(existing UnitOfWork, next *UnitOfWork, now time.Time) UnitOfWorkTransitionState {
	if next == nil {
		return UnitOfWorkTransitionState{}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if shouldPreserveExistingTransition(existing, *next) {
		next.ID = existing.ID
		next.RootID = firstNonEmpty(existing.RootID, existing.ID)
		next.PredecessorUnitOfWorkID = existing.PredecessorUnitOfWorkID
		next.TransitionReason = existing.TransitionReason
		next.TransitionState = existing.TransitionState
		if next.CreatedAt.IsZero() {
			next.CreatedAt = existing.CreatedAt
		}
		return existing.TransitionState
	}
	if strings.TrimSpace(existing.ID) == "" {
		next.RootID = firstNonEmpty(next.RootID, next.ID)
		state := UnitOfWorkTransitionState{
			CurrentUnitOfWorkID:        next.ID,
			RootUnitOfWorkID:           next.RootID,
			CurrentModeID:              next.ModeID,
			CurrentPrimaryCapabilityID: next.PrimaryRelurpicCapabilityID,
			TransitionCompatibilityOK:  true,
			UpdatedAt:                  now,
		}
		next.TransitionState = state
		return state
	}

	previousArchaeo := workUnitUsesArchaeoContext(existing)
	currentArchaeo := workUnitUsesArchaeoContext(*next)
	compatOK := transitionCompatible(existing.PrimaryRelurpicCapabilityID, *next)
	reason := transitionReason(existing, *next, previousArchaeo, currentArchaeo, compatOK)
	rebind := shouldRebindUnitOfWork(existing, *next, previousArchaeo, currentArchaeo, compatOK)

	rootID := firstNonEmpty(existing.RootID, existing.ID)
	state := UnitOfWorkTransitionState{
		PreviousUnitOfWorkID:        existing.ID,
		CurrentUnitOfWorkID:         next.ID,
		RootUnitOfWorkID:            rootID,
		PreviousModeID:              existing.ModeID,
		CurrentModeID:               next.ModeID,
		PreviousPrimaryCapabilityID: existing.PrimaryRelurpicCapabilityID,
		CurrentPrimaryCapabilityID:  next.PrimaryRelurpicCapabilityID,
		Reason:                      reason,
		PreviousArchaeoContext:      previousArchaeo,
		CurrentArchaeoContext:       currentArchaeo,
		TransitionCompatibilityOK:   compatOK,
		UpdatedAt:                   now,
	}
	if rebind {
		next.PredecessorUnitOfWorkID = existing.ID
		next.RootID = rootID
		next.ID = transitionedUnitOfWorkID(*next, now)
		state.CurrentUnitOfWorkID = next.ID
		state.Rebound = true
		next.CreatedAt = now
	} else {
		next.ID = existing.ID
		next.RootID = rootID
		if next.CreatedAt.IsZero() {
			next.CreatedAt = existing.CreatedAt
		}
		state.Preserved = true
	}
	next.TransitionReason = reason
	next.TransitionState = state
	return state
}

func shouldPreserveExistingTransition(existing, next UnitOfWork) bool {
	if strings.TrimSpace(existing.ID) == "" {
		return false
	}
	if strings.TrimSpace(existing.TransitionState.PreviousUnitOfWorkID) == "" {
		return false
	}
	return existing.ModeID == next.ModeID &&
		existing.PrimaryRelurpicCapabilityID == next.PrimaryRelurpicCapabilityID &&
		existing.ID == firstNonEmpty(next.ID, existing.ID)
}

func shouldRebindUnitOfWork(previous, current UnitOfWork, previousArchaeo, currentArchaeo, compatOK bool) bool {
	if strings.TrimSpace(previous.ID) == "" {
		return false
	}
	if primaryArchaeoAssociated(previous.PrimaryRelurpicCapabilityID) && !primaryArchaeoAssociated(current.PrimaryRelurpicCapabilityID) {
		return true
	}
	if previousArchaeo && !currentArchaeo {
		return true
	}
	if !compatOK {
		return true
	}
	return false
}

func transitionReason(previous, current UnitOfWork, previousArchaeo, currentArchaeo, compatOK bool) string {
	switch {
	case primaryArchaeoAssociated(previous.PrimaryRelurpicCapabilityID) && !primaryArchaeoAssociated(current.PrimaryRelurpicCapabilityID):
		return "archaeo_to_local_rebind"
	case previousArchaeo && !currentArchaeo:
		return "archaeo_to_local_rebind"
	case !compatOK:
		return "incompatible_mode_transition"
	case previous.PrimaryRelurpicCapabilityID != current.PrimaryRelurpicCapabilityID:
		return "capability_owner_transition"
	case previous.ModeID != current.ModeID:
		return "mode_transition"
	default:
		return "preserve_continuity"
	}
}

func primaryArchaeoAssociated(capabilityID string) bool {
	desc, ok := euclorelurpic.DefaultRegistry().Lookup(capabilityID)
	return ok && desc.ArchaeoAssociated
}

func transitionCompatible(primaryCapabilityID string, next UnitOfWork) bool {
	target := strings.TrimSpace(next.ModeID)
	if desc, ok := euclorelurpic.DefaultRegistry().Lookup(next.PrimaryRelurpicCapabilityID); ok && strings.TrimSpace(desc.PrimaryMode()) != "" {
		target = strings.TrimSpace(desc.PrimaryMode())
	}
	if strings.TrimSpace(primaryCapabilityID) == "" || target == "" {
		return true
	}
	desc, ok := euclorelurpic.DefaultRegistry().Lookup(primaryCapabilityID)
	if !ok || len(desc.TransitionCompatible) == 0 {
		return true
	}
	for _, mode := range desc.TransitionCompatible {
		if strings.EqualFold(strings.TrimSpace(mode), target) {
			return true
		}
	}
	return false
}

func workUnitUsesArchaeoContext(work UnitOfWork) bool {
	if work.PlanBinding != nil && work.PlanBinding.IsPlanBacked {
		return true
	}
	if semanticBundleMaterial(work.SemanticInputs) {
		return true
	}
	if desc, ok := euclorelurpic.DefaultRegistry().Lookup(work.PrimaryRelurpicCapabilityID); ok && desc.ArchaeoAssociated {
		return true
	}
	for _, id := range work.SupportingRelurpicCapabilityIDs {
		if desc, ok := euclorelurpic.DefaultRegistry().Lookup(id); ok && desc.ArchaeoAssociated {
			return true
		}
	}
	return false
}

func transitionedUnitOfWorkID(work UnitOfWork, now time.Time) string {
	base := firstNonEmpty(work.ExecutionID, work.RunID, work.WorkflowID, work.ID, "uow")
	return fmt.Sprintf("%s-transition-%d", base, now.UnixNano())
}

func UpdateUnitOfWorkHistory(existing []UnitOfWorkHistoryEntry, work UnitOfWork, now time.Time) []UnitOfWorkHistoryEntry {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	entry := UnitOfWorkHistoryEntry{
		UnitOfWorkID:                work.ID,
		RootUnitOfWorkID:            firstNonEmpty(work.RootID, work.ID),
		PredecessorUnitOfWorkID:     work.PredecessorUnitOfWorkID,
		ModeID:                      work.ModeID,
		PrimaryRelurpicCapabilityID: work.PrimaryRelurpicCapabilityID,
		TransitionReason:            work.TransitionReason,
		Rebound:                     work.TransitionState.Rebound,
		Preserved:                   work.TransitionState.Preserved,
		UpdatedAt:                   now,
	}
	out := append([]UnitOfWorkHistoryEntry(nil), existing...)
	for idx := range out {
		if out[idx].UnitOfWorkID == entry.UnitOfWorkID {
			out[idx] = entry
			return out
		}
	}
	return append(out, entry)
}
