package blackboard_test

import (
	"context"
	"fmt"
	"testing"

	"codeburg.org/lexbit/relurpify/agents/blackboard"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	// "codeburg.org/lexbit/relurpify/framework/memory"  // use memory after rework
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
	if err := bb.AddArtifact("a1", "patch", "some patch content", "Executor"); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	if bb.IsGoalSatisfied() {
		t.Error("unverified artifact should not satisfy goal")
	}
	if !bb.VerifyArtifact("a1") {
		t.Fatal("expected artifact verification")
	}
	if !bb.IsGoalSatisfied() {
		t.Error("verified artifact should satisfy goal")
	}
}

func TestBlackboard_HasUnverifiedArtifacts(t *testing.T) {
	bb := blackboard.NewBlackboard("goal")
	if err := bb.AddArtifact("a1", "patch", "content", "Executor"); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	if !bb.HasUnverifiedArtifacts() {
		t.Error("should have unverified artifacts")
	}
	if !bb.VerifyArtifact("a1") {
		t.Fatal("expected artifact verification")
	}
	if bb.HasUnverifiedArtifacts() {
		t.Error("all verified: should not report unverified artifacts")
	}
}

// --- KS activation condition tests -----------------------------------------

func TestExplorerKS_ActivatesWhenNoFacts(t *testing.T) {
	ks := &blackboard.ExplorerKS{}
	bb := blackboard.NewBlackboard("goal")
	if !ks.CanActivate(bb) {
		t.Error("Explorer should activate when exploration.status not set")
	}
	bb.AddFact("exploration.status", "explored", "test")
	if ks.CanActivate(bb) {
		t.Error("Explorer should not activate when exploration.status is set")
	}
}

func TestAnalyzerKS_ActivatesWithFactsButNoIssues(t *testing.T) {
	ks := &blackboard.AnalyzerKS{}
	bb := blackboard.NewBlackboard("goal")
	if ks.CanActivate(bb) {
		t.Error("Analyzer should not activate without exploration.status")
	}
	bb.AddFact("exploration.status", "explored", "test")
	if !ks.CanActivate(bb) {
		t.Error("Analyzer should activate with exploration.status and no issues")
	}
	if err := bb.AddIssue("i1", "desc", "low", "test"); err != nil {
		t.Fatalf("AddIssue: %v", err)
	}
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
	if err := bb.AddIssue("i1", "desc", "low", "test"); err != nil {
		t.Fatalf("AddIssue: %v", err)
	}
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
	if err := bb.EnqueueAction(blackboard.ActionRequest{
		ID: "a1", ToolOrAgent: "react", Description: "do something",
	}); err != nil {
		t.Fatalf("EnqueueAction: %v", err)
	}
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
	if err := bb.AddArtifact("a1", "patch", "content", "Executor"); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	if !ks.CanActivate(bb) {
		t.Error("Verifier should activate with unverified artifacts")
	}
	if !bb.VerifyArtifact("a1") {
		t.Fatal("expected artifact verification")
	}
	if ks.CanActivate(bb) {
		t.Error("Verifier should not activate when all artifacts are verified")
	}
}

