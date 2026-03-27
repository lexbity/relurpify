package requests

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	archaeodecisions "github.com/lexcodex/relurpify/archaeo/decisions"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestServiceRequestLifecycle(t *testing.T) {
	ctx := context.Background()
	store := openWorkflowStore(t, "wf-requests-lifecycle")
	now := time.Date(2026, 3, 27, 18, 0, 0, 0, time.UTC)
	svc := Service{
		Store: store,
		Now:   func() time.Time { return now },
		NewID: func(prefix string) string { return prefix + "-1" },
	}

	version := 2
	record, err := svc.Create(ctx, CreateInput{
		WorkflowID:      "wf-requests-lifecycle",
		ExplorationID:   "explore-1",
		PlanID:          "plan-1",
		PlanVersion:     &version,
		Kind:            archaeodomain.RequestPlanReformation,
		Title:           "Reform active plan",
		Description:     "Recompute after drift.",
		RequestedBy:     "test",
		SubjectRefs:     []string{"tension-1"},
		Input:           map[string]any{"reason": "drift"},
		BasedOnRevision: "rev-1",
	})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.RequestStatusPending, record.Status)

	record, err = svc.Dispatch(ctx, record.WorkflowID, record.ID, map[string]any{"provider": "relurpic"})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.RequestStatusDispatched, record.Status)

	record, err = svc.Start(ctx, record.WorkflowID, record.ID, map[string]any{"dispatch_id": "disp-1"})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.RequestStatusRunning, record.Status)
	require.NotNil(t, record.StartedAt)

	record, err = svc.Complete(ctx, CompleteInput{
		WorkflowID: record.WorkflowID,
		RequestID:  record.ID,
		Result: archaeodomain.RequestResult{
			Kind:    "plan_version",
			RefID:   "plan-1:v3",
			Summary: "Created successor draft",
		},
	})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.RequestStatusCompleted, record.Status)
	require.NotNil(t, record.Result)
	require.Equal(t, "plan-1:v3", record.Result.RefID)
	require.NotNil(t, record.CompletedAt)

	loaded, ok, err := svc.Load(ctx, record.WorkflowID, record.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, archaeodomain.RequestStatusCompleted, loaded.Status)
	require.Equal(t, "plan-1:v3", loaded.Result.RefID)

	pending, err := svc.Pending(ctx, record.WorkflowID)
	require.NoError(t, err)
	require.Empty(t, pending)

	log := &archaeoevents.WorkflowLog{Store: store}
	events, err := log.Read(ctx, record.WorkflowID, 0, 0, false)
	require.NoError(t, err)
	require.Len(t, events, 4)
	require.Equal(t, archaeoevents.EventRequestCreated, events[0].Type)
	require.Equal(t, archaeoevents.EventRequestDispatched, events[1].Type)
	require.Equal(t, archaeoevents.EventRequestStarted, events[2].Type)
	require.Equal(t, archaeoevents.EventRequestCompleted, events[3].Type)
}

func TestServiceFailAndPending(t *testing.T) {
	ctx := context.Background()
	store := openWorkflowStore(t, "wf-requests-failed")
	now := time.Date(2026, 3, 27, 19, 0, 0, 0, time.UTC)
	svc := Service{
		Store: store,
		Now:   func() time.Time { return now },
		NewID: func(prefix string) string { return prefix + "-1" },
	}

	record, err := svc.Create(ctx, CreateInput{
		WorkflowID: "wf-requests-failed",
		Kind:       archaeodomain.RequestTensionAnalysis,
		Title:      "Analyze tensions",
	})
	require.NoError(t, err)

	pending, err := svc.Pending(ctx, record.WorkflowID)
	require.NoError(t, err)
	require.Len(t, pending, 1)

	record, err = svc.Fail(ctx, record.WorkflowID, record.ID, "provider unavailable", true)
	require.NoError(t, err)
	require.Equal(t, archaeodomain.RequestStatusFailed, record.Status)
	require.Equal(t, 1, record.RetryCount)
	require.Equal(t, "provider unavailable", record.ErrorText)

	pending, err = svc.Pending(ctx, record.WorkflowID)
	require.NoError(t, err)
	require.Empty(t, pending)
}

