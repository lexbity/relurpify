package session

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/memory"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

// stubWorkflowStore is a test double for memory.WorkflowStateStore
type stubWorkflowStore struct {
	workflows map[string]memory.WorkflowRecord
	runs      map[string][]memory.WorkflowRunRecord
}

func (s *stubWorkflowStore) SchemaVersion(ctx context.Context) (int, error) {
	return 1, nil
}

func (s *stubWorkflowStore) CreateWorkflow(ctx context.Context, workflow memory.WorkflowRecord) error {
	if s.workflows == nil {
		s.workflows = make(map[string]memory.WorkflowRecord)
	}
	s.workflows[workflow.WorkflowID] = workflow
	return nil
}

func (s *stubWorkflowStore) GetWorkflow(ctx context.Context, workflowID string) (*memory.WorkflowRecord, bool, error) {
	wf, ok := s.workflows[workflowID]
	if !ok {
		return nil, false, nil
	}
	return &wf, true, nil
}

func (s *stubWorkflowStore) ListWorkflows(ctx context.Context, limit int) ([]memory.WorkflowRecord, error) {
	var result []memory.WorkflowRecord
	for _, wf := range s.workflows {
		result = append(result, wf)
	}
	return result, nil
}

func (s *stubWorkflowStore) UpdateWorkflowStatus(ctx context.Context, workflowID string, expectedVersion int64, status memory.WorkflowRunStatus, cursorStepID string) (int64, error) {
	return 0, nil
}

func (s *stubWorkflowStore) CreateRun(ctx context.Context, run memory.WorkflowRunRecord) error {
	if s.runs == nil {
		s.runs = make(map[string][]memory.WorkflowRunRecord)
	}
	s.runs[run.WorkflowID] = append(s.runs[run.WorkflowID], run)
	return nil
}

func (s *stubWorkflowStore) GetRun(ctx context.Context, runID string) (*memory.WorkflowRunRecord, bool, error) {
	return nil, false, nil
}

func (s *stubWorkflowStore) UpdateRunStatus(ctx context.Context, runID string, status memory.WorkflowRunStatus, finishedAt *time.Time) error {
	return nil
}

func (s *stubWorkflowStore) SavePlan(ctx context.Context, plan memory.WorkflowPlanRecord) error {
	return nil
}

func (s *stubWorkflowStore) GetActivePlan(ctx context.Context, workflowID string) (*memory.WorkflowPlanRecord, bool, error) {
	return nil, false, nil
}

func (s *stubWorkflowStore) ListSteps(ctx context.Context, workflowID string) ([]memory.WorkflowStepRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) ListReadySteps(ctx context.Context, workflowID string) ([]memory.WorkflowStepRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) UpdateStepStatus(ctx context.Context, workflowID, stepID string, status memory.StepStatus, summary string) error {
	return nil
}

func (s *stubWorkflowStore) InvalidateDependents(ctx context.Context, workflowID, sourceStepID, reason string) ([]memory.InvalidationRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) ListInvalidations(ctx context.Context, workflowID string) ([]memory.InvalidationRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) CreateStepRun(ctx context.Context, run memory.StepRunRecord) error {
	return nil
}

func (s *stubWorkflowStore) ListStepRuns(ctx context.Context, workflowID, stepID string) ([]memory.StepRunRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) UpsertArtifact(ctx context.Context, artifact memory.StepArtifactRecord) error {
	return nil
}

func (s *stubWorkflowStore) ListArtifacts(ctx context.Context, workflowID, stepRunID string) ([]memory.StepArtifactRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) UpsertWorkflowArtifact(ctx context.Context, artifact memory.WorkflowArtifactRecord) error {
	return nil
}

func (s *stubWorkflowStore) ListWorkflowArtifacts(ctx context.Context, workflowID, runID string) ([]memory.WorkflowArtifactRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) SaveStageResult(ctx context.Context, record memory.WorkflowStageResultRecord) error {
	return nil
}

func (s *stubWorkflowStore) ListStageResults(ctx context.Context, workflowID, runID string) ([]memory.WorkflowStageResultRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) GetLatestValidStageResult(ctx context.Context, workflowID, runID, stageName string) (*memory.WorkflowStageResultRecord, bool, error) {
	return nil, false, nil
}

func (s *stubWorkflowStore) SavePipelineCheckpoint(ctx context.Context, record memory.PipelineCheckpointRecord) error {
	return nil
}

func (s *stubWorkflowStore) LoadPipelineCheckpoint(ctx context.Context, taskID, checkpointID string) (*memory.PipelineCheckpointRecord, bool, error) {
	return nil, false, nil
}

func (s *stubWorkflowStore) ListPipelineCheckpoints(ctx context.Context, taskID string) ([]string, error) {
	return nil, nil
}

func (s *stubWorkflowStore) PutKnowledge(ctx context.Context, record memory.KnowledgeRecord) error {
	return nil
}

