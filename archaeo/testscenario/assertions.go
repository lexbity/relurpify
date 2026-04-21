package testscenario

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"

	archaeoarch "codeburg.org/lexbit/relurpify/archaeo/archaeology"
	archaeoconvergence "codeburg.org/lexbit/relurpify/archaeo/convergence"
	archaeodeferred "codeburg.org/lexbit/relurpify/archaeo/deferred"
	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeophases "codeburg.org/lexbit/relurpify/archaeo/phases"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
)

func RequireActiveTensionIDs(tb testing.TB, svc archaeotensions.Service, workflowID string, want ...string) {
	tb.Helper()
	records, err := svc.ActiveByWorkflow(context.Background(), workflowID)
	if err != nil {
		tb.Fatalf("load active tensions: %v", err)
	}
	got := make([]string, 0, len(records))
	for _, record := range records {
		got = append(got, record.ID)
	}
	requireStringSet(tb, got, want, "active tension ids")
}

func RequirePendingLearningIDs(tb testing.TB, svc archaeolearning.Service, workflowID string, want ...string) {
	tb.Helper()
	records, err := svc.Pending(context.Background(), workflowID)
	if err != nil {
		tb.Fatalf("load pending learning: %v", err)
	}
	got := make([]string, 0, len(records))
	for _, record := range records {
		got = append(got, record.ID)
	}
	requireStringSet(tb, got, want, "pending learning ids")
}

func RequirePhase(tb testing.TB, svc archaeophases.Service, workflowID string, want archaeodomain.EucloPhase) {
	tb.Helper()
	record, ok, err := svc.Load(context.Background(), workflowID)
	if err != nil {
		tb.Fatalf("load phase state: %v", err)
	}
	if !ok || record == nil {
		tb.Fatalf("expected phase %q, got none", want)
	}
	if record.CurrentPhase != want {
		tb.Fatalf("expected phase %q, got %q", want, record.CurrentPhase)
	}
}

func RequireActivePlanVersion(tb testing.TB, svc archaeoplans.Service, workflowID string, want int) {
	tb.Helper()
	record, err := svc.LoadActiveVersion(context.Background(), workflowID)
	if err != nil {
		tb.Fatalf("load active plan version: %v", err)
	}
	if record == nil {
		tb.Fatalf("expected active plan version %d, got none", want)
	}
	if record.Version != want {
		tb.Fatalf("expected active plan version %d, got %d", want, record.Version)
	}
}

func RequireLineageVersions(tb testing.TB, svc archaeoplans.Service, workflowID string, want ...int) {
	tb.Helper()
	lineage, err := svc.LoadLineage(context.Background(), workflowID)
	if err != nil {
		tb.Fatalf("load lineage: %v", err)
	}
	if lineage == nil {
		tb.Fatalf("expected lineage versions %v, got none", want)
	}
	got := make([]int, 0, len(lineage.Versions))
	for _, version := range lineage.Versions {
		got = append(got, version.Version)
	}
	requireIntSlice(tb, got, want, "lineage versions")
}

func RequireExplorationStatus(tb testing.TB, svc archaeoarch.Service, explorationID string, want archaeodomain.ExplorationStatus) {
	tb.Helper()
	record, err := svc.LoadExplorationSession(context.Background(), explorationID)
	if err != nil {
		tb.Fatalf("load exploration session: %v", err)
	}
	if record == nil {
		tb.Fatalf("expected exploration status %q, got none", want)
	}
	if record.Status != want {
		tb.Fatalf("expected exploration status %q, got %q", want, record.Status)
	}
}

func RequireExplorationSnapshotRevision(tb testing.TB, svc archaeoarch.Service, workflowID, snapshotID, want string) {
	tb.Helper()
	record, err := svc.LoadExplorationSnapshotByWorkflow(context.Background(), workflowID, snapshotID)
	if err != nil {
		tb.Fatalf("load exploration snapshot: %v", err)
	}
	if record == nil {
		tb.Fatalf("expected snapshot revision %q, got none", want)
	}
	if record.BasedOnRevision != want {
		tb.Fatalf("expected snapshot revision %q, got %q", want, record.BasedOnRevision)
	}
}

