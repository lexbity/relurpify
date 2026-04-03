package local

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type stubVerificationPlanner struct {
	plan agentenv.VerificationPlan
	ok   bool
	err  error
	req  agentenv.VerificationPlanRequest
}

func (s *stubVerificationPlanner) SelectVerificationPlan(_ context.Context, req agentenv.VerificationPlanRequest) (agentenv.VerificationPlan, bool, error) {
	s.req = req
	return s.plan, s.ok, s.err
}

func TestBuildVerificationPlan_UsesExplicitCommands(t *testing.T) {
	planner := &stubVerificationPlanner{
		ok: true,
		plan: agentenv.VerificationPlan{
			ScopeKind: "external",
			Commands: []agentenv.VerificationCommand{{
				Name:             "go_test_external",
				Command:          "go",
				Args:             []string{"test", "./..."},
				WorkingDirectory: ".",
			}},
			Source: "framework_plan",
		},
	}
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			Instruction: "verify this change",
			Context: map[string]any{
				"workspace":             ".",
				"verification_commands": []string{"sh -c true"},
			},
		},
		Environment: agentenv.AgentEnvironment{VerificationPlanner: planner},
	}

	plan := buildVerificationPlan(env)
	if plan.ScopeKind != "explicit" {
		t.Fatalf("expected explicit scope, got %q", plan.ScopeKind)
	}
	if len(plan.Commands) != 1 {
		t.Fatalf("expected one command, got %d", len(plan.Commands))
	}
	if plan.Commands[0].Command != "sh" {
		t.Fatalf("expected sh command, got %q", plan.Commands[0].Command)
	}
	if planner.req.TaskInstruction != "" {
		t.Fatalf("expected explicit commands to bypass external planner, got request %#v", planner.req)
	}
}

func TestBuildVerificationPlan_UsesGoPackageTargetsFromFiles(t *testing.T) {
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			Instruction: "verify this Go change",
			Context:     map[string]any{"workspace": "."},
		},
		State: core.NewContext(),
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{{
		ID:         "edit",
		Kind:       euclotypes.ArtifactKindEditIntent,
		Summary:    "updated Go files",
		Payload:    map[string]any{"files": []string{"named/euclo/runtime/verification.go", "named/euclo/runtime/assurance/assurance.go"}},
		ProducerID: "test",
		Status:     "produced",
	}})

	plan := buildVerificationPlan(env)
	if plan.ScopeKind != "package_tests" {
		t.Fatalf("expected package_tests scope, got %q", plan.ScopeKind)
	}
	if len(plan.Commands) != 2 {
		t.Fatalf("expected two package commands, got %d", len(plan.Commands))
	}
	if plan.Commands[0].Args[1] != "./named/euclo/runtime" {
		t.Fatalf("expected runtime package target, got %#v", plan.Commands[0].Args)
	}
	if plan.Commands[1].Args[1] != "./named/euclo/runtime/assurance" {
		t.Fatalf("expected session package target, got %#v", plan.Commands[1].Args)
	}
}

func TestBuildVerificationPlan_CarriesResolvedPolicyHints(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.resolved_execution_policy", map[string]any{
		"preferred_verify_capabilities":     []string{"go_test", "go_build"},
		"verification_success_capabilities": []string{"go_test"},
		"require_verification_step":         true,
	})
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			Instruction: "verify this Go change",
			Context:     map[string]any{"workspace": "."},
		},
		State: state,
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{{
		ID:         "edit",
		Kind:       euclotypes.ArtifactKindEditIntent,
		Summary:    "updated Go files",
		Payload:    map[string]any{"files": []string{"named/euclo/runtime/verification.go"}},
		ProducerID: "test",
		Status:     "produced",
	}})

	plan := buildVerificationPlan(env)
	if plan.Source != "skill_policy+heuristic_go" {
		t.Fatalf("expected skill-policy-aware source, got %q", plan.Source)
	}
	if !plan.PolicyRequiresVerification {
		t.Fatal("expected policy_requires_verification to be true")
	}
	if len(plan.PolicyPreferredCapabilities) != 2 {
		t.Fatalf("expected preferred capabilities, got %#v", plan.PolicyPreferredCapabilities)
	}
	if len(plan.SelectionInputs) < 3 {
		t.Fatalf("expected policy-aware selection inputs, got %#v", plan.SelectionInputs)
	}
}

