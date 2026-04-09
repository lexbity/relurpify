package deferred

import (
	"context"
	"strconv"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/stretchr/testify/require"
)

func TestServiceCollectionsAndHelpers(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-deferred-more")
	now := time.Date(2026, 3, 28, 1, 0, 0, 0, time.UTC)
	seq := 0
	svc := Service{
		Store: store,
		Now:   func() time.Time { return now },
		NewID: func(prefix string) string {
			seq++
			return prefix + "-custom-" + strconv.Itoa(seq)
		},
	}

	rec, err := (Service{}).CreateOrUpdate(ctx, CreateInput{})
	require.NoError(t, err)
	require.Nil(t, rec)
	rec, err = svc.CreateOrUpdate(ctx, CreateInput{WorkspaceID: "/workspace/deferred-more", WorkflowID: "wf-deferred-more"})
	require.NoError(t, err)
	require.Nil(t, rec)

	open, err := svc.CreateOrUpdate(ctx, CreateInput{
		WorkspaceID:   "/workspace/deferred-more",
		WorkflowID:    "wf-deferred-more",
		PlanID:        "plan-1",
		RequestID:     "request-1",
		PlanVersion:   intPtr(1),
		AmbiguityKey:  "step-1:type-choice",
		Title:         "Need type choice",
		CommentRefs:   []string{" comment-1 ", "comment-1"},
		Metadata:      map[string]any{"note": "keep"},
		Description:   "open draft",
		ExplorationID: "explore-1",
	})
	require.NoError(t, err)
	require.NotNil(t, open)
	require.Equal(t, archaeodomain.DeferredDraftPending, open.Status)
	require.Equal(t, "deferred-custom-1", open.ID)

	second, err := svc.CreateOrUpdate(ctx, CreateInput{
		WorkspaceID:  "/workspace/deferred-more",
		WorkflowID:   "wf-deferred-more",
		PlanID:       "plan-2",
		RequestID:    "request-2",
		PlanVersion:  intPtr(2),
		AmbiguityKey: "step-2:api-choice",
		Title:        "Need API choice",
	})
	require.NoError(t, err)
	require.NotEqual(t, open.ID, second.ID)

	updated, err := svc.CreateOrUpdate(ctx, CreateInput{
		WorkspaceID:        "/workspace/deferred-more",
		WorkflowID:         "wf-deferred-more",
		AmbiguityKey:       "step-1:type-choice",
		LinkedDraftVersion: intPtr(3),
		LinkedDraftPlanID:  "plan-1-v2",
		CommentRefs:        []string{"comment-2", "comment-1"},
		Metadata:           map[string]any{"status": "updated"},
	})
	require.NoError(t, err)
	require.Equal(t, open.ID, updated.ID)
	require.Equal(t, archaeodomain.DeferredDraftFormed, updated.Status)

	finalized, err := svc.Finalize(ctx, FinalizeInput{
		WorkflowID:  "wf-deferred-more",
		RecordID:    updated.ID,
		CommentRefs: []string{"done"},
		Metadata:    map[string]any{"final": "yes"},
	})
	require.NoError(t, err)
	require.NotNil(t, finalized.FinalizedAt)
	require.Equal(t, archaeodomain.DeferredDraftFinalized, finalized.Status)

	loaded, err := svc.Load(ctx, "wf-deferred-more", updated.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, updated.ID, loaded.ID)
	require.Nil(t, mustDeferredLoad(t, svc, "wf-other", updated.ID))

	records, err := svc.ListByWorkspace(ctx, "/workspace/deferred-more")
	require.NoError(t, err)
	require.Len(t, records, 2)

	ids, err := svc.IDsByWorkspace(ctx, "/workspace/deferred-more")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{open.ID, second.ID}, ids)

	summary, err := svc.SummaryByWorkspace(ctx, "/workspace/deferred-more")
	require.NoError(t, err)
	require.Equal(t, 1, summary[archaeodomain.DeferredDraftFinalized])
	require.Equal(t, 1, summary[archaeodomain.DeferredDraftPending])

	require.Nil(t, mustDeferredLoad(t, svc, "", updated.ID))
	require.Nil(t, mustDeferredLoad(t, svc, "wf-deferred-more", ""))
	require.Nil(t, mustDeferredList(t, svc, ""))
	require.Nil(t, mustDeferredIDs(t, svc, ""))
	require.Nil(t, mustDeferredSummary(t, svc, ""))

	require.Equal(t, "fallback", firstNonEmpty(" ", "fallback"))
	require.Nil(t, cloneInt(nil))
	clonedVersion := cloneInt(open.PlanVersion)
	require.NotNil(t, clonedVersion)
	require.NotSame(t, open.PlanVersion, clonedVersion)
	require.Equal(t, *open.PlanVersion, *clonedVersion)

	require.Nil(t, cloneMap(nil))
	src := map[string]any{"a": 1}
	copied := cloneMap(src)
	require.Equal(t, src, copied)
	copied["a"] = 2
	require.Equal(t, 1, src["a"])

	require.Nil(t, mergeMap(nil, nil))
	merged := mergeMap(map[string]any{"a": 1}, map[string]any{"b": 2})
	require.Equal(t, map[string]any{"a": 1, "b": 2}, merged)

	require.Equal(t, []string{"a", "b"}, mergeStrings([]string{"a", " ", "a"}, []string{"b", "a"}))
	require.Equal(t, "pending", metadataString(map[string]any{"status": " pending "}, "status"))
	require.Equal(t, "", metadataString(map[string]any{"status": 1}, "status"))

	require.Equal(t, now.UTC(), svc.now())
	require.NotEmpty(t, svc.newID("deferred"))
	require.Contains(t, Service{}.newID("deferred"), "deferred-")
	require.Equal(t, archaeodomain.DeferredDraftFinalized, finalized.Status)
}