func RequireMutationDisposition(tb testing.TB, store archaeoevents.WorkflowLog, workflowID string, want archaeodomain.ExecutionDisposition) {
	tb.Helper()
	records, err := archaeoevents.ReadMutationEvents(context.Background(), store.Store, workflowID)
	if err != nil {
		tb.Fatalf("read mutation events: %v", err)
	}
	if len(records) == 0 {
		tb.Fatalf("expected mutation disposition %q, got no mutation events", want)
	}
	got := records[len(records)-1].Disposition
	if got != want {
		tb.Fatalf("expected mutation disposition %q, got %q", want, got)
	}
}

func RequireTensionStatus(tb testing.TB, svc archaeotensions.Service, workflowID, tensionID string, want archaeodomain.TensionStatus) {
	tb.Helper()
	record, err := svc.Load(context.Background(), workflowID, tensionID)
	if err != nil {
		tb.Fatalf("load tension: %v", err)
	}
	if record == nil {
		tb.Fatalf("expected tension status %q, got none", want)
	}
	if record.Status != want {
		tb.Fatalf("expected tension status %q, got %q", want, record.Status)
	}
}

func RequireConvergenceState(tb testing.TB, svc archaeoconvergence.Service, workspaceID string, status archaeodomain.ConvergenceResolutionStatus) {
	tb.Helper()
	proj, err := svc.CurrentByWorkspace(context.Background(), workspaceID)
	if err != nil {
		tb.Fatalf("load convergence projection: %v", err)
	}
	if proj == nil || proj.Current == nil {
		tb.Fatalf("expected convergence state %q, got none", status)
	}
	if proj.Current.Status != status {
		tb.Fatalf("expected convergence status %q, got %q", status, proj.Current.Status)
	}
}

func RequireDeferredDraftStatus(tb testing.TB, svc archaeodeferred.Service, workflowID, recordID string, status archaeodomain.DeferredDraftStatus) {
	tb.Helper()
	record, err := svc.Load(context.Background(), workflowID, recordID)
	if err != nil {
		tb.Fatalf("load deferred draft: %v", err)
	}
	if record == nil {
		tb.Fatalf("expected deferred draft %s with status %q, got none", recordID, status)
	}
	if record.Status != status {
		tb.Fatalf("expected deferred draft status %q, got %q", status, record.Status)
	}
}

func requireStringSet(tb testing.TB, got []string, want []string, label string) {
	tb.Helper()
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	slices.Sort(got)
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		tb.Fatalf("%s mismatch: got %v want %v", label, got, want)
	}
}

func requireIntSlice(tb testing.TB, got []int, want []int, label string) {
	tb.Helper()
	got = append([]int(nil), got...)
	want = append([]int(nil), want...)
	slices.Sort(got)
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		tb.Fatalf("%s mismatch: got %v want %v", label, got, want)
	}
}

func RequireEqual[T any](tb testing.TB, got, want T, label string) {
	tb.Helper()
	if !reflect.DeepEqual(got, want) {
		tb.Fatalf("%s mismatch: got=%v want=%v", label, got, want)
	}
}

func RequireNotNil(tb testing.TB, value any, label string) {
	tb.Helper()
	if value == nil {
		tb.Fatalf("%s unexpectedly nil", label)
	}
}

func Require(tb testing.TB, ok bool, format string, args ...any) {
	tb.Helper()
	if !ok {
		tb.Fatalf(format, args...)
	}
}

func Failf(tb testing.TB, format string, args ...any) {
	tb.Helper()
	tb.Fatalf(format, args...)
}

func Message(label string, value any) string {
	return fmt.Sprintf("%s=%v", label, value)
}
