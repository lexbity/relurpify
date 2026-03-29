package relurpicadapters

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/agents/relurpic"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/archaeo/providers"
	archaeoretrieval "github.com/lexcodex/relurpify/archaeo/retrieval"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/framework/retrieval"
	platformfs "github.com/lexcodex/relurpify/platform/fs"
	"github.com/stretchr/testify/require"
)

type queueModel struct {
	responses []*core.LLMResponse
	index     int
}

func (m *queueModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	if m.index >= len(m.responses) {
		return nil, context.DeadlineExceeded
	}
	response := m.responses[m.index]
	m.index++
	return response, nil
}

func (m *queueModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *queueModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{}, nil
}

func (m *queueModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return m.Generate(context.Background(), "", nil)
}

func TestRuntimeBundleBuildsRelurpicBackedProviders(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(&platformfs.ReadFileTool{BasePath: "."}))
	indexManager, graphEngine, patternStore, retrievalDB, _ := newAdapterFixtures(t)
	runtime := Runtime{
		Model:        &queueModel{},
		Config:       &core.Config{Name: "coding", Model: "stub"},
		Registry:     registry,
		IndexManager: indexManager,
		GraphDB:      graphEngine,
		PatternStore: patternStore,
		Retrieval:    archaeoretrieval.NewSQLStore(retrievalDB),
	}

	bundle := runtime.Bundle()
	require.NotNil(t, bundle.PatternSurfacer)
	require.NotNil(t, bundle.TensionAnalyzer)
	require.NotNil(t, bundle.ProspectiveAnalyzer)
	require.NotNil(t, bundle.ConvergenceReviewer)
	patternProvider, ok := bundle.PatternSurfacer.(relurpic.PatternSurfacingProvider)
	require.True(t, ok)
	require.NotNil(t, patternProvider.Service)
	tensionProvider, ok := bundle.TensionAnalyzer.(relurpic.TensionAnalysisProvider)
	require.True(t, ok)
	require.NotNil(t, tensionProvider.Service)
}

func TestServiceSurfacePatternsAndSyncLearningCreatesInteractions(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(&platformfs.ReadFileTool{BasePath: "."}))
	indexManager, graphEngine, patternStore, retrievalDB, sourcePath := newAdapterFixtures(t)
	model := &queueModel{
		responses: []*core.LLMResponse{
			{Text: `{"proposals":[{"kind":"boundary","title":"Error wrapping at boundaries","description":"Boundary functions wrap returned errors consistently.","instances":[{"file_path":"","start_line":3,"end_line":7,"excerpt":"func Wrap(err error) error {\n\tif err == nil { return nil }\n\treturn err\n}"}],"confidence":0.87}]}`},
		},
	}
	workflowStore := openWorkflowStore(t)
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    "analysis",
		Instruction: "analyze workspace",
		Status:      memory.WorkflowRunStatusRunning,
	}))

	service := Service{
		Providers: Runtime{
			Model:        model,
			Config:       &core.Config{Name: "coding", Model: "stub"},
			Registry:     registry,
			IndexManager: indexManager,
			GraphDB:      graphEngine,
			PatternStore: patternStore,
			Retrieval:    archaeoretrieval.NewSQLStore(retrievalDB),
		}.Bundle(),
		Learning: archaeolearning.Service{
			Store:        workflowStore,
			PatternStore: patternStore,
		},
	}

	records, interactions, err := service.SurfacePatternsAndSyncLearning(context.Background(), providers.PatternSurfacingRequest{
		WorkflowID:    "wf-1",
		ExplorationID: "explore-1",
		SymbolScope:   sourcePath,
		CorpusScope:   "workspace",
		MaxProposals:  3,
	})
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Len(t, interactions, 1)
	require.Equal(t, archaeolearning.SubjectPattern, interactions[0].SubjectType)
	require.Equal(t, records[0].ID, interactions[0].SubjectID)
}

