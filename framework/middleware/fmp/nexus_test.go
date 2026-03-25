package fmp

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

type stubRuleLookup struct {
	rules []core.PolicyRule
}

func (s stubRuleLookup) ListRules(context.Context) ([]core.PolicyRule, error) {
	out := make([]core.PolicyRule, len(s.rules))
	copy(out, s.rules)
	return out, nil
}

func TestAuthorizationPolicyResolverEvaluatesResumeRulesFromStore(t *testing.T) {
	t.Parallel()

	resolver := &AuthorizationPolicyResolver{
		Rules: stubRuleLookup{rules: []core.PolicyRule{{
			ID:       "resume-deny-remote",
			Name:     "deny remote mesh resume",
			Priority: 1,
			Enabled:  true,
			Conditions: core.PolicyConditions{
				SourceDomains:     []string{"mesh.remote"},
				ExportNames:       []string{"agent.resume"},
				SessionOperations: []core.SessionOperation{core.SessionOperationResume},
			},
			Effect: core.PolicyEffect{Action: "deny", Reason: "remote mesh blocked"},
		}}},
		Now: func() time.Time { return time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC) },
	}

	decision, err := resolver.EvaluateResume(context.Background(), ResumePolicyRequest{
		Lineage:      core.LineageRecord{LineageID: "lineage-1", TenantID: "tenant-1"},
		Offer:        core.HandoffOffer{SourceAttemptID: "attempt-1", ContextClass: "workflow-runtime"},
		Destination:  core.ExportDescriptor{ExportName: "agent.resume"},
		SourceDomain: "mesh.remote",
		Actor:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
		IsOwner:      true,
		RouteMode:    core.RouteModeGateway,
	})
	if err != nil {
		t.Fatalf("EvaluateResume() error = %v", err)
	}
	if decision.Effect != "deny" {
		t.Fatalf("decision = %+v", decision)
	}
}
