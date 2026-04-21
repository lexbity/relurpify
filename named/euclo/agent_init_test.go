package euclo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/langdetect"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

func TestInitializeEnvironment_WiresDefaultVerificationPlanner(t *testing.T) {
	agent := &Agent{}
	err := agent.InitializeEnvironment(ayenitd.WorkspaceEnvironment{
		Registry: capability.NewRegistry(),
		Config:   &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if agent.Environment.VerificationPlanner == nil {
		t.Fatal("expected default verification planner to be wired")
	}
}

func TestInitializeEnvironment_PreservesExistingVerificationPlanner(t *testing.T) {
	custom := frameworkplan.NewVerificationScopePlanner()
	agent := &Agent{}
	err := agent.InitializeEnvironment(ayenitd.WorkspaceEnvironment{
		Registry:            capability.NewRegistry(),
		Config:              &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
		VerificationPlanner: custom,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if agent.Environment.VerificationPlanner != custom {
		t.Fatal("expected existing verification planner to be preserved")
	}
}

func TestInitializeEnvironment_WiresCompatibilitySurfaceWhenNil(t *testing.T) {
	agent := &Agent{}
	if err := agent.InitializeEnvironment(ayenitd.WorkspaceEnvironment{
		Registry: capability.NewRegistry(),
		Config:   &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
	}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if agent.Environment.CompatibilitySurfaceExtractor == nil {
		t.Fatal("expected default compatibility surface extractor to be wired")
	}
}

func TestInitializeEnvironment_DetectsWorkspaceFromIndexManager(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	originalDetect := detectWorkspaceLanguages
	t.Cleanup(func() {
		detectWorkspaceLanguages = originalDetect
	})

	var detectedWorkspace string
	detectWorkspaceLanguages = func(workspacePath string) langdetect.WorkspaceLanguages {
		detectedWorkspace = workspacePath
		return langdetect.WorkspaceLanguages{Go: true}
	}

	agent := &Agent{}
	err := agent.InitializeEnvironment(ayenitd.WorkspaceEnvironment{
		Registry:     capability.NewRegistry(),
		Config:       &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
		IndexManager: ast.NewIndexManager(nil, ast.IndexConfig{WorkspacePath: workspace}),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if detectedWorkspace != workspace {
		t.Fatalf("expected detection to use workspace %q, got %q", workspace, detectedWorkspace)
	}
	if agent.Environment.VerificationPlanner == nil {
		t.Fatal("expected verification planner to be wired from detected workspace")
	}
	if agent.Environment.CompatibilitySurfaceExtractor == nil {
		t.Fatal("expected compatibility planner to be wired from detected workspace")
	}
}

func TestWorkspacePathFromConfigExtension(t *testing.T) {
	workspace := t.TempDir()
	cfg := &core.Config{
		Name:  "test",
		Model: "stub",
		AgentSpec: &core.AgentRuntimeSpec{
			Extensions: map[string]any{"workspace": workspace},
		},
	}

	if got := workspacePathFromConfig(cfg); got != workspace {
		t.Fatalf("expected workspace path %q from config, got %q", workspace, got)
	}
}

func TestEnsureDeferralPlanRegistersResolveRoutine(t *testing.T) {
	agent := &Agent{}
	if err := agent.InitializeEnvironment(ayenitd.WorkspaceEnvironment{
		Registry: capability.NewRegistry(),
		Config:   &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
	}); err != nil {
		t.Fatalf("InitializeEnvironment: %v", err)
	}
	agent.GuidanceBroker = guidance.NewGuidanceBroker(50)
	task := &core.Task{Context: map[string]any{"workflow_id": "wf-1"}}
	agent.ensureDeferralPlan(task, core.NewContext())

	if agent.BehaviorDispatcher == nil {
		t.Fatal("expected behavior dispatcher")
	}
	_, err := agent.BehaviorDispatcher.ExecuteRoutine(context.Background(), euclorelurpic.CapabilityDeferralsResolve, task, core.NewContext(), eucloruntime.UnitOfWork{}, agent.Environment, execution.ServiceBundle{})
	if err == nil {
		t.Fatal("expected routine to return validation error for missing input, got nil")
	}
	if strings.Contains(err.Error(), "not registered") {
		t.Fatalf("expected registered routine, got %v", err)
	}
}

func TestInitializeEnvironmentRegistersLearningPromoteRoutine(t *testing.T) {
	agent := &Agent{}
	if err := agent.InitializeEnvironment(ayenitd.WorkspaceEnvironment{
		Registry: capability.NewRegistry(),
		Config:   &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
	}); err != nil {
		t.Fatalf("InitializeEnvironment: %v", err)
	}
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-1")
	state.Set("euclo.active_exploration_id", "explore-1")
	state.Set("euclo.learning_promote_input", eucloruntime.LearningPromoteInput{
		Title:       "Remember this",
		Description: "Keep this insight for later",
		Kind:        string(archaeolearning.InteractionKnowledgeProposal),
	})
	_, err := agent.BehaviorDispatcher.ExecuteRoutine(context.Background(), euclorelurpic.CapabilityLearningPromote, nil, state, eucloruntime.UnitOfWork{}, agent.Environment, execution.ServiceBundle{})
	if err == nil {
		t.Fatal("expected routine to return an error because the service store is unavailable")
	}
	if strings.Contains(err.Error(), "not registered") {
		t.Fatalf("expected registered routine, got %v", err)
	}
}

func TestInitializeEnvironmentRegistersDeferralLoaderFirst(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	agent := &Agent{}
	if err := agent.InitializeEnvironment(ayenitd.WorkspaceEnvironment{
		Registry:     capability.NewRegistry(),
		Config:       &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
		IndexManager: ast.NewIndexManager(nil, ast.IndexConfig{WorkspacePath: workspace}),
	}); err != nil {
		t.Fatalf("InitializeEnvironment: %v", err)
	}
	if agent.ContextPipeline == nil {
		t.Fatal("expected context pipeline")
	}
	if got := agent.ContextPipeline.PreRunStepIDs(); len(got) == 0 || got[0] != "euclo:deferrals.load" {
		t.Fatalf("expected deferral loader to be first pre-run step, got %#v", got)
	}
}

func TestInitializeEnvironmentRegistersLearningSyncStep(t *testing.T) {
	workspace := t.TempDir()
	patternStore := &stubPatternStore{}
	agent := &Agent{}
	if err := agent.InitializeEnvironment(ayenitd.WorkspaceEnvironment{
		Registry:     capability.NewRegistry(),
		Config:       &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
		IndexManager: ast.NewIndexManager(nil, ast.IndexConfig{WorkspacePath: workspace}),
		PatternStore: patternStore,
	}); err != nil {
		t.Fatalf("InitializeEnvironment: %v", err)
	}
	if agent.ContextPipeline == nil {
		t.Fatal("expected context pipeline")
	}
	got := agent.ContextPipeline.PreRunStepIDs()
	if len(got) < 1 {
		t.Fatalf("expected pre-run steps, got %#v", got)
	}
	if len(got) < 2 {
		t.Fatalf("expected learning sync and delta pre-run steps, got %#v", got)
	}
	if got[len(got)-2] != "euclo:learning.sync" {
		t.Fatalf("expected learning sync before delta, got %#v", got)
	}
	if got[len(got)-1] != "euclo:learning.delta" {
		t.Fatalf("expected learning delta to be appended after sync, got %#v", got)
	}
}

type stubPatternStore struct{}

func (s *stubPatternStore) Save(context.Context, patterns.PatternRecord) error { return nil }
func (s *stubPatternStore) Load(context.Context, string) (*patterns.PatternRecord, error) {
	return nil, nil
}
func (s *stubPatternStore) ListByStatus(context.Context, patterns.PatternStatus, string) ([]patterns.PatternRecord, error) {
	return nil, nil
}
func (s *stubPatternStore) ListByKind(context.Context, patterns.PatternKind, string) ([]patterns.PatternRecord, error) {
	return nil, nil
}
func (s *stubPatternStore) UpdateStatus(context.Context, string, patterns.PatternStatus, string) error {
	return nil
}
func (s *stubPatternStore) Supersede(context.Context, string, patterns.PatternRecord) error {
	return nil
}
