package testscenario

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
	archaeoconvergence "codeburg.org/lexbit/relurpify/archaeo/convergence"
	archaeodecisions "codeburg.org/lexbit/relurpify/archaeo/decisions"
	archaeodeferred "codeburg.org/lexbit/relurpify/archaeo/deferred"
	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoevents "codeburg.org/lexbit/relurpify/archaeo/events"
	archaeoexec "codeburg.org/lexbit/relurpify/archaeo/execution"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeophases "codeburg.org/lexbit/relurpify/archaeo/phases"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	archaeoprojections "codeburg.org/lexbit/relurpify/archaeo/projections"
	archaeoprovenance "codeburg.org/lexbit/relurpify/archaeo/provenance"
	archaeoproviders "codeburg.org/lexbit/relurpify/archaeo/providers"
	archaeorequests "codeburg.org/lexbit/relurpify/archaeo/requests"
	archaeoretrieval "codeburg.org/lexbit/relurpify/archaeo/retrieval"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	frameworkretrieval "codeburg.org/lexbit/relurpify/framework/retrieval"
	_ "github.com/mattn/go-sqlite3"
)

type Option func(*Fixture)

type Fixture struct {
	T testing.TB

	BaseDir   string
	Workspace string

	now   func() time.Time
	newID func(prefix string) string

	WorkflowStore *memorydb.SQLiteWorkflowStateStore
	PlanDB        *sql.DB
	PlanStore     *frameworkplan.SQLitePlanStore
	PatternDB     *sql.DB
	PatternStore  *patterns.SQLitePatternStore
	CommentStore  *patterns.SQLiteCommentStore
	RetrievalDB   *sql.DB
	Retrieval     *archaeoretrieval.SQLStore
	Graph         *graphdb.Engine

	Providers archaeologyProviderStubs
}

type archaeologyProviderStubs struct {
	PatternSurfacer     *PatternSurfacerStub
	TensionAnalyzer     *TensionAnalyzerStub
	ProspectiveAnalyzer *ProspectiveAnalyzerStub
	ConvergenceReviewer *ConvergenceReviewerStub
}

func WithClock(now func() time.Time) Option {
	return func(f *Fixture) {
		if now != nil {
			f.now = now
		}
	}
}

func WithWorkspaceName(name string) Option {
	return func(f *Fixture) {
		if name != "" {
			f.Workspace = filepath.Join(f.BaseDir, name)
		}
	}
}

func WithIDGenerator(gen func(prefix string) string) Option {
	return func(f *Fixture) {
		if gen != nil {
			f.newID = gen
		}
	}
}

func NewFixture(t testing.TB, opts ...Option) *Fixture {
	t.Helper()

	baseDir := t.TempDir()
	f := &Fixture{
		T:         t,
		BaseDir:   baseDir,
		Workspace: filepath.Join(baseDir, "workspace"),
		now:       fixedSequenceClock(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)),
		newID:     sequenceID(),
	}
	for _, opt := range opts {
		opt(f)
	}

	mustInitGitWorkspace(t, f.Workspace)
	f.WorkflowStore = mustWorkflowStore(t, filepath.Join(baseDir, "workflow.db"))
	f.PlanDB = mustPlanDB(t, filepath.Join(baseDir, "plans.db"))
	f.PlanStore = mustPlanStore(t, f.PlanDB)
	f.PatternDB, f.PatternStore, f.CommentStore = mustPatternStores(t, filepath.Join(baseDir, "patterns.db"))
	f.RetrievalDB = mustRetrievalDB(t, filepath.Join(baseDir, "retrieval.db"))
	f.Retrieval = archaeoretrieval.NewSQLStore(f.RetrievalDB)
	f.Graph = mustGraph(t, filepath.Join(baseDir, "graphdb"))

	f.Providers = archaeologyProviderStubs{
		PatternSurfacer:     &PatternSurfacerStub{},
		TensionAnalyzer:     &TensionAnalyzerStub{},
		ProspectiveAnalyzer: &ProspectiveAnalyzerStub{},
		ConvergenceReviewer: &ConvergenceReviewerStub{},
	}
	return f
}

func New(t *testing.T, opts ...Option) *Fixture {
	t.Helper()
	return NewFixture(t, opts...)
}

