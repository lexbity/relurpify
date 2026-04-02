package testscenario

import (
	"strings"
	"testing"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func AssertExplorationActive(t testing.TB, fixture *Fixture, workflowID string) {
	t.Helper()
	session, err := fixture.ArchaeologyService().LoadExplorationByWorkflow(fixture.Context(), workflowID)
	if err != nil {
		t.Fatalf("load exploration: %v", err)
	}
	if session == nil || session.Status != archaeodomain.ExplorationStatusActive {
		t.Fatalf("expected active exploration, got %#v", session)
	}
}

func AssertExplorationStale(t testing.TB, fixture *Fixture, workflowID string) {
	t.Helper()
	session, err := fixture.ArchaeologyService().LoadExplorationByWorkflow(fixture.Context(), workflowID)
	if err != nil {
		t.Fatalf("load exploration: %v", err)
	}
	if session == nil || session.Status != archaeodomain.ExplorationStatusStale || !session.RecomputeRequired {
		t.Fatalf("expected stale exploration, got %#v", session)
	}
}

func AssertTensionStatus(t testing.TB, fixture *Fixture, workflowID, tensionID string, want archaeodomain.TensionStatus) {
	t.Helper()
	RequireTensionStatus(t, fixture.TensionService(), workflowID, tensionID, want)
}

func AssertPlanVersion(t testing.TB, fixture *Fixture, workflowID string, wantVersion int, wantStatus archaeodomain.LivingPlanVersionStatus) {
	t.Helper()
	record, err := fixture.PlansService().LoadVersion(fixture.Context(), workflowID, wantVersion)
	if err != nil {
		t.Fatalf("load plan version: %v", err)
	}
	if record == nil {
		t.Fatalf("expected plan version %d", wantVersion)
	}
	if record.Status != wantStatus {
		t.Fatalf("expected plan version status %q, got %q", wantStatus, record.Status)
	}
}

func AssertDisposition(t testing.TB, eval interface {
	GetDisposition() archaeodomain.ExecutionDisposition
}, want archaeodomain.ExecutionDisposition) {
	t.Helper()
	if eval == nil || eval.GetDisposition() != want {
		t.Fatalf("expected disposition %q, got %#v", want, eval)
	}
}

func AssertProvenanceComplete(t testing.TB, record *archaeodomain.ProvenanceRecord) {
	t.Helper()
	if record == nil {
		t.Fatal("expected provenance record")
	}
	if record.WorkflowID == "" {
		t.Fatal("expected workflow id")
	}
	if record.Learning == nil || record.Tensions == nil || record.PlanVersions == nil || record.Mutations == nil {
		t.Fatalf("expected provenance slices to be initialized: %#v", record)
	}
}

func AssertPhase(t testing.TB, fixture *Fixture, workflowID string, want archaeodomain.EucloPhase) {
	t.Helper()
	RequirePhase(t, fixture.PhaseService(), workflowID, want)
}

func AssertConvergenceFailureContains(t testing.TB, failure *frameworkplan.ConvergenceFailure, needle string) {
	t.Helper()
	if failure == nil || !strings.Contains(failure.Description, needle) {
		t.Fatalf("expected convergence failure containing %q, got %#v", needle, failure)
	}
}
