package runtime

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

func TestBuildResolvedExecutionPolicyUsesAgentSkillPolicy(t *testing.T) {
	progressive := true
	cfg := &core.Config{
		AgentSpec: &core.AgentRuntimeSpec{
			Context: core.AgentContextSpec{
				MaxTokens:           4096,
				CompressionStrategy: "summary",
				ProgressiveLoading:  &progressive,
			},
			SkillConfig: core.AgentSkillConfig{
				Review: core.AgentReviewPolicy{
					Criteria: []string{"protect public api"},
				},
				Planning: core.AgentPlanningPolicy{
					RequireVerificationStep: true,
				},
			},
		},
	}
	policy := BuildResolvedExecutionPolicy(&core.Task{Instruction: "review this change"}, cfg, capability.NewRegistry(), ModeResolution{ModeID: "review"}, ExecutionProfileSelection{ProfileID: "review_suggest_implement"})
	if !policy.ResolvedFromSkillPolicy {
		t.Fatal("expected resolved skill policy")
	}
	if !policy.RequireVerificationStep {
		t.Fatal("expected verification step requirement")
	}
	if policy.ContextPolicy.MaxTokens != 4096 {
		t.Fatalf("unexpected context policy: %#v", policy.ContextPolicy)
	}
	if len(policy.ReviewCriteria) != 1 || policy.ReviewCriteria[0] != "protect public api" {
		t.Fatalf("unexpected review criteria: %#v", policy.ReviewCriteria)
	}
}

func TestSelectExecutorDescriptor(t *testing.T) {
	if got := SelectExecutorDescriptor(ModeResolution{ModeID: "planning"}, ExecutionProfileSelection{ProfileID: "plan_stage_execute"}, TaskClassification{}, ResolvedExecutionPolicy{}, &UnitOfWorkPlanBinding{IsLongRunning: true}, "euclo:archaeology.implement-plan", []string{"euclo:archaeology.compile-plan"}); got.Family != ExecutorFamilyRewoo {
		t.Fatalf("expected rewoo executor, got %#v", got)
	} else if got.RecipeID != "archaeology.implement-plan.rewoo_execution" {
		t.Fatalf("expected relurpic recipe id, got %#v", got)
	} else if got.Compatibility {
		t.Fatalf("expected managed executor descriptor, got %#v", got)
	}
	if got := SelectExecutorDescriptor(ModeResolution{ModeID: "review"}, ExecutionProfileSelection{ProfileID: "review_suggest_implement"}, TaskClassification{}, ResolvedExecutionPolicy{}, nil, "euclo:chat.inspect", []string{"euclo:chat.local-review"}); got.Family != ExecutorFamilyReflection {
		t.Fatalf("expected reflection executor, got %#v", got)
	} else if got.Compatibility {
		t.Fatalf("expected managed executor descriptor, got %#v", got)
	}
	if got := SelectExecutorDescriptor(ModeResolution{ModeID: "code"}, ExecutionProfileSelection{ProfileID: "edit_verify_repair"}, TaskClassification{}, ResolvedExecutionPolicy{}, nil, "euclo:chat.ask", []string{"euclo:chat.inspect"}); got.Family != ExecutorFamilyReact {
		t.Fatalf("expected react executor, got %#v", got)
	} else if got.RecipeID != "chat.ask.react_inquiry" {
		t.Fatalf("expected relurpic recipe id, got %#v", got)
	} else if got.Compatibility {
		t.Fatalf("expected managed react executor descriptor, got %#v", got)
	}
}