func TestBuildVerificationPlan_PrefersExternalResolver(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.resolved_execution_policy", map[string]any{
		"preferred_verify_capabilities": []string{"go_test"},
	})
	planner := &stubVerificationPlanner{
		ok: true,
		plan: agentenv.VerificationPlan{
			ScopeKind: "external_package_tests",
			Files:     []string{"pkg/foo.go"},
			TestFiles: []string{"pkg/foo_test.go"},
			Commands: []agentenv.VerificationCommand{{
				Name:             "go_test_external",
				Command:          "go",
				Args:             []string{"test", "./pkg"},
				WorkingDirectory: ".",
			}},
			Source:                 "framework_skill",
			PlannerID:              "framework.skill.go",
			Rationale:              "Use package tests plus explicit test file context",
			AuditTrail:             []string{"skill_policy", "changed_files", "test_files"},
			CompatibilitySensitive: true,
		},
	}
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			Instruction: "verify this Go change",
			Context:     map[string]any{"workspace": "."},
		},
		State: state,
		Environment: agentenv.AgentEnvironment{
			VerificationPlanner: planner,
		},
		Mode:    euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"},
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{{
		ID:         "edit",
		Kind:       euclotypes.ArtifactKindEditIntent,
		Summary:    "updated Go files",
		Payload:    map[string]any{"files": []string{"pkg/foo.go", "pkg/foo_test.go"}},
		ProducerID: "test",
		Status:     "produced",
	}})

	plan := buildVerificationPlanWithContext(context.Background(), env)
	if plan.ScopeKind != "external_package_tests" {
		t.Fatalf("expected external scope, got %q", plan.ScopeKind)
	}
	if len(plan.Commands) != 1 || plan.Commands[0].Name != "go_test_external" {
		t.Fatalf("expected external commands, got %#v", plan.Commands)
	}
	if plan.Source != "skill_policy+external:framework_skill" {
		t.Fatalf("expected external source, got %q", plan.Source)
	}
	if len(plan.SelectionInputs) == 0 || plan.SelectionInputs[0] != "external_resolver" {
		t.Fatalf("expected external selection input, got %#v", plan.SelectionInputs)
	}
	if plan.PlannerID != "framework.skill.go" {
		t.Fatalf("expected planner id, got %q", plan.PlannerID)
	}
	if plan.Rationale == "" {
		t.Fatal("expected rationale to be preserved")
	}
	if !plan.CompatibilitySensitive {
		t.Fatal("expected compatibility_sensitive to be preserved")
	}
	if len(plan.TestFiles) != 1 || plan.TestFiles[0] != "pkg/foo_test.go" {
		t.Fatalf("expected test files, got %#v", plan.TestFiles)
	}
	if planner.req.ModeID != "code" || planner.req.ProfileID != "edit_verify_repair" {
		t.Fatalf("expected mode/profile in planner request, got %#v", planner.req)
	}
	if len(planner.req.TestFiles) != 1 || planner.req.TestFiles[0] != "pkg/foo_test.go" {
		t.Fatalf("expected test files in planner request, got %#v", planner.req.TestFiles)
	}
}

func TestVerificationExecuteCapability_ExecutesPlanAndStoresEvidence(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.verification_plan", verificationPlan{
		ScopeKind: "explicit",
		Files:     []string{"foo.go"},
		Commands: []verificationCommandSpec{{
			Name:             "shell_true",
			Command:          "sh",
			Args:             []string{"-c", "true"},
			WorkingDirectory: ".",
		}},
		Source: "test",
	})
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			Instruction: "verify this change",
			Context:     map[string]any{"workspace": "."},
		},
		State: state,
		RunID: "run-1",
	}

	result := (&verificationExecuteCapability{}).Execute(context.Background(), env)
	if result.Status != euclotypes.ExecutionStatusCompleted {
		t.Fatalf("expected completed status, got %q", result.Status)
	}
	raw, ok := state.Get("pipeline.verify")
	if !ok || raw == nil {
		t.Fatal("expected pipeline.verify to be stored")
	}
	record, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected structured verification payload, got %#v", raw)
	}
	if record["provenance"] != "executed" {
		t.Fatalf("expected executed provenance, got %#v", record["provenance"])
	}
}

