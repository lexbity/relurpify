package session

import (
	"context"
	"strings"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/memory"
)

func TestSessionResumeContext_IsEmpty_TrueWhenNoWorkflowID(t *testing.T) {
	ctx := SessionResumeContext{}
	if !ctx.IsEmpty() {
		t.Error("IsEmpty() = false, want true for empty context")
	}
}

func TestSessionResumeContext_IsEmpty_FalseWhenHasRootChunkIDs(t *testing.T) {
	ctx := SessionResumeContext{
		WorkflowID:   "wf-1",
		RootChunkIDs: []string{"chunk-1", "chunk-2"},
	}
	if ctx.IsEmpty() {
		t.Error("IsEmpty() = true, want false when has RootChunkIDs")
	}
}

func TestSessionResumeContext_IsEmpty_FalseWhenHasPlanVersion(t *testing.T) {
	ctx := SessionResumeContext{
		WorkflowID:        "wf-1",
		ActivePlanVersion: 5,
	}
	if ctx.IsEmpty() {
		t.Error("IsEmpty() = true, want false when has ActivePlanVersion")
	}
}

func TestSessionResumeContext_IsEmpty_FalseWhenHasPhaseState(t *testing.T) {
	ctx := SessionResumeContext{
		WorkflowID: "wf-1",
		PhaseState: &archaeodomain.WorkflowPhaseState{},
	}
	if ctx.IsEmpty() {
		t.Error("IsEmpty() = true, want false when has PhaseState")
	}
}

func TestSessionResumeResolver_Resolve_MissingWorkflow_Error(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	resolver := &SessionResumeResolver{
		WorkflowStore: store,
		PlanStore:     &stubPlanStore{},
	}

	_, err := resolver.Resolve(ctx, "non-existent")
	if err == nil {
		t.Error("Resolve() returned nil error, want error for missing workflow")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Resolve() error = %v, want 'not found'", err)
	}
}

func TestSessionResumeResolver_Resolve_PopulatesRootChunkIDs(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}
	planStore := &stubPlanStore{}

	now := time.Now().UTC()
	store.workflows["wf-1"] = memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		Instruction: "test task",
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now,
	}

	resolver := &SessionResumeResolver{
		WorkflowStore: store,
		PlanStore:     planStore,
	}

	resume, err := resolver.Resolve(ctx, "wf-1")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if resume.WorkflowID != "wf-1" {
		t.Errorf("WorkflowID = %q, want wf-1", resume.WorkflowID)
	}

	if resume.RunID == "" {
		t.Error("RunID is empty, want generated run ID")
	}

	if !strings.HasPrefix(resume.RunID, "wf-1-resume-") {
		t.Errorf("RunID = %q, want prefix 'wf-1-resume-'", resume.RunID)
	}
	if resume.SessionStartTime.IsZero() {
		t.Fatal("SessionStartTime is zero, want workflow CreatedAt")
	}
	if !resume.SessionStartTime.Equal(now.Add(-time.Hour)) {
		t.Fatalf("SessionStartTime = %v, want %v", resume.SessionStartTime, now.Add(-time.Hour))
	}
}

func TestSessionResumeResolver_Resolve_PopulatesModeFromMetadata(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	now := time.Now().UTC()
	store.workflows["wf-1"] = memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		Instruction: "test task",
		CreatedAt:   now.Add(-time.Hour),
		Metadata:    map[string]any{"mode": "debug"},
		UpdatedAt:   now,
	}

	resolver := &SessionResumeResolver{
		WorkflowStore: store,
		PlanStore:     &stubPlanStore{},
	}

	resume, err := resolver.Resolve(ctx, "wf-1")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if resume.Mode != "debug" {
		t.Errorf("Mode = %q, want 'debug'", resume.Mode)
	}
	if !resume.SessionStartTime.Equal(now.Add(-time.Hour)) {
		t.Fatalf("SessionStartTime = %v, want %v", resume.SessionStartTime, now.Add(-time.Hour))
	}
}

