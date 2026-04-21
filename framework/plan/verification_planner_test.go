package plan

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

type stubResolver struct {
	backendID string
	support   bool
	plan      agentenv.VerificationPlan
	ok        bool
	err       error
}

func (s stubResolver) BackendID() string { return s.backendID }

func (s stubResolver) Supports(agentenv.VerificationPlanRequest) bool { return s.support }

func (s stubResolver) BuildPlan(context.Context, agentenv.VerificationPlanRequest) (agentenv.VerificationPlan, bool, error) {
	return s.plan, s.ok, s.err
}

func TestVerificationScopePlanner_DelegatesToMatchingResolver(t *testing.T) {
	planner := NewVerificationScopePlanner(
		stubResolver{backendID: "go", support: false},
		stubResolver{
			backendID: "python",
			support:   true,
			ok:        true,
			plan: agentenv.VerificationPlan{
				ScopeKind: "test_files",
				Commands: []agentenv.VerificationCommand{{
					Name:             "python_pytest",
					Command:          "python",
					Args:             []string{"-m", "pytest", "-q"},
					WorkingDirectory: ".",
				}},
				Source:    "platform.lang.python",
				Rationale: "python resolver selected pytest",
			},
		},
	)

	plan, ok, err := planner.SelectVerificationPlan(context.Background(), agentenv.VerificationPlanRequest{
		TaskInstruction: "verify this Python change",
		Workspace:       ".",
		Files:           []string{"app/service.py"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ok {
		t.Fatal("expected planner to return a plan")
	}
	if plan.ScopeKind != "test_files" {
		t.Fatalf("expected delegated scope, got %q", plan.ScopeKind)
	}
	if plan.Metadata["backend"] != "python" {
		t.Fatalf("expected backend metadata, got %#v", plan.Metadata["backend"])
	}
	if len(plan.AuditTrail) < 3 {
		t.Fatalf("expected generic audit trail wrapping delegated plan, got %#v", plan.AuditTrail)
	}
}

func TestVerificationScopePlanner_NoResolverMatch(t *testing.T) {
	planner := NewVerificationScopePlanner(stubResolver{backendID: "go", support: false})
	_, ok, err := planner.SelectVerificationPlan(context.Background(), agentenv.VerificationPlanRequest{
		TaskInstruction: "verify this markdown change",
		Files:           []string{"docs/plan.md"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ok {
		t.Fatal("expected no plan without a matching resolver")
	}
}

func TestVerificationScopePlanner_PropagatesResolverFailure(t *testing.T) {
	planner := NewVerificationScopePlanner(stubResolver{
		backendID: "rust",
		support:   true,
		ok:        false,
		err:       context.DeadlineExceeded,
	})
	_, ok, err := planner.SelectVerificationPlan(context.Background(), agentenv.VerificationPlanRequest{
		TaskInstruction: "verify this Rust change",
		Files:           []string{"src/lib.rs"},
	})
	if err == nil {
		t.Fatal("expected resolver error")
	}
	if ok {
		t.Fatal("expected no plan on resolver failure")
	}
}
