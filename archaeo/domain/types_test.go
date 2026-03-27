package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMutationEventJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Round(time.Second)
	version := 3
	event := MutationEvent{
		ID:            "mutation-1",
		WorkflowID:    "wf-1",
		ExplorationID: "explore-1",
		PlanID:        "plan-1",
		PlanVersion:   &version,
		StepID:        "step-1",
		Category:      MutationBlockingSemantic,
		SourceKind:    "tension",
		SourceRef:     "tension-1",
		Description:   "critical contradiction on active step",
		BlastRadius: BlastRadius{
			Scope:              BlastRadiusStep,
			AffectedStepIDs:    []string{"step-1"},
			AffectedSymbolIDs:  []string{"sym-1"},
			AffectedPatternIDs: []string{"pat-1"},
			AffectedAnchorRefs: []string{"anchor-1"},
			AffectedNodeIDs:    []string{"node-1"},
			EstimatedCount:     1,
		},
		Impact:              ImpactHandoffInvalidating,
		Disposition:         DispositionBlockExecution,
		Blocking:            true,
		BasedOnRevision:     "rev-1",
		SemanticSnapshotRef: "snap-1",
		Metadata:            map[string]any{"reason": "test"},
		CreatedAt:           now,
	}

	raw, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded MutationEvent
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, event.ID, decoded.ID)
	require.Equal(t, event.WorkflowID, decoded.WorkflowID)
	require.Equal(t, event.Category, decoded.Category)
	require.Equal(t, event.BlastRadius.Scope, decoded.BlastRadius.Scope)
	require.Equal(t, event.BlastRadius.AffectedStepIDs, decoded.BlastRadius.AffectedStepIDs)
	require.Equal(t, event.Impact, decoded.Impact)
	require.Equal(t, event.Disposition, decoded.Disposition)
	require.True(t, decoded.Blocking)
	require.Equal(t, event.BasedOnRevision, decoded.BasedOnRevision)
	require.Equal(t, event.SemanticSnapshotRef, decoded.SemanticSnapshotRef)
	require.Equal(t, event.CreatedAt, decoded.CreatedAt)
	require.Equal(t, "test", decoded.Metadata["reason"])
	require.NotNil(t, decoded.PlanVersion)
	require.Equal(t, version, *decoded.PlanVersion)
}

func TestRequestRecordJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Round(time.Second)
	version := 4
	started := now.Add(time.Minute)
	completed := now.Add(2 * time.Minute)
	record := RequestRecord{
		ID:            "req-1",
		WorkflowID:    "wf-1",
		ExplorationID: "explore-1",
		PlanID:        "plan-1",
		PlanVersion:   &version,
		Kind:          RequestPlanReformation,
		Status:        RequestStatusCompleted,
		Title:         "Reform active plan",
		Description:   "Recompute after tension drift.",
		RequestedBy:   "user",
		SubjectRefs:   []string{"tension-1", "mutation-1"},
		Input:         map[string]any{"reason": "drift"},
		DispatchMetadata: map[string]any{
			"provider": "relurpic",
		},
		Result: &RequestResult{
			Kind:    "plan_version",
			RefID:   "plan-1:v4",
			Summary: "Created successor draft",
		},
		RetryCount:      1,
		BasedOnRevision: "rev-2",
		RequestedAt:     now,
		UpdatedAt:       completed,
		StartedAt:       &started,
		CompletedAt:     &completed,
	}

	raw, err := json.Marshal(record)
	require.NoError(t, err)

	var decoded RequestRecord
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, record.ID, decoded.ID)
	require.Equal(t, record.WorkflowID, decoded.WorkflowID)
	require.Equal(t, record.Kind, decoded.Kind)
	require.Equal(t, record.Status, decoded.Status)
	require.Equal(t, record.SubjectRefs, decoded.SubjectRefs)
	require.Equal(t, record.Result.RefID, decoded.Result.RefID)
	require.Equal(t, record.RequestedBy, decoded.RequestedBy)
	require.Equal(t, record.RetryCount, decoded.RetryCount)
	require.Equal(t, record.BasedOnRevision, decoded.BasedOnRevision)
	require.NotNil(t, decoded.PlanVersion)
	require.Equal(t, version, *decoded.PlanVersion)
}
