package tensions_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestServiceCreateOrUpdateDedupesBySourceRef(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "track tensions",
		Status:      memory.WorkflowRunStatusRunning,
	}))

	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	var seq int
	svc := tensions.Service{
		Store: store,
		Now:   func() time.Time { return now },
		NewID: func(prefix string) string {
			seq++
			return fmt.Sprintf("%s-%d", prefix, seq)
		},
	}

	first, err := svc.CreateOrUpdate(ctx, tensions.CreateInput{
		WorkflowID:      "wf-1",
		ExplorationID:   "explore-1",
		SnapshotID:      "snapshot-1",
		SourceRef:       "gap-1",
		AnchorRefs:      []string{"anchor-1"},
		SymbolScope:     []string{"symbol-1"},
		Kind:            "intent_gap",
		Description:     "Boundary contract drift",
		Severity:        "significant",
		Status:          archaeodomain.TensionUnresolved,
		BasedOnRevision: "rev-a",
	})
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, "tension-1", first.ID)

	second, err := svc.CreateOrUpdate(ctx, tensions.CreateInput{
		WorkflowID:         "wf-1",
		ExplorationID:      "explore-1",
		SnapshotID:         "snapshot-2",
		SourceRef:          "gap-1",
		AnchorRefs:         []string{"anchor-1"},
		SymbolScope:        []string{"symbol-1"},
		Kind:               "intent_gap",
		Description:        "Boundary contract drift updated",
		Severity:           "critical",
		Status:             archaeodomain.TensionUnresolved,
		RelatedPlanStepIDs: []string{"step-1"},
		BasedOnRevision:    "rev-b",
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, "Boundary contract drift updated", second.Description)
	require.Equal(t, []string{"step-1"}, second.RelatedPlanStepIDs)
	require.Equal(t, "snapshot-2", second.SnapshotID)

	all, err := svc.ListByWorkflow(ctx, "wf-1")
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, "rev-b", all[0].BasedOnRevision)

	mutations, err := archaeoevents.ReadMutationEvents(ctx, store, "wf-1")
	require.NoError(t, err)
	require.Len(t, mutations, 2)
	require.Equal(t, archaeodomain.MutationBlockingSemantic, mutations[0].Category)
	require.Equal(t, archaeodomain.MutationStepInvalidation, mutations[1].Category)
}

func TestServiceActiveTensionsExcludesResolvedAndAccepted(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-2",
		TaskID:      "task-2",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "track tensions",
		Status:      memory.WorkflowRunStatusRunning,
	}))

	var seq int
	svc := tensions.Service{
		Store: store,
		NewID: func(string) string {
			seq++
			return fmt.Sprintf("tension-%d", seq)
		},
	}
	_, err := svc.CreateOrUpdate(ctx, tensions.CreateInput{
		WorkflowID:  "wf-2",
		SourceRef:   "gap-live",
		Kind:        "intent_gap",
		Description: "Live contradiction",
		Status:      archaeodomain.TensionUnresolved,
	})
	require.NoError(t, err)
	resolved, err := svc.CreateOrUpdate(ctx, tensions.CreateInput{
		WorkflowID:  "wf-2",
		SourceRef:   "gap-done",
		Kind:        "intent_gap",
		Description: "Resolved contradiction",
		Status:      archaeodomain.TensionResolved,
	})
	require.NoError(t, err)
	_, err = svc.UpdateStatus(ctx, "wf-2", resolved.ID, archaeodomain.TensionResolved, nil)
	require.NoError(t, err)

	mutations, err := archaeoevents.ReadMutationEvents(ctx, store, "wf-2")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(mutations), 2)
	require.Equal(t, archaeodomain.MutationObservation, mutations[len(mutations)-1].Category)

	active, err := svc.ActiveTensions(ctx)
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, []string{"tension-1"}, active)
}

func TestServiceSummariesReflectWorkflowAndExplorationState(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-summary",
		TaskID:      "task-summary",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "track tensions",
		Status:      memory.WorkflowRunStatusRunning,
	}))

	var seq int
	svc := tensions.Service{
		Store: store,
		NewID: func(string) string {
			seq++
			return fmt.Sprintf("tension-%d", seq)
		},
	}
	_, err := svc.CreateOrUpdate(ctx, tensions.CreateInput{
		WorkflowID:    "wf-summary",
		ExplorationID: "explore-1",
		SourceRef:     "gap-a",
		Kind:          "intent_gap",
		Description:   "critical unresolved",
		Severity:      "critical",
		Status:        archaeodomain.TensionUnresolved,
	})
	require.NoError(t, err)
	_, err = svc.CreateOrUpdate(ctx, tensions.CreateInput{
		WorkflowID:    "wf-summary",
		ExplorationID: "explore-1",
		SourceRef:     "gap-b",
		Kind:          "intent_gap",
		Description:   "accepted debt",
		Severity:      "minor",
		Status:        archaeodomain.TensionAccepted,
	})
	require.NoError(t, err)

	workflowSummary, err := svc.SummaryByWorkflow(ctx, "wf-summary")
	require.NoError(t, err)
	require.NotNil(t, workflowSummary)
	require.Equal(t, "wf-summary", workflowSummary.WorkflowID)
	require.Equal(t, 2, workflowSummary.Total)
	require.Equal(t, 1, workflowSummary.Active)
	require.Equal(t, 1, workflowSummary.Accepted)
	require.Equal(t, 1, workflowSummary.Unresolved)
	require.Equal(t, 1, workflowSummary.BlockingCount)
	require.Equal(t, 1, workflowSummary.AcceptedDebt)
	require.Equal(t, 1, workflowSummary.BySeverity["critical"])

	explorationSummary, err := svc.SummaryByExploration(ctx, "explore-1")
	require.NoError(t, err)
	require.NotNil(t, explorationSummary)
	require.Equal(t, "explore-1", explorationSummary.ExplorationID)
	require.Equal(t, 2, explorationSummary.Total)
}

func newWorkflowStore(t *testing.T) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})
	return store
}
