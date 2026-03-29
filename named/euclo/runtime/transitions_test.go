package runtime

import (
	"testing"
	"time"

	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpic"
)

func TestApplyUnitOfWorkTransitionPreservesDebugToChatImplement(t *testing.T) {
	now := time.Unix(300, 0).UTC()
	existing := UnitOfWork{
		ID:                          "uow-debug",
		RootID:                      "uow-debug",
		ModeID:                      "debug",
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityDebugInvestigate,
		CreatedAt:                   now.Add(-time.Minute),
	}
	next := UnitOfWork{
		ID:                          "task-next",
		ModeID:                      "code",
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatImplement,
	}

	state := ApplyUnitOfWorkTransition(existing, &next, now)
	if !state.Preserved || state.Rebound {
		t.Fatalf("expected preserved transition, got %#v", state)
	}
	if next.ID != existing.ID || next.PredecessorUnitOfWorkID != "" {
		t.Fatalf("expected same unit of work identity, got %#v", next)
	}
}

func TestApplyUnitOfWorkTransitionRebindsArchaeoToLocal(t *testing.T) {
	now := time.Unix(301, 0).UTC()
	existing := UnitOfWork{
		ID:                          "uow-plan",
		RootID:                      "uow-plan",
		ModeID:                      "planning",
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityArchaeologyCompilePlan,
		SemanticInputs: SemanticInputBundle{
			PatternRefs: []string{"pattern:1"},
		},
		PlanBinding: &UnitOfWorkPlanBinding{IsPlanBacked: true},
		CreatedAt:   now.Add(-time.Minute),
	}
	next := UnitOfWork{
		ID:                          "task-local",
		ModeID:                      "code",
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatAsk,
	}

	state := ApplyUnitOfWorkTransition(existing, &next, now)
	if !state.Rebound || state.Preserved {
		t.Fatalf("expected rebound transition, got %#v", state)
	}
	if next.ID == existing.ID || next.PredecessorUnitOfWorkID != existing.ID {
		t.Fatalf("expected successor linkage, got %#v", next)
	}
	if next.RootID != existing.ID {
		t.Fatalf("expected root id preserved, got %#v", next)
	}
	if next.TransitionReason != "archaeo_to_local_rebind" {
		t.Fatalf("unexpected transition reason: %#v", next)
	}
}

func TestUpdateUnitOfWorkHistoryAppendsAndReplaces(t *testing.T) {
	now := time.Unix(302, 0).UTC()
	history := UpdateUnitOfWorkHistory(nil, UnitOfWork{
		ID:                          "uow-1",
		RootID:                      "uow-1",
		ModeID:                      "debug",
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityDebugInvestigate,
		TransitionState:             UnitOfWorkTransitionState{Preserved: true},
	}, now)
	history = UpdateUnitOfWorkHistory(history, UnitOfWork{
		ID:                          "uow-2",
		RootID:                      "uow-1",
		PredecessorUnitOfWorkID:     "uow-1",
		ModeID:                      "code",
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatAsk,
		TransitionReason:            "archaeo_to_local_rebind",
		TransitionState:             UnitOfWorkTransitionState{Rebound: true},
	}, now)
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %#v", history)
	}
	if history[1].PredecessorUnitOfWorkID != "uow-1" || !history[1].Rebound {
		t.Fatalf("unexpected successor history entry: %#v", history[1])
	}
}
