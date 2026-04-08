package plans

import (
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

var _ = frameworkplan.LivingPlan{}

func TestFormationMatchesVersion(t *testing.T) {
	record := &archaeodomain.VersionedLivingPlan{
		DerivedFromExploration:  "exp1",
		BasedOnRevision:         "rev1",
		SemanticSnapshotRef:     "snap1",
		PatternRefs:             []string{"p1"},
		AnchorRefs:              []string{"a1"},
		TensionRefs:             []string{"t1"},
		FormationProvenanceRefs: []string{"prov1"},
	}
	input := FormationInput{
		ExplorationID:    "exp1",
		BasedOnRevision:  "rev1",
		SemanticSnapshot: "snap1",
		PatternRefs:      []string{"p1"},
		AnchorRefs:       []string{"a1"},
		TensionRefs:      []string{"t1"},
		ProvenanceRefs:   []string{"prov1"},
	}
	require.True(t, formationMatchesVersion(record, input))

	input2 := FormationInput{ExplorationID: "exp2"}
	require.False(t, formationMatchesVersion(record, input2))
}

func TestDraftMatchesFormationInput(t *testing.T) {
	record := &archaeodomain.VersionedLivingPlan{
		Status:                  archaeodomain.LivingPlanVersionDraft,
		DerivedFromExploration:  "exp1",
		BasedOnRevision:         "rev1",
		SemanticSnapshotRef:     "snap1",
		PatternRefs:             []string{"p1"},
		AnchorRefs:              []string{"a1"},
		TensionRefs:             []string{"t1"},
		FormationProvenanceRefs: []string{"prov1"},
	}
	input := FormationInput{
		ExplorationID:    "exp1",
		BasedOnRevision:  "rev1",
		SemanticSnapshot: "snap1",
		PatternRefs:      []string{"p1"},
		AnchorRefs:       []string{"a1"},
		TensionRefs:      []string{"t1"},
		ProvenanceRefs:   []string{"prov1"},
	}
	require.True(t, draftMatchesFormationInput(record, input))

	record.Status = archaeodomain.LivingPlanVersionActive
	require.False(t, draftMatchesFormationInput(record, input))
}

func TestFormPlan(t *testing.T) {
	now := time.Now().UTC()
	svc := Service{Now: func() time.Time { return now }}
	input := FormationInput{
		WorkflowID:      "wf1",
		PatternRefs:     []string{"p1"},
		AnchorRefs:      []string{"a1"},
		TensionRefs:     []string{"t1"},
		PendingLearning: []string{"l1"},
		RequestRefs:     []string{"r1"},
		MutationRefs:    []string{"m1"},
	}
	plan := svc.formPlan(input)
	require.NotNil(t, plan)
	require.Equal(t, "wf1", plan.WorkflowID)
	require.Len(t, plan.StepOrder, 5)
	require.Equal(t, "resolve_learning", plan.StepOrder[0])
	require.Equal(t, "resolve_tensions", plan.StepOrder[1])
	require.Equal(t, "ground_findings", plan.StepOrder[2])
	require.Equal(t, "reconcile_state", plan.StepOrder[3])
	require.Equal(t, "advance_execution", plan.StepOrder[4])

	// check dependencies
	require.Empty(t, plan.Steps["resolve_learning"].DependsOn)
	require.Equal(t, []string{"resolve_learning"}, plan.Steps["resolve_tensions"].DependsOn)
	require.Equal(t, []string{"resolve_tensions"}, plan.Steps["ground_findings"].DependsOn)
	require.Equal(t, []string{"ground_findings"}, plan.Steps["reconcile_state"].DependsOn)
	require.Equal(t, []string{"reconcile_state"}, plan.Steps["advance_execution"].DependsOn)
}