func (f *Fixture) Now() time.Time {
	if f == nil || f.now == nil {
		return time.Now().UTC()
	}
	return f.now()
}

func (f *Fixture) NewID(prefix string) string {
	if f == nil || f.newID == nil {
		return prefix
	}
	return f.newID(prefix)
}

func (f *Fixture) Context() context.Context {
	return context.Background()
}

func (f *Fixture) NewState() *core.Context {
	return core.NewContext()
}

func (f *Fixture) Task(workflowID, instruction string, extra map[string]any) *core.Task {
	contextMap := map[string]any{
		"workspace":    f.Workspace,
		"workflow_id":  workflowID,
		"corpus_scope": "workspace",
	}
	for key, value := range extra {
		contextMap[key] = value
	}
	return &core.Task{
		ID:          "task-" + workflowID,
		Type:        core.TaskTypeAnalysis,
		Instruction: instruction,
		Context:     contextMap,
	}
}

func (f *Fixture) WorkflowRecord(workflowID, instruction string) memory.WorkflowRecord {
	now := f.Now()
	return memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task-" + workflowID,
		TaskType:    core.TaskTypeCodeModification,
		Instruction: instruction,
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func (f *Fixture) SeedWorkflow(workflowID, instruction string) memory.WorkflowRecord {
	f.T.Helper()
	record := f.WorkflowRecord(workflowID, instruction)
	if err := f.WorkflowStore.CreateWorkflow(f.Context(), record); err != nil {
		f.T.Fatalf("create workflow %s: %v", workflowID, err)
	}
	return record
}

func (f *Fixture) SeedExploration(workflowID, workspaceID, basedOnRevision string, input archaeoarch.SnapshotInput) (*archaeodomain.ExplorationSession, *archaeodomain.ExplorationSnapshot) {
	f.T.Helper()
	if workspaceID == "" {
		workspaceID = f.Workspace
	}
	svc := f.ArchaeologyService()
	session, err := svc.EnsureExplorationSession(f.Context(), workflowID, workspaceID, basedOnRevision)
	if err != nil {
		f.T.Fatalf("ensure exploration session: %v", err)
	}
	if input.WorkflowID == "" {
		input.WorkflowID = workflowID
	}
	if input.WorkspaceID == "" {
		input.WorkspaceID = workspaceID
	}
	if input.BasedOnRevision == "" {
		input.BasedOnRevision = basedOnRevision
	}
	snapshot, err := svc.CreateExplorationSnapshot(f.Context(), session, input)
	if err != nil {
		f.T.Fatalf("create exploration snapshot: %v", err)
	}
	return session, snapshot
}

func (f *Fixture) SeedPlanVersion(plan *frameworkplan.LivingPlan, input archaeoplans.DraftVersionInput) *archaeodomain.VersionedLivingPlan {
	f.T.Helper()
	record, err := f.PlansService().DraftVersion(f.Context(), plan, input)
	if err != nil {
		f.T.Fatalf("draft plan version: %v", err)
	}
	return record
}

func (f *Fixture) SeedActivePlan(plan *frameworkplan.LivingPlan, input archaeoplans.DraftVersionInput) *archaeodomain.VersionedLivingPlan {
	f.T.Helper()
	record := f.SeedPlanVersion(plan, input)
	active, err := f.PlansService().ActivateVersion(f.Context(), record.WorkflowID, record.Version)
	if err != nil {
		f.T.Fatalf("activate plan version: %v", err)
	}
	return active
}

func (f *Fixture) SeedTension(input archaeotensions.CreateInput) *archaeodomain.Tension {
	f.T.Helper()
	record, err := f.TensionService().CreateOrUpdate(f.Context(), input)
	if err != nil {
		f.T.Fatalf("seed tension: %v", err)
	}
	return record
}

func (f *Fixture) SeedLearningInteraction(input archaeolearning.CreateInput) *archaeolearning.Interaction {
	f.T.Helper()
	record, err := f.LearningService().Create(f.Context(), input)
	if err != nil {
		f.T.Fatalf("seed learning interaction: %v", err)
	}
	return record
}

func (f *Fixture) SeedMutation(event archaeodomain.MutationEvent) archaeodomain.MutationEvent {
	f.T.Helper()
	if event.WorkflowID == "" {
		f.T.Fatalf("seed mutation requires workflow id")
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = f.Now()
	}
	if event.ID == "" {
		event.ID = f.NewID("mutation")
	}
	if err := archaeoevents.AppendMutationEvent(f.Context(), f.WorkflowStore, event); err != nil {
		f.T.Fatalf("append mutation event: %v", err)
	}
	return event
}

func (f *Fixture) SeedConvergenceRecord(input archaeoconvergence.CreateInput) *archaeodomain.ConvergenceRecord {
	f.T.Helper()
	record, err := f.ConvergenceService().Create(f.Context(), input)
	if err != nil {
		f.T.Fatalf("seed convergence record: %v", err)
	}
	return record
}

func (f *Fixture) SeedDeferredDraft(input archaeodeferred.CreateInput) *archaeodomain.DeferredDraftRecord {
	f.T.Helper()
	record, err := f.DeferredService().CreateOrUpdate(f.Context(), input)
	if err != nil {
		f.T.Fatalf("seed deferred draft: %v", err)
	}
	return record
}

func (f *Fixture) SeedAnchor(corpusScope string, decl frameworkretrieval.AnchorDeclaration, trustClass string) *frameworkretrieval.AnchorRecord {
	f.T.Helper()
	record, err := f.Retrieval.DeclareAnchor(f.Context(), decl, corpusScope, trustClass)
	if err != nil {
		f.T.Fatalf("declare anchor: %v", err)
	}
	return record
}

func (f *Fixture) ProviderBundle() archaeoproviders.Bundle {
	return archaeoproviders.Bundle{
		PatternSurfacer:     f.Providers.PatternSurfacer,
		TensionAnalyzer:     f.Providers.TensionAnalyzer,
		ProspectiveAnalyzer: f.Providers.ProspectiveAnalyzer,
		ConvergenceReviewer: f.Providers.ConvergenceReviewer,
	}
}

func (f *Fixture) PhaseService() archaeophases.Service {
	return archaeophases.Service{Store: f.WorkflowStore, Now: f.Now}
}

func (f *Fixture) PlansService() archaeoplans.Service {
	return archaeoplans.Service{Store: f.PlanStore, WorkflowStore: f.WorkflowStore, Now: f.Now}
}

func (f *Fixture) TensionService() archaeotensions.Service {
	return archaeotensions.Service{Store: f.WorkflowStore, Now: f.Now, NewID: f.NewID}
}

func (f *Fixture) LearningService() archaeolearning.Service {
	return archaeolearning.Service{
		Store:        f.WorkflowStore,
		PatternStore: f.PatternStore,
		CommentStore: f.CommentStore,
		PlanStore:    f.PlanStore,
		Retrieval:    f.Retrieval,
		Phases:       ptr(f.PhaseService()),
		Now:          f.Now,
		NewID:        f.NewID,
	}
}

func (f *Fixture) RequestsService() archaeorequests.Service {
	return archaeorequests.Service{Store: f.WorkflowStore, Now: f.Now, NewID: f.NewID}
}

func (f *Fixture) ArchaeologyService() archaeoarch.Service {
	return archaeoarch.Service{
		Store:     f.WorkflowStore,
		Plans:     f.PlansService(),
		Learning:  f.LearningService(),
		Providers: f.ProviderBundle(),
		Requests:  f.RequestsService(),
		Now:       f.Now,
		NewID:     f.NewID,
	}
}

func (f *Fixture) PersistPhaseFunc() archaeoarch.PhasePersister {
	return func(ctx context.Context, task *core.Task, state *core.Context, phase archaeodomain.EucloPhase, reason string, step *frameworkplan.PlanStep) {
		f.T.Helper()
		if _, err := f.PhaseService().RecordState(ctx, task, state, nil, phase, reason, step); err != nil {
			f.T.Fatalf("persist phase %s: %v", phase, err)
		}
	}
}

func (f *Fixture) ExecutionService() archaeoexec.Service {
	return archaeoexec.Service{
		WorkflowStore: f.WorkflowStore,
		Retrieval:     f.Retrieval,
		Now:           f.Now,
	}
}

func (f *Fixture) PreflightCoordinator() archaeoexec.PreflightCoordinator {
	return archaeoexec.PreflightCoordinator{
		Service: f.ExecutionService(),
		Plans:   f.PlansService(),
	}
}

func (f *Fixture) ConvergenceService() archaeoconvergence.Service {
	return archaeoconvergence.Service{Store: f.WorkflowStore, Now: f.Now, NewID: f.NewID}
}

func (f *Fixture) DeferredService() archaeodeferred.Service {
	return archaeodeferred.Service{Store: f.WorkflowStore, Now: f.Now, NewID: f.NewID}
}

func (f *Fixture) DecisionService() archaeodecisions.Service {
	return archaeodecisions.Service{Store: f.WorkflowStore, Now: f.Now, NewID: f.NewID}
}

func (f *Fixture) ProjectionService() *archaeoprojections.Service {
	return &archaeoprojections.Service{Store: f.WorkflowStore, Now: f.Now}
}

func (f *Fixture) ProvenanceService() *archaeoprovenance.Service {
	return &archaeoprovenance.Service{Store: f.WorkflowStore}
}

func ptr[T any](value T) *T {
	return &value
}

func fixedSequenceClock(start time.Time) func() time.Time {
	current := start.UTC()
	return func() time.Time {
		value := current
		current = current.Add(time.Second)
		return value
	}
}

func sequenceID() func(prefix string) string {
	seq := 0
	return func(prefix string) string {
		seq++
		return fmt.Sprintf("%s-%d", prefix, seq)
	}
}

func mustMkdirAll(tb testing.TB, dir string) {
	tb.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatalf("mkdir %s: %v", dir, err)
	}
}

