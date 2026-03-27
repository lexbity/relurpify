package relurpic

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/archaeo/providers"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/framework/retrieval"
	"github.com/stretchr/testify/require"
)

func TestPatternSurfacingProviderSurfacesPatternRecords(t *testing.T) {
	registry := capability.NewRegistry()
	registerRelurpicReadTool(t, registry, ".")
	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `{"proposals":[{"kind":"boundary","title":"Error wrapping at boundaries","description":"Boundary functions wrap returned errors consistently.","instances":[{"file_path":"","start_line":3,"end_line":7,"excerpt":"func Wrap(err error) error {\n\tif err == nil {\n\t\treturn nil\n\t}\n\treturn err\n}"}],"confidence":0.87}]}`},
		},
	}
	indexManager, graphEngine, patternStore, retrievalDB, sourcePath := newPatternDetectorFixtures(t)

	provider := PatternSurfacingProvider{
		Model:        model,
		Config:       &core.Config{Name: "coding", Model: "stub"},
		Registry:     registry,
		IndexManager: indexManager,
		GraphDB:      graphEngine,
		PatternStore: patternStore,
		RetrievalDB:  retrievalDB,
	}

	records, err := provider.SurfacePatterns(context.Background(), providers.PatternSurfacingRequest{
		WorkflowID:    "wf-1",
		ExplorationID: "explore-1",
		SymbolScope:   sourcePath,
		CorpusScope:   "workspace",
		MaxProposals:  3,
	})
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, patterns.PatternKindBoundary, records[0].Kind)
	require.Equal(t, patterns.PatternStatusProposed, records[0].Status)
}

func TestProspectiveAnalysisProviderReturnsMatchedPatterns(t *testing.T) {
	_, _, patternStore, retrievalDB, _ := newPatternDetectorFixtures(t)
	record := patterns.PatternRecord{
		ID:          "pattern-1",
		CorpusScope: "workspace",
		Kind:        patterns.PatternKindStructural,
		Title:       "Helper wrapper",
		Description: "A helper wraps one operation.",
		Status:      patterns.PatternStatusConfirmed,
	}
	require.NoError(t, patternStore.Save(context.Background(), record))
	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `[{"pattern_id":"pattern-1","relevance":0.92}]`},
		},
	}
	provider := ProspectiveAnalysisProvider{
		Model:        model,
		Config:       &core.Config{Name: "coding", Model: "stub"},
		PatternStore: patternStore,
		RetrievalDB:  retrievalDB,
	}

	records, err := provider.AnalyzeProspective(context.Background(), providers.ProspectiveAnalysisRequest{
		CorpusScope: "workspace",
		Description: "A helper wraps one operation.",
		Limit:       3,
		MinScore:    0.5,
	})
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, record.ID, records[0].ID)
}

func TestTensionAnalysisProviderReturnsTensionsFromGapDetection(t *testing.T) {
	registry := capability.NewRegistry()
	registerRelurpicReadTool(t, registry, ".")
	model := &relurpicCapabilityQueueModel{
		responses: []*core.LLMResponse{
			{Text: `{"results":[{"severity":"significant","description":"Wrap returns the raw error instead of wrapping it at the boundary.","evidence_lines":[5]}]}`},
		},
	}
	indexManager, graphEngine, _, retrievalDB, sourcePath := newPatternDetectorFixtures(t)
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, workflowStore.Close())
	})
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    "analysis",
		Instruction: "analyze workspace",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	anchor, err := retrieval.DeclareAnchor(context.Background(), retrievalDB, retrieval.AnchorDeclaration{
		Term:       "error wrapping",
		Definition: "Boundary functions wrap returned errors.",
		Class:      "commitment",
	}, "workspace", string(patterns.TrustClassBuiltinTrusted))
	require.NoError(t, err)

	provider := TensionAnalysisProvider{
		Model:         model,
		Config:        &core.Config{Name: "coding", Model: "stub"},
		Registry:      registry,
		IndexManager:  indexManager,
		GraphDB:       graphEngine,
		RetrievalDB:   retrievalDB,
		PlanStore:     &stubRelurpicPlanStore{plan: &frameworkplan.LivingPlan{WorkflowID: "wf-1"}},
		Guidance:      &guidance.GuidanceBroker{},
		WorkflowStore: workflowStore,
	}

	records, err := provider.AnalyzeTensions(context.Background(), providers.TensionAnalysisRequest{
		WorkflowID:  "wf-1",
		FilePath:    sourcePath,
		WorkspaceID: "workspace",
		AnchorIDs:   []string{anchor.AnchorID},
	})
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "intent_gap", records[0].Kind)
	require.Equal(t, []string{anchor.AnchorID}, records[0].AnchorRefs)
}

func TestConvergenceReviewProviderUsesPatternAndTensionState(t *testing.T) {
	_, _, patternStore, _, _ := newPatternDetectorFixtures(t)
	require.NoError(t, patternStore.Save(context.Background(), patterns.PatternRecord{
		ID:          "pattern-1",
		CorpusScope: "workspace",
		Kind:        patterns.PatternKindBoundary,
		Title:       "Boundary wrapping",
		Status:      patterns.PatternStatusConfirmed,
	}))
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, workflowStore.Close())
	})
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    "analysis",
		Instruction: "analyze workspace",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	tensionRecord, err := (archaeotensions.Service{
		Store: workflowStore,
		NewID: func(string) string { return "tension-1" },
	}).CreateOrUpdate(context.Background(), archaeotensions.CreateInput{
		WorkflowID:    "wf-1",
		ExplorationID: "explore-1",
		SourceRef:     "gap-1",
		Kind:          "intent_gap",
		Description:   "Intent gap remains unresolved.",
	})
	require.NoError(t, err)

	provider := ConvergenceReviewProvider{
		PatternStore: patternStore,
		TensionStore: workflowStore,
	}

	failure, err := provider.ReviewConvergence(context.Background(), providers.ConvergenceReviewRequest{
		WorkflowID: "wf-1",
		Plan: &frameworkplan.LivingPlan{
			ConvergenceTarget: &frameworkplan.ConvergenceTarget{
				PatternIDs: []string{"pattern-1"},
				TensionIDs: []string{tensionRecord.ID},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, failure)
	require.Equal(t, []string{tensionRecord.ID}, failure.UnresolvedTensions)
}
