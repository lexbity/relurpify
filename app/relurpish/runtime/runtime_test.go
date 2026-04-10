package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	relurpic "github.com/lexcodex/relurpify/agents/relurpic"
	nexusdb "github.com/lexcodex/relurpify/app/nexus/db"
	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeophases "github.com/lexcodex/relurpify/archaeo/phases"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/config"
	contract "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	frameworksearch "github.com/lexcodex/relurpify/framework/search"
	"github.com/lexcodex/relurpify/named/euclo"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestProbeEnvironmentHandlesMissingRunsc surfaces a helpful error message.
func TestProbeEnvironmentHandlesMissingRunsc(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Workspace = dir
	cfg.ManifestPath = filepath.Join(dir, "agent.manifest.yaml")
	cfg.ConfigPath = config.New(dir).ConfigFile()
	cfg.Sandbox.RunscPath = "runsc-missing"
	report := ProbeEnvironment(context.Background(), cfg, nil)
	require.Contains(t, strings.Join(report.Sandbox.Errors, " "), "runsc not found")
}

func TestSummarizeManifestMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.manifest.yaml")
	summary := summarizeManifest(path)
	require.Equal(t, path, summary.Path)
	require.False(t, summary.Exists)
	require.Empty(t, summary.Error)
}

func TestInitializeWorkspaceFromTemplatesCreatesWorkspaceFiles(t *testing.T) {
	shared := t.TempDir()
	t.Setenv("RELURPIFY_SHARED_DIR", shared)
	configTemplate := filepath.Join(shared, "templates", "workspace", "config.yaml")
	manifestTemplate := filepath.Join(shared, "templates", "workspace", "agent.manifest.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configTemplate), 0o755))
	require.NoError(t, os.WriteFile(configTemplate, []byte("model: test-model\n"), 0o644))
	require.NoError(t, os.WriteFile(manifestTemplate, []byte("path: ${workspace}\n"), 0o644))

	dir := t.TempDir()
	cfg := Config{Workspace: dir}
	require.NoError(t, cfg.Normalize())

	require.NoError(t, InitializeWorkspaceFromTemplates(cfg, false))
	data, err := os.ReadFile(cfg.ConfigPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "test-model")
	manifestData, err := os.ReadFile(cfg.ManifestPath)
	require.NoError(t, err)
	require.Contains(t, string(manifestData), filepath.ToSlash(dir))
	for _, path := range []string{
		config.New(dir).AgentsDir(),
		config.New(dir).SkillsDir(),
		config.New(dir).LogsDir(),
		config.New(dir).TelemetryDir(),
		config.New(dir).MemoryDir(),
		config.New(dir).SessionsDir(),
		config.New(dir).TestRunsDir(),
	} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.True(t, info.IsDir())
	}
}

func TestInitializeWorkspaceFromTemplatesDoesNotOverwriteWithoutFix(t *testing.T) {
	shared := t.TempDir()
	t.Setenv("RELURPIFY_SHARED_DIR", shared)
	configTemplate := filepath.Join(shared, "templates", "workspace", "config.yaml")
	manifestTemplate := filepath.Join(shared, "templates", "workspace", "agent.manifest.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configTemplate), 0o755))
	require.NoError(t, os.WriteFile(configTemplate, []byte("model: replacement\n"), 0o644))
	require.NoError(t, os.WriteFile(manifestTemplate, []byte("name: replacement\n"), 0o644))

	dir := t.TempDir()
	cfg := Config{Workspace: dir}
	require.NoError(t, cfg.Normalize())
	require.NoError(t, os.MkdirAll(filepath.Dir(cfg.ConfigPath), 0o755))
	require.NoError(t, os.WriteFile(cfg.ConfigPath, []byte("model: keep\n"), 0o644))
	require.NoError(t, os.WriteFile(cfg.ManifestPath, []byte("name: keep\n"), 0o644))

	require.NoError(t, InitializeWorkspaceFromTemplates(cfg, false))
	configData, err := os.ReadFile(cfg.ConfigPath)
	require.NoError(t, err)
	require.Contains(t, string(configData), "keep")
	manifestData, err := os.ReadFile(cfg.ManifestPath)
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "keep")
}

