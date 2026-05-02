package authorization

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type ApprovalRequest struct {
	AgentID            string
	Manager            *PermissionManager
	Permission         contracts.PermissionDescriptor
	Justification      string
	Scope              GrantScope
	Risk               RiskLevel
	Duration           time.Duration
	MissingManagerErr  string
	DenyReasonFallback string
}

func EvaluatePolicyRequest(ctx context.Context, engine PolicyEngine, req core.PolicyRequest) (core.PolicyDecision, error) {
	if engine == nil {
		return core.PolicyDecisionAllow("no policy engine"), nil
	}
	return engine.Evaluate(ctx, req)
}

func EnforcePolicyRequest(ctx context.Context, engine PolicyEngine, req core.PolicyRequest, approval ApprovalRequest) (core.PolicyDecision, error) {
	decision, err := EvaluatePolicyRequest(ctx, engine, req)
	if err != nil {
		return core.PolicyDecision{}, err
	}
	switch decision.Effect {
	case "", "allow":
		return decision, nil
	case "deny":
		reason := decision.Reason
		if reason == "" {
			reason = approval.DenyReasonFallback
		}
		if reason == "" {
			reason = "denied by policy"
		}
		return decision, fmt.Errorf("%s", reason)
	case "require_approval":
		if approval.Manager == nil {
			reason := approval.MissingManagerErr
			if reason == "" {
				reason = "approval required but permission manager unavailable"
			}
			return decision, fmt.Errorf("%s", reason)
		}
		if err := approval.Manager.RequireApproval(
			ctx,
			approval.AgentID,
			approval.Permission,
			approval.Justification,
			approval.Scope,
			approval.Risk,
			approval.Duration,
		); err != nil {
			return decision, err
		}
		return decision, nil
	default:
		return decision, fmt.Errorf("unsupported policy effect %q", decision.Effect)
	}
}
