package decisions

import (
	"context"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/stretchr/testify/require"
)

func TestServiceCollectionsAndHelpers(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-decision-more")
	now := time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC)
	seq := 0
	svc := Service{
		Store: store,
		Now:   func() time.Time { return now },
		NewID: func(prefix string) string {
			seq++
			return prefix + "-custom-" + string(rune('a'+seq-1))
		},
	}

	rec, err := (Service{}).Create(ctx, CreateInput{})
	require.NoError(t, err)
	require.Nil(t, rec)
	rec, err = svc.Create(ctx, CreateInput{WorkspaceID: "/workspace/decision-more", WorkflowID: "wf-decision-more"})
	require.NoError(t, err)
	require.Nil(t, rec)

	open, err := svc.Create(ctx, CreateInput{
		WorkspaceID:      "/workspace/decision-more",
		WorkflowID:       "wf-decision-more",
		Kind:             archaeodomain.DecisionKindStaleResult,
		RelatedRequestID: "request-1",
		RelatedPlanID:    "plan-1",
		RelatedPlanVersion: intPtr(1),
		Title:            "Open decision",
		Summary:          "open",
		CommentRefs:      []string{" comment-1 ", "comment-1", "comment-2"},
		Metadata:         map[string]any{"note": "keep"},
	})
	require.NoError(t, err)
	require.NotNil(t, open)
	require.Equal(t, archaeodomain.DecisionStatusOpen, open.Status)
	require.Equal(t, "decision-custom-a", open.ID)
	require.NotNil(t, open.RelatedPlanVersion)

	resolved, err := svc.Create(ctx, CreateInput{
		WorkspaceID:  "/workspace/decision-more",
		WorkflowID:   "wf-decision-more",
		Kind:         archaeodomain.DecisionKindDeferredDraft,
		Title:        "Resolved decision",
		Summary:      "resolved",
		CommentRefs:  []string{"x"},
		Metadata:     map[string]any{"status": "ignored"},
	})
	require.NoError(t, err)
	require.NotNil(t, resolved)

	resolved, err = svc.Resolve(ctx, ResolveInput{
		WorkflowID:  "wf-decision-more",
		RecordID:    resolved.ID,
		Status:      archaeodomain.DecisionStatusResolved,
		CommentRefs: []string{"comment-3", "comment-1"},
		Metadata:    map[string]any{"resolved": "yes"},
	})
	require.NoError(t, err)
	require.NotNil(t, resolved.ResolvedAt)
	require.Equal(t, archaeodomain.DecisionStatusResolved, resolved.Status)
	require.Equal(t, []string{"x", "comment-3", "comment-1"}, resolved.CommentRefs)

	loaded, err := svc.Load(ctx, "wf-decision-more", resolved.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, resolved.ID, loaded.ID)
	require.Nil(t, mustDecisionLoad(t, svc, "wf-other", resolved.ID))

	records, err := svc.ListByWorkspace(ctx, "/workspace/decision-more")
	require.NoError(t, err)
	require.Len(t, records, 2)

	ids, err := svc.IDsByWorkspace(ctx, "/workspace/decision-more")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{open.ID, resolved.ID}, ids)

	summary, err := svc.SummaryByWorkspace(ctx, "/workspace/decision-more")
	require.NoError(t, err)
	require.Equal(t, 1, summary[archaeodomain.DecisionStatusOpen])
	require.Equal(t, 1, summary[archaeodomain.DecisionStatusResolved])

	require.Nil(t, mustDecisionLoad(t, svc, "", resolved.ID))
	require.Nil(t, mustDecisionLoad(t, svc, "wf-decision-more", ""))
	require.Nil(t, mustDecisionList(t, svc, ""))
	require.Nil(t, mustDecisionIDs(t, svc, ""))
	require.Nil(t, mustDecisionSummary(t, svc, ""))

	require.Nil(t, cloneInt(nil))
	clonedPlanVersion := cloneInt(open.RelatedPlanVersion)
	require.NotNil(t, clonedPlanVersion)
	require.NotSame(t, open.RelatedPlanVersion, clonedPlanVersion)
	require.Equal(t, *open.RelatedPlanVersion, *clonedPlanVersion)

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
	require.Equal(t, "fallback", firstNonEmpty(" ", "fallback"))
	require.Equal(t, "note", metadataString(map[string]any{"status": " note "}, "status"))
	require.Equal(t, "", metadataString(map[string]any{"status": 1}, "status"))

	require.Equal(t, now.UTC(), svc.now())
	require.NotEmpty(t, svc.newID("decision"))
	require.Contains(t, Service{}.newID("decision"), "decision-")
}

func TestServiceMissingLookups(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-decision-lookup")
	svc := Service{Store: store}

	_, err := svc.Resolve(ctx, ResolveInput{
		WorkflowID: "wf-decision-lookup",
		RecordID:   "missing",
		Status:     archaeodomain.DecisionStatusResolved,
	})
	require.NoError(t, err)

	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:    "wrong-kind",
		WorkflowID:    "wf-decision-lookup",
		Kind:          "not-the-right-kind",
		ContentType:   "application/json",
		StorageKind:   memory.ArtifactStorageInline,
		InlineRawText: "{}",
	}))
	record, err := svc.Load(ctx, "wf-decision-lookup", "wrong-kind")
	require.NoError(t, err)
	require.Nil(t, record)

	require.Nil(t, mustDecisionLoad(t, svc, "wf-decision-lookup", ""))
	require.Nil(t, mustDecisionList(t, svc, ""))
	require.Nil(t, mustDecisionIDs(t, svc, ""))
	require.Nil(t, mustDecisionSummary(t, svc, ""))
	require.Equal(t, "", firstNonEmpty(" ", ""))
	require.Equal(t, "", metadataString(nil, "status"))
	require.Equal(t, map[string]any{"a": 1}, mergeMap(map[string]any{"a": 1}, nil))
}

func mustDecisionLoad(t *testing.T, svc Service, workflowID, recordID string) *archaeodomain.DecisionRecord {
	t.Helper()
	record, err := svc.Load(context.Background(), workflowID, recordID)
	require.NoError(t, err)
	return record
}

func mustDecisionList(t *testing.T, svc Service, workspaceID string) []archaeodomain.DecisionRecord {
	t.Helper()
	records, err := svc.ListByWorkspace(context.Background(), workspaceID)
	require.NoError(t, err)
	return records
}

func mustDecisionIDs(t *testing.T, svc Service, workspaceID string) []string {
	t.Helper()
	ids, err := svc.IDsByWorkspace(context.Background(), workspaceID)
	require.NoError(t, err)
	return ids
}

func mustDecisionSummary(t *testing.T, svc Service, workspaceID string) map[archaeodomain.DecisionStatus]int {
	t.Helper()
	summary, err := svc.SummaryByWorkspace(context.Background(), workspaceID)
	require.NoError(t, err)
	return summary
}

func intPtr(v int) *int { return &v }
