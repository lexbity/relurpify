package benchmark

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	archaeoarch "codeburg.org/lexbit/relurpify/archaeo/archaeology"
	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	archaeoverification "codeburg.org/lexbit/relurpify/archaeo/verification"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/named/euclo"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	platformfs "codeburg.org/lexbit/relurpify/platform/fs"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

type benchScale struct {
	name               string
	patternCount       int
	interactionCount   int
	tensionCount       int
	timelineEventCount int
	planStepCount      int
	planVersionCount   int
}

var benchScales = []benchScale{
	{name: "small", patternCount: 8, interactionCount: 5, tensionCount: 4, timelineEventCount: 10, planStepCount: 4, planVersionCount: 1},
	{name: "medium", patternCount: 32, interactionCount: 25, tensionCount: 12, timelineEventCount: 100, planStepCount: 12, planVersionCount: 3},
	{name: "large", patternCount: 96, interactionCount: 100, tensionCount: 30, timelineEventCount: 1000, planStepCount: 32, planVersionCount: 10},
}

type benchmarkFixture struct {
	baseDir       string
	workspace     string
	workflowStore *memorydb.SQLiteWorkflowStateStore
	planStore     *frameworkplan.SQLitePlanStore
	patternStore  *patterns.SQLitePatternStore
	commentStore  *patterns.SQLiteCommentStore
	memoryStore   *memory.HybridMemory
	registry      *capability.Registry
	graph         *graphdb.Engine
	indexManager  *ast.IndexManager
	retrievalDB   *sql.DB
	sourcePath    string
}

func newBenchmarkFixture(b *testing.B, name string) *benchmarkFixture {
	b.Helper()
	baseDir := filepath.Join(b.TempDir(), name)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		b.Fatalf("mkdir %s: %v", baseDir, err)
	}
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(baseDir, "workflow.db"))
	if err != nil {
		b.Fatalf("open workflow store: %v", err)
	}
	b.Cleanup(func() { _ = workflowStore.Close() })
	planDB, err := frameworkplan.OpenSQLite(filepath.Join(baseDir, "plans.db"))
	if err != nil {
		b.Fatalf("open plan db: %v", err)
	}
	b.Cleanup(func() { _ = planDB.Close() })
	planStore, err := frameworkplan.NewSQLitePlanStore(planDB)
	if err != nil {
		b.Fatalf("open plan store: %v", err)
	}
	patternDB, err := sql.Open("sqlite3", filepath.Join(baseDir, "patterns.db"))
	if err != nil {
		b.Fatalf("open patterns db: %v", err)
	}
	b.Cleanup(func() { _ = patternDB.Close() })
	patternStore, err := patterns.NewSQLitePatternStore(patternDB)
	if err != nil {
		b.Fatalf("open pattern store: %v", err)
	}
	commentStore, err := patterns.NewSQLiteCommentStore(patternDB)
	if err != nil {
		b.Fatalf("open comment store: %v", err)
	}
	retrievalDB, err := patterns.OpenSQLite(filepath.Join(baseDir, "retrieval.db"))
	if err != nil {
		b.Fatalf("open retrieval db: %v", err)
	}
	b.Cleanup(func() { _ = retrievalDB.Close() })
	if err := retrieval.EnsureSchema(context.Background(), retrievalDB); err != nil {
		b.Fatalf("ensure retrieval schema: %v", err)
	}
	memoryStore, err := memory.NewHybridMemory(filepath.Join(baseDir, "memory"))
	if err != nil {
		b.Fatalf("open memory store: %v", err)
	}
	graph, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(baseDir, "graphdb")))
	if err != nil {
		b.Fatalf("open graphdb: %v", err)
	}
	b.Cleanup(func() { _ = graph.Close() })
	registry := capability.NewRegistry()
	workspace := initGitRepo(b, filepath.Join(baseDir, "workspace"))
	registerCliGitForRepo(b, registry, workspace)
	if err := registry.Register(&platformfs.ReadFileTool{BasePath: workspace}); err != nil {
		b.Fatalf("register read file tool: %v", err)
	}
	indexStore, err := ast.NewSQLiteStore(filepath.Join(baseDir, "index.db"))
	if err != nil {
		b.Fatalf("open index store: %v", err)
	}
	indexManager := ast.NewIndexManager(indexStore, ast.IndexConfig{WorkspacePath: workspace, ParallelWorkers: 1})
	indexManager.GraphDB = graph
	sourcePath := seedRelurpicSourceFile(b, workspace, indexManager)
	return &benchmarkFixture{
		baseDir:       baseDir,
		workspace:     workspace,
		workflowStore: workflowStore,
		planStore:     planStore,
		patternStore:  patternStore,
		commentStore:  commentStore,
		memoryStore:   memoryStore,
		registry:      registry,
		graph:         graph,
		indexManager:  indexManager,
		retrievalDB:   retrievalDB,
		sourcePath:    sourcePath,
	}
}

