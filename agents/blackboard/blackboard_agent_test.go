package blackboard_test

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/agents/blackboard"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// --- Blackboard workspace tests ---------------------------------------------

func TestBlackboard_InitiallyEmpty(t *testing.T) {
	bb := blackboard.NewBlackboard("fix the bug")
	if bb.IsGoalSatisfied() {
		t.Error("new blackboard should not have goal satisfied")
	}
	if bb.HasUnverifiedArtifacts() {
		t.Error("new blackboard should have no artifacts")
	}
}

func TestBlackboard_GoalSatisfiedAfterVerifiedArtifact(t *testing.T) {
	bb := blackboard.NewBlackboard("fix the bug")
	bb.AddArtifact("a1", "patch", "some patch content", "Executor")
	if bb.IsGoalSatisfied() {
		t.Error("unverified artifact should not satisfy goal")
	}
	bb.Artifacts[0].Verified = true
	if !bb.IsGoalSatisfied() {
		t.Error("verified artifact should satisfy goal")
	}
}

func TestBlackboard_HasUnverifiedArtifacts(t *testing.T) {
	bb := blackboard.NewBlackboard("goal")
	bb.AddArtifact("a1", "patch", "content", "Executor")
	if !bb.HasUnverifiedArtifacts() {
		t.Error("should have unverified artifacts")
	}
	bb.Artifacts[0].Verified = true
	if bb.HasUnverifiedArtifacts() {
		t.Error("all verified: should not report unverified artifacts")
	}
}

// --- KS activation condition tests -----------------------------------------

func TestExplorerKS_ActivatesWhenNoFacts(t *testing.T) {
	ks := &blackboard.ExplorerKS{}
	bb := blackboard.NewBlackboard("goal")
	if !ks.CanActivate(bb) {
		t.Error("Explorer should activate when Facts is empty")
	}
	bb.AddFact("k", "v", "test")
	if ks.CanActivate(bb) {
		t.Error("Explorer should not activate when Facts is non-empty")
	}
}

func TestAnalyzerKS_ActivatesWithFactsButNoIssues(t *testing.T) {
	ks := &blackboard.AnalyzerKS{}
	bb := blackboard.NewBlackboard("goal")
	if ks.CanActivate(bb) {
		t.Error("Analyzer should not activate without facts")
	}
	bb.AddFact("k", "v", "test")
	if !ks.CanActivate(bb) {
		t.Error("Analyzer should activate with facts and no issues")
	}
	bb.AddIssue("i1", "desc", "low", "test")
	if ks.CanActivate(bb) {
		t.Error("Analyzer should not activate when issues already present")
	}
}

func TestPlannerKS_ActivatesWithIssuesButNoPendingActions(t *testing.T) {
	ks := &blackboard.PlannerKS{}
	bb := blackboard.NewBlackboard("goal")
	if ks.CanActivate(bb) {
		t.Error("Planner should not activate without issues")
	}
	bb.AddIssue("i1", "desc", "low", "test")
	if !ks.CanActivate(bb) {
		t.Error("Planner should activate with issues")
	}
}

func TestExecutorKS_ActivatesWithPendingActions(t *testing.T) {
	ks := &blackboard.ExecutorKS{}
	bb := blackboard.NewBlackboard("goal")
	if ks.CanActivate(bb) {
		t.Error("Executor should not activate with no pending actions")
	}
	bb.PendingActions = append(bb.PendingActions, blackboard.ActionRequest{
		ID: "a1", ToolOrAgent: "react", Description: "do something",
	})
	if !ks.CanActivate(bb) {
		t.Error("Executor should activate with pending actions")
	}
}

func TestVerifierKS_ActivatesWithUnverifiedArtifacts(t *testing.T) {
	ks := &blackboard.VerifierKS{}
	bb := blackboard.NewBlackboard("goal")
	if ks.CanActivate(bb) {
		t.Error("Verifier should not activate with no artifacts")
	}
	bb.AddArtifact("a1", "patch", "content", "Executor")
	if !ks.CanActivate(bb) {
		t.Error("Verifier should activate with unverified artifacts")
	}
	bb.Artifacts[0].Verified = true
	if ks.CanActivate(bb) {
		t.Error("Verifier should not activate when all artifacts are verified")
	}
}

// --- Controller tests -------------------------------------------------------

