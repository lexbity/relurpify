package plan

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

func openTestStore(t *testing.T) *SQLitePlanStore {
	t.Helper()
	db, err := OpenSQLite(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	store, err := NewSQLitePlanStore(db)
	require.NoError(t, err)
	return store
}

func TestFoldInManifestSerializes(t *testing.T) {
	manifest := FoldInManifest{
		ManifestID:         "manifest-1",
		PlanID:             "plan-1",
		FeatureDescription: "add guardrails",
		StructuralReadiness: []ReadinessGap{{
			Description:  "extract auth helper",
			Scope:        []string{"auth.Check"},
			LinkedStepID: "step-1",
		}},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	var decoded FoldInManifest
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, manifest, decoded)
}

func TestSQLitePlanStoreRoundTripAndStepUpdates(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Nanosecond)
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "workflow-1",
		Title:      "living plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:              "step-1",
				Description:     "first",
				Status:          frameworkplan.PlanStepPending,
				ConfidenceScore: 0.9,
				CreatedAt:       now,
				UpdatedAt:       now,
			},
			"step-2": {
				ID:              "step-2",
				Description:     "second",
				Status:          frameworkplan.PlanStepPending,
				ConfidenceScore: 0.8,
				DependsOn:       []string{"step-1"},
				CreatedAt:       now,
				UpdatedAt:       now,
			},
		},
		StepOrder: []string{"step-1", "step-2"},
		ConvergenceTarget: &frameworkplan.ConvergenceTarget{
			PatternIDs: []string{"pattern-1"},
			TensionIDs: []string{"tension-1"},
			Commentary: "done means coherent",
		},
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, store.SavePlan(ctx, plan))

	loaded, err := store.LoadPlanByWorkflow(ctx, "workflow-1")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.NotNil(t, loaded.ConvergenceTarget)
	require.Equal(t, []string{"pattern-1"}, loaded.ConvergenceTarget.PatternIDs)

	step := loaded.Steps["step-1"]
	step.Status = frameworkplan.PlanStepCompleted
	step.ConfidenceScore = 0.95
	step.UpdatedAt = now.Add(time.Minute)
	require.NoError(t, store.UpdateStep(ctx, loaded.ID, step.ID, step))

	reloaded, err := store.LoadPlan(ctx, loaded.ID)
	require.NoError(t, err)
	require.Equal(t, frameworkplan.PlanStepCompleted, reloaded.Steps["step-1"].Status)
	require.InDelta(t, 0.95, reloaded.Steps["step-1"].ConfidenceScore, 0.0001)

	require.NoError(t, store.InvalidateStep(ctx, loaded.ID, "step-2", frameworkplan.InvalidationRule{
		Kind:   frameworkplan.InvalidationAnchorDrifted,
		Target: "anchor:policy",
	}))
	reloaded, err = store.LoadPlan(ctx, loaded.ID)
	require.NoError(t, err)
	require.Equal(t, frameworkplan.PlanStepInvalidated, reloaded.Steps["step-2"].Status)

	summaries, err := store.ListPlans(ctx)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.Equal(t, "plan-1", summaries[0].ID)
	require.Equal(t, 2, summaries[0].StepCount)
}