func (f *benchmarkFixture) newAgent() *euclo.Agent {
	agent := euclo.New(ayenitd.WorkspaceEnvironment{
		Model:    testutil.StubModel{},
		Registry: f.registry,
		Memory:   f.memoryStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-bench", Model: "stub", MaxIterations: 1},
	})
	agent.WorkflowStore = f.workflowStore
	agent.PlanStore = f.planStore
	agent.PatternStore = f.patternStore
	agent.CommentStore = f.commentStore
	agent.GraphDB = f.graph
	return agent
}

func createWorkflowRecord(workflowID, instruction string) memory.WorkflowRecord {
	now := time.Now().UTC()
	return memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task-" + workflowID,
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: instruction,
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func initGitRepo(tb testing.TB, dir string) string {
	tb.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatalf("mkdir workspace: %v", err)
	}
	run := func(args ...string) {
		tb.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			tb.Fatalf("git %v failed: %v\n%s", args, err, string(output))
		}
	}
	run("init")
	run("config", "user.email", "bench@example.com")
	run("config", "user.name", "Euclo Bench")
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte("bench\n"), 0o644); err != nil {
		tb.Fatalf("write seed file: %v", err)
	}
	run("add", "README.md")
	run("commit", "-m", "init")
	return dir
}

func registerCliGitForRepo(tb testing.TB, registry *capability.Registry, repo string) {
	tb.Helper()
	tool := clinix.NewCommandTool(repo, clinix.CommandToolConfig{
		Name:        "cli_git",
		Description: "Runs git with the provided arguments.",
		Command:     "git",
		Category:    "git",
		Tags:        []string{core.TagExecute},
	})
	tool.SetCommandRunner(fsandbox.NewLocalCommandRunner(repo, nil))
	if err := registry.Register(tool); err != nil {
		tb.Fatalf("register cli_git: %v", err)
	}
}

func seedRelurpicSourceFile(tb testing.TB, workspace string, indexManager *ast.IndexManager) string {
	tb.Helper()
	sourcePath := filepath.Join(workspace, "sample.go")
	content := []byte(`package sample

func Wrap(err error) error {
	if err == nil {
		return nil
	}
	return err
}
`)
	if err := os.WriteFile(sourcePath, content, 0o644); err != nil {
		tb.Fatalf("write relurpic source file: %v", err)
	}
	if indexManager != nil {
		if err := indexManager.IndexFile(sourcePath); err != nil {
			tb.Fatalf("index relurpic source file: %v", err)
		}
	}
	return sourcePath
}

func mustCreateWorkflow(tb testing.TB, store *memorydb.SQLiteWorkflowStateStore, workflowID, instruction string) {
	tb.Helper()
	if err := store.CreateWorkflow(context.Background(), createWorkflowRecord(workflowID, instruction)); err != nil {
		tb.Fatalf("create workflow %s: %v", workflowID, err)
	}
}

