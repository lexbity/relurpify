package reporting

import (
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func TestBuildChatCapabilityRuntimeState_CapturesVerificationPlan(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.verification_plan", map[string]any{
		"scope_kind":                    "package_tests",
		"source":                        "skill_policy+heuristic_go",
		"files":                         []string{"named/euclo/runtime/verification.go"},
		"test_files":                    []string{"named/euclo/runtime/verification_test.go"},
		"planner_id":                    "framework.skill.go",
		"rationale":                     "Use package tests for targeted verification",
		"audit_trail":                   []string{"skill_policy", "changed_files"},
		"compatibility_sensitive":       true,
		"selection_inputs":              []string{"heuristic_go", "changed_files", "policy_preferred_verify_capabilities"},
		"policy_preferred_capabilities": []string{"go_test"},
		"policy_requires_verification":  true,
		"commands": []any{
			map[string]any{"name": "go_test_runtime", "command": "go"},
		},
	})
	state.Set("pipeline.verify", map[string]any{
		"status": "pass",
		"checks": []any{map[string]any{"name": "go_test_runtime"}},
	})

	rt := BuildChatCapabilityRuntimeState(eucloruntime.UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatImplement,
	}, state, time.Now().UTC())

	if rt.VerificationPlanScope != "package_tests" {
		t.Fatalf("expected plan scope, got %q", rt.VerificationPlanScope)
	}
	if rt.VerificationPlanSource != "skill_policy+heuristic_go" {
		t.Fatalf("expected plan source, got %q", rt.VerificationPlanSource)
	}
	if len(rt.VerificationPlanCommands) != 1 || rt.VerificationPlanCommands[0] != "go_test_runtime" {
		t.Fatalf("expected plan commands, got %#v", rt.VerificationPlanCommands)
	}
	if len(rt.VerificationPlanFiles) != 1 || rt.VerificationPlanFiles[0] != "named/euclo/runtime/verification.go" {
		t.Fatalf("expected plan files, got %#v", rt.VerificationPlanFiles)
	}
	if len(rt.VerificationPlanTestFiles) != 1 || rt.VerificationPlanTestFiles[0] != "named/euclo/runtime/verification_test.go" {
		t.Fatalf("expected plan test files, got %#v", rt.VerificationPlanTestFiles)
	}
	if rt.VerificationPlanPlannerID != "framework.skill.go" {
		t.Fatalf("expected planner id, got %q", rt.VerificationPlanPlannerID)
	}
	if rt.VerificationPlanRationale == "" {
		t.Fatal("expected plan rationale")
	}
	if !rt.VerificationPlanCompatibilitySensitive {
		t.Fatal("expected compatibility-sensitive plan")
	}
	if len(rt.VerificationPlanPolicyPreferences) != 1 || rt.VerificationPlanPolicyPreferences[0] != "go_test" {
		t.Fatalf("expected policy preferences, got %#v", rt.VerificationPlanPolicyPreferences)
	}
	if !rt.VerificationPlanPolicyRequiresVerification {
		t.Fatal("expected policy_requires_verification to be true")
	}
}

func TestBuildDebugCapabilityRuntimeState_CapturesVerificationPlan(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.verification_plan", map[string]any{
		"scope_kind":                  "explicit",
		"source":                      "skill_policy+task_context",
		"files":                       []string{"foo.go"},
		"planner_id":                  "framework.manual",
		"rationale":                   "operator supplied explicit verification commands",
		"audit_trail":                 []string{"task_context"},
		"selection_inputs":            []string{"task_context", "policy_success_capabilities"},
		"policy_success_capabilities": []string{"go_test"},
		"commands": []any{
			map[string]any{"name": "sh", "command": "sh"},
		},
	})

	rt := BuildDebugCapabilityRuntimeState(eucloruntime.UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityDebugInvestigateRepair,
	}, state, time.Now().UTC())

	if rt.VerificationPlanScope != "explicit" {
		t.Fatalf("expected plan scope, got %q", rt.VerificationPlanScope)
	}
	if rt.VerificationPlanSource != "skill_policy+task_context" {
		t.Fatalf("expected plan source, got %q", rt.VerificationPlanSource)
	}
	if len(rt.VerificationPlanCommands) != 1 || rt.VerificationPlanCommands[0] != "sh" {
		t.Fatalf("expected plan commands, got %#v", rt.VerificationPlanCommands)
	}
	if rt.VerificationPlanPlannerID != "framework.manual" {
		t.Fatalf("expected planner id, got %q", rt.VerificationPlanPlannerID)
	}
	if rt.VerificationPlanRationale == "" {
		t.Fatal("expected rationale")
	}
	if len(rt.VerificationPlanPolicyPreferences) != 1 || rt.VerificationPlanPolicyPreferences[0] != "go_test" {
		t.Fatalf("expected policy preferences, got %#v", rt.VerificationPlanPolicyPreferences)
	}
	if len(rt.ToolOutputSources) == 0 || rt.ToolOutputSources[0] != "euclo.verification_plan" {
		t.Fatalf("expected verification plan tool output source, got %#v", rt.ToolOutputSources)
	}
}

func TestBuildChatCapabilityRuntimeState_AskModeCapturesBehaviorTrace(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.relurpic_behavior_trace", execution.Trace{
		RecipeIDs: []string{"chat.ask.inquiry", "chat.ask.review"},
		Path:      "ask",
	})
	rt := BuildChatCapabilityRuntimeState(eucloruntime.UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatAsk,
		SupportingRelurpicCapabilityIDs: []string{
			euclorelurpic.CapabilityChatLocalReview,
			euclorelurpic.CapabilityArchaeologyExplore,
		},
	}, state, time.Now().UTC())

	if !rt.AskActive || !rt.NonMutating || !rt.LocalReviewActive {
		t.Fatalf("expected ask/local review flags, got ask=%v nonmut=%v local=%v", rt.AskActive, rt.NonMutating, rt.LocalReviewActive)
	}
	if len(rt.ExecutedRecipeIDs) != 2 || rt.BehaviorPath != "ask" {
		t.Fatalf("unexpected trace projection: recipes=%v path=%q", rt.ExecutedRecipeIDs, rt.BehaviorPath)
	}
	if !strings.Contains(rt.Summary, "ask=true") {
		t.Fatalf("expected ask summary segment, got %q", rt.Summary)
	}
}

func TestBuildDebugCapabilityRuntimeState_IncludesEscalationInSummary(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.capability_contract_runtime", eucloruntime.CapabilityContractRuntimeState{
		DebugEscalationTarget:    euclorelurpic.CapabilityChatImplement,
		DebugEscalationTriggered: true,
	})
	rt := BuildDebugCapabilityRuntimeState(eucloruntime.UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityDebugInvestigateRepair,
	}, state, time.Now().UTC())
	if !strings.Contains(rt.Summary, "escalated="+euclorelurpic.CapabilityChatImplement) {
		t.Fatalf("expected escalation in summary, got %q", rt.Summary)
	}
	if len(rt.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics when escalation triggers, got %#v", rt.Diagnostics)
	}
}