func mustInitGitWorkspace(tb testing.TB, dir string) {
	tb.Helper()
	mustMkdirAll(tb, dir)
	run := func(args ...string) {
		tb.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			tb.Fatalf("git %v failed: %v\n%s", args, err, string(output))
		}
	}
	run("init")
	run("config", "user.email", "testscenario@example.com")
	run("config", "user.name", "Archaeo Test Fixture")
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("test scenario fixture\n"), 0o644); err != nil {
		tb.Fatalf("write workspace seed: %v", err)
	}
	run("add", "README.md")
	run("commit", "-m", "init")
}

func mustWorkflowStore(tb testing.TB, path string) *memorydb.SQLiteWorkflowStateStore {
	tb.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(path)
	if err != nil {
		tb.Fatalf("open workflow store: %v", err)
	}
	tb.Cleanup(func() { _ = store.Close() })
	return store
}

func mustPlanDB(tb testing.TB, path string) *sql.DB {
	tb.Helper()
	db, err := frameworkplan.OpenSQLite(path)
	if err != nil {
		tb.Fatalf("open plan db: %v", err)
	}
	tb.Cleanup(func() { _ = db.Close() })
	return db
}

func mustPlanStore(tb testing.TB, db *sql.DB) *frameworkplan.SQLitePlanStore {
	tb.Helper()
	store, err := frameworkplan.NewSQLitePlanStore(db)
	if err != nil {
		tb.Fatalf("open plan store: %v", err)
	}
	return store
}