func (s *stubWorkflowStore) ListKnowledge(ctx context.Context, workflowID string, kind memory.KnowledgeKind, unresolvedOnly bool) ([]memory.KnowledgeRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) AppendEvent(ctx context.Context, event memory.WorkflowEventRecord) error {
	return nil
}

func (s *stubWorkflowStore) ListEvents(ctx context.Context, workflowID string, limit int) ([]memory.WorkflowEventRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) ReplaceProviderSnapshots(ctx context.Context, workflowID, runID string, snapshots []memory.WorkflowProviderSnapshotRecord) error {
	return nil
}

func (s *stubWorkflowStore) ListProviderSnapshots(ctx context.Context, workflowID, runID string) ([]memory.WorkflowProviderSnapshotRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) ReplaceProviderSessionSnapshots(ctx context.Context, workflowID, runID string, snapshots []memory.WorkflowProviderSessionSnapshotRecord) error {
	return nil
}

func (s *stubWorkflowStore) ListProviderSessionSnapshots(ctx context.Context, workflowID, runID string) ([]memory.WorkflowProviderSessionSnapshotRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) UpsertDelegation(ctx context.Context, record memory.WorkflowDelegationRecord) error {
	return nil
}

func (s *stubWorkflowStore) ListDelegations(ctx context.Context, workflowID, runID string) ([]memory.WorkflowDelegationRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) AppendDelegationTransition(ctx context.Context, record memory.WorkflowDelegationTransitionRecord) error {
	return nil
}

func (s *stubWorkflowStore) ListDelegationTransitions(ctx context.Context, delegationID string) ([]memory.WorkflowDelegationTransitionRecord, error) {
	return nil, nil
}

func (s *stubWorkflowStore) LoadStepSlice(ctx context.Context, workflowID, stepID string, eventLimit int) (*memory.WorkflowStepSlice, bool, error) {
	return nil, false, nil
}

func (s *stubWorkflowStore) UpdateWorkflowMetadata(ctx context.Context, workflowID string, updates map[string]any) error {
	return nil
}

// stubPlanStore is a test double for frameworkplan.PlanStore
type stubPlanStore struct {
	plans map[string]*frameworkplan.LivingPlan
}

func (s *stubPlanStore) SavePlan(ctx context.Context, plan *frameworkplan.LivingPlan) error {
	if s.plans == nil {
		s.plans = make(map[string]*frameworkplan.LivingPlan)
	}
	s.plans[plan.ID] = plan
	return nil
}

func (s *stubPlanStore) LoadPlan(ctx context.Context, planID string) (*frameworkplan.LivingPlan, error) {
	return s.plans[planID], nil
}

func (s *stubPlanStore) LoadPlanByWorkflow(ctx context.Context, workflowID string) (*frameworkplan.LivingPlan, error) {
	for _, plan := range s.plans {
		if plan.WorkflowID == workflowID {
			return plan, nil
		}
	}
	return nil, nil
}

func (s *stubPlanStore) UpdateStep(ctx context.Context, planID, stepID string, step *frameworkplan.PlanStep) error {
	return nil
}

func (s *stubPlanStore) InvalidateStep(ctx context.Context, planID, stepID string, rule frameworkplan.InvalidationRule) error {
	return nil
}

func (s *stubPlanStore) DeletePlan(ctx context.Context, planID string) error {
	return nil
}

func (s *stubPlanStore) ListPlans(ctx context.Context) ([]frameworkplan.PlanSummary, error) {
	return nil, nil
}

func TestSessionIndex_List_Returns(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	now := time.Now().UTC()
	wf := memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		Instruction: "test instruction",
		Status:      memory.WorkflowRunStatusCompleted,
		Metadata:    map[string]any{"workspace": "/test/workspace"},
		UpdatedAt:   now,
	}
	store.workflows["wf-1"] = wf

	index := &SessionIndex{WorkflowStore: store, PlanStore: &stubPlanStore{}}

	list, err := index.List(ctx, "/test/workspace", 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list.Sessions) != 1 {
		t.Errorf("List() returned %d sessions, want 1", len(list.Sessions))
	}

	if list.Workspace != "/test/workspace" {
		t.Errorf("List() workspace = %q, want %q", list.Workspace, "/test/workspace")
	}
}