func seedPatternRecords(tb testing.TB, store *patterns.SQLitePatternStore, corpusScope, prefix string, count int) []patterns.PatternRecord {
	tb.Helper()
	now := time.Now().UTC()
	out := make([]patterns.PatternRecord, 0, count)
	for i := 0; i < count; i++ {
		record := patterns.PatternRecord{
			ID:           fmt.Sprintf("%s-pattern-%03d", prefix, i),
			Kind:         patterns.PatternKindStructural,
			Title:        fmt.Sprintf("Pattern %03d", i),
			Description:  "benchmark pattern",
			Status:       patterns.PatternStatusProposed,
			CorpusScope:  corpusScope,
			CorpusSource: corpusScope,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := store.Save(context.Background(), record); err != nil {
			tb.Fatalf("save pattern: %v", err)
		}
		out = append(out, record)
	}
	return out
}

func seedActivePlan(tb testing.TB, fixture *benchmarkFixture, workflowID, explorationID string, stepCount int) *archaeodomain.VersionedLivingPlan {
	tb.Helper()
	now := time.Now().UTC()
	plan := &frameworkplan.LivingPlan{
		ID:         fmt.Sprintf("plan-%s", workflowID),
		WorkflowID: workflowID,
		Title:      "benchmark plan",
		Steps:      make(map[string]*frameworkplan.PlanStep, stepCount),
		StepOrder:  make([]string, 0, stepCount),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	for i := 0; i < stepCount; i++ {
		stepID := fmt.Sprintf("step-%03d", i)
		plan.Steps[stepID] = &frameworkplan.PlanStep{
			ID:              stepID,
			Description:     fmt.Sprintf("step %03d", i),
			Status:          frameworkplan.PlanStepPending,
			ConfidenceScore: 0.5,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		plan.StepOrder = append(plan.StepOrder, stepID)
	}
	if err := fixture.planStore.SavePlan(context.Background(), plan); err != nil {
		tb.Fatalf("save plan: %v", err)
	}
	versionSvc := archaeoplans.Service{Store: fixture.planStore, WorkflowStore: fixture.workflowStore}
	version, err := versionSvc.DraftVersion(context.Background(), plan, archaeoplans.DraftVersionInput{
		WorkflowID:             workflowID,
		DerivedFromExploration: explorationID,
		BasedOnRevision:        "rev-bench",
		SemanticSnapshotRef:    "semantic-bench",
	})
	if err != nil {
		tb.Fatalf("draft plan version: %v", err)
	}
	active, err := versionSvc.ActivateVersion(context.Background(), workflowID, version.Version)
	if err != nil {
		tb.Fatalf("activate plan version: %v", err)
	}
	return active
}

func seedPlanVersionHistory(tb testing.TB, fixture *benchmarkFixture, workflowID, explorationID string, stepCount, versionCount int) *archaeodomain.VersionedLivingPlan {
	tb.Helper()
	active := seedActivePlan(tb, fixture, workflowID, explorationID, stepCount)
	if versionCount <= 1 {
		return active
	}
	svc := archaeoplans.Service{Store: fixture.planStore, WorkflowStore: fixture.workflowStore}
	baseVersion := active.Version
	for i := 1; i < versionCount; i++ {
		record, err := svc.EnsureDraftSuccessor(context.Background(), workflowID, baseVersion, fmt.Sprintf("bench successor %d", i))
		if err != nil {
			tb.Fatalf("ensure draft successor: %v", err)
		}
		if _, err := svc.ActivateVersion(context.Background(), workflowID, record.Version); err != nil {
			tb.Fatalf("activate successor version: %v", err)
		}
		baseVersion = record.Version
	}
	current, err := svc.LoadActiveVersion(context.Background(), workflowID)
	if err != nil {
		tb.Fatalf("load active version: %v", err)
	}
	return current
}

func seedExploration(tb testing.TB, fixture *benchmarkFixture, workflowID, workspaceID string) (*archaeodomain.ExplorationSession, *archaeodomain.ExplorationSnapshot) {
	tb.Helper()
	svc := archaeoarch.Service{Store: fixture.workflowStore}
	session, err := svc.EnsureExplorationSession(context.Background(), workflowID, workspaceID, "rev-bench")
	if err != nil {
		tb.Fatalf("ensure exploration session: %v", err)
	}
	snapshot, err := svc.CreateExplorationSnapshot(context.Background(), session, archaeoarch.SnapshotInput{
		WorkflowID:          workflowID,
		WorkspaceID:         workspaceID,
		BasedOnRevision:     "rev-bench",
		SemanticSnapshotRef: "semantic-bench",
		Summary:             "benchmark exploration",
	})
	if err != nil {
		tb.Fatalf("create exploration snapshot: %v", err)
	}
	return session, snapshot
}

func seedLearningInteractions(tb testing.TB, fixture *benchmarkFixture, workflowID, explorationID, prefix string, count int) []archaeolearning.Interaction {
	tb.Helper()
	svc := archaeolearning.Service{
		Store:        fixture.workflowStore,
		PatternStore: fixture.patternStore,
		CommentStore: fixture.commentStore,
		PlanStore:    fixture.planStore,
	}
	out := make([]archaeolearning.Interaction, 0, count)
	for i := 0; i < count; i++ {
		interaction, err := svc.Create(context.Background(), archaeolearning.CreateInput{
			WorkflowID:    workflowID,
			ExplorationID: explorationID,
			Kind:          archaeolearning.InteractionIntentRefinement,
			SubjectType:   archaeolearning.SubjectExploration,
			SubjectID:     fmt.Sprintf("%s-subject-%03d", prefix, i),
			Title:         fmt.Sprintf("Learning %03d", i),
			Description:   "benchmark interaction",
			Blocking:      i%3 == 0,
		})
		if err != nil {
			tb.Fatalf("create learning interaction: %v", err)
		}
		out = append(out, *interaction)
	}
	return out
}

func seedTensions(tb testing.TB, fixture *benchmarkFixture, workflowID, explorationID, snapshotID, prefix string, count int) []archaeodomain.Tension {
	tb.Helper()
	svc := archaeotensions.Service{Store: fixture.workflowStore}
	out := make([]archaeodomain.Tension, 0, count)
	for i := 0; i < count; i++ {
		record, err := svc.CreateOrUpdate(context.Background(), archaeotensions.CreateInput{
			WorkflowID:         workflowID,
			ExplorationID:      explorationID,
			SnapshotID:         snapshotID,
			Kind:               "contradiction",
			Description:        "benchmark tension",
			Severity:           "warning",
			Status:             archaeodomain.TensionUnresolved,
			RelatedPlanStepIDs: []string{fmt.Sprintf("step-%03d", i%4)},
			BasedOnRevision:    "rev-bench",
			PatternIDs:         []string{fmt.Sprintf("%s-pattern-%03d", prefix, i%max(1, count))},
			AnchorRefs:         []string{fmt.Sprintf("%s-anchor-%03d", prefix, i)},
			SymbolScope:        []string{fmt.Sprintf("symbol.%03d", i)},
			SourceRef:          fmt.Sprintf("%s-source-%03d", prefix, i),
		})
		if err != nil {
			tb.Fatalf("create tension: %v", err)
		}
		out = append(out, *record)
	}
	return out
}

func seedTimelineEvents(tb testing.TB, store *memorydb.SQLiteWorkflowStateStore, workflowID string, count int) {
	tb.Helper()
	now := time.Now().UTC()
	eventTypes := []string{
		archaeoevents.EventWorkflowPhaseTransitioned,
		archaeoevents.EventExplorationSessionUpserted,
		archaeoevents.EventExplorationSnapshotUpserted,
		archaeoevents.EventLearningInteractionRequested,
		archaeoevents.EventPlanVersionUpserted,
		archaeoevents.EventExecutionHandoffRecorded,
		archaeoevents.EventTensionUpserted,
	}
	for i := 0; i < count; i++ {
		eventType := eventTypes[i%len(eventTypes)]
		if err := archaeoevents.AppendWorkflowEvent(context.Background(), store, workflowID, eventType, fmt.Sprintf("event-%03d", i), map[string]any{
			"index": i,
			"type":  eventType,
		}, now.Add(time.Duration(i)*time.Millisecond)); err != nil {
			tb.Fatalf("append workflow event: %v", err)
		}
	}
}

func seedAnchorDrifts(tb testing.TB, db *sql.DB, corpusScope, prefix string, count int) {
	tb.Helper()
	ctx := context.Background()
	for i := 0; i < count; i++ {
		record, err := retrieval.DeclareAnchor(ctx, db, retrieval.AnchorDeclaration{
			Term:       fmt.Sprintf("term-%03d", i),
			Definition: fmt.Sprintf("definition-%03d", i),
			Context:    map[string]string{"source": "benchmark"},
		}, corpusScope, "workspace_trusted")
		if err != nil {
			tb.Fatalf("declare anchor: %v", err)
		}
		if err := retrieval.RecordAnchorDrift(ctx, db, record.AnchorID, "warning", fmt.Sprintf("%s benchmark drift", prefix)); err != nil {
			tb.Fatalf("record anchor drift: %v", err)
		}
	}
}

type benchVerifier struct{}

func (benchVerifier) Verify(context.Context, frameworkplan.ConvergenceTarget) (*frameworkplan.ConvergenceFailure, error) {
	return nil, nil
}

func finalizeConvergence(tb testing.TB, fixture *benchmarkFixture, plan *frameworkplan.LivingPlan) {
	tb.Helper()
	svc := archaeoverification.Service{
		Store:    fixture.planStore,
		Workflow: fixture.workflowStore,
		Verifier: benchVerifier{},
	}
	if _, err := svc.FinalizeConvergence(context.Background(), plan, &core.Result{Success: true, Data: map[string]any{}}); err != nil {
		tb.Fatalf("finalize convergence: %v", err)
	}
}

func benchmarkTask(workflowID, instruction string, extra map[string]any) *core.Task {
	ctx := map[string]any{
		"workspace":    "/tmp/ws",
		"workflow_id":  workflowID,
		"corpus_scope": "workspace",
	}
	for k, v := range extra {
		ctx[k] = v
	}
	return &core.Task{
		ID:          "task-" + workflowID,
		Instruction: instruction,
		Context:     ctx,
	}
}

func benchmarkState() *core.Context {
	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	return state
}

func runtimeEnvelope(instruction string) eucloruntime.TaskEnvelope {
	return eucloruntime.TaskEnvelope{
		Instruction:   instruction,
		Workspace:     "/tmp/ws",
		EditPermitted: true,
	}
}

func seedWorkflowKnowledge(tb testing.TB, store *memorydb.SQLiteWorkflowStateStore, workflowID string, count int) {
	tb.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		if err := store.PutKnowledge(ctx, memory.KnowledgeRecord{
			RecordID:   fmt.Sprintf("knowledge-%s-%03d", workflowID, i),
			WorkflowID: workflowID,
			Kind:       memory.KnowledgeKindFact,
			Title:      fmt.Sprintf("Knowledge %03d", i),
			Content:    fmt.Sprintf("Useful workflow context %03d", i),
			Status:     "active",
			CreatedAt:  now.Add(time.Duration(i) * time.Millisecond),
		}); err != nil {
			tb.Fatalf("put knowledge: %v", err)
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