func TestController_SelectsHighestPriorityEligibleKS(t *testing.T) {
	var selected []string
	low := &recordingKS{name: "low", priority: 10, canActivate: func(_ *blackboard.Blackboard) bool { return true }}
	high := &recordingKS{name: "high", priority: 99, canActivate: func(_ *blackboard.Blackboard) bool { return true }}

	// Only run one cycle by having high KS satisfy the goal.
	high.exec = func(bb *blackboard.Blackboard) error {
		selected = append(selected, high.name)
		bb.AddArtifact("a1", "result", "done", "high")
		bb.Artifacts[0].Verified = true
		return nil
	}
	low.exec = func(bb *blackboard.Blackboard) error {
		selected = append(selected, low.name)
		return nil
	}

	ctrl := &blackboard.Controller{
		Sources:   []blackboard.KnowledgeSource{low, high},
		MaxCycles: 5,
	}
	bb := blackboard.NewBlackboard("goal")
	if err := ctrl.Run(context.Background(), bb, nil, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(selected) == 0 || selected[0] != "high" {
		t.Errorf("expected 'high' to run first, got %v", selected)
	}
	if len(selected) > 1 && selected[1] == "low" {
		t.Error("low-priority KS should not run after goal is satisfied")
	}
}

func TestController_ErrorsWhenStuck(t *testing.T) {
	never := &recordingKS{
		name:        "never",
		priority:    10,
		canActivate: func(_ *blackboard.Blackboard) bool { return false },
		exec:        func(_ *blackboard.Blackboard) error { return nil },
	}
	ctrl := &blackboard.Controller{
		Sources:   []blackboard.KnowledgeSource{never},
		MaxCycles: 5,
	}
	bb := blackboard.NewBlackboard("goal")
	err := ctrl.Run(context.Background(), bb, nil, nil)
	if err == nil {
		t.Error("expected error when no KS can activate")
	}
}

func TestController_TerminatesWhenGoalSatisfied(t *testing.T) {
	cycles := 0
	ks := &recordingKS{
		name:        "satisfier",
		priority:    10,
		canActivate: func(_ *blackboard.Blackboard) bool { return true },
		exec: func(bb *blackboard.Blackboard) error {
			cycles++
			bb.AddArtifact("a1", "result", "done", "satisfier")
			bb.Artifacts[0].Verified = true
			return nil
		},
	}
	ctrl := &blackboard.Controller{Sources: []blackboard.KnowledgeSource{ks}, MaxCycles: 20}
	bb := blackboard.NewBlackboard("goal")
	if err := ctrl.Run(context.Background(), bb, nil, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if cycles != 1 {
		t.Errorf("expected 1 cycle, got %d", cycles)
	}
}

// --- BlackboardAgent interface tests ----------------------------------------

func TestBlackboardAgent_ImplementsGraphAgent(t *testing.T) {
	a := &blackboard.BlackboardAgent{
		Config: &core.Config{MaxIterations: 8},
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	caps := a.Capabilities()
	if len(caps) == 0 {
		t.Error("Capabilities returned empty slice")
	}
	g, err := a.BuildGraph(&core.Task{})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if g == nil {
		t.Error("BuildGraph returned nil")
	}
}

func TestBlackboardAgent_ExecuteProducesArtifacts(t *testing.T) {
	a := &blackboard.BlackboardAgent{
		Config:    &core.Config{MaxIterations: 8},
		MaxCycles: 20,
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	task := &core.Task{
		ID:          "test-task",
		Type:        core.TaskTypeCodeModification,
		Instruction: "fix the login bug",
	}
	result, err := a.Execute(context.Background(), task, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	count, _ := result.Data["artifact_count"].(int)
	if count == 0 {
		t.Error("expected at least one artifact")
	}
}

func TestBlackboardAgent_ExecuteWithCustomKS(t *testing.T) {
	var ran bool
	customKS := &recordingKS{
		name:        "custom",
		priority:    200,
		canActivate: func(_ *blackboard.Blackboard) bool { return !ran },
		exec: func(bb *blackboard.Blackboard) error {
			ran = true
			bb.AddArtifact("custom-artifact", "summary", "custom output", "custom")
			bb.Artifacts[0].Verified = true
			return nil
		},
	}

	a := &blackboard.BlackboardAgent{
		Config:    &core.Config{},
		Sources:   []blackboard.KnowledgeSource{customKS},
		MaxCycles: 5,
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	result, err := a.Execute(context.Background(), &core.Task{Instruction: "custom task"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if !ran {
		t.Error("custom KS was not executed")
	}
}

// --- helpers ----------------------------------------------------------------

// recordingKS is a test-double KnowledgeSource.
type recordingKS struct {
	name        string
	priority    int
	canActivate func(*blackboard.Blackboard) bool
	exec        func(*blackboard.Blackboard) error
}

func (r *recordingKS) Name() string { return r.name }
func (r *recordingKS) Priority() int { return r.priority }
func (r *recordingKS) CanActivate(bb *blackboard.Blackboard) bool { return r.canActivate(bb) }
func (r *recordingKS) Execute(_ context.Context, bb *blackboard.Blackboard, _ *capability.Registry, _ core.LanguageModel) error {
	return r.exec(bb)
}
