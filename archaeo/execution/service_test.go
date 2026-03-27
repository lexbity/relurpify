package execution_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/archaeo/execution"
	archaeoretrieval "github.com/lexcodex/relurpify/archaeo/retrieval"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/framework/retrieval"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestGuidanceTimeoutBehaviorUsesDeferralPolicy(t *testing.T) {
	svc := execution.Service{
		DeferralPolicy: guidance.DeferralPolicy{
			MaxBlastRadiusForDefer: 10,
			DeferrableKinds:        []guidance.GuidanceKind{guidance.GuidanceConfidence},
		},
	}
	require.Equal(t, guidance.GuidanceTimeoutDefer, svc.GuidanceTimeoutBehavior(guidance.GuidanceConfidence, 5))
	require.Equal(t, guidance.GuidanceTimeoutFail, svc.GuidanceTimeoutBehavior(guidance.GuidanceScopeExpansion, 5))
	require.Equal(t, guidance.GuidanceTimeoutFail, svc.GuidanceTimeoutBehavior(guidance.GuidanceConfidence, 11))
}

func TestApplyGuidanceDecisionHandlesSkipAndReplan(t *testing.T) {
	now := time.Date(2026, 3, 26, 18, 0, 0, 0, time.UTC)
	svc := execution.Service{Now: func() time.Time { return now }}
	plan := &frameworkplan.LivingPlan{
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
	}
	step := plan.Steps["step-1"]

	result, err, handled := svc.ApplyGuidanceDecision(plan, step, guidance.GuidanceDecision{ChoiceID: "skip", DecidedBy: "user"}, "low confidence")
	require.True(t, handled)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, frameworkplan.PlanStepSkipped, step.Status)

	step = &frameworkplan.PlanStep{ID: "step-2", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now}
	result, err, handled = svc.ApplyGuidanceDecision(plan, step, guidance.GuidanceDecision{ChoiceID: "replan", DecidedBy: "user"}, "blast radius")
	require.True(t, handled)
	require.Error(t, err)
	require.False(t, result.Success)
	require.Equal(t, frameworkplan.PlanStepFailed, step.Status)
}

func TestRequestGuidanceFallsBackWithoutBroker(t *testing.T) {
	svc := execution.Service{}
	decision := svc.RequestGuidance(context.Background(), guidance.GuidanceRequest{
		Kind:  guidance.GuidanceConfidence,
		Title: "test",
	}, "proceed")
	require.Equal(t, "proceed", decision.ChoiceID)
}

func TestGateHelpersResolveEvidenceAndSymbols(t *testing.T) {
	step := &frameworkplan.PlanStep{
		ID:                 "step-1",
		Scope:              []string{"symbol.present"},
		AnchorDependencies: []string{"anchor-1"},
		EvidenceGate: &frameworkplan.EvidenceGate{
			RequiredSymbols: []string{"symbol.missing"},
			RequiredAnchors: []string{"anchor-1"},
		},
	}
	opts := graphdb.DefaultOptions(t.TempDir())
	engine, err := graphdb.Open(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	require.NoError(t, engine.UpsertNode(graphdb.NodeRecord{ID: "symbol.present", Kind: "symbol"}))

	missing := execution.MissingPlanSymbols(step, engine)
	require.Equal(t, []string{"symbol.missing"}, missing)

	available := execution.AvailableSymbolMap(step, engine)
	require.True(t, available["symbol.present"])
	require.False(t, available["symbol.missing"])

	state := core.NewContext()
	state.Set("pipeline.workflow_retrieval", map[string]any{
		"results": []any{
			map[string]any{
				"anchors": []any{
					map[string]any{"anchor_id": "anchor-1"},
				},
			},
		},
	})
	evidence, ok := execution.MixedEvidenceForStep(state, step)
	require.True(t, ok)
	require.Len(t, evidence.Anchors, 1)
	require.Equal(t, "anchor-1", evidence.Anchors[0].AnchorID)
}

func TestCorpusScopeForTaskDefaultsToWorkspace(t *testing.T) {
	require.Equal(t, "workspace", execution.CorpusScopeForTask(&core.Task{}))
	task := &core.Task{Context: map[string]any{"corpus_scope": "repo"}}
	require.Equal(t, "repo", execution.CorpusScopeForTask(task))
}

func TestAssessPlanStepBuildsGateAssessment(t *testing.T) {
	opts := graphdb.DefaultOptions(t.TempDir())
	engine, err := graphdb.Open(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	require.NoError(t, engine.UpsertNode(graphdb.NodeRecord{ID: "symbol.present", Kind: "symbol"}))
	for i := 0; i < 7; i++ {
		nodeID := fmt.Sprintf("symbol.related.%d", i)
		require.NoError(t, engine.UpsertNode(graphdb.NodeRecord{ID: nodeID, Kind: "symbol"}))
		require.NoError(t, engine.Link("symbol.present", nodeID, graphdb.EdgeKind("references"), "", 1, nil))
	}

	db, err := sql.Open("sqlite3", t.TempDir()+"/retrieval.db")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, retrieval.EnsureSchema(context.Background(), db))
	record, err := retrieval.DeclareAnchor(context.Background(), db, retrieval.AnchorDeclaration{
		Term:       "policy",
		Definition: "definition",
		Class:      "policy",
	}, "workspace", "workspace_trusted")
	require.NoError(t, err)

	svc := execution.Service{Retrieval: archaeoretrieval.NewSQLStore(db)}
	step := &frameworkplan.PlanStep{
		ID:                 "step-1",
		Scope:              []string{"symbol.present"},
		AnchorDependencies: []string{record.AnchorID},
		EvidenceGate: &frameworkplan.EvidenceGate{
			RequiredSymbols: []string{"symbol.present"},
			RequiredAnchors: []string{record.AnchorID},
		},
	}
	state := core.NewContext()
	state.Set("pipeline.workflow_retrieval", map[string]any{
		"results": []any{
			map[string]any{
				"anchors": []any{
					map[string]any{"anchor_id": record.AnchorID},
				},
			},
		},
	})
	assessment, err := svc.AssessPlanStep(context.Background(), &core.Task{}, state, step, engine)
	require.NoError(t, err)
	require.Empty(t, assessment.MissingSymbols)
	require.True(t, assessment.ActiveAnchors[record.AnchorID])
	require.Len(t, assessment.DriftedDependencies, 0)
	require.True(t, assessment.HasEvidence)
	require.NotNil(t, assessment.BlastRadius)
	require.Equal(t, 1, assessment.BlastRadius.Expected)
	require.Greater(t, assessment.BlastRadius.Actual, execution.BlastRadiusExpansionThreshold(assessment.BlastRadius.Expected))
}