func TestServiceMissingLookups(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-deferred-lookup")
	svc := Service{Store: store}

	_, err := svc.Finalize(ctx, FinalizeInput{
		WorkflowID: "wf-deferred-lookup",
		RecordID:   "missing",
	})
	require.NoError(t, err)

	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:    "wrong-kind",
		WorkflowID:    "wf-deferred-lookup",
		Kind:          "not-the-right-kind",
		ContentType:   "application/json",
		StorageKind:   memory.ArtifactStorageInline,
		InlineRawText: "{}",
	}))
	record, err := svc.Load(ctx, "wf-deferred-lookup", "wrong-kind")
	require.NoError(t, err)
	require.Nil(t, record)

	require.Nil(t, mustDeferredLoad(t, svc, "wf-deferred-lookup", ""))
	require.Nil(t, mustDeferredList(t, svc, ""))
	require.Nil(t, mustDeferredIDs(t, svc, ""))
	require.Nil(t, mustDeferredSummary(t, svc, ""))
	require.Equal(t, "", firstNonEmpty(" ", ""))
	require.Equal(t, "", metadataString(nil, "status"))
	require.Equal(t, map[string]any{"a": 1}, mergeMap(map[string]any{"a": 1}, nil))
}

func mustDeferredLoad(t *testing.T, svc Service, workflowID, recordID string) *archaeodomain.DeferredDraftRecord {
	t.Helper()
	record, err := svc.Load(context.Background(), workflowID, recordID)
	require.NoError(t, err)
	return record
}

func mustDeferredList(t *testing.T, svc Service, workspaceID string) []archaeodomain.DeferredDraftRecord {
	t.Helper()
	records, err := svc.ListByWorkspace(context.Background(), workspaceID)
	require.NoError(t, err)
	return records
}

func mustDeferredIDs(t *testing.T, svc Service, workspaceID string) []string {
	t.Helper()
	ids, err := svc.IDsByWorkspace(context.Background(), workspaceID)
	require.NoError(t, err)
	return ids
}

func mustDeferredSummary(t *testing.T, svc Service, workspaceID string) map[archaeodomain.DeferredDraftStatus]int {
	t.Helper()
	summary, err := svc.SummaryByWorkspace(context.Background(), workspaceID)
	require.NoError(t, err)
	return summary
}