func TestServiceAnalyzeAndPersistTensionsCreatesArtifactsAndLearning(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(&platformfs.ReadFileTool{BasePath: "."}))
	indexManager, graphEngine, _, retrievalDB, sourcePath := newAdapterFixtures(t)
	model := &queueModel{
		responses: []*core.LLMResponse{
			{Text: `{"results":[{"severity":"significant","description":"Wrap returns the raw error instead of wrapping it at the boundary.","evidence_lines":[5]}]}`},
		},
	}
	workflowStore := openWorkflowStore(t)
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

	service := Service{
		Providers: Runtime{
			Model:         model,
			Config:        &core.Config{Name: "coding", Model: "stub"},
			Registry:      registry,
			IndexManager:  indexManager,
			GraphDB:       graphEngine,
			Retrieval:     archaeoretrieval.NewSQLStore(retrievalDB),
			PlanStore:     &stubPlanStore{plan: nil},
			WorkflowStore: workflowStore,
		}.Bundle(),
		Learning: archaeolearning.Service{
			Store: workflowStore,
		},
		Tensions: archaeotensions.Service{
			Store: workflowStore,
		},
	}

	tensionRecords, interactions, err := service.AnalyzeAndPersistTensions(context.Background(), providers.TensionAnalysisRequest{
		WorkflowID:      "wf-1",
		ExplorationID:   "explore-1",
		SnapshotID:      "snap-1",
		WorkspaceID:     "workspace",
		FilePath:        sourcePath,
		AnchorIDs:       []string{anchor.AnchorID},
		BasedOnRevision: "rev-1",
	})
	require.NoError(t, err)
	require.Len(t, tensionRecords, 1)
	require.Len(t, interactions, 1)
	require.Equal(t, archaeolearning.SubjectTension, interactions[0].SubjectType)

	stored, err := service.Tensions.ListByWorkflow(context.Background(), "wf-1")
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, tensionRecords[0].ID, stored[0].ID)
}

func newAdapterFixtures(t *testing.T) (*ast.IndexManager, *graphdb.Engine, *patterns.SQLitePatternStore, *sql.DB, string) {
	t.Helper()

	tmpDir := t.TempDir()
	store, err := ast.NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: tmpDir, ParallelWorkers: 1})

	graphEngine, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(tmpDir, "graphdb")))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, graphEngine.Close())
	})
	indexManager.GraphDB = graphEngine

	sourcePath := filepath.Join(tmpDir, "sample.go")
	require.NoError(t, os.WriteFile(sourcePath, []byte(`package sample

func Wrap(err error) error {
	if err == nil {
		return nil
	}
	return err
}
`), 0o644))
	require.NoError(t, indexManager.IndexFile(sourcePath))

	patternDB, err := patterns.OpenSQLite(filepath.Join(tmpDir, "patterns.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, patternDB.Close())
	})
	patternStore, err := patterns.NewSQLitePatternStore(patternDB)
	require.NoError(t, err)

	retrievalDB, err := patterns.OpenSQLite(filepath.Join(tmpDir, "retrieval.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, retrievalDB.Close())
	})
	require.NoError(t, retrieval.EnsureSchema(context.Background(), retrievalDB))

	return indexManager, graphEngine, patternStore, retrievalDB, sourcePath
}

func openWorkflowStore(t *testing.T) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})
	return store
}

type stubPlanStore struct {
	plan *frameworkplan.LivingPlan
}

func (s *stubPlanStore) SavePlan(context.Context, *frameworkplan.LivingPlan) error { return nil }
func (s *stubPlanStore) LoadPlan(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return s.plan, nil
}
func (s *stubPlanStore) LoadPlanByWorkflow(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return s.plan, nil
}
func (s *stubPlanStore) UpdateStep(context.Context, string, string, *frameworkplan.PlanStep) error {
	return nil
}
func (s *stubPlanStore) InvalidateStep(context.Context, string, string, frameworkplan.InvalidationRule) error {
	return nil
}
func (s *stubPlanStore) DeletePlan(context.Context, string) error { return nil }
func (s *stubPlanStore) ListPlans(context.Context) ([]frameworkplan.PlanSummary, error) {
	return nil, nil
}