func TestExecuteVerificationFlow_ExecutesScopeAndVerification(t *testing.T) {
	state := core.NewContext()
	mergeStateArtifactsToContext(state, []euclotypes.Artifact{{
		ID:         "intake",
		Kind:       euclotypes.ArtifactKindIntake,
		Summary:    "verification request",
		Payload:    map[string]any{"instruction": "verify this change"},
		ProducerID: "test",
		Status:     "produced",
	}})
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			Instruction: "verify this change",
			Context: map[string]any{
				"workspace":             ".",
				"verification_commands": []string{"sh -c true"},
			},
		},
		State: state,
		RunID: "run-flow",
	}

	artifacts, executed, err := ExecuteVerificationFlow(context.Background(), env, euclotypes.CapabilitySnapshot{
		HasExecuteTools:      true,
		HasVerificationTools: true,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !executed {
		t.Fatal("expected verification flow to execute")
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected scope and verification artifacts, got %d", len(artifacts))
	}
	if artifacts[0].Kind != euclotypes.ArtifactKindVerificationPlan {
		t.Fatalf("expected verification plan artifact, got %q", artifacts[0].Kind)
	}
	if artifacts[1].Kind != euclotypes.ArtifactKindVerification {
		t.Fatalf("expected verification artifact, got %q", artifacts[1].Kind)
	}
	raw, ok := state.Get("pipeline.verify")
	if !ok || raw == nil {
		t.Fatal("expected verification evidence to be stored")
	}
}

func TestExecuteVerificationFlow_NoPlanDoesNotExecute(t *testing.T) {
	state := core.NewContext()
	mergeStateArtifactsToContext(state, []euclotypes.Artifact{{
		ID:         "intake",
		Kind:       euclotypes.ArtifactKindIntake,
		Summary:    "verification request",
		Payload:    map[string]any{"instruction": "verify this change"},
		ProducerID: "test",
		Status:     "produced",
	}})
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			Instruction: "verify this change",
			Context:     map[string]any{"workspace": "."},
		},
		State: state,
		RunID: "run-no-plan",
	}

	artifacts, executed, err := ExecuteVerificationFlow(context.Background(), env, euclotypes.CapabilitySnapshot{
		HasExecuteTools:      true,
		HasVerificationTools: true,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if executed {
		t.Fatal("expected verification flow not to execute")
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected only scope artifact, got %d", len(artifacts))
	}
	if artifacts[0].Kind != euclotypes.ArtifactKindVerificationPlan {
		t.Fatalf("expected verification plan artifact, got %q", artifacts[0].Kind)
	}
	if _, ok := state.Get("pipeline.verify"); ok {
		t.Fatal("expected no verification evidence to be stored")
	}
}

func TestExecuteVerificationFlow_DoesNotRequireMaterializedIntakeArtifactForInternalCall(t *testing.T) {
	state := core.NewContext()
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			Instruction: "verify this change",
			Context: map[string]any{
				"workspace":             ".",
				"verification_commands": []string{"sh -c true"},
			},
		},
		State: state,
		RunID: "run-internal-no-intake",
	}

	artifacts, executed, err := ExecuteVerificationFlow(context.Background(), env, euclotypes.CapabilitySnapshot{
		HasExecuteTools:      true,
		HasVerificationTools: true,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !executed {
		t.Fatal("expected verification flow to execute without materialized intake artifact")
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected scope and verification artifacts, got %d", len(artifacts))
	}
	if _, ok := state.Get("euclo.verification_plan"); !ok {
		t.Fatal("expected verification plan to be stored")
	}
	if _, ok := state.Get("pipeline.verify"); !ok {
		t.Fatal("expected verification evidence to be stored")
	}
}