func TestDetectChromiumStatusMissingIsWarningOnly(t *testing.T) {
	orig := execLookPath
	execLookPath = func(file string) (string, error) {
		return "", os.ErrNotExist
	}
	defer func() { execLookPath = orig }()

	status := detectChromiumStatus(context.Background())
	require.Equal(t, "chromium", status.Name)
	require.False(t, status.Required)
	require.False(t, status.Available)
	require.False(t, status.Blocking)
	require.Equal(t, "not found", status.Details)
}

func TestBuildCapabilityRegistryDoesNotRegisterGenericExecutionToolsByDefault(t *testing.T) {
	dir := t.TempDir()
	runner := fsandbox.NewLocalCommandRunner(dir, nil)

	capabilities, err := BuildBuiltinCapabilityBundle(dir, runner)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, capabilities.Close()) })
	registry := capabilities.Registry
	indexManager := capabilities.IndexManager
	searchEngine := capabilities.SearchEngine
	require.NotNil(t, registry)
	require.NotNil(t, indexManager)
	require.NotNil(t, indexManager.GraphDB)
	require.NotNil(t, searchEngine)

	_, ok := registry.Get("exec_run_tests")
	require.False(t, ok)
	_, ok = registry.Get("exec_run_linter")
	require.False(t, ok)
	_, ok = registry.Get("exec_run_build")
	require.False(t, ok)
	_, ok = registry.Get("exec_run_code")
	require.False(t, ok)
	_, ok = registry.Get("search_grep")
	require.False(t, ok)

	_, ok = registry.Get("go_test")
	require.True(t, ok)
	_, ok = registry.Get("file_search")
	require.True(t, ok)
}

func TestBuildCapabilityRegistryReturnsUsableSearchEngine(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "match.go"), []byte("package sample\nfunc Hello() string { return \"needle\" }\n"), 0o644))
	runner := fsandbox.NewLocalCommandRunner(dir, nil)

	capabilities, err := BuildBuiltinCapabilityBundle(dir, runner)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, capabilities.Close()) })
	searchEngine := capabilities.SearchEngine
	require.NotNil(t, searchEngine)

	results, err := searchEngine.Search(frameworksearch.SearchQuery{
		Text:       "needle",
		Mode:       frameworksearch.SearchKeyword,
		MaxResults: 5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Contains(t, results[0].File, "match.go")
}

func TestBuildCapabilityRegistryContinuesWhenBootstrapContextCanceled(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "match.go"), []byte("package sample\nfunc Hello() string { return \"needle\" }\n"), 0o644))
	runner := fsandbox.NewLocalCommandRunner(dir, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	capabilities, err := BuildBuiltinCapabilityBundle(dir, runner, CapabilityRegistryOptions{Context: ctx})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, capabilities.Close()) })
	require.NotNil(t, capabilities)
	require.NotNil(t, capabilities.Registry)
	require.NotNil(t, capabilities.IndexManager)
	require.NotNil(t, capabilities.IndexManager.GraphDB)
	require.NotNil(t, capabilities.SearchEngine)
}