func mustPatternStores(tb testing.TB, path string) (*sql.DB, *patterns.SQLitePatternStore, *patterns.SQLiteCommentStore) {
	tb.Helper()
	db, err := patterns.OpenSQLite(path)
	if err != nil {
		tb.Fatalf("open patterns db: %v", err)
	}
	tb.Cleanup(func() { _ = db.Close() })
	patternStore, err := patterns.NewSQLitePatternStore(db)
	if err != nil {
		tb.Fatalf("open pattern store: %v", err)
	}
	commentStore, err := patterns.NewSQLiteCommentStore(db)
	if err != nil {
		tb.Fatalf("open comment store: %v", err)
	}
	return db, patternStore, commentStore
}

func mustRetrievalDB(tb testing.TB, path string) *sql.DB {
	tb.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		tb.Fatalf("open retrieval db: %v", err)
	}
	if err := frameworkretrieval.EnsureSchema(context.Background(), db); err != nil {
		tb.Fatalf("ensure retrieval schema: %v", err)
	}
	tb.Cleanup(func() { _ = db.Close() })
	return db
}

func mustGraph(tb testing.TB, path string) *graphdb.Engine {
	tb.Helper()
	graph, err := graphdb.Open(graphdb.DefaultOptions(path))
	if err != nil {
		tb.Fatalf("open graphdb: %v", err)
	}
	tb.Cleanup(func() { _ = graph.Close() })
	return graph
}