func TestSessionResumeResolver_Resolve_SemanticSummaryNonFatal(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	now := time.Now().UTC()
	store.workflows["wf-1"] = memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		Instruction: "test task",
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now,
	}

	resolver := &SessionResumeResolver{
		WorkflowStore: store,
		PlanStore:     &stubPlanStore{},
	}

	// Services that always error - should not cause Resolve to fail
	errorServices := SemanticResolutionServices{
		Tensions: &errorTensionLister{},
		Learning: &errorLearningLister{},
	}

	resume, err := resolver.ResolveWithServices(ctx, "wf-1", errorServices)
	if err != nil {
		t.Fatalf("ResolveWithServices() error = %v, want nil (non-fatal)", err)
	}

	// Semantic summary should be empty but not nil
	if len(resume.SemanticSummary.Tensions) != 0 {
		t.Error("Tensions should be empty when service errors")
	}
	if len(resume.SemanticSummary.LearningInteractions) != 0 {
		t.Error("LearningInteractions should be empty when service errors")
	}
}

// stub error listers for testing non-fatal behavior
type errorTensionLister struct{}

func (e *errorTensionLister) ListForWorkflow(ctx context.Context, workflowID string) ([]archaeodomain.Tension, error) {
	return nil, context.Canceled
}

type errorLearningLister struct{}

func (e *errorLearningLister) ListForWorkflow(ctx context.Context, workflowID string) ([]archaeodomain.ExplorationSnapshot, error) {
	return nil, context.Canceled
}

// success listers for testing happy path
type successTensionLister struct {
	tensions []archaeodomain.Tension
}

func (s *successTensionLister) ListForWorkflow(ctx context.Context, workflowID string) ([]archaeodomain.Tension, error) {
	return s.tensions, nil
}

func TestSessionResumeResolver_Resolve_PopulatesTensions(t *testing.T) {
	ctx := context.Background()
	store := &stubWorkflowStore{workflows: make(map[string]memory.WorkflowRecord)}

	now := time.Now().UTC()
	store.workflows["wf-1"] = memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		Instruction: "test task",
		UpdatedAt:   now,
	}

	resolver := &SessionResumeResolver{
		WorkflowStore: store,
		PlanStore:     &stubPlanStore{},
	}

	tensions := []archaeodomain.Tension{
		{
			ID:          "tension-1",
			Kind:        "conflict",
			Description: "test tension",
			Status:      archaeodomain.TensionConfirmed,
		},
	}

	services := SemanticResolutionServices{
		Tensions: &successTensionLister{tensions: tensions},
	}

	resume, err := resolver.ResolveWithServices(ctx, "wf-1", services)
	if err != nil {
		t.Fatalf("ResolveWithServices() error = %v", err)
	}

	if len(resume.SemanticSummary.Tensions) != 1 {
		t.Fatalf("Tensions count = %d, want 1", len(resume.SemanticSummary.Tensions))
	}

	tension := resume.SemanticSummary.Tensions[0]
	if tension.ID != "tension-1" {
		t.Errorf("Tension.ID = %q, want 'tension-1'", tension.ID)
	}
	if tension.Kind != "tension" {
		t.Errorf("Tension.Kind = %q, want 'tension'", tension.Kind)
	}
	if tension.Status != string(archaeodomain.TensionConfirmed) {
		t.Errorf("Tension.Status = %q, want 'confirmed'", tension.Status)
	}
}

func TestSemanticFindingSummary_Fields(t *testing.T) {
	summary := SemanticFindingSummary{
		ID:      "finding-1",
		Title:   "Test Finding",
		Summary: "This is a test",
		Kind:    "pattern",
		Status:  "active",
	}

	if summary.ID != "finding-1" {
		t.Errorf("ID = %q, want 'finding-1'", summary.ID)
	}
	if summary.Title != "Test Finding" {
		t.Errorf("Title = %q, want 'Test Finding'", summary.Title)
	}
	if summary.Kind != "pattern" {
		t.Errorf("Kind = %q, want 'pattern'", summary.Kind)
	}
}