func TestAnalyzerKS_UsesReviewerCapabilityWhenAvailable(t *testing.T) {
	registry := capability.NewRegistry()
	reviewer := &stubInvocableCapability{
		desc: capabilityDescriptor("reviewer.review"),
		invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			return &core.CapabilityExecutionResult{
				Success: true,
				Data: map[string]interface{}{
					"findings": []any{
						map[string]any{"severity": "high", "description": "missing validation"},
					},
				},
			}, nil
		},
	}
	if err := registry.RegisterInvocableCapability(reviewer); err != nil {
		t.Fatalf("RegisterInvocableCapability: %v", err)
	}
	ks := &blackboard.AnalyzerKS{}
	bb := blackboard.NewBlackboard("goal")
	bb.AddFact("k", "v", "test")
	if err := ks.Execute(context.Background(), bb, registry, nil, core.AgentSemanticContext{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(bb.Issues) != 1 || bb.Issues[0].Description != "missing validation" {
		t.Fatalf("unexpected issues: %#v", bb.Issues)
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
		if err := bb.AddArtifact("a1", "result", "done", "high"); err != nil {
			return err
		}
		bb.VerifyArtifact("a1")
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
	if err := ctrl.Run(context.Background(), bb, nil, nil, core.AgentSemanticContext{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(selected) == 0 || selected[0] != "high" {
		t.Errorf("expected 'high' to run first, got %v", selected)
	}
	if len(selected) > 1 && selected[1] == "low" {
		t.Error("low-priority KS should not run after goal is satisfied")
	}
}

func TestController_BreaksPriorityTiesByStableName(t *testing.T) {
	var selected []string
	alpha := &recordingKS{
		name:        "alpha",
		priority:    50,
		canActivate: func(_ *blackboard.Blackboard) bool { return true },
		exec: func(bb *blackboard.Blackboard) error {
			selected = append(selected, "alpha")
			if err := bb.AddArtifact("a1", "result", "done", "alpha"); err != nil {
				return err
			}
			bb.VerifyArtifact("a1")
			return nil
		},
	}
	zeta := &recordingKS{
		name:        "zeta",
		priority:    50,
		canActivate: func(_ *blackboard.Blackboard) bool { return true },
		exec: func(_ *blackboard.Blackboard) error {
			selected = append(selected, "zeta")
			return nil
		},
	}
	ctrl := &blackboard.Controller{Sources: []blackboard.KnowledgeSource{zeta, alpha}, MaxCycles: 3}
	if err := ctrl.Run(context.Background(), blackboard.NewBlackboard("goal"), nil, nil, core.AgentSemanticContext{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(selected) == 0 || selected[0] != "alpha" {
		t.Fatalf("expected alpha to win tie-break, got %v", selected)
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
	err := ctrl.Run(context.Background(), bb, nil, nil, core.AgentSemanticContext{})
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
			if err := bb.AddArtifact("a1", "result", "done", "satisfier"); err != nil {
				return err
			}
			bb.VerifyArtifact("a1")
			return nil
		},
	}
	ctrl := &blackboard.Controller{Sources: []blackboard.KnowledgeSource{ks}, MaxCycles: 20}
	bb := blackboard.NewBlackboard("goal")
	if err := ctrl.Run(context.Background(), bb, nil, nil, core.AgentSemanticContext{}); err != nil {
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

func TestBlackboardAgent_ExecutePublishesNamespacedContextState(t *testing.T) {
	a := &blackboard.BlackboardAgent{
		Config:    &core.Config{MaxIterations: 8},
		MaxCycles: 20,
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	state := core.NewContext()
	task := &core.Task{
		ID:          "test-task-state",
		Type:        core.TaskTypeCodeModification,
		Instruction: "fix the login bug",
	}
	if _, err := a.Execute(context.Background(), task, state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if raw, ok := state.Get("blackboard.goals"); !ok {
		t.Fatal("expected blackboard.goals")
	} else if goals, ok := raw.([]string); !ok || len(goals) != 1 || goals[0] != task.Instruction {
		t.Fatalf("unexpected blackboard.goals: %#v", raw)
	}

	if raw, ok := state.Get("blackboard.facts"); !ok {
		t.Fatal("expected blackboard.facts")
	} else if facts, ok := raw.([]blackboard.Fact); !ok || len(facts) == 0 {
		t.Fatalf("unexpected blackboard.facts: %#v", raw)
	}

	if raw, ok := state.Get("blackboard.metrics"); !ok {
		t.Fatal("expected blackboard.metrics")
	} else if metrics, ok := raw.(blackboard.Metrics); !ok || metrics.ArtifactCount == 0 || metrics.VerifiedCount == 0 {
		t.Fatalf("unexpected blackboard.metrics: %#v", raw)
	}

	if raw, ok := state.Get("blackboard.controller"); !ok {
		t.Fatal("expected blackboard.controller")
	} else if controller, ok := raw.(blackboard.ControllerState); !ok || controller.Termination != "goal_satisfied" || !controller.GoalSatisfied {
		t.Fatalf("unexpected blackboard.controller: %#v", raw)
	}

	if raw, ok := state.Get("blackboard.controller.selected_spec"); !ok {
		t.Fatal("expected blackboard.controller.selected_spec")
	} else if spec, ok := raw.(blackboard.KnowledgeSourceSpec); !ok || spec.Name == "" || spec.Priority == 0 {
		t.Fatalf("unexpected blackboard.controller.selected_spec: %#v", raw)
	}

	if raw, ok := state.Get("blackboard.controller.execution_mode"); !ok {
		t.Fatal("expected blackboard.controller.execution_mode")
	} else if mode, ok := raw.(string); !ok || mode != "single_fire_serial" {
		t.Fatalf("unexpected blackboard.controller.execution_mode: %#v", raw)
	}

	if raw, ok := state.Get("blackboard.controller.selection_policy"); !ok {
		t.Fatal("expected blackboard.controller.selection_policy")
	} else if policy, ok := raw.(string); !ok || policy != "highest_priority_then_name" {
		t.Fatalf("unexpected blackboard.controller.selection_policy: %#v", raw)
	}

	if raw, ok := state.Get("blackboard.controller.merge_policy"); !ok {
		t.Fatal("expected blackboard.controller.merge_policy")
	} else if policy, ok := raw.(string); !ok || policy != "reject_conflicts" {
		t.Fatalf("unexpected blackboard.controller.merge_policy: %#v", raw)
	}

	if raw, ok := state.Get("blackboard.controller.contenders"); !ok {
		t.Fatal("expected blackboard.controller.contenders")
	} else if contenders, ok := raw.([]blackboard.KnowledgeSourceSpec); !ok || len(contenders) == 0 {
		t.Fatalf("unexpected blackboard.controller.contenders: %#v", raw)
	}

	if raw, ok := state.GetKnowledge("blackboard.summary"); !ok {
		t.Fatal("expected blackboard.summary knowledge")
	} else if summary, ok := raw.(string); !ok || summary == "" {
		t.Fatalf("unexpected blackboard.summary knowledge: %#v", raw)
	}
	if raw, ok := state.Get("blackboard.summary_ref"); ok {
		ref, ok := raw.(core.ArtifactReference)
		if !ok || ref.ArtifactID == "" {
			t.Fatalf("unexpected blackboard.summary_ref: %#v", raw)
		}
	}

	if raw, ok := state.Get("blackboard.execution_summary"); !ok {
		t.Fatal("expected blackboard.execution_summary")
	} else if summary, ok := raw.(map[string]any); !ok || summary["termination"] != "goal_satisfied" {
		t.Fatalf("unexpected blackboard.execution_summary: %#v", raw)
	}

	if raw, ok := state.Get("blackboard.audit"); !ok {
		t.Fatal("expected blackboard.audit")
	} else if _, hasRef := state.Get("blackboard.summary_ref"); hasRef {
		if audit, ok := raw.(map[string]any); !ok || audit["entry_count"] == nil {
			t.Fatalf("unexpected compact blackboard.audit: %#v", raw)
		}
	} else if entries, ok := raw.([]map[string]any); !ok || len(entries) == 0 {
		t.Fatalf("unexpected blackboard.audit: %#v", raw)
	}
}

func TestBlackboardAgent_ExecuteHydratesFromNamespacedContextState(t *testing.T) {
	a := &blackboard.BlackboardAgent{
		Config:    &core.Config{},
		Sources:   []blackboard.KnowledgeSource{&blackboard.VerifierKS{}},
		MaxCycles: 5,
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	state := core.NewContext()
	state.Set("blackboard.goals", []string{"verify carried artifact"})
	state.Set("blackboard.artifacts", []blackboard.Artifact{{
		ID:      "artifact-1",
		Kind:    "summary",
		Content: "carry-forward",
		Source:  "seed",
	}})

	result, err := a.Execute(context.Background(), &core.Task{Instruction: "ignored"}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	raw, ok := state.Get("blackboard.artifacts")
	if !ok {
		t.Fatal("expected blackboard.artifacts")
	}
	artifacts, ok := raw.([]blackboard.Artifact)
	if !ok || len(artifacts) != 1 || !artifacts[0].Verified {
		t.Fatalf("unexpected blackboard.artifacts: %#v", raw)
	}
}

func TestBlackboard_ValidateRejectsDuplicateArtifactID(t *testing.T) {
	bb := blackboard.NewBlackboard("goal")
	if err := bb.AddArtifact("dup", "summary", "first", "test"); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	if err := bb.AddArtifact("dup", "summary", "second", "test"); err == nil {
		t.Fatal("expected duplicate artifact error")
	}
}

func TestBlackboard_CompleteActionRemovesPendingRequest(t *testing.T) {
	bb := blackboard.NewBlackboard("goal")
	if err := bb.EnqueueAction(blackboard.ActionRequest{
		ID:          "action-1",
		ToolOrAgent: "react",
		Description: "do something",
	}); err != nil {
		t.Fatalf("EnqueueAction: %v", err)
	}
	if err := bb.CompleteAction(blackboard.ActionResult{
		RequestID: "action-1",
		Success:   true,
		Output:    "ok",
	}); err != nil {
		t.Fatalf("CompleteAction: %v", err)
	}
	if bb.HasPendingAction("action-1") {
		t.Fatal("expected pending action removed")
	}
	if !bb.HasCompletedAction("action-1") {
		t.Fatal("expected completed action recorded")
	}
}

func TestBlackboard_AddIssueRejectsInvalidSeverity(t *testing.T) {
	bb := blackboard.NewBlackboard("goal")
	if err := bb.AddIssue("issue-1", "desc", "urgent", "test"); err == nil {
		t.Fatal("expected invalid severity error")
	}
}

func TestPlannerKS_UsesPlannerCapabilityWhenAvailable(t *testing.T) {
	registry := capability.NewRegistry()
	planner := &stubInvocableCapability{
		desc: capabilityDescriptor("planner.plan"),
		invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			return &core.CapabilityExecutionResult{
				Success: true,
				Data: map[string]interface{}{
					"steps": []any{
						map[string]any{
							"id":          "step-1",
							"description": "delegate to react",
							"tool":        "agent:react",
							"params":      map[string]any{"instruction": "delegate to react"},
						},
					},
				},
			}, nil
		},
	}
	if err := registry.RegisterInvocableCapability(planner); err != nil {
		t.Fatalf("RegisterInvocableCapability: %v", err)
	}
	ks := &blackboard.PlannerKS{}
	bb := blackboard.NewBlackboard("goal")
	if err := bb.AddIssue("issue-1", "fix bug", "medium", "test"); err != nil {
		t.Fatalf("AddIssue: %v", err)
	}
	if err := ks.Execute(context.Background(), bb, registry, nil, core.AgentSemanticContext{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(bb.PendingActions) != 1 {
		t.Fatalf("expected one pending action, got %#v", bb.PendingActions)
	}
	if bb.PendingActions[0].ToolOrAgent != "agent:react" {
		t.Fatalf("unexpected action target: %#v", bb.PendingActions[0])
	}
}

func TestExecutorKS_InvokesExplicitAgentCapability(t *testing.T) {
	registry := capability.NewRegistry()
	var called int
	agent := &stubInvocableCapability{
		desc: capabilityDescriptor("agent:react"),
		invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			called++
			return &core.CapabilityExecutionResult{
				Success: true,
				Data: map[string]interface{}{
					"summary": "react agent completed delegated task",
				},
			}, nil
		},
	}
	if err := registry.RegisterInvocableCapability(agent); err != nil {
		t.Fatalf("RegisterInvocableCapability: %v", err)
	}
	ks := &blackboard.ExecutorKS{}
	bb := blackboard.NewBlackboard("goal")
	if err := bb.EnqueueAction(blackboard.ActionRequest{
		ID:          "action-1",
		ToolOrAgent: "agent:react",
		Args:        map[string]any{"instruction": "do work"},
		Description: "do work",
	}); err != nil {
		t.Fatalf("EnqueueAction: %v", err)
	}
	if err := ks.Execute(context.Background(), bb, registry, nil, core.AgentSemanticContext{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected capability invocation, got %d", called)
	}
	if len(bb.CompletedActions) != 1 || !bb.CompletedActions[0].Success {
		t.Fatalf("unexpected completed actions: %#v", bb.CompletedActions)
	}
	if len(bb.Artifacts) != 1 || bb.Artifacts[0].Content != "react agent completed delegated task" {
		t.Fatalf("unexpected artifacts: %#v", bb.Artifacts)
	}
}

func TestVerifierKS_UsesVerifierCapabilityWhenAvailable(t *testing.T) {
	registry := capability.NewRegistry()
	verifier := &stubInvocableCapability{
		desc: capabilityDescriptor("verifier.verify"),
		invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			return &core.CapabilityExecutionResult{
				Success: true,
				Data: map[string]interface{}{
					"verified": true,
					"summary":  "verified",
				},
			}, nil
		},
	}
	if err := registry.RegisterInvocableCapability(verifier); err != nil {
		t.Fatalf("RegisterInvocableCapability: %v", err)
	}
	ks := &blackboard.VerifierKS{}
	bb := blackboard.NewBlackboard("goal")
	if err := bb.AddArtifact("a1", "summary", "content", "Executor"); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	if err := ks.Execute(context.Background(), bb, registry, nil, core.AgentSemanticContext{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !bb.Artifacts[0].Verified {
		t.Fatalf("expected verified artifact: %#v", bb.Artifacts)
	}
}

func TestReviewKS_UsesReviewerCapabilityWhenArtifactsNeedReview(t *testing.T) {
	registry := capability.NewRegistry()
	reviewer := &stubInvocableCapability{
		desc: capabilityDescriptor("reviewer.review"),
		invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			return &core.CapabilityExecutionResult{
				Success: true,
				Data: map[string]interface{}{
					"findings": []any{
						map[string]any{"severity": "medium", "description": "artifact misses changelog note"},
					},
				},
			}, nil
		},
	}
	if err := registry.RegisterInvocableCapability(reviewer); err != nil {
		t.Fatalf("RegisterInvocableCapability: %v", err)
	}
	ks := &blackboard.ReviewKS{}
	bb := blackboard.NewBlackboard("goal")
	if err := bb.AddArtifact("a1", "patch", "content", "Executor"); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	bb.VerifyAllArtifacts()
	if err := ks.Execute(context.Background(), bb, registry, nil, core.AgentSemanticContext{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(bb.Issues) != 1 || bb.Issues[0].Description != "artifact misses changelog note" {
		t.Fatalf("unexpected issues: %#v", bb.Issues)
	}
}

func TestFailureTriageKS_EnqueuesRecoveryActionForFailedExecution(t *testing.T) {
	ks := &blackboard.FailureTriageKS{}
	bb := blackboard.NewBlackboard("goal")
	if err := bb.CompleteAction(blackboard.ActionResult{
		RequestID: "action-1",
		Success:   false,
		Error:     "tests failed",
	}); err != nil {
		t.Fatalf("CompleteAction: %v", err)
	}
	if err := ks.Execute(context.Background(), bb, nil, nil, core.AgentSemanticContext{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(bb.Issues) != 1 {
		t.Fatalf("expected one triage issue, got %#v", bb.Issues)
	}
	if len(bb.PendingActions) != 1 || bb.PendingActions[0].ID != "retry-action-1" {
		t.Fatalf("unexpected pending actions: %#v", bb.PendingActions)
	}
}

func TestSummarizerKS_UsesSummarizerCapabilityWhenAvailable(t *testing.T) {
	registry := capability.NewRegistry()
	summarizer := &stubInvocableCapability{
		desc: capabilityDescriptor("summarizer.summarize"),
		invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			return &core.CapabilityExecutionResult{
				Success: true,
				Data: map[string]interface{}{
					"summary": "final verified summary",
				},
			}, nil
		},
	}
	if err := registry.RegisterInvocableCapability(summarizer); err != nil {
		t.Fatalf("RegisterInvocableCapability: %v", err)
	}
	ks := &blackboard.SummarizerKS{}
	bb := blackboard.NewBlackboard("goal")
	if err := bb.AddArtifact("a1", "patch", "content", "Executor"); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	bb.VerifyAllArtifacts()
	if err := bb.CompleteAction(blackboard.ActionResult{
		RequestID: "action-1",
		Success:   true,
		Output:    "ok",
	}); err != nil {
		t.Fatalf("CompleteAction: %v", err)
	}
	if err := ks.Execute(context.Background(), bb, registry, nil, core.AgentSemanticContext{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !bb.HasArtifact("blackboard-summary") {
		t.Fatalf("expected summary artifact: %#v", bb.Artifacts)
	}
	if bb.Artifacts[len(bb.Artifacts)-1].Content != "final verified summary" {
		t.Fatalf("unexpected summary artifact: %#v", bb.Artifacts[len(bb.Artifacts)-1])
	}
}

func TestDefaultKnowledgeSources_IncludesExpandedBuiltins(t *testing.T) {
	sources := blackboard.DefaultKnowledgeSources()
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, source.Name())
	}
	expected := []string{"Explorer", "Analyzer", "Planner", "Review", "Executor", "FailureTriage", "Verifier", "Summarizer"}
	if len(names) != len(expected) {
		t.Fatalf("unexpected source count: got %v want %v", names, expected)
	}
	for idx, name := range expected {
		if names[idx] != name {
			t.Fatalf("unexpected source order: got %v want %v", names, expected)
		}
	}
}

func TestMergeBlackboardBranches_MergesNonConflictingState(t *testing.T) {
	parent := core.NewContext()
	parent.Set("task.instruction", "merge branches")

	branchA := parent.Clone()
	branchA.Set("blackboard.facts", []blackboard.Fact{{Key: "a", Value: "one", Source: "A"}})
	branchB := parent.Clone()
	branchB.Set("blackboard.issues", []blackboard.Issue{{ID: "i1", Description: "issue", Severity: "low", Source: "B"}})

	err := blackboard.MergeBlackboardBranches(parent, []blackboard.BlackboardBranchResult{
		{SourceName: "A", State: branchA, Delta: branchA.BranchDelta()},
		{SourceName: "B", State: branchB, Delta: branchB.BranchDelta()},
	})
	if err != nil {
		t.Fatalf("MergeBlackboardBranches: %v", err)
	}
	if _, ok := parent.Get("blackboard.facts"); !ok {
		t.Fatal("expected merged blackboard.facts")
	}
	if _, ok := parent.Get("blackboard.issues"); !ok {
		t.Fatal("expected merged blackboard.issues")
	}
}

func TestMergeBlackboardBranches_RejectsConflictingWrites(t *testing.T) {
	parent := core.NewContext()
	branchA := parent.Clone()
	branchA.Set("blackboard.summary", "summary a")
	branchB := parent.Clone()
	branchB.Set("blackboard.summary", "summary b")

	err := blackboard.MergeBlackboardBranches(parent, []blackboard.BlackboardBranchResult{
		{SourceName: "A", State: branchA, Delta: branchA.BranchDelta()},
		{SourceName: "B", State: branchB, Delta: branchB.BranchDelta()},
	})
	if err == nil {
		t.Fatal("expected merge conflict")
	}
}

func TestResolveKnowledgeSource_DefaultSpecUsesLegacyInterfaceValues(t *testing.T) {
	source := &recordingKS{
		name:        "plain",
		priority:    42,
		canActivate: func(_ *blackboard.Blackboard) bool { return false },
		exec:        func(_ *blackboard.Blackboard) error { return nil },
	}
	resolved := blackboard.ResolveKnowledgeSource(source)
	if resolved.Spec.Name != "plain" {
		t.Fatalf("unexpected spec name: %#v", resolved.Spec)
	}
	if resolved.Spec.Priority != 42 {
		t.Fatalf("unexpected spec priority: %#v", resolved.Spec)
	}
	if resolved.Contract.SideEffectClass != graph.SideEffectContext {
		t.Fatalf("unexpected contract: %#v", resolved.Contract)
	}
}

func TestResolveKnowledgeSource_UsesExplicitMetadataAndContract(t *testing.T) {
	source := &specRecordingKS{
		recordingKS: recordingKS{
			name:        "legacy-name",
			priority:    10,
			canActivate: func(_ *blackboard.Blackboard) bool { return false },
			exec:        func(_ *blackboard.Blackboard) error { return nil },
		},
		spec: blackboard.KnowledgeSourceSpec{
			Name:     "explicit-name",
			Priority: 77,
			RequiredCapabilities: []core.CapabilitySelector{{
				Name: "file_read",
				Kind: core.CapabilityKindTool,
			}},
			Contract: graph.NodeContract{
				RequiredCapabilities: []core.CapabilitySelector{{
					Name: "file_read",
					Kind: core.CapabilityKindTool,
				}},
				SideEffectClass: graph.SideEffectLocal,
				Idempotency:     graph.IdempotencyReplaySafe,
			},
		},
	}
	resolved := blackboard.ResolveKnowledgeSource(source)
	if resolved.Spec.Name != "explicit-name" || resolved.Spec.Priority != 77 {
		t.Fatalf("unexpected resolved spec: %#v", resolved.Spec)
	}
	if len(resolved.Contract.RequiredCapabilities) != 1 || resolved.Contract.SideEffectClass != graph.SideEffectLocal {
		t.Fatalf("unexpected resolved contract: %#v", resolved.Contract)
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
			if err := bb.AddArtifact("custom-artifact", "summary", "custom output", "custom"); err != nil {
				return err
			}
			bb.VerifyArtifact("custom-artifact")
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

func TestBlackboardAgent_ExecuteUsesExplicitCheckpointNodes(t *testing.T) {
	a := &blackboard.BlackboardAgent{
		Config: &core.Config{
			UseExplicitCheckpointNodes: boolPtr(true),
		},
		CheckpointPath: t.TempDir(),
		MaxCycles:      20,
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	task := &core.Task{
		ID:          "blackboard-explicit-checkpoint",
		Type:        core.TaskTypeCodeModification,
		Instruction: "finish the task",
	}
	state := core.NewContext()
	result, err := a.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if _, ok := state.Get("graph.checkpoint"); !ok {
		t.Fatal("expected graph.checkpoint state")
	}
	if _, ok := state.Get("graph.checkpoint_ref"); !ok {
		t.Fatal("expected graph.checkpoint_ref state")
	}
	if raw, ok := state.Get("blackboard.checkpoint_ref"); !ok {
		t.Fatal("expected blackboard.checkpoint_ref state")
	} else if ref, ok := raw.(core.ArtifactReference); !ok || ref.ArtifactID == "" {
		t.Fatalf("unexpected blackboard.checkpoint_ref: %#v", raw)
	}
	// checkpoint store
}

func TestBlackboardAgent_ExecuteCanUseLegacyCheckpointCallbackWhenExplicitNodesDisabled(t *testing.T) {
	a := &blackboard.BlackboardAgent{
		Config: &core.Config{
			UseExplicitCheckpointNodes: boolPtr(false),
		},
		CheckpointPath: t.TempDir(),
		MaxCycles:      20,
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	task := &core.Task{
		ID:          "blackboard-legacy-checkpoint",
		Type:        core.TaskTypeCodeModification,
		Instruction: "finish the task",
	}
	state := core.NewContext()
	result, err := a.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if _, ok := state.Get("graph.checkpoint"); ok {
		t.Fatal("did not expect explicit graph.checkpoint state")
	}
	// checkpoint store happens
}

func TestBlackboardAgent_ExecuteCanResumeFromLatestCheckpoint(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := &checkpointResumeKS{
		name:     "resume",
		priority: 100,
		cancel:   cancel,
	}
	a := &blackboard.BlackboardAgent{
		Config: &core.Config{
			UseExplicitCheckpointNodes: boolPtr(false),
		},
		Sources:        []blackboard.KnowledgeSource{source},
		CheckpointPath: t.TempDir(),
		MaxCycles:      5,
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	task := &core.Task{
		ID:          "blackboard-resume-latest",
		Type:        core.TaskTypeCodeModification,
		Instruction: "resume from checkpoint",
	}
	firstState := core.NewContext()
	if _, err := a.Execute(ctx, task, firstState); err == nil {
		t.Fatal("expected first run to stop on canceled context")
	}

	// checkpoint store happens

	resumeState := core.NewContext()
	resumeState.Set("blackboard.resume_latest", true)
	result, err := a.Execute(context.Background(), task, resumeState)
	if err != nil {
		t.Fatalf("resume Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected resumed success")
	}
	rawArtifacts, ok := resumeState.Get("blackboard.artifacts")
	if !ok {
		t.Fatal("expected blackboard.artifacts after resume")
	}
	artifacts, ok := rawArtifacts.([]blackboard.Artifact)
	if !ok || len(artifacts) != 1 || !artifacts[0].Verified {
		t.Fatalf("unexpected artifacts after resume: %#v", rawArtifacts)
	}
}

func TestBlackboardAgent_ExecuteHydratesFromRuntimeMemoryRetrieval(t *testing.T) {
	t.Skip("runtime-memory retrieval lane removed")
}

func TestBlackboardAgent_ExecutePersistsStructuredMemory(t *testing.T) {
	t.Skip("runtime-memory persistence lane removed")
}

func TestBlackboardAgent_ExecuteEmitsTelemetryForSelectionDispatchAndFinish(t *testing.T) {
	collector := &eventCollector{}
	a := &blackboard.BlackboardAgent{
		Config: &core.Config{
			Telemetry: collector,
		},
		MaxCycles: 20,
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	state := core.NewContext()
	task := &core.Task{
		ID:          "blackboard-telemetry",
		Type:        core.TaskTypeCodeModification,
		Instruction: "finish with telemetry",
	}
	result, err := a.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if !collector.hasEvent(core.EventAgentStart, "blackboard agent start") {
		t.Fatalf("missing agent start event: %#v", collector.events)
	}
	if !collector.hasEvent(core.EventStateChange, "blackboard knowledge source selected") {
		t.Fatalf("missing selection event: %#v", collector.events)
	}
	if !collector.hasEvent(core.EventCapabilityCall, "blackboard dispatch start") {
		t.Fatalf("missing dispatch start event: %#v", collector.events)
	}
	if !collector.hasEvent(core.EventCapabilityResult, "blackboard dispatch complete") {
		t.Fatalf("missing dispatch complete event: %#v", collector.events)
	}
	if !collector.hasEvent(core.EventAgentFinish, "blackboard agent finished") {
		t.Fatalf("missing agent finish event: %#v", collector.events)
	}
}

func TestBlackboardAgent_GraphPreflightRejectsMissingRequiredCapability(t *testing.T) {
	required := core.CapabilitySelector{Name: "tool:missing_capability", Kind: core.CapabilityKindTool}
	source := &specRecordingKS{
		recordingKS: recordingKS{
			name:        "requires-missing",
			priority:    100,
			canActivate: func(_ *blackboard.Blackboard) bool { return true },
			exec:        func(_ *blackboard.Blackboard) error { return nil },
		},
		spec: blackboard.KnowledgeSourceSpec{
			Name:                 "requires-missing",
			Priority:             100,
			RequiredCapabilities: []core.CapabilitySelector{required},
			Contract: graph.NodeContract{
				RequiredCapabilities: []core.CapabilitySelector{required},
				SideEffectClass:      graph.SideEffectLocal,
				Idempotency:          graph.IdempotencyReplaySafe,
			},
		},
	}
	registry := capability.NewRegistry()
	if err := registry.RegisterInvocableCapability(&stubInvocableCapability{
		desc: capabilityDescriptor("tool:present"),
		invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			return &core.CapabilityExecutionResult{Success: true, Data: map[string]interface{}{}}, nil
		},
	}); err != nil {
		t.Fatalf("RegisterInvocableCapability: %v", err)
	}
	a := &blackboard.BlackboardAgent{
		Config:  &core.Config{},
		Tools:   registry,
		Sources: []blackboard.KnowledgeSource{source},
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	g, err := a.BuildGraph(&core.Task{ID: "blackboard-preflight", Instruction: "preflight"})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	report, err := g.Preflight()
	if err == nil {
		t.Fatal("expected preflight error for missing capability")
	}
	if report == nil || !report.HasBlockingIssues() {
		t.Fatalf("expected blocking preflight report: %#v", report)
	}
}

func TestBlackboardAgent_ExecuteDefaultBuiltinsThroughCapabilityPath(t *testing.T) {
	registry := capability.NewRegistry()
	var delegated int
	for _, handler := range []*stubInvocableCapability{
		{
			desc: capabilityDescriptor("reviewer.review"),
			invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
				mode := ""
				if args != nil {
					mode = stateString(args["mode"])
				}
				if mode == "artifact_review" {
					return &core.CapabilityExecutionResult{Success: true, Data: map[string]interface{}{"findings": []any{}}}, nil
				}
				return &core.CapabilityExecutionResult{
					Success: true,
					Data: map[string]interface{}{"findings": []any{
						map[string]any{"severity": "medium", "description": "need a code change"},
					}},
				}, nil
			},
		},
		{
			desc: capabilityDescriptor("planner.plan"),
			invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
				return &core.CapabilityExecutionResult{
					Success: true,
					Data: map[string]interface{}{
						"steps": []any{
							map[string]any{
								"id":          "step-1",
								"description": "delegate implementation",
								"tool":        "agent:react",
								"params":      map[string]any{"instruction": "implement requested fix"},
							},
						},
					},
				}, nil
			},
		},
		{
			desc: capabilityDescriptor("agent:react"),
			invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
				delegated++
				return &core.CapabilityExecutionResult{
					Success: true,
					Data:    map[string]interface{}{"summary": "delegated fix applied"},
				}, nil
			},
		},
		{
			desc: capabilityDescriptor("verifier.verify"),
			invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
				return &core.CapabilityExecutionResult{
					Success: true,
					Data:    map[string]interface{}{"verified": true},
				}, nil
			},
		},
		{
			desc: capabilityDescriptor("summarizer.summarize"),
			invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
				return &core.CapabilityExecutionResult{
					Success: true,
					Data:    map[string]interface{}{"summary": "final summary from summarizer"},
				}, nil
			},
		},
	} {
		if err := registry.RegisterInvocableCapability(handler); err != nil {
			t.Fatalf("RegisterInvocableCapability: %v", err)
		}
	}

	a := &blackboard.BlackboardAgent{
		Config:    &core.Config{},
		Tools:     registry,
		MaxCycles: 20,
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	state := core.NewContext()
	task := &core.Task{ID: "blackboard-builtins-e2e", Instruction: "fix the bug with builtins"}
	result, err := a.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if delegated != 1 {
		t.Fatalf("expected one delegated execution, got %d", delegated)
	}
	rawArtifacts, ok := state.Get("blackboard.artifacts")
	if !ok {
		t.Fatal("expected blackboard.artifacts")
	}
	artifacts, ok := rawArtifacts.([]blackboard.Artifact)
	if !ok || len(artifacts) == 0 {
		t.Fatalf("unexpected artifacts: %#v", rawArtifacts)
	}
	if !hasArtifactContent(artifacts, "delegated fix applied") {
		t.Fatalf("expected delegated artifact: %#v", artifacts)
	}
}

func TestBlackboardAgent_ResumeCheckpointDoesNotReplayCompletedDelegatedAction(t *testing.T) {
	registry := capability.NewRegistry()
	var delegated int
	cancelOnDelegate := true
	var cancel context.CancelFunc
	agent := &stubInvocableCapability{
		desc: capabilityDescriptor("agent:react"),
		invoke: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
			delegated++
			if cancelOnDelegate {
				cancelOnDelegate = false
				cancel()
			}
			return &core.CapabilityExecutionResult{
				Success: true,
				Data:    map[string]interface{}{"summary": "delegated once"},
			}, nil
		},
	}
	if err := registry.RegisterInvocableCapability(agent); err != nil {
		t.Fatalf("RegisterInvocableCapability: %v", err)
	}
	source := &recordingKS{
		name:     "resume-once",
		priority: 100,
		canActivate: func(bb *blackboard.Blackboard) bool {
			return !bb.IsGoalSatisfied() && !bb.HasPendingAction("action-1") && !bb.HasCompletedAction("action-1")
		},
		exec: func(bb *blackboard.Blackboard) error {
			return bb.EnqueueAction(blackboard.ActionRequest{
				ID:          "action-1",
				ToolOrAgent: "agent:react",
				Description: "delegate once",
				Args:        map[string]any{},
			})
		},
	}
	verify := &recordingKS{
		name:     "verify",
		priority: 90,
		canActivate: func(bb *blackboard.Blackboard) bool {
			return len(bb.Artifacts) > 0 && bb.HasUnverifiedArtifacts()
		},
		exec: func(bb *blackboard.Blackboard) error {
			bb.VerifyAllArtifacts()
			return nil
		},
	}
	checkpointDir := t.TempDir()
	a := &blackboard.BlackboardAgent{
		Config: &core.Config{
			UseExplicitCheckpointNodes: boolPtr(false),
		},
		Tools:          registry,
		Sources:        []blackboard.KnowledgeSource{source, &blackboard.ExecutorKS{}, verify},
		CheckpointPath: checkpointDir,
		MaxCycles:      5,
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	task := &core.Task{ID: "blackboard-no-replay", Instruction: "resume without replay"}
	state := core.NewContext()
	if _, err := a.Execute(ctx, task, state); err == nil {
		t.Fatal("expected interrupted first run")
	}
	if delegated != 1 {
		t.Fatalf("expected one delegated call before resume, got %d", delegated)
	}

	// checkpoint store trigger

	// use BKC for recovery
}

// --- helpers ----------------------------------------------------------------

// recordingKS is a test-double KnowledgeSource.
type recordingKS struct {
	name        string
	priority    int
	canActivate func(*blackboard.Blackboard) bool
	exec        func(*blackboard.Blackboard) error
}

func (r *recordingKS) Name() string                               { return r.name }
func (r *recordingKS) Priority() int                              { return r.priority }
func (r *recordingKS) CanActivate(bb *blackboard.Blackboard) bool { return r.canActivate(bb) }
func (r *recordingKS) Execute(_ context.Context, bb *blackboard.Blackboard, _ *capability.Registry, _ core.LanguageModel, _ core.AgentSemanticContext) error {
	return r.exec(bb)
}

type checkpointResumeKS struct {
	name     string
	priority int
	cancel   context.CancelFunc
}

func (k *checkpointResumeKS) Name() string                              { return k.name }
func (k *checkpointResumeKS) Priority() int                             { return k.priority }
func (k *checkpointResumeKS) CanActivate(_ *blackboard.Blackboard) bool { return true }
func (k *checkpointResumeKS) Execute(_ context.Context, bb *blackboard.Blackboard, _ *capability.Registry, _ core.LanguageModel, _ core.AgentSemanticContext) error {
	hasResumeFact := false
	for _, fact := range bb.Facts {
		if fact.Key == "resume-phase" {
			hasResumeFact = true
			break
		}
	}
	if !hasResumeFact {
		bb.AddFact("resume-phase", "checkpoint-created", "checkpointResumeKS")
		if k.cancel != nil {
			k.cancel()
			k.cancel = nil
		}
		return nil
	}
	if err := bb.AddArtifact("resume-artifact", "summary", "resumed successfully", "checkpointResumeKS"); err != nil {
		return err
	}
	bb.VerifyArtifact("resume-artifact")
	return nil
}

type specRecordingKS struct {
	recordingKS
	spec blackboard.KnowledgeSourceSpec
}

func (s *specRecordingKS) KnowledgeSourceSpec() blackboard.KnowledgeSourceSpec { return s.spec }

type stubInvocableCapability struct {
	desc   core.CapabilityDescriptor
	invoke func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error)
}

func (s *stubInvocableCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return s.desc
}

func (s *stubInvocableCapability) Invoke(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	return s.invoke(ctx, state, args)
}

func capabilityDescriptor(name string) core.CapabilityDescriptor {
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:            name,
		Name:          name,
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeBuiltin,
		},
		TrustClass:   core.TrustClassBuiltinTrusted,
		Availability: core.AvailabilitySpec{Available: true},
	})
}

func boolPtr(v bool) *bool { return &v }

func stateString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func hasArtifactContent(artifacts []blackboard.Artifact, content string) bool {
	for _, artifact := range artifacts {
		if artifact.Content == content {
			return true
		}
	}
	return false
}

type eventCollector struct {
	events []core.Event
}

func (c *eventCollector) Emit(event core.Event) {
	c.events = append(c.events, event)
}

func (c *eventCollector) hasEvent(eventType core.EventType, message string) bool {
	for _, event := range c.events {
		if event.Type == eventType && event.Message == message {
			return true
		}
	}
	return false
}