func TestSessionIndex_List_SortsByRecency(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	now := time.Now().UTC()
	store.workflows["wf-1"] = memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		Instruction: "first",
		UpdatedAt:   now.Add(-2 * time.Hour),
	}
	store.workflows["wf-2"] = memory.WorkflowRecord{
		WorkflowID:  "wf-2",
		Instruction: "second",
		UpdatedAt:   now.Add(-1 * time.Hour),
	}
	store.workflows["wf-3"] = memory.WorkflowRecord{
		WorkflowID:  "wf-3",
		Instruction: "third",
		UpdatedAt:   now,
	}

	index := &SessionIndex{WorkflowStore: store, PlanStore: &stubPlanStore{}}

	list, err := index.List(ctx, "", 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list.Sessions) != 3 {
		t.Fatalf("List() returned %d sessions, want 3", len(list.Sessions))
	}

	// Should be ordered most-recent-first: wf-3, wf-2, wf-1
	if list.Sessions[0].WorkflowID != "wf-3" {
		t.Errorf("First session = %q, want wf-3 (most recent)", list.Sessions[0].WorkflowID)
	}
	if list.Sessions[1].WorkflowID != "wf-2" {
		t.Errorf("Second session = %q, want wf-2", list.Sessions[1].WorkflowID)
	}
	if list.Sessions[2].WorkflowID != "wf-1" {
		t.Errorf("Third session = %q, want wf-1 (oldest)", list.Sessions[2].WorkflowID)
	}
}

func TestSessionIndex_List_FiltersToWorkspace(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	now := time.Now().UTC()
	store.workflows["wf-1"] = memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		Instruction: "workspace a",
		Metadata:    map[string]any{"workspace": "/workspace/a"},
		UpdatedAt:   now,
	}
	store.workflows["wf-2"] = memory.WorkflowRecord{
		WorkflowID:  "wf-2",
		Instruction: "workspace b",
		Metadata:    map[string]any{"workspace": "/workspace/b"},
		UpdatedAt:   now,
	}
	store.workflows["wf-3"] = memory.WorkflowRecord{
		WorkflowID:  "wf-3",
		Instruction: "untagged",
		UpdatedAt:   now,
	}

	index := &SessionIndex{WorkflowStore: store, PlanStore: &stubPlanStore{}}

	list, err := index.List(ctx, "/workspace/a", 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// Should include wf-1 (matching workspace) and wf-3 (untagged)
	if len(list.Sessions) != 2 {
		t.Errorf("List() returned %d sessions, want 2 (matching + untagged)", len(list.Sessions))
	}

	for _, s := range list.Sessions {
		if s.WorkflowID == "wf-2" {
			t.Error("List() included wf-2 from different workspace")
		}
	}
}

func TestSessionIndex_List_HandlesMissingPlan(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}
	planStore := &stubPlanStore{plans: make(map[string]*frameworkplan.LivingPlan)}

	now := time.Now().UTC()
	store.workflows["wf-1"] = memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		Instruction: "no plan",
		UpdatedAt:   now,
	}

	index := &SessionIndex{WorkflowStore: store, PlanStore: planStore}

	list, err := index.List(ctx, "", 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list.Sessions) != 1 {
		t.Fatalf("List() returned %d sessions, want 1", len(list.Sessions))
	}

	session := list.Sessions[0]
	if session.HasBKCContext {
		t.Error("HasBKCContext = true, want false for session without plan")
	}
	if session.ActivePlanVersion != 0 {
		t.Errorf("ActivePlanVersion = %d, want 0", session.ActivePlanVersion)
	}
	if len(session.RootChunkIDs) > 0 {
		t.Errorf("RootChunkIDs = %v, want empty", session.RootChunkIDs)
	}
}

func TestSessionIndex_Get_ReturnsRecord(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	now := time.Now().UTC()
	store.workflows["wf-1"] = memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		Instruction: "test instruction",
		Status:      memory.WorkflowRunStatusRunning,
		Metadata: map[string]any{
			"workspace": "/test",
			"mode":      "code",
			"phase":     "execution",
		},
		UpdatedAt: now,
	}

	index := &SessionIndex{WorkflowStore: store, PlanStore: &stubPlanStore{}}

	record, ok, err := index.Get(ctx, "wf-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() returned ok=false, want true")
	}

	if record.WorkflowID != "wf-1" {
		t.Errorf("WorkflowID = %q, want wf-1", record.WorkflowID)
	}
	if record.Instruction != "test instruction" {
		t.Errorf("Instruction = %q, want 'test instruction'", record.Instruction)
	}
	if record.Mode != "code" {
		t.Errorf("Mode = %q, want 'code'", record.Mode)
	}
	if record.Phase != "execution" {
		t.Errorf("Phase = %q, want 'execution'", record.Phase)
	}
	if record.WorkspaceRoot != "/test" {
		t.Errorf("WorkspaceRoot = %q, want '/test'", record.WorkspaceRoot)
	}
}

func TestSessionIndex_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	index := &SessionIndex{WorkflowStore: store, PlanStore: &stubPlanStore{}}

	record, ok, err := index.Get(ctx, "non-existent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if ok {
		t.Error("Get() returned ok=true, want false for non-existent workflow")
	}
	if record.WorkflowID != "" {
		t.Errorf("Record.WorkflowID = %q, want empty", record.WorkflowID)
	}
}