func TestBuildCapabilityRegistrySkipASTIndexSkipsSemanticBootstrap(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "match.go"), []byte("package sample\nfunc Hello() string { return \"needle\" }\n"), 0o644))
	runner := fsandbox.NewLocalCommandRunner(dir, nil)

	var embedCalls atomic.Int32
	embedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embedCalls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"embeddings":[[1,2]]}`))
	}))
	defer embedServer.Close()

	capabilities, err := BuildBuiltinCapabilityBundle(dir, runner, CapabilityRegistryOptions{
		InferenceEndpoint: embedServer.URL,
		InferenceModel:    "test-embedder",
		SkipASTIndex:      true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, capabilities.Close()) })
	require.NotNil(t, capabilities)
	require.NotNil(t, capabilities.IndexManager)
	require.NotNil(t, capabilities.IndexManager.GraphDB)
	require.NotNil(t, capabilities.SearchEngine)
	require.Zero(t, embedCalls.Load())

	results, err := capabilities.SearchEngine.Search(frameworksearch.SearchQuery{
		Text:       "needle",
		Mode:       frameworksearch.SearchKeyword,
		MaxResults: 5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Contains(t, results[0].File, "match.go")
}

func TestWireRuntimeAgentDependenciesConfiguresEucloAgent(t *testing.T) {
	dir := t.TempDir()
	graphEngine, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(dir, "graphdb")))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, graphEngine.Close())
	})

	workflowStore, planStore, patternStore, commentStore, patternDB, err := openRuntimeStores(dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, workflowStore.Close())
		require.NoError(t, patternDB.Close())
	})

	rt := &Runtime{
		GraphDB:        graphEngine,
		PlanStore:      planStore,
		PatternStore:   patternStore,
		CommentStore:   commentStore,
		WorkflowStore:  workflowStore,
		GuidanceBroker: guidance.NewGuidanceBroker(time.Second),
		LearningBroker: archaeolearning.NewBroker(time.Second),
	}
	agent := &euclo.Agent{}

	rt.wireRuntimeAgentDependencies(agent)

	require.Same(t, graphEngine, agent.GraphDB)
	require.Same(t, workflowStore.DB(), agent.RetrievalDB)
	require.Same(t, planStore, agent.PlanStore)
	require.Same(t, workflowStore, agent.WorkflowStore)
	require.Same(t, patternStore, agent.PatternStore)
	require.Same(t, commentStore, agent.CommentStore)
	require.Same(t, rt.GuidanceBroker, agent.GuidanceBroker)
	require.Same(t, rt.LearningBroker, agent.LearningBroker)
	require.Equal(t, guidance.DefaultDeferralPolicy(), agent.DeferralPolicy)
	verifier, ok := agent.ConvVerifier.(*relurpic.PatternCoherenceVerifier)
	require.True(t, ok)
	require.NotNil(t, verifier.TensionDetector)
}

func TestOpenRuntimeStoresReturnsSharedPersistenceSurfaces(t *testing.T) {
	dir := t.TempDir()

	workflowStore, planStore, patternStore, commentStore, patternDB, err := openRuntimeStores(dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, workflowStore.Close())
		require.NoError(t, patternDB.Close())
	})

	require.NotNil(t, workflowStore)
	require.NotNil(t, workflowStore.DB())
	require.Implements(t, (*frameworkplan.PlanStore)(nil), planStore)
	require.Implements(t, (*patterns.PatternStore)(nil), patternStore)
	require.Implements(t, (*patterns.CommentStore)(nil), commentStore)
}

func TestRuntimePendingAndResolveLearning(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	workflowStore, _, patternStore, commentStore, patternDB, err := openRuntimeStores(dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, workflowStore.Close())
		require.NoError(t, patternDB.Close())
	})
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-learning",
		TaskID:      "task-learning",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, patternStore.Save(ctx, patterns.PatternRecord{
		ID:           "pattern-1",
		Kind:         patterns.PatternKindStructural,
		Title:        "Adapter",
		Description:  "Use adapters",
		Status:       patterns.PatternStatusProposed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))
	learningBroker := archaeolearning.NewBroker(time.Second)
	learnSvc := archaeolearning.Service{
		Store:        workflowStore,
		PatternStore: patternStore,
		CommentStore: commentStore,
		PlanStore:    nil,
		Broker:       learningBroker,
	}
	interaction, err := learnSvc.Create(ctx, archaeolearning.CreateInput{
		WorkflowID:    "wf-learning",
		ExplorationID: "explore-1",
		Kind:          archaeolearning.InteractionPatternProposal,
		SubjectType:   archaeolearning.SubjectPattern,
		SubjectID:     "pattern-1",
		Title:         "Confirm pattern",
	})
	require.NoError(t, err)

	rt := &Runtime{
		WorkflowStore:  workflowStore,
		PatternStore:   patternStore,
		CommentStore:   commentStore,
		LearningBroker: learningBroker,
	}

	pending := rt.PendingLearning()
	require.Len(t, pending, 1)
	require.Equal(t, interaction.ID, pending[0].ID)

	require.NoError(t, rt.ResolveLearning("wf-learning", archaeolearning.ResolveInput{
		InteractionID: interaction.ID,
		Kind:          archaeolearning.ResolutionConfirm,
		ResolvedBy:    "tui",
	}))

	record, err := patternStore.Load(ctx, "pattern-1")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, patterns.PatternStatusConfirmed, record.Status)
	require.Empty(t, rt.PendingLearning())
}

func TestRuntimeExplorationAndPlanVersionQueries(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	workflowStore, planStore, _, _, patternDB, err := openRuntimeStores(dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, workflowStore.Close())
		require.NoError(t, patternDB.Close())
	})
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-archaeo",
		TaskID:      "task-archaeo",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "explore and plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))

	archSvc := archaeoarch.Service{Store: workflowStore}
	session, err := archSvc.EnsureExplorationSession(ctx, "wf-archaeo", "/workspace/a", "rev-1")
	require.NoError(t, err)
	snapshot, err := archSvc.CreateExplorationSnapshot(ctx, session, archaeoarch.SnapshotInput{
		WorkflowID:           "wf-archaeo",
		WorkspaceID:          "/workspace/a",
		BasedOnRevision:      "rev-1",
		SemanticSnapshotRef:  "snap-1",
		CandidatePatternRefs: []string{"pattern-a"},
		OpenLearningIDs:      []string{"learn-1"},
		Summary:              "initial exploration",
	})
	require.NoError(t, err)
	require.NotNil(t, snapshot)

	planSvc := archaeoplans.Service{Store: planStore, WorkflowStore: workflowStore}
	planA := &frameworkplan.LivingPlan{
		ID:         "plan-a",
		WorkflowID: "wf-archaeo",
		Title:      "Plan A",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	v1, err := planSvc.DraftVersion(ctx, planA, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-archaeo",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    snapshot.ID,
		PatternRefs:            []string{"pattern-a"},
	})
	require.NoError(t, err)
	_, err = planSvc.ActivateVersion(ctx, "wf-archaeo", v1.Version)
	require.NoError(t, err)

	planB := &frameworkplan.LivingPlan{
		ID:         "plan-b",
		WorkflowID: "wf-archaeo",
		Title:      "Plan B",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
			"step-2": {ID: "step-2", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		},
		StepOrder: []string{"step-1", "step-2"},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	v2, err := planSvc.DraftVersion(ctx, planB, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-archaeo",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-2",
		SemanticSnapshotRef:    snapshot.ID,
		PatternRefs:            []string{"pattern-a", "pattern-b"},
		AnchorRefs:             []string{"anchor-b"},
	})
	require.NoError(t, err)
	_, err = planSvc.ActivateVersion(ctx, "wf-archaeo", v2.Version)
	require.NoError(t, err)

	rt := &Runtime{
		WorkflowStore: workflowStore,
		PlanStore:     planStore,
	}

	view, err := rt.ActiveExploration("/workspace/a")
	require.NoError(t, err)
	require.NotNil(t, view)
	require.NotNil(t, view.Session)
	require.Equal(t, session.ID, view.Session.ID)
	require.Len(t, view.Snapshots, 1)

	versions, err := rt.PlanVersions("wf-archaeo")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	require.Equal(t, archaeodomain.LivingPlanVersionActive, versions[1].Status)

	active, err := rt.ActivePlanVersion("wf-archaeo")
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, 2, active.Version)

	diff, err := rt.ComparePlanVersions("wf-archaeo", 1, 2)
	require.NoError(t, err)
	require.NotNil(t, diff)
	require.Equal(t, 1, diff["step_count_delta"])
}

func TestRuntimeTensionQueriesAndUpdates(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	workflowStore, _, _, _, patternDB, err := openRuntimeStores(dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, workflowStore.Close())
		require.NoError(t, patternDB.Close())
	})
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-tension",
		TaskID:      "task-tension",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "inspect tensions",
		Status:      memory.WorkflowRunStatusRunning,
	}))

	tensionSvc := archaeotensions.Service{Store: workflowStore}
	record, err := tensionSvc.CreateOrUpdate(ctx, archaeotensions.CreateInput{
		WorkflowID:    "wf-tension",
		ExplorationID: "explore-1",
		SourceRef:     "gap-1",
		Kind:          "intent_gap",
		Description:   "Behavior contradicts declared intent",
		Severity:      "significant",
		Status:        archaeodomain.TensionUnresolved,
	})
	require.NoError(t, err)

	rt := &Runtime{
		WorkflowStore: workflowStore,
	}

	byWorkflow, err := rt.TensionsByWorkflow("wf-tension")
	require.NoError(t, err)
	require.Len(t, byWorkflow, 1)
	require.Equal(t, record.ID, byWorkflow[0].ID)

	byExploration, err := rt.TensionsByExploration("explore-1")
	require.NoError(t, err)
	require.Len(t, byExploration, 1)
	require.Equal(t, record.ID, byExploration[0].ID)

	updated, err := rt.UpdateTensionStatus("wf-tension", record.ID, archaeodomain.TensionAccepted, []string{"comment-1"})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.TensionAccepted, updated.Status)
	require.Equal(t, []string{"comment-1"}, updated.CommentRefs)

	workflowSummary, err := rt.TensionSummaryByWorkflow("wf-tension")
	require.NoError(t, err)
	require.NotNil(t, workflowSummary)
	require.Equal(t, 1, workflowSummary.Total)
	require.Equal(t, 1, workflowSummary.Accepted)

	explorationSummary, err := rt.TensionSummaryByExploration("explore-1")
	require.NoError(t, err)
	require.NotNil(t, explorationSummary)
	require.Equal(t, 1, explorationSummary.Total)
}

func TestRuntimeWorkflowProjectionQueries(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	workflowStore, planStore, _, _, _, err := openRuntimeStores(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, workflowStore.Close()) })
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-proj-runtime",
		TaskID:      "task-proj-runtime",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "projection runtime test",
		Status:      memory.WorkflowRunStatusRunning,
	}))

	phaseSvc := archaeophases.Service{Store: workflowStore}
	_, err = phaseSvc.Transition(ctx, "wf-proj-runtime", archaeodomain.PhaseArchaeology, archaeodomain.PhaseTransition{To: archaeodomain.PhasePlanFormation})
	require.NoError(t, err)
	_, err = phaseSvc.Transition(ctx, "wf-proj-runtime", archaeodomain.PhaseArchaeology, archaeodomain.PhaseTransition{To: archaeodomain.PhaseExecution})
	require.NoError(t, err)

	archSvc := archaeoarch.Service{Store: workflowStore}
	session, err := archSvc.EnsureExplorationSession(ctx, "wf-proj-runtime", "/workspace/p", "rev-runtime")
	require.NoError(t, err)
	_, err = archSvc.CreateExplorationSnapshot(ctx, session, archaeoarch.SnapshotInput{
		WorkflowID:          "wf-proj-runtime",
		WorkspaceID:         "/workspace/p",
		BasedOnRevision:     "rev-runtime",
		SemanticSnapshotRef: "semantic-runtime",
		Summary:             "runtime exploration",
	})
	require.NoError(t, err)

	planSvc := archaeoplans.Service{Store: planStore, WorkflowStore: workflowStore}
	now := time.Now().UTC()
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-runtime",
		WorkflowID: "wf-proj-runtime",
		Title:      "Runtime Plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Description: "run", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	version, err := planSvc.DraftVersion(ctx, plan, archaeoplans.DraftVersionInput{
		WorkflowID:             "wf-proj-runtime",
		DerivedFromExploration: session.ID,
		BasedOnRevision:        "rev-runtime",
		SemanticSnapshotRef:    "semantic-runtime",
	})
	require.NoError(t, err)
	_, err = planSvc.ActivateVersion(ctx, "wf-proj-runtime", version.Version)
	require.NoError(t, err)

	rt := &Runtime{
		WorkflowStore: workflowStore,
		PlanStore:     planStore,
	}

	model, err := rt.WorkflowProjection("wf-proj-runtime")
	require.NoError(t, err)
	require.NotNil(t, model)
	require.NotNil(t, model.ActivePlanVersion)
	require.Equal(t, version.Version, model.ActivePlanVersion.Version)
	require.NotNil(t, model.ActiveExploration)

	explorationProj, err := rt.ExplorationProjection("wf-proj-runtime")
	require.NoError(t, err)
	require.NotNil(t, explorationProj)
	require.NotNil(t, explorationProj.ActiveExploration)

	activePlanProj, err := rt.ActivePlanProjection("wf-proj-runtime")
	require.NoError(t, err)
	require.NotNil(t, activePlanProj)
	require.NotNil(t, activePlanProj.ActivePlanVersion)

	timeline, err := rt.WorkflowTimeline("wf-proj-runtime")
	require.NoError(t, err)
	require.NotEmpty(t, timeline)

	ch, unsub := rt.SubscribeWorkflowProjection("wf-proj-runtime")
	defer unsub()
	select {
	case event := <-ch:
		require.NotNil(t, event.Workflow)
		require.Equal(t, "wf-proj-runtime", event.Workflow.WorkflowID)
	case <-time.After(time.Second):
		t.Fatal("expected workflow projection subscription event")
	}
}

func TestReloadEffectiveContractRefreshesDefinitionOverlay(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := filepath.Join(workspace, "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	defPath := filepath.Join(agentsDir, "reviewer.yaml")
	writeAgentDefinitionFixture(t, defPath, core.AgentPermissionDeny)

	permMgr, err := authorization.NewPermissionManager(workspace, &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "**"}},
	}, nil, nil)
	require.NoError(t, err)
	rt := &Runtime{
		Config: Config{
			Workspace:         workspace,
			AgentsDir:         agentsDir,
			AgentName:         "reviewer",
			InferenceModel:    "manifest-model",
			InferenceEndpoint: "http://localhost:11434",
		},
		Tools: capability.NewRegistry(),
		Registration: &authorization.AgentRegistration{
			ID: "coding",
			Manifest: &manifest.AgentManifest{
				Metadata: manifest.ManifestMetadata{Name: "coding"},
				Spec: manifest.ManifestSpec{
					Agent: &core.AgentRuntimeSpec{
						Implementation: "react",
						Mode:           core.AgentModePrimary,
						Model:          core.AgentModelConfig{Provider: "ollama", Name: "manifest-model"},
					},
				},
			},
			Permissions: permMgr,
		},
		Context: core.NewContext(),
	}
	require.NoError(t, rt.ReloadEffectiveContract())
	require.Equal(t, core.AgentPermissionDeny, rt.EffectiveContract.AgentSpec.ProviderPolicies["browser"].Activate)

	writeAgentDefinitionFixture(t, defPath, core.AgentPermissionAllow)
	require.NoError(t, rt.ReloadEffectiveContract())
	require.Equal(t, core.AgentPermissionAllow, rt.EffectiveContract.AgentSpec.ProviderPolicies["browser"].Activate)
	require.NotNil(t, rt.CompiledPolicy)
}

func TestReloadEffectiveContractRejectsSkillTopologyChanges(t *testing.T) {
	workspace := t.TempDir()
	skillRoot := filepath.Join(workspace, "relurpify_cfg", "skills", "reviewer")
	for _, dir := range []string{"scripts", "resources", "templates"} {
		require.NoError(t, os.MkdirAll(filepath.Join(skillRoot, dir), 0o755))
	}
	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: "reviewer", Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			PromptSnippets: []string{"Review carefully."},
			AllowedCapabilities: []core.CapabilitySelector{{
				Name: "reviewer.prompt.1",
				Kind: core.CapabilityKindPrompt,
			}},
		},
	}
	data, err := yaml.Marshal(skill)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(skillRoot, "skill.manifest.yaml"), data, 0o644))

	permMgr, err := authorization.NewPermissionManager(workspace, &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "**"}},
	}, nil, nil)
	require.NoError(t, err)
	rt := &Runtime{
		Config: Config{
			Workspace:         workspace,
			AgentName:         "coding",
			InferenceModel:    "manifest-model",
			InferenceEndpoint: "http://localhost:11434",
		},
		Tools: capability.NewRegistry(),
		Registration: &authorization.AgentRegistration{
			ID: "coding",
			Manifest: &manifest.AgentManifest{
				Metadata: manifest.ManifestMetadata{Name: "coding"},
				Spec: manifest.ManifestSpec{
					Agent: &core.AgentRuntimeSpec{
						Implementation: "react",
						Mode:           core.AgentModePrimary,
						Model:          core.AgentModelConfig{Provider: "ollama", Name: "manifest-model"},
					},
				},
			},
			Permissions: permMgr,
		},
		Context: core.NewContext(),
		EffectiveContract: &contract.EffectiveAgentContract{
			AgentID: "coding",
			AgentSpec: &core.AgentRuntimeSpec{
				Implementation: "react",
				Mode:           core.AgentModePrimary,
				Model:          core.AgentModelConfig{Provider: "ollama", Name: "manifest-model"},
			},
		},
	}
	rt.Registration.Manifest.Spec.Skills = []string{"reviewer"}

	err = rt.ReloadEffectiveContract()
	require.Error(t, err)
	require.Contains(t, err.Error(), "skill capability topology")
}

func TestEmitManifestReloadedEventPersistsAuditRecord(t *testing.T) {
	dir := t.TempDir()
	eventLog, err := nexusdb.NewSQLiteEventLog(filepath.Join(dir, "events.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, eventLog.Close()) })

	snapshot := &manifest.AgentManifestSnapshot{
		SourcePath: filepath.Join(dir, "agent.manifest.yaml"),
		Fingerprint: [32]byte{
			0x01, 0x02, 0x03, 0x04,
		},
		Warnings: []string{"spec.agent.native_tool_calling is deprecated; use spec.agent.tool_calling_intent"},
	}

	emitManifestReloadedEvent(context.Background(), eventLog, "agent-1", "relurpish", snapshot)

	events, err := eventLog.ReadByType(context.Background(), "local", core.FrameworkEventManifestReloaded, 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(events[0].Payload, &payload))
	require.Equal(t, snapshot.SourcePath, payload["manifest_path"])
	require.NotEmpty(t, payload["fingerprint"])
	require.NotEmpty(t, payload["warnings"])
}

func writeAgentDefinitionFixture(t *testing.T, path string, level core.AgentPermissionLevel) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(`
kind: AgentDefinition
name: reviewer
spec:
  implementation: react
  mode: primary
  model:
    provider: ollama
    name: manifest-model
  provider_policies:
    browser:
      activate: `+string(level)+`
`), 0o644))
}
