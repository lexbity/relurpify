package plan

import (
	"context"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

type VerificationBackendResolver interface {
	BackendID() string
	Supports(agentenv.VerificationPlanRequest) bool
	BuildPlan(context.Context, agentenv.VerificationPlanRequest) (agentenv.VerificationPlan, bool, error)
}

type VerificationScopePlanner struct {
	resolvers []VerificationBackendResolver
}

func NewVerificationScopePlanner(resolvers ...VerificationBackendResolver) *VerificationScopePlanner {
	return &VerificationScopePlanner{resolvers: append([]VerificationBackendResolver(nil), resolvers...)}
}

func (p *VerificationScopePlanner) SelectVerificationPlan(ctx context.Context, req agentenv.VerificationPlanRequest) (agentenv.VerificationPlan, bool, error) {
	resolver := p.selectResolver(req)
	if resolver == nil {
		return agentenv.VerificationPlan{}, false, nil
	}
	now := time.Now().UTC()
	living := &LivingPlan{
		ID:         "verification-scope-plan",
		WorkflowID: "",
		Title:      "Verification Scope Selection",
		Steps: map[string]*PlanStep{
			"detect_scope": {
				ID:          "detect_scope",
				Description: "Classify changed language, scope, and candidate verification backend",
				Status:      PlanStepCompleted,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			"delegate_backend": {
				ID:          "delegate_backend",
				Description: "Delegate verification plan synthesis to the selected language backend",
				DependsOn:   []string{"detect_scope"},
				Status:      PlanStepCompleted,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
		StepOrder: []string{"detect_scope", "delegate_backend"},
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if req.PublicSurfaceChanged {
		living.Steps["compatibility_sweep"] = &PlanStep{
			ID:          "compatibility_sweep",
			Description: "Escalate verification breadth for compatibility-sensitive changes",
			DependsOn:   []string{"delegate_backend"},
			Status:      PlanStepCompleted,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		living.StepOrder = append(living.StepOrder, "compatibility_sweep")
	}

	plan, ok, err := resolver.BuildPlan(ctx, req)
	if err != nil || !ok || len(plan.Commands) == 0 {
		return agentenv.VerificationPlan{}, false, err
	}
	plan.PlannerID = firstNonEmpty(strings.TrimSpace(plan.PlannerID), "framework.plan.verification_scope")
	plan.Source = firstNonEmpty(strings.TrimSpace(plan.Source), "framework_plan")
	plan.AuditTrail = uniqueStrings(append([]string{"detect_scope", "delegate_backend", "backend:" + resolver.BackendID()}, plan.AuditTrail...))
	if req.PublicSurfaceChanged {
		plan.AuditTrail = uniqueStrings(append(plan.AuditTrail, "compatibility_sweep"))
	}
	plan.Metadata = cloneMap(plan.Metadata)
	if plan.Metadata == nil {
		plan.Metadata = map[string]any{}
	}
	plan.Metadata["plan_id"] = living.ID
	plan.Metadata["step_order"] = append([]string(nil), living.StepOrder...)
	plan.Metadata["backend"] = resolver.BackendID()
	plan.Metadata["workspace"] = strings.TrimSpace(req.Workspace)
	plan.Metadata["policy_preferences"] = append([]string(nil), req.PreferredVerifyCapabilities...)
	plan.Metadata["policy_success_caps"] = append([]string(nil), req.VerificationSuccessCapabilities...)
	return plan, true, nil
}

func (p *VerificationScopePlanner) selectResolver(req agentenv.VerificationPlanRequest) VerificationBackendResolver {
	for _, resolver := range p.resolvers {
		if resolver == nil {
			continue
		}
		if resolver.Supports(req) {
			return resolver
		}
	}
	return nil
}

func uniqueStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