func TestExpireClaimsAndCreateStaleDecision(t *testing.T) {
	ctx := context.Background()
	store := openWorkflowStore(t, "wf-requests-expire")
	now := time.Date(2026, 3, 27, 20, 0, 0, 0, time.UTC)
	svc := Service{
		Store: store,
		Now:   func() time.Time { return now },
		NewID: func(prefix string) string { return prefix + "-1" },
	}

	record, err := svc.Create(ctx, CreateInput{
		WorkflowID:      "wf-requests-expire",
		Kind:            archaeodomain.RequestPatternSurfacing,
		Title:           "Surface patterns",
		CorrelationID:   "req-1",
		IdempotencyKey:  "req-1",
		Input:           map[string]any{"workspace_id": "/workspace/req"},
		BasedOnRevision: "rev-1",
	})
	require.NoError(t, err)
	record, err = svc.Dispatch(ctx, record.WorkflowID, record.ID, nil)
	require.NoError(t, err)
	record, err = svc.Claim(ctx, ClaimInput{
		WorkflowID: record.WorkflowID,
		RequestID:  record.ID,
		ClaimedBy:  "executor-1",
		LeaseTTL:   time.Minute,
	})
	require.NoError(t, err)

	svc.Now = func() time.Time { return now.Add(2 * time.Minute) }
	expired, err := svc.ExpireClaims(ctx, record.WorkflowID)
	require.NoError(t, err)
	require.Len(t, expired, 1)
	require.Equal(t, archaeodomain.RequestStatusDispatched, expired[0].Status)

	record, validity, err := svc.ApplyFulfillment(ctx, ApplyFulfillmentInput{
		WorkflowID:      expired[0].WorkflowID,
		RequestID:       expired[0].ID,
		CurrentRevision: "rev-2",
		Fulfillment: archaeodomain.RequestFulfillment{
			Kind:        "pattern_records",
			RefID:       "pattern-1",
			Summary:     "stale",
			ExecutorRef: "euclo-run-1",
			SessionRef:  "session-1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.RequestValidityInvalidated, validity)
	require.Equal(t, archaeodomain.RequestStatusInvalidated, record.Status)

	decisions, err := (archaeodecisions.Service{Store: store}).ListByWorkspace(ctx, "/workspace/req")
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	require.Equal(t, archaeodomain.DecisionKindStaleResult, decisions[0].Kind)
	require.Equal(t, record.ID, decisions[0].RelatedRequestID)
}

func TestConcurrentClaimSingleWinner(t *testing.T) {
	ctx := context.Background()
	store := openWorkflowStore(t, "wf-requests-concurrent-claim")
	now := time.Date(2026, 3, 27, 20, 30, 0, 0, time.UTC)
	svc := Service{
		Store: store,
		Now:   func() time.Time { return now },
		NewID: func(prefix string) string { return prefix + "-1" },
	}

	record, err := svc.Create(ctx, CreateInput{
		WorkflowID: "wf-requests-concurrent-claim",
		Kind:       archaeodomain.RequestPatternSurfacing,
		Title:      "Concurrent claim",
	})
	require.NoError(t, err)
	record, err = svc.Dispatch(ctx, record.WorkflowID, record.ID, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	results := make(chan *archaeodomain.RequestRecord, 2)
	for _, claimer := range []string{"executor-a", "executor-b"} {
		wg.Add(1)
		go func(claimer string) {
			defer wg.Done()
			updated, err := svc.Claim(ctx, ClaimInput{
				WorkflowID: record.WorkflowID,
				RequestID:  record.ID,
				ClaimedBy:  claimer,
				LeaseTTL:   time.Minute,
			})
			require.NoError(t, err)
			results <- updated
		}(claimer)
	}
	wg.Wait()
	close(results)

	final, ok, err := svc.Load(ctx, record.WorkflowID, record.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, archaeodomain.RequestStatusRunning, final.Status)
	require.NotEmpty(t, final.ClaimedBy)
	require.Equal(t, 1, final.Attempt)
	for updated := range results {
		require.NotNil(t, updated)
		require.Equal(t, final.ClaimedBy, updated.ClaimedBy)
	}
}

func TestConcurrentApplyIndependentFulfillments(t *testing.T) {
	ctx := context.Background()
	store := openWorkflowStore(t, "wf-requests-concurrent-apply")
	now := time.Date(2026, 3, 27, 21, 0, 0, 0, time.UTC)
	var seq atomic.Int64
	svc := Service{
		Store: store,
		Now:   func() time.Time { return now },
		NewID: func(prefix string) string { return fmt.Sprintf("%s-%d", prefix, seq.Add(1)) },
	}

	requestIDs := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		record, err := svc.Create(ctx, CreateInput{
			WorkflowID:      "wf-requests-concurrent-apply",
			Kind:            archaeodomain.RequestProspectiveAnalysis,
			Title:           "Concurrent apply",
			IdempotencyKey:  "apply-" + string(rune('a'+i)),
			Input:           map[string]any{"ordinal": i},
			BasedOnRevision: "rev-1",
		})
		require.NoError(t, err)
		record, err = svc.Dispatch(ctx, record.WorkflowID, record.ID, nil)
		require.NoError(t, err)
		record, err = svc.Claim(ctx, ClaimInput{
			WorkflowID: record.WorkflowID,
			RequestID:  record.ID,
			ClaimedBy:  "executor",
			LeaseTTL:   time.Minute,
		})
		require.NoError(t, err)
		requestIDs = append(requestIDs, record.ID)
	}

	var wg sync.WaitGroup
	for i, requestID := range requestIDs {
		wg.Add(1)
		go func(i int, requestID string) {
			defer wg.Done()
			updated, validity, err := svc.ApplyFulfillment(ctx, ApplyFulfillmentInput{
				WorkflowID:      "wf-requests-concurrent-apply",
				RequestID:       requestID,
				CurrentRevision: "rev-1",
				Fulfillment: archaeodomain.RequestFulfillment{
					Kind:        "result",
					RefID:       "result-" + string(rune('a'+i)),
					Summary:     "fulfilled",
					ExecutorRef: "executor",
				},
			})
			require.NoError(t, err)
			require.Equal(t, archaeodomain.RequestValidityValid, validity)
			require.Equal(t, archaeodomain.RequestStatusCompleted, updated.Status)
		}(i, requestID)
	}
	wg.Wait()

	all, err := svc.ListByWorkflow(ctx, "wf-requests-concurrent-apply")
	require.NoError(t, err)
	require.Len(t, all, len(requestIDs))
	for _, record := range all {
		require.Equal(t, archaeodomain.RequestStatusCompleted, record.Status)
	}
}

func openWorkflowStore(t *testing.T, workflowID string) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task-" + workflowID,
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "request workflow",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))
	return store
}
